package youtube

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/raitonoberu/ytsearch"

	domyoutube "autodj/domain/youtube"
)

// innerTubeMax caps how many results a free search returns, mirroring the Data
// API adapter's maxResults=10 so both backends fill the same panel.
const innerTubeMax = 10

// innerTubeTimeout bounds one InnerTube round-trip. The library's Next() takes no
// context, so the deadline has to live in the http.Client it is handed.
const innerTubeTimeout = 12 * time.Second

// InnerTube implements outbound.YouTubeCatalog by asking YouTube the way the
// website itself does — the public InnerTube endpoint behind
// github.com/raitonoberu/ytsearch — instead of the Data API's search.list.
//
// Why it exists: search.list costs ~100 units of the ~10k/day free quota (~100
// searches for EVERY visitor of a public instance), which is the only reason the
// panel ever gated search behind a Google login. InnerTube needs no API key, no
// OAuth and no quota, so search works logged out and cannot exhaust anything.
//
// It REPLACES nothing: Data is the Data API adapter and stays the source of
// truth for everything InnerTube cannot answer well —
//   - Videos/PlaylistItems delegate to it verbatim (1 unit per batch), so imports
//     keep exact ISO-8601 durations and, crucially, the status.embeddable drop;
//   - Search enriches its own hits through Data.Videos when available (1 unit
//     instead of 100, same embeddable filter);
//   - a parse/network failure falls back to Data.Search entirely.
//
// With Data nil (or its quota spent) search still answers from InnerTube alone:
// durations come from the library and non-embeddable videos are no longer
// filtered out — the browser's embed-block detector skips those at playout.
type InnerTube struct {
	// Data is the Data API catalog used for delegation, enrichment and fallback.
	// Optional: nil means "free search only, no Data API at all".
	Data *Catalog
	// Timeout overrides innerTubeTimeout for one InnerTube round-trip.
	Timeout time.Duration
	// BaseURL overrides the InnerTube host (tests point it at an httptest server).
	BaseURL string
	// search performs the actual InnerTube call. nil → the live implementation;
	// tests inject a stub (the library talks to youtube.com and takes no context).
	search func(ctx context.Context, query, kind string) (*ytsearch.SearchResult, error)
}

// NewInnerTube builds a free-search catalog that delegates to data for video
// lookup, playlist contents and fallback. data may be nil.
func NewInnerTube(data *Catalog) *InnerTube { return &InnerTube{Data: data} }

// Search queries YouTube without an API key and without spending Data API quota.
// token is ignored (shape parity with the Data API adapter — InnerTube is
// anonymous by construction).
func (i *InnerTube) Search(ctx context.Context, _, query, kind string) ([]domyoutube.CatalogItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domyoutube.CatalogItem{}, nil
	}
	res, err := i.searchFn()(ctx, query, kind)
	if err != nil || res == nil {
		// InnerTube scrapes an undocumented renderer tree: YouTube can reshape it
		// any day. Rather than break search, pay the 100 units and answer.
		if i.Data != nil {
			return i.Data.Search(ctx, "", query, kind)
		}
		if err == nil {
			err = domyoutube.ErrCatalogUnavailable
		}
		return nil, err
	}
	if kind == domyoutube.KindPlaylist {
		return innerTubePlaylists(res), nil
	}
	items := innerTubeVideos(res)
	if len(items) == 0 {
		if i.Data != nil {
			return i.Data.Search(ctx, "", query, kind)
		}
		return []domyoutube.CatalogItem{}, nil
	}
	// One videos.list (1 unit) upgrades the whole page: exact durations and the
	// embeddable drop the free path cannot see. Quota spent / API down → keep the
	// free results instead of failing the search.
	if i.Data != nil {
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.URI)
		}
		if got, gerr := i.Data.Videos(ctx, "", ids); gerr == nil && len(got) > 0 {
			return got, nil
		}
	}
	return items, nil
}

// PlaylistItems lê o conteúdo de uma playlist.
//
// Data API PRIMEIRO quando há key: ela traz o filtro `status.embeddable` e pagina a
// playlist inteira, coisas que o caminho grátis não faz. Sem key (ou com a Data API
// falhando/sem cota), cai no InnerTube — que é o que torna a aba utilizável com ZERO
// credencial. Ver innertube_meta.go para os limites do caminho grátis.
func (i *InnerTube) PlaylistItems(ctx context.Context, token, playlistID string) ([]domyoutube.CatalogItem, error) {
	if i.Data != nil {
		got, err := i.Data.PlaylistItems(ctx, token, playlistID)
		// Só cai no grátis quando a Data API FALHOU. Lista vazia é RESPOSTA, não
		// falha: significa que o filtro `status.embeddable` do Google derrubou tudo.
		// Tratar vazio como falha (bug da v0.2.22) fazia o caminho grátis — que não
		// tem esse filtro — reimportar exatamente os vídeos que o Google acabara de
		// recusar. Eles entram na biblioteca e no ar não sai som.
		if err == nil {
			return got, nil
		}
	}
	return i.playlistItemsFree(ctx, playlistID)
}

