package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domyoutube "autodj/domain/youtube"
)

// defaultAPIBase is the YouTube Data API v3 root. Overridable so httptest can
// point the catalog client at a local server.
const defaultAPIBase = "https://www.googleapis.com/youtube/v3"

// Catalog implements outbound.YouTubeCatalog against the YouTube Data API v3. It
// reads metadata only (search, playlist contents, video lookup) — no audio ever
// leaves here, so imported tracks stay non-mixable. It authenticates with an API
// KEY (not the OAuth token); the token argument is ignored (shape parity with the
// Spotify catalog). apiBase and HTTP are injectable for tests.
type Catalog struct {
	APIKey  string
	HTTP    *http.Client
	apiBase string
}

// NewCatalog builds a catalog client hitting the public Data API with apiKey.
func NewCatalog(apiKey string) *Catalog {
	return &Catalog{
		APIKey:  apiKey,
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

// searchResponse is GET /search.
type searchResponse struct {
	Items []struct {
		ID struct {
			VideoID    string `json:"videoId"`
			PlaylistID string `json:"playlistId"`
		} `json:"id"`
		Snippet snippet `json:"snippet"`
	} `json:"items"`
}

// snippet is the shared snippet shape across search/playlistItems/videos.
type snippet struct {
	Title        string `json:"title"`
	ChannelTitle string `json:"channelTitle"`
	Thumbnails   struct {
		Medium struct {
			URL string `json:"url"`
		} `json:"medium"`
		Default struct {
			URL string `json:"url"`
		} `json:"default"`
	} `json:"thumbnails"`
}

func (s snippet) thumb() string {
	if s.Thumbnails.Medium.URL != "" {
		return s.Thumbnails.Medium.URL
	}
	return s.Thumbnails.Default.URL
}

// Search finds videos or playlists matching query. Videos are enriched with real
// durations via a videos.list follow-up (search.list omits contentDetails);
// playlists carry no duration.
func (c *Catalog) Search(ctx context.Context, _ /*token*/, query, kind string) ([]domyoutube.CatalogItem, error) {
	if kind != domyoutube.KindVideo && kind != domyoutube.KindPlaylist {
		kind = domyoutube.KindVideo
	}
	q := url.Values{}
	q.Set("part", "snippet")
	q.Set("type", kind)
	q.Set("q", query)
	q.Set("maxResults", "10")
	q.Set("key", c.APIKey)
	var sr searchResponse
	if err := c.get(ctx, "/search?"+q.Encode(), &sr); err != nil {
		return nil, err
	}
	if kind == domyoutube.KindPlaylist {
		out := make([]domyoutube.CatalogItem, 0, len(sr.Items))
		for _, it := range sr.Items {
			if it.ID.PlaylistID == "" {
				continue
			}
			out = append(out, domyoutube.CatalogItem{
				URI:        it.ID.PlaylistID,
				Title:      it.Snippet.Title,
				Artist:     it.Snippet.ChannelTitle,
				ArtworkURL: it.Snippet.thumb(),
				Kind:       domyoutube.KindPlaylist,
			})
		}
		return out, nil
	}
	ids := make([]string, 0, len(sr.Items))
	for _, it := range sr.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	return c.Videos(ctx, "", ids)
}

// playlistItemsResponse is GET /playlistItems.
type playlistItemsResponse struct {
	Items []struct {
		Snippet        snippet `json:"snippet"`
		ContentDetails struct {
			VideoID string `json:"videoId"`
		} `json:"contentDetails"`
	} `json:"items"`
}

// PlaylistItems lists a playlist's videos (metadata only), enriched with real
// durations via videos.list.
func (c *Catalog) PlaylistItems(ctx context.Context, _ /*token*/, playlistID string) ([]domyoutube.CatalogItem, error) {
	q := url.Values{}
	q.Set("part", "snippet,contentDetails")
	q.Set("playlistId", youtubeBareID(playlistID))
	q.Set("maxResults", "50")
	q.Set("key", c.APIKey)
	var pr playlistItemsResponse
	if err := c.get(ctx, "/playlistItems?"+q.Encode(), &pr); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(pr.Items))
	for _, it := range pr.Items {
		if it.ContentDetails.VideoID != "" {
			ids = append(ids, it.ContentDetails.VideoID)
		}
	}
	return c.Videos(ctx, "", ids)
}

// videosResponse is GET /videos. Status.Embeddable gates whether the video may
// play inside the IFrame Player — non-embeddable videos ("- Topic" auto-channels,
// owner-restricted, region-blocked) load metadata but never produce audio, so the
// catalog drops them before they reach the library.
type videosResponse struct {
	Items []struct {
		ID             string  `json:"id"`
		Snippet        snippet `json:"snippet"`
		ContentDetails struct {
			Duration string `json:"duration"` // ISO-8601, e.g. PT4M13S
		} `json:"contentDetails"`
		Status struct {
			Embeddable bool `json:"embeddable"`
		} `json:"status"`
	} `json:"items"`
}

// Videos resolves video ids/URLs to their metadata (snippet + duration). ids may
// be bare ids or full watch URLs; they are reduced to bare ids. The Data API caps
// videos.list id= at 50 — callers pass ≤50 (search/playlist limits guarantee it).
func (c *Catalog) Videos(ctx context.Context, _ /*token*/ string, ids []string) ([]domyoutube.CatalogItem, error) {
	bare := make([]string, 0, len(ids))
	for _, id := range ids {
		if b := youtubeBareID(id); b != "" {
			bare = append(bare, b)
		}
	}
	if len(bare) == 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("part", "snippet,contentDetails,status")
	q.Set("id", strings.Join(bare, ","))
	q.Set("maxResults", "50")
	q.Set("key", c.APIKey)
	var vr videosResponse
	if err := c.get(ctx, "/videos?"+q.Encode(), &vr); err != nil {
		return nil, err
	}
	out := make([]domyoutube.CatalogItem, 0, len(vr.Items))
	for _, it := range vr.Items {
		if it.ID == "" {
			continue
		}
		// non-embeddable → skip: the IFrame Player can't produce audio for it, so
		// letting it into the library would silently stall the auto-DJ queue.
		if !it.Status.Embeddable {
			continue
		}
		out = append(out, domyoutube.CatalogItem{
			URI:        it.ID,
			Title:      it.Snippet.Title,
			Artist:     it.Snippet.ChannelTitle,
			ArtworkURL: it.Snippet.thumb(),
			DurationMs: parseISO8601ms(it.ContentDetails.Duration),
			Kind:       domyoutube.KindVideo,
		})
	}
	return out, nil
}

// get performs a GET against the Data API and decodes the JSON body into v.
func (c *Catalog) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(snip))
		// Daily-quota exhaustion surfaces as 429 (Too Many Requests) or 403 whose
		// body carries a quota/rate reason ("quotaExceeded"/"rateLimitExceeded").
		// Tag it as domyoutube.ErrQuotaExceeded so the HTTP layer answers 429 with a
		// clear "try again tomorrow" message instead of a blind 500.
		if isQuotaStatus(resp.StatusCode, msg) {
			return fmt.Errorf("%w (upstream status %d)", domyoutube.ErrQuotaExceeded, resp.StatusCode)
		}
		if msg == "" {
			return fmt.Errorf("youtube: data api status %d", resp.StatusCode)
		}
		return fmt.Errorf("youtube: data api status %d: %s", resp.StatusCode, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("youtube: decode data api response: %w", err)
	}
	return nil
}

