package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/raitonoberu/ytsearch"

	domyoutube "autodj/domain/youtube"
)

func stubResult() *ytsearch.SearchResult {
	res := &ytsearch.SearchResult{}
	v := &ytsearch.VideoItem{ID: "vid1", Title: "Song One", Duration: 260}
	v.Channel.Title = "Chan"
	v.Thumbnails = []ytsearch.Thumbnail{{URL: "http://img/720", Width: 720}, {URL: "http://img/320", Width: 320}}
	res.Videos = []*ytsearch.VideoItem{v}
	p := &ytsearch.PlaylistItem{ID: "pl1", Title: "Set"}
	p.Channel.Title = "DJ"
	p.Thumbnails = []ytsearch.Thumbnail{{URL: "http://img/pl", Width: 120}}
	res.Playlists = []*ytsearch.PlaylistItem{p}
	return res
}

func newStubInnerTube(res *ytsearch.SearchResult, err error) *InnerTube {
	return &InnerTube{search: func(context.Context, string, string) (*ytsearch.SearchResult, error) {
		return res, err
	}}
}

func TestInnerTubeSearchVideos(t *testing.T) {
	it := newStubInnerTube(stubResult(), nil)
	got, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	w := domyoutube.CatalogItem{
		URI: "vid1", Title: "Song One", Artist: "Chan",
		ArtworkURL: "http://img/320", DurationMs: 260000, Kind: domyoutube.KindVideo,
	}
	if got[0] != w {
		t.Fatalf("item = %+v, want %+v", got[0], w)
	}
}

func TestInnerTubeSearchPlaylists(t *testing.T) {
	it := newStubInnerTube(stubResult(), nil)
	got, err := it.Search(context.Background(), "", "set", domyoutube.KindPlaylist)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].URI != "pl1" || got[0].Kind != domyoutube.KindPlaylist {
		t.Fatalf("playlist mapping wrong: %+v", got)
	}
	if got[0].Artist != "DJ" || got[0].DurationMs != 0 {
		t.Fatalf("playlist artist/duration wrong: %+v", got[0])
	}
}

func TestInnerTubeSearchEmptyQuery(t *testing.T) {
	it := newStubInnerTube(nil, errors.New("must not be called"))
	got, err := it.Search(context.Background(), "", "   ", domyoutube.KindVideo)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty query: got %v, %v", got, err)
	}
}

func TestInnerTubeSearchDropsLiveAndEmptyID(t *testing.T) {
	res := &ytsearch.SearchResult{Videos: []*ytsearch.VideoItem{
		nil,
		{ID: "", Duration: 10},
		{ID: "live", Duration: 0},
		{ID: "ok", Duration: 5},
	}}
	it := newStubInnerTube(res, nil)
	got, _ := it.Search(context.Background(), "", "q", domyoutube.KindVideo)
	if len(got) != 1 || got[0].URI != "ok" {
		t.Fatalf("want only ok, got %+v", got)
	}
}

func TestInnerTubeSearchCapsAtMax(t *testing.T) {
	res := &ytsearch.SearchResult{}
	for i := 0; i < innerTubeMax+5; i++ {
		res.Videos = append(res.Videos, &ytsearch.VideoItem{ID: "v", Duration: 1})
		res.Playlists = append(res.Playlists, &ytsearch.PlaylistItem{ID: "p"})
	}
	it := newStubInnerTube(res, nil)
	vids, _ := it.Search(context.Background(), "", "q", domyoutube.KindVideo)
	if len(vids) != innerTubeMax {
		t.Fatalf("videos len = %d, want %d", len(vids), innerTubeMax)
	}
	pls, _ := it.Search(context.Background(), "", "q", domyoutube.KindPlaylist)
	if len(pls) != innerTubeMax {
		t.Fatalf("playlists len = %d, want %d", len(pls), innerTubeMax)
	}
}

func TestInnerTubeSearchDropsEmptyPlaylistID(t *testing.T) {
	res := &ytsearch.SearchResult{Playlists: []*ytsearch.PlaylistItem{nil, {ID: ""}, {ID: "keep"}}}
	it := newStubInnerTube(res, nil)
	got, _ := it.Search(context.Background(), "", "q", domyoutube.KindPlaylist)
	if len(got) != 1 || got[0].URI != "keep" {
		t.Fatalf("want only keep, got %+v", got)
	}
}

