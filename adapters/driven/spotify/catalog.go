package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domspotify "autodj/domain/spotify"
)

// defaultAPIBase is the Spotify Web API root. Overridable so httptest can point
// the catalog client at a local server.
const defaultAPIBase = "https://api.spotify.com/v1"

// Catalog implements outbound.SpotifyCatalog against api.spotify.com/v1. It reads
// metadata only (search, playlist contents, track lookup) — no audio ever leaves
// here, so imported tracks stay non-mixable. The access token is passed per call
// (the app service refreshes it). apiBase and HTTP are injectable for tests.
type Catalog struct {
	HTTP    *http.Client
	apiBase string
}

// NewCatalog builds a catalog client hitting the public Spotify Web API.
func NewCatalog() *Catalog {
	return &Catalog{
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		apiBase: defaultAPIBase,
	}
}

func (c *Catalog) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Catalog) base() string {
	if c.apiBase != "" {
		return c.apiBase
	}
	return defaultAPIBase
}

// apiTrack is the subset of a Spotify track object the import needs.
type apiTrack struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Duration int    `json:"duration_ms"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name   string `json:"name"`
		Images []struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"images"`
	} `json:"album"`
}

func (t apiTrack) toItem() domspotify.CatalogItem {
	artist := ""
	if len(t.Artists) > 0 {
		artist = t.Artists[0].Name
	}
	return domspotify.CatalogItem{
		URI:        t.URI,
		Title:      t.Name,
		Artist:     artist,
		Album:      t.Album.Name,
		ArtworkURL: t.albumImage(),
		DurationMs: t.Duration,
		Kind:       domspotify.KindTrack,
	}
}

// albumImage picks a mid-size cover from the album images. Spotify orders images
// largest-first (640/300/64); the ~300px middle one is ideal for the player/queue
// thumbnails — falls back to the first (or "") when fewer are present.
func (t apiTrack) albumImage() string {
	imgs := t.Album.Images
	if len(imgs) == 0 {
		return ""
	}
	if len(imgs) >= 2 {
		return imgs[1].URL
	}
	return imgs[0].URL
}

