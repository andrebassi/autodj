package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domyoutube "autodj/domain/youtube"
)

func newTestCatalog(t *testing.T, h http.HandlerFunc) *Catalog {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Catalog{APIKey: "api-key", HTTP: srv.Client(), apiBase: srv.URL}
}

// Search for videos runs search.list then enriches ids via videos.list; the API
// key travels on every request.
func TestCatalogSearchVideos(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "api-key" {
			t.Fatalf("key = %q", r.URL.Query().Get("key"))
		}
		switch r.URL.Path {
		case "/search":
			q := r.URL.Query()
			if q.Get("type") != "video" || q.Get("q") != "midnight" || q.Get("maxResults") != "10" {
				t.Fatalf("search query = %v", q)
			}
			w.Write([]byte(`{"items":[
				{"id":{"videoId":"vid1"},"snippet":{"title":"S","channelTitle":"Ch"}},
				{"id":{"videoId":""},"snippet":{"title":"ghost"}}
			]}`))
		case "/videos":
			if r.URL.Query().Get("id") != "vid1" {
				t.Fatalf("videos id = %q", r.URL.Query().Get("id"))
			}
			w.Write([]byte(`{"items":[
				{"id":"vid1","snippet":{"title":"Neon Skyline","channelTitle":"The Paper Kites",
				 "thumbnails":{"medium":{"url":"http://img/m.jpg"}}},
				 "contentDetails":{"duration":"PT3M30S"},"status":{"embeddable":true}}
			]}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	})
	items, err := c.Search(context.Background(), "tok", "midnight", domyoutube.KindVideo)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	got := items[0]
	if got.URI != "vid1" || got.Title != "Neon Skyline" || got.Artist != "The Paper Kites" ||
		got.ArtworkURL != "http://img/m.jpg" || got.DurationMs != 210000 || got.Kind != domyoutube.KindVideo {
		t.Fatalf("item = %+v", got)
	}
}

// Playlist search returns playlist ids straight from search.list (no videos.list).
func TestCatalogSearchPlaylists(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("type") != "playlist" {
			t.Fatalf("type = %s", r.URL.Query().Get("type"))
		}
		w.Write([]byte(`{"items":[
			{"id":{"playlistId":"pl9"},"snippet":{"title":"Late Drive","channelTitle":"dj-nova",
			 "thumbnails":{"default":{"url":"http://img/d.jpg"}}}},
			{"id":{"playlistId":""},"snippet":{"title":"ghost"}}
		]}`))
	})
	items, err := c.Search(context.Background(), "tok", "drive", domyoutube.KindPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1 (empty id dropped)", len(items))
	}
	got := items[0]
	if got.URI != "pl9" || got.Title != "Late Drive" || got.Artist != "dj-nova" ||
		got.ArtworkURL != "http://img/d.jpg" || got.Kind != domyoutube.KindPlaylist {
		t.Fatalf("item = %+v", got)
	}
}

func TestCatalogSearchUnknownKindDefaultsVideo(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" && r.URL.Query().Get("type") != "video" {
			t.Fatalf("type = %s, want video", r.URL.Query().Get("type"))
		}
		w.Write([]byte(`{"items":[]}`))
	})
	items, err := c.Search(context.Background(), "tok", "x", "channel")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("len = %d", len(items))
	}
}

func TestCatalogPlaylistItems(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlistItems":
			if r.URL.Query().Get("playlistId") != "pl9" {
				t.Fatalf("playlistId = %q", r.URL.Query().Get("playlistId"))
			}
			w.Write([]byte(`{"items":[
				{"snippet":{"title":"A"},"contentDetails":{"videoId":"v1"}},
				{"snippet":{"title":"B"},"contentDetails":{"videoId":""}}
			]}`))
		case "/videos":
			if r.URL.Query().Get("id") != "v1" {
				t.Fatalf("videos id = %q", r.URL.Query().Get("id"))
			}
			w.Write([]byte(`{"items":[
				{"id":"v1","snippet":{"title":"Amber Waves","channelTitle":"Hollow Coves"},
				 "contentDetails":{"duration":"PT3M"},"status":{"embeddable":true}}
			]}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	})
	items, err := c.PlaylistItems(context.Background(), "tok", "https://youtube.com/playlist?list=pl9")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "v1" || items[0].DurationMs != 180000 ||
		items[0].Kind != domyoutube.KindVideo {
		t.Fatalf("items = %+v", items)
	}
}

func TestCatalogVideosNoIDsSkipsCall(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("must not call API for empty ids")
	})
	items, err := c.Videos(context.Background(), "tok", []string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatalf("items = %+v, want nil", items)
	}
}