func TestInnerTubeSearchEnrichesViaDataAPI(t *testing.T) {
	var gotIDs string
	data := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		gotIDs = r.URL.Query().Get("id")
		_, _ = w.Write([]byte(`{"items":[{"id":"vid1","snippet":{"title":"Exact","channelTitle":"Chan","thumbnails":{"medium":{"url":"http://cdn/x"}}},"contentDetails":{"duration":"PT4M21S"},"status":{"embeddable":true}}]}`))
	})
	it := newStubInnerTube(stubResult(), nil)
	it.Data = data
	got, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotIDs != "vid1" {
		t.Fatalf("videos.list ids = %q, want vid1", gotIDs)
	}
	if len(got) != 1 || got[0].Title != "Exact" || got[0].DurationMs != 261000 {
		t.Fatalf("enrichment not applied: %+v", got)
	}
}

func TestInnerTubeSearchKeepsFreeItemsWhenEnrichmentFails(t *testing.T) {
	data := newTestCatalog(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	it := newStubInnerTube(stubResult(), nil)
	it.Data = data
	got, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil {
		t.Fatalf("quota-exhausted enrichment must not fail search: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Song One" {
		t.Fatalf("want free item kept, got %+v", got)
	}
}

func TestInnerTubeSearchFallsBackToDataAPIOnError(t *testing.T) {
	data := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"fb1"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"fb1","snippet":{"title":"Fallback"},"contentDetails":{"duration":"PT1M"},"status":{"embeddable":true}}]}`))
	})
	it := newStubInnerTube(nil, errors.New("renderer changed"))
	it.Data = data
	got, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Fallback" {
		t.Fatalf("want Data API fallback, got %+v", got)
	}
}

func TestInnerTubeSearchNilResultNoDataIsUnavailable(t *testing.T) {
	it := newStubInnerTube(nil, nil)
	if _, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo); !errors.Is(err, domyoutube.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestInnerTubeSearchErrorNoDataPropagates(t *testing.T) {
	boom := errors.New("boom")
	it := newStubInnerTube(nil, boom)
	if _, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestInnerTubeSearchNoHitsFallsBackAndEmpty(t *testing.T) {
	empty := &ytsearch.SearchResult{}
	it := newStubInnerTube(empty, nil)
	got, err := it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil || len(got) != 0 {
		t.Fatalf("no data, no hits: got %v, %v", got, err)
	}

	data := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"z"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"z","snippet":{"title":"Z"},"contentDetails":{"duration":"PT2S"},"status":{"embeddable":true}}]}`))
	})
	it.Data = data
	got, err = it.Search(context.Background(), "", "song", domyoutube.KindVideo)
	if err != nil || len(got) != 1 || got[0].Title != "Z" {
		t.Fatalf("want Data API fallback on zero hits, got %+v (%v)", got, err)
	}
}