// apiPlaylist is the subset of a Spotify playlist object shown in search results.
type apiPlaylist struct {
	URI   string `json:"uri"`
	Name  string `json:"name"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
}

func (p apiPlaylist) toItem() domspotify.CatalogItem {
	return domspotify.CatalogItem{
		URI:    p.URI,
		Title:  p.Name,
		Artist: p.Owner.DisplayName,
		Kind:   domspotify.KindPlaylist,
	}
}

// searchResponse is GET /v1/search. Só o bloco de tracks é lido — playlists
// vêm de /me/playlists (ver myPlaylists), não da busca global.
type searchResponse struct {
	Tracks struct {
		Items []apiTrack `json:"items"`
	} `json:"tracks"`
}

// Search queries /v1/search for tracks; playlists resolve to the user's OWN
// library (/v1/me/playlists) filtered by query — nunca playlists de terceiros.
func (c *Catalog) Search(ctx context.Context, token, query, kind string) ([]domspotify.CatalogItem, error) {
	if kind != domspotify.KindTrack && kind != domspotify.KindPlaylist {
		kind = domspotify.KindTrack
	}
	if kind == domspotify.KindPlaylist {
		return c.myPlaylists(ctx, token, query)
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("type", kind)
	// Apps em Development mode capam o limit de /v1/search em 10 — valores
	// maiores devolvem 400 "Invalid limit". 10 é o teto seguro.
	q.Set("limit", "10")
	var sr searchResponse
	if err := c.get(ctx, token, "/search?"+q.Encode(), &sr); err != nil {
		return nil, err
	}
	out := make([]domspotify.CatalogItem, 0, len(sr.Tracks.Items))
	for _, t := range sr.Tracks.Items {
		if t.URI == "" {
			continue
		}
		out = append(out, t.toItem())
	}
	return out, nil
}

// myPlaylistsResponse is GET /v1/me/playlists.
type myPlaylistsResponse struct {
	Items []apiPlaylist `json:"items"`
}

// myPlaylists lists the linked user's own playlists (/me/playlists, até 50) e
// filtra por substring do nome (case-insensitive). Query vazia devolve todas —
// deixa a busca sem termo listar a biblioteca inteira do usuário.
func (c *Catalog) myPlaylists(ctx context.Context, token, query string) ([]domspotify.CatalogItem, error) {
	var pr myPlaylistsResponse
	if err := c.get(ctx, token, "/me/playlists?limit=50", &pr); err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	out := make([]domspotify.CatalogItem, 0, len(pr.Items))
	for _, p := range pr.Items {
		if p.URI == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(p.Name), needle) {
			continue
		}
		out = append(out, p.toItem())
	}
	return out, nil
}

// playlistItemsResponse is GET /v1/playlists/{id}/items. The Mar/2026 Spotify API
// migration deprecated /playlists/{id}/tracks (now 403 Forbidden for Development
// mode apps) in favor of /items, and renamed the per-entry key `track` → `item`.
// We decode BOTH keys so a pre- or post-migration server both parse (whichever is
// present wins).
type playlistItemsResponse struct {
	Items []struct {
		Item  apiTrack `json:"item"`
		Track apiTrack `json:"track"`
	} `json:"items"`
}

// PlaylistTracks lists a playlist's tracks (metadata only). Uses the post-migration
// /playlists/{id}/items endpoint — /tracks was retired (403) on 2026-03-09.
func (c *Catalog) PlaylistTracks(ctx context.Context, token, playlistID string) ([]domspotify.CatalogItem, error) {
	var pr playlistItemsResponse
	if err := c.get(ctx, token, "/playlists/"+url.PathEscape(spotifyBareID(playlistID))+"/items?limit=100", &pr); err != nil {
		return nil, err
	}
	out := make([]domspotify.CatalogItem, 0, len(pr.Items))
	for _, it := range pr.Items {
		t := it.Item
		if t.URI == "" {
			t = it.Track // pre-migration shape fallback
		}
		if t.URI == "" {
			continue
		}
		out = append(out, t.toItem())
	}
	return out, nil
}

// Tracks resolves track URIs/ids to metadata. Apps em Development mode não têm
// acesso ao batch GET /v1/tracks?ids= (403 Forbidden) — mas o single GET
// /v1/tracks/{id} funciona; então resolvemos um id por vez. URIs
// (spotify:track:ID) são reduzidas ao id bare.
func (c *Catalog) Tracks(ctx context.Context, token string, uris []string) ([]domspotify.CatalogItem, error) {
	ids := make([]string, 0, len(uris))
	for _, u := range uris {
		if id := spotifyBareID(u); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]domspotify.CatalogItem, 0, len(ids))
	for _, id := range ids {
		var t apiTrack
		if err := c.get(ctx, token, "/tracks/"+url.PathEscape(id), &t); err != nil {
			return nil, err
		}
		if t.URI == "" {
			continue
		}
		out = append(out, t.toItem())
	}
	return out, nil
}

// get performs an authorized GET and decodes the JSON body into v.
func (c *Catalog) get(ctx context.Context, token, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Vaza a mensagem do Spotify (ex.: "Invalid limit") em vez de só o
		// status — o body de erro é curto; limita a 512 bytes por segurança.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(snippet))
		if msg == "" {
			return fmt.Errorf("spotify: web api status %d", resp.StatusCode)
		}
		return fmt.Errorf("spotify: web api status %d: %s", resp.StatusCode, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("spotify: decode web api response: %w", err)
	}
	return nil
}

// spotifyBareID reduces a spotify:track:ID / spotify:playlist:ID URI to its bare
// id; a value with no colon is returned unchanged (already an id).
func spotifyBareID(uriOrID string) string {
	if i := strings.LastIndex(uriOrID, ":"); i >= 0 {
		return uriOrID[i+1:]
	}
	return uriOrID
}