// isQuotaStatus reports whether an upstream error response means the daily quota
// is spent. 429 is unambiguous. 403 is overloaded (also used for forbidden/auth),
// so it only counts as quota when the body names a quota/rate reason — the Data API
// puts "quotaExceeded"/"rateLimitExceeded"/"dailyLimitExceeded" in the error body.
func isQuotaStatus(status int, body string) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	b := strings.ToLower(body)
	return strings.Contains(b, "quotaexceeded") ||
		strings.Contains(b, "ratelimitexceeded") ||
		strings.Contains(b, "dailylimitexceeded") ||
		strings.Contains(b, "quota")
}

// youtubeBareID reduces a watch URL / youtu.be link / bare id to the bare video
// or playlist id. A value with no URL markers is returned unchanged.
func youtubeBareID(uriOrID string) string {
	s := strings.TrimSpace(uriOrID)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "/") && !strings.Contains(s, "?") {
		return s // already a bare id
	}
	if u, err := url.Parse(s); err == nil {
		if v := u.Query().Get("v"); v != "" {
			return v
		}
		if list := u.Query().Get("list"); list != "" {
			return list
		}
		// youtu.be/<id> short form
		if p := strings.Trim(u.Path, "/"); p != "" {
			parts := strings.Split(p, "/")
			return parts[len(parts)-1]
		}
	}
	return s
}

// parseISO8601ms parses an ISO-8601 duration (PT#H#M#S, the subset YouTube
// returns) into milliseconds. Malformed input yields 0.
func parseISO8601ms(d string) int {
	if !strings.HasPrefix(d, "PT") {
		return 0
	}
	d = d[2:]
	var total, cur int
	for _, r := range d {
		switch {
		case r >= '0' && r <= '9':
			cur = cur*10 + int(r-'0')
		case r == 'H':
			total += cur * 3600
			cur = 0
		case r == 'M':
			total += cur * 60
			cur = 0
		case r == 'S':
			total += cur
			cur = 0
		default:
			return 0
		}
	}
	return total * 1000
}