// Com API key configurada a Data API continua na FRENTE: é ela que traz o filtro
// `status.embeddable` do Google e a playlist paginada inteira. O caminho grátis só
// entra quando ela não responde.
func TestInnerTubeDelegatesVideosAndPlaylistItems(t *testing.T) {
	var paths []string
	data := newTestCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "playlistItems") {
			// playlistItems.list devolve os ids em contentDetails.videoId; o adapter
			// então chama /videos com eles — por isso a sequência tem 3 chamadas.
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"z"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"z","snippet":{"title":"Z"},"contentDetails":{"duration":"PT2S"},"status":{"embeddable":true}}]}`))
	})
	it := NewInnerTube(data)
	it.BaseURL = "http://127.0.0.1:1" // grátis inalcançável de propósito: NÃO pode ser usado
	got, err := it.Videos(context.Background(), "", []string{"a"})
	if err != nil || len(got) != 1 || got[0].Title != "Z" {
		t.Fatalf("Videos deve vir da Data API: %+v (%v)", got, err)
	}
	if _, err := it.PlaylistItems(context.Background(), "", "pl"); err != nil {
		t.Fatalf("PlaylistItems: %v", err)
	}
	if len(paths) != 3 || paths[0] != "/videos" || paths[1] != "/playlistItems" || paths[2] != "/videos" {
		t.Fatalf("delegation paths = %v (esperado /videos, /playlistItems, /videos)", paths)
	}
}

// SEM API key (v0.2.22) o adapter resolve sozinho pelo InnerTube — é isto que torna a
// aba utilizável com zero credencial. Antes devolvia ErrCatalogUnavailable.
func TestInnerTubeResolvesWithoutDataAPI(t *testing.T) {
	stub := innerTubeStub(t, func(endpoint string, body map[string]any) (any, int) {
		if endpoint == "player" {
			id, _ := body["videoId"].(string)
			return playerBody(id, "T-"+id, "Canal", "90", "OK"), 200
		}
		return nextBody(map[string]any{
			"videoId":    "v1",
			"title":      map[string]any{"simpleText": "Faixa da playlist"},
			"lengthText": map[string]any{"simpleText": "2:30"},
		}), 200
	})
	// mesmo shape do wiring sem key: Data == nil
	if stub.Data != nil {
		t.Fatal("stub deveria vir sem Data API")
	}
	got, err := stub.Videos(context.Background(), "", []string{"abc"})
	if err != nil || len(got) != 1 || got[0].DurationMs != 90000 {
		t.Fatalf("Videos sem key: %+v (%v)", got, err)
	}
	pl, err := stub.PlaylistItems(context.Background(), "", "PLx")
	if err != nil || len(pl) != 1 || pl[0].DurationMs != 150000 {
		t.Fatalf("PlaylistItems sem key: %+v (%v)", pl, err)
	}
}

// Data API configurada mas FALHANDO (cota estourada, rede) não pode derrubar o
// import: cai no caminho grátis em vez de propagar o erro.
func TestInnerTubeFallsBackToFreeWhenDataAPIFails(t *testing.T) {
	data := newTestCatalog(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"quotaExceeded"}]}}`))
	})
	stub := innerTubeStub(t, func(endpoint string, body map[string]any) (any, int) {
		if endpoint == "player" {
			id, _ := body["videoId"].(string)
			return playerBody(id, "Resgatado pelo gratis", "Canal", "60", "OK"), 200
		}
		return nextBody(), 200
	})
	stub.Data = data
	got, err := stub.Videos(context.Background(), "", []string{"abc"})
	if err != nil || len(got) != 1 || got[0].Title != "Resgatado pelo gratis" {
		t.Fatalf("fallback nao aconteceu: %+v (%v)", got, err)
	}
}

func TestInnerTubeTimeoutDefaultAndOverride(t *testing.T) {
	it := NewInnerTube(nil)
	if it.timeout() != innerTubeTimeout {
		t.Fatalf("default timeout = %v", it.timeout())
	}
	it.Timeout = 3 * time.Second
	if it.timeout() != 3*time.Second {
		t.Fatalf("override timeout = %v", it.timeout())
	}
}

// TestInnerTubeLiveSearchHonoursContext exercises the real client wiring without
// touching the network: a cancelled context fails inside the RoundTripper.
func TestInnerTubeLiveSearchHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	it := NewInnerTube(nil)
	for _, kind := range []string{domyoutube.KindVideo, domyoutube.KindPlaylist} {
		if _, err := it.liveSearch(ctx, "q", kind); err == nil {
			t.Fatalf("kind %s: want error from cancelled context", kind)
		}
	}
	// searchFn falls through to liveSearch when no stub is installed.
	if _, err := it.Search(ctx, "", "q", domyoutube.KindVideo); err == nil {
		t.Fatal("want error from cancelled context")
	}
}

func TestCtxTransportDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// nil ctx + nil base → plain round-trip through the default transport.
	resp, err := (ctxTransport{}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestInnerTubeThumb(t *testing.T) {
	cases := []struct {
		name string
		in   []ytsearch.Thumbnail
		want string
	}{
		{"prefers widest under cap", []ytsearch.Thumbnail{{URL: "a", Width: 120}, {URL: "b", Width: 480}, {URL: "c", Width: 1280}}, "b"},
		{"falls back to widest", []ytsearch.Thumbnail{{URL: "big", Width: 1280}}, "big"},
		{"skips empty url", []ytsearch.Thumbnail{{URL: "", Width: 320}}, ""},
		{"none", nil, ""},
	}
	for _, tc := range cases {
		if got := innerTubeThumb(tc.in); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