// A videos response row with empty id is dropped.
func TestCatalogVideosDropsEmptyID(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[
			{"id":"","snippet":{"title":"ghost"},"status":{"embeddable":true}},
			{"id":"keep","snippet":{"title":"Keep"},"contentDetails":{"duration":"PT10S"},"status":{"embeddable":true}}
		]}`))
	})
	items, err := c.Videos(context.Background(), "tok", []string{"keep", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "keep" || items[0].DurationMs != 10000 {
		t.Fatalf("items = %+v", items)
	}
}

// Non-embeddable videos are dropped: they load metadata but never produce audio
// in the IFrame Player, so they must never enter the library. Also asserts the
// status part is requested.
func TestCatalogVideosDropsNonEmbeddable(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if part := r.URL.Query().Get("part"); !strings.Contains(part, "status") {
			t.Fatalf("part = %q, want status included", part)
		}
		w.Write([]byte(`{"items":[
			{"id":"blocked","snippet":{"title":"- Topic"},"contentDetails":{"duration":"PT4M"},"status":{"embeddable":false}},
			{"id":"ok","snippet":{"title":"Playable"},"contentDetails":{"duration":"PT2M"},"status":{"embeddable":true}}
		]}`))
	})
	items, err := c.Videos(context.Background(), "tok", []string{"blocked", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "ok" {
		t.Fatalf("items = %+v, want only embeddable 'ok'", items)
	}
}

func TestCatalogErrorStatusWithBody(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("quota exceeded"))
	})
	_, err := c.Search(context.Background(), "tok", "x", domyoutube.KindVideo)
	if err == nil || !strings.Contains(err.Error(), "status 403") || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("err = %v, want status 403 with body", err)
	}
}

func TestCatalogErrorStatusNoBody(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.Search(context.Background(), "tok", "x", domyoutube.KindVideo)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("err = %v, want status 401", err)
	}
}

func TestCatalogDecodeError(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})
	_, err := c.PlaylistItems(context.Background(), "tok", "pl9")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v, want decode error", err)
	}
}

func TestCatalogQuota429TaggedErrQuotaExceeded(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Too Many Requests"))
	})
	_, err := c.Search(context.Background(), "tok", "x", domyoutube.KindVideo)
	if !errors.Is(err, domyoutube.ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
}

func TestCatalogQuota403WithQuotaReasonTagged(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"errors":[{"reason":"quotaExceeded"}]}}`))
	})
	_, err := c.Search(context.Background(), "tok", "x", domyoutube.KindVideo)
	if !errors.Is(err, domyoutube.ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
}

func TestCatalog403WithoutQuotaReasonStaysPlain(t *testing.T) {
	c := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"errors":[{"reason":"forbidden"}]}}`))
	})
	_, err := c.Search(context.Background(), "tok", "x", domyoutube.KindVideo)
	if errors.Is(err, domyoutube.ErrQuotaExceeded) {
		t.Fatalf("err = %v, should NOT be tagged quota (no quota reason)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("err = %v, want plain status 403", err)
	}
}

func TestYoutubeBareID(t *testing.T) {
	cases := map[string]string{
		"abc":                                  "abc",
		"https://www.youtube.com/watch?v=xyz":  "xyz",
		"https://youtu.be/short1":              "short1",
		"https://youtube.com/playlist?list=p9": "p9",
		"":                                     "",
	}
	for in, want := range cases {
		if got := youtubeBareID(in); got != want {
			t.Fatalf("youtubeBareID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseISO8601ms(t *testing.T) {
	cases := map[string]int{
		"PT1H2M3S": (3600 + 120 + 3) * 1000,
		"PT4M13S":  253000,
		"PT45S":    45000,
		"PT2H":     7200000,
		"garbage":  0,
		"PT9X":     0, // unknown unit → 0
		"":         0,
	}
	for in, want := range cases {
		if got := parseISO8601ms(in); got != want {
			t.Fatalf("parseISO8601ms(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNewCatalogDefaults(t *testing.T) {
	c := NewCatalog("k")
	if c.base() != defaultAPIBase {
		t.Fatalf("base = %s", c.base())
	}
	if c.httpClient() == nil {
		t.Fatal("nil http client")
	}
	empty := &Catalog{}
	if empty.base() != defaultAPIBase || empty.httpClient() != http.DefaultClient {
		t.Fatal("zero-value fallbacks wrong")
	}
}
