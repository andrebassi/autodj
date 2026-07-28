package spotify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domspotify "autodj/domain/spotify"
)

// newTestCatalog points a Catalog at a stub server and asserts the bearer token
// is forwarded on every call.
func newTestCatalog(t *testing.T, h http.HandlerFunc) *Catalog {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Catalog{HTTP: srv.Client(), apiBase: srv.URL}
}

func requireBearer(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+want {
		t.Fatalf("Authorization = %q, want Bearer %s", got, want)
	}
}

func TestCatalogSearchTracks(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, r, "tok")
		if r.URL.Path != "/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("type") != "track" || q.Get("q") != "midnight" || q.Get("limit") != "10" {
			t.Fatalf("query = %v", q)
		}
		w.Write([]byte(`{"tracks":{"items":[
			{"uri":"spotify:track:1","name":"Neon Skyline","duration_ms":210000,
			 "artists":[{"name":"The Paper Kites"}],"album":{"name":"Dusk Sessions"}},
			{"uri":"","name":"ghost"}
		]}}`))
	})
	items, err := c.Search(context.Background(), "tok", "midnight", domspotify.KindTrack)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1 (empty-uri dropped)", len(items))
	}
	got := items[0]
	if got.URI != "spotify:track:1" || got.Title != "Neon Skyline" || got.Artist != "The Paper Kites" ||
		got.Album != "Dusk Sessions" || got.DurationMs != 210000 || got.Kind != domspotify.KindTrack {
		t.Fatalf("item = %+v", got)
	}
}

// Busca de playlist agora vem de /me/playlists (só as do usuário), filtrada por
// substring do nome — nunca /v1/search (playlists de terceiros).
func TestCatalogSearchPlaylists(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/playlists" {
			t.Fatalf("path = %s, want /me/playlists", r.URL.Path)
		}
		w.Write([]byte(`{"items":[
			{"uri":"spotify:playlist:9","name":"Late Drive","owner":{"display_name":"dj-nova"}},
			{"uri":"spotify:playlist:7","name":"Morning Coffee","owner":{"display_name":"dj-nova"}},
			{"uri":""}
		]}}`))
	})
	items, err := c.Search(context.Background(), "tok", "drive", domspotify.KindPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1 (só a que casa 'drive')", len(items))
	}
	got := items[0]
	if got.URI != "spotify:playlist:9" || got.Title != "Late Drive" || got.Artist != "dj-nova" ||
		got.Kind != domspotify.KindPlaylist {
		t.Fatalf("item = %+v", got)
	}
}

// Query vazia lista todas as playlists do usuário (sem filtro).
func TestCatalogSearchPlaylistsEmptyQueryListsAll(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/playlists" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"items":[
			{"uri":"spotify:playlist:1","name":"A","owner":{"display_name":"me"}},
			{"uri":"spotify:playlist:2","name":"B","owner":{"display_name":"me"}}
		]}}`))
	})
	items, err := c.Search(context.Background(), "tok", "", domspotify.KindPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2 (query vazia = todas)", len(items))
	}
}

func TestCatalogSearchUnknownKindDefaultsTrack(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "track" {
			t.Fatalf("type = %s, want track", r.URL.Query().Get("type"))
		}
		w.Write([]byte(`{"tracks":{"items":[]}}`))
	})
	items, err := c.Search(context.Background(), "tok", "x", "album")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("len = %d", len(items))
	}
}

func TestCatalogPlaylistTracks(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, r, "tok")
		// Migração Mar/2026: /tracks foi aposentado (403 em Dev mode) → /items,
		// e a chave por entrada virou `item` (não mais `track`).
		if r.URL.Path != "/playlists/9/items" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}
		w.Write([]byte(`{"items":[
			{"item":{"uri":"spotify:track:2","name":"Amber Waves","duration_ms":180000,
			 "artists":[{"name":"Hollow Coves"}],"album":{"name":"Coastlines"}}},
			{"item":{"uri":""}}
		]}}`))
	})
	// URI form — bare id extracted.
	items, err := c.PlaylistTracks(context.Background(), "tok", "spotify:playlist:9")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "spotify:track:2" || items[0].Kind != domspotify.KindTrack {
		t.Fatalf("items = %+v", items)
	}
}

// Servidor pré-migração ainda manda `track` em vez de `item` — aceitamos ambos.
func TestCatalogPlaylistTracksLegacyShape(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, r, "tok")
		if r.URL.Path != "/playlists/9/items" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"items":[
			{"track":{"uri":"spotify:track:5","name":"Old Key","duration_ms":120000,
			 "artists":[{"name":"Legacy"}]}}
		]}}`))
	})
	items, err := c.PlaylistTracks(context.Background(), "tok", "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "spotify:track:5" {
		t.Fatalf("items = %+v", items)
	}
}

func TestCatalogTracks(t *testing.T) {
	// Dev-mode não tem batch /tracks?ids= (403) — resolve um id por vez via
	// GET /tracks/{id}. bbb devolve uri vazia (some do resultado).
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tracks/aaa":
			w.Write([]byte(`{"uri":"spotify:track:aaa","name":"Glass Harbor","duration_ms":200000,
			 "artists":[{"name":"Wintergreen"}]}`))
		case "/tracks/bbb":
			w.Write([]byte(`{"uri":""}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	})
	items, err := c.Tracks(context.Background(), "tok", []string{"spotify:track:aaa", "bbb"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "spotify:track:aaa" || items[0].Artist != "Wintergreen" {
		t.Fatalf("items = %+v", items)
	}
}

func TestCatalogTracksNoIDsSkipsCall(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("must not call API for empty uris")
	})
	items, err := c.Tracks(context.Background(), "tok", []string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatalf("items = %+v, want nil", items)
	}
}

func TestCatalogErrorStatus(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.Search(context.Background(), "tok", "x", domspotify.KindTrack)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("err = %v, want status 401", err)
	}
}

func TestCatalogDecodeError(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})
	_, err := c.PlaylistTracks(context.Background(), "tok", "9")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v, want decode error", err)
	}
}

func TestSpotifyBareID(t *testing.T) {
	cases := map[string]string{
		"spotify:track:xyz": "xyz",
		"xyz":               "xyz",
		"spotify:playlist:9": "9",
	}
	for in, want := range cases {
		if got := spotifyBareID(in); got != want {
			t.Fatalf("spotifyBareID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewCatalogDefaults(t *testing.T) {
	c := NewCatalog()
	if c.base() != defaultAPIBase {
		t.Fatalf("base = %s", c.base())
	}
	if c.httpClient() == nil {
		t.Fatal("nil http client")
	}
	// zero-value fallbacks
	empty := &Catalog{}
	if empty.base() != defaultAPIBase || empty.httpClient() != http.DefaultClient {
		t.Fatal("zero-value fallbacks wrong")
	}
}