// Videos resolve ids em metadados. Mesma ordem de preferência do PlaylistItems: Data
// API quando configurada (durações exatas + drop de não-embeddable, 1 unidade por
// lote), InnerTube quando não há key ou quando ela falha.
func (i *InnerTube) Videos(ctx context.Context, token string, ids []string) ([]domyoutube.CatalogItem, error) {
	if i.Data != nil {
		got, err := i.Data.Videos(ctx, token, ids)
		// idem PlaylistItems: vazio é resposta (todos não-incorporáveis), não falha.
		if err == nil {
			return got, nil
		}
	}
	return i.videosFree(ctx, ids)
}

func (i *InnerTube) searchFn() func(context.Context, string, string) (*ytsearch.SearchResult, error) {
	if i.search != nil {
		return i.search
	}
	return i.liveSearch
}

func (i *InnerTube) timeout() time.Duration {
	if i.Timeout > 0 {
		return i.Timeout
	}
	return innerTubeTimeout
}

// liveSearch hits the real InnerTube endpoint. The library's Next() has no
// context parameter, so cancellation rides in through the http.Client: a
// RoundTripper re-attaches ctx to the outgoing request, and Timeout bounds it.
func (i *InnerTube) liveSearch(ctx context.Context, query, kind string) (*ytsearch.SearchResult, error) {
	var cl *ytsearch.SearchClient
	if kind == domyoutube.KindPlaylist {
		cl = ytsearch.PlaylistSearch(query)
	} else {
		cl = ytsearch.VideoSearch(query)
	}
	cl.Language, cl.Region = "pt", "BR"
	cl.HTTPClient = &http.Client{
		Timeout:   i.timeout(),
		Transport: ctxTransport{ctx: ctx, base: http.DefaultTransport},
	}
	return cl.Next()
}

// ctxTransport binds a context to every request of a client whose caller has no
// way to pass one (ytsearch builds its requests with http.NewRequest).
type ctxTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t ctxTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.ctx == nil {
		return base.RoundTrip(r)
	}
	return base.RoundTrip(r.WithContext(t.ctx))
}

// innerTubeVideos maps library video hits to catalog items. Entries without a
// duration (live streams, premieres) are dropped: the import path rejects dur<=0
// anyway, so showing them would only offer rows that cannot be imported.
func innerTubeVideos(res *ytsearch.SearchResult) []domyoutube.CatalogItem {
	out := make([]domyoutube.CatalogItem, 0, innerTubeMax)
	for _, v := range res.Videos {
		if v == nil || v.ID == "" || v.Duration <= 0 {
			continue
		}
		out = append(out, domyoutube.CatalogItem{
			URI:        v.ID,
			Title:      v.Title,
			Artist:     v.Channel.Title,
			ArtworkURL: innerTubeThumb(v.Thumbnails),
			DurationMs: v.Duration * 1000, // library reports SECONDS
			Kind:       domyoutube.KindVideo,
		})
		if len(out) == innerTubeMax {
			break
		}
	}
	return out
}

// innerTubePlaylists maps library playlist hits. Duration is unknown up front
// (it is the sum of the videos), so it stays 0 — same as the Data API path.
func innerTubePlaylists(res *ytsearch.SearchResult) []domyoutube.CatalogItem {
	out := make([]domyoutube.CatalogItem, 0, innerTubeMax)
	for _, p := range res.Playlists {
		if p == nil || p.ID == "" {
			continue
		}
		out = append(out, domyoutube.CatalogItem{
			URI:        p.ID,
			Title:      p.Title,
			Artist:     p.Channel.Title,
			ArtworkURL: innerTubeThumb(p.Thumbnails),
			Kind:       domyoutube.KindPlaylist,
		})
		if len(out) == innerTubeMax {
			break
		}
	}
	return out
}

// innerTubeThumb picks the widest thumbnail that still fits a 60×60 deck slot's
// budget (≤480px), matching the Data API adapter's "medium" preference; with
// nothing under the cap it takes the widest available.
func innerTubeThumb(ts []ytsearch.Thumbnail) string {
	best, bestW := "", 0
	widest, widestW := "", 0
	for _, t := range ts {
		if t.URL == "" {
			continue
		}
		if t.Width > widestW {
			widest, widestW = t.URL, t.Width
		}
		if t.Width <= 480 && t.Width > bestW {
			best, bestW = t.URL, t.Width
		}
	}
	if best != "" {
		return best
	}
	return widest
}
