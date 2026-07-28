package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domyoutube "autodj/domain/youtube"
)

// innerTubeStub sobe um servidor que responde /player e /next como o InnerTube.
func innerTubeStub(t *testing.T, h func(endpoint string, body map[string]any) (any, int)) *InnerTube {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		out, code := h(parts[len(parts)-1], body)
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return &InnerTube{BaseURL: srv.URL}
}

func playerBody(id, title, author, lenSecs, playability string) map[string]any {
	return map[string]any{
		"playabilityStatus": map[string]any{"status": playability},
		"videoDetails": map[string]any{
			"videoId": id, "title": title, "author": author, "lengthSeconds": lenSecs,
		},
	}
}

func TestPlayerItem_mapeiaMetadados(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return playerBody("dGFxdmuDA4A", "Diário de um Detento", "Racionais MC's", "451", "OK"), 200
	})
	got, err := it.playerItem(context.Background(), "dGFxdmuDA4A")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Diário de um Detento" || got.Artist != "Racionais MC's" {
		t.Fatalf("metadados = %+v", got)
	}
	if got.DurationMs != 451000 { // lengthSeconds vem STRING e em segundos
		t.Fatalf("DurationMs = %d, want 451000", got.DurationMs)
	}
	if got.ArtworkURL != "https://i.ytimg.com/vi/dGFxdmuDA4A/mqdefault.jpg" {
		t.Fatalf("thumb = %q", got.ArtworkURL)
	}
	if got.Kind != domyoutube.KindVideo {
		t.Fatalf("kind = %q", got.Kind)
	}
}

// INVARIANTE: playabilityStatus medido do servidor é bot-detection, não bloqueio de
// incorporação — voltou UNPLAYABLE para vídeo que toca normal. Se alguém passar a
// filtrar por ele, o import para de trazer quase tudo.
func TestPlayerItem_ignoraPlayabilityStatus(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return playerBody("abc", "Toca sim", "Canal", "200", "UNPLAYABLE"), 200
	})
	got, err := it.playerItem(context.Background(), "abc")
	if err != nil {
		t.Fatalf("UNPLAYABLE não pode descartar o vídeo: %v", err)
	}
	if got.URI != "abc" {
		t.Fatalf("item = %+v", got)
	}
}

func TestPlayerItem_WithoutDurationIsDropped(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return playerBody("live1", "Transmissão ao vivo", "Canal", "0", "OK"), 200
	})
	if _, err := it.playerItem(context.Background(), "live1"); err == nil {
		t.Fatal("duração 0 (live/premiere) deve ser recusada — o import rejeita dur<=0")
	}
}

func TestVideosFree_loteDedupeEDescarteParcial(t *testing.T) {
	it := innerTubeStub(t, func(_ string, body map[string]any) (any, int) {
		id, _ := body["videoId"].(string)
		if id == "ruim" {
			return map[string]any{}, 200 // sem videoDetails
		}
		return playerBody(id, "T-"+id, "Canal", "120", "OK"), 200
	})
	got, err := it.videosFree(context.Background(), []string{"a", "b", "a", "", "ruim"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (dedupe de 'a', vazio fora, 'ruim' descartado)", len(got))
	}
	if got[0].URI != "a" || got[1].URI != "b" {
		t.Fatalf("ordem de entrada não preservada: %+v", got)
	}
}

func TestVideosFree_AllFailBecomeErrCatalogUnavailable(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return map[string]any{}, 500
	})
	_, err := it.videosFree(context.Background(), []string{"a", "b"})
	if !errors.Is(err, domyoutube.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable (pro chamador poder cair na Data API)", err)
	}
}

func nextBody(items ...map[string]any) map[string]any {
	conts := make([]any, 0, len(items))
	for _, it := range items {
		conts = append(conts, map[string]any{"playlistPanelVideoRenderer": it})
	}
	// aninhado de propósito: o parser tem que ACHAR o renderer, não assumir o caminho
	return map[string]any{
		"contents": map[string]any{
			"twoColumnWatchNextResults": map[string]any{
				"playlist": map[string]any{"playlist": map[string]any{"contents": conts}},
			},
		},
	}
}

func TestPlaylistItemsFree_parseDoPainel(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return nextBody(
			map[string]any{
				"videoId":         "0VH1Lim8gL8",
				"title":           map[string]any{"runs": []any{map[string]any{"text": "Deep Learning "}, map[string]any{"text": "2020"}}},
				"lengthText":      map[string]any{"simpleText": "1:27:41"},
				"shortBylineText": map[string]any{"runs": []any{map[string]any{"text": "Lex Fridman"}}},
			},
			map[string]any{ // sem duração → fora
				"videoId": "semdur",
				"title":   map[string]any{"simpleText": "Sem duração"},
			},
		), 200
	})
	got, err := it.playlistItemsFree(context.Background(), "PLabc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Title != "Deep Learning 2020" { // runs concatenados
		t.Fatalf("title = %q", got[0].Title)
	}
	if got[0].Artist != "Lex Fridman" {
		t.Fatalf("artist = %q", got[0].Artist)
	}
	if got[0].DurationMs != (1*3600+27*60+41)*1000 {
		t.Fatalf("dur = %d", got[0].DurationMs)
	}
}

func TestPlaylistItemsFree_EmptyBecomesError(t *testing.T) {
	it := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		return nextBody(), 200
	})
	if _, err := it.playlistItemsFree(context.Background(), "PLvazia"); !errors.Is(err, domyoutube.ErrCatalogUnavailable) {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

// REGRESSÃO (v0.2.22 → corrigido): a Data API devolvendo lista VAZIA é resposta, não
// falha — quer dizer que o filtro `status.embeddable` do Google derrubou tudo. Cair no
// caminho grátis nessa hora reimporta justamente o que o Google recusou; a faixa entra
// na biblioteca e no ar não sai som. Só falha (err != nil) autoriza o fallback.
func TestVideos_dataAPIVaziaNaoCaiNoGratis(t *testing.T) {
	var freeChamado bool
	data := newTestCatalog(t, func(w http.ResponseWriter, _ *http.Request) {
		// videos.list respondeu OK, porém sem itens: todos não-incorporáveis.
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	stub := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		freeChamado = true
		return playerBody("naodeviaentrar", "Nao incorporavel", "Canal", "180", "OK"), 200
	})
	stub.Data = data

	got, err := stub.Videos(context.Background(), "", []string{"naodeviaentrar"})
	if err != nil {
		t.Fatalf("err = %v, want nil (vazio é resposta válida)", err)
	}
	if len(got) != 0 {
		t.Fatalf("importou %d faixa(s) que o Google recusou: %+v", len(got), got)
	}
	if freeChamado {
		t.Fatal("caminho grátis foi chamado: o filtro de incorporação do Google foi contornado")
	}
}

func TestPlaylistItems_dataAPIVaziaNaoCaiNoGratis(t *testing.T) {
	var freeChamado bool
	data := newTestCatalog(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	stub := innerTubeStub(t, func(_ string, _ map[string]any) (any, int) {
		freeChamado = true
		return nextBody(map[string]any{
			"videoId":    "x",
			"title":      map[string]any{"simpleText": "Nao incorporavel"},
			"lengthText": map[string]any{"simpleText": "3:00"},
		}), 200
	})
	stub.Data = data

	got, err := stub.PlaylistItems(context.Background(), "", "PLx")
	if err != nil || len(got) != 0 || freeChamado {
		t.Fatalf("got=%d err=%v freeChamado=%v — vazio da Data API deve ser final", len(got), err, freeChamado)
	}
}

func TestParseClock(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"3:50", 230000},
		{"1:27:41", 5261000},
		{"0:05", 5000},
		{"", 0},
		{"abc", 0},
		{"1:2:3:4", 0},
		{"12", 0},
	} {
		if got := parseClock(tc.in); got != tc.want {
			t.Fatalf("parseClock(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRenderText(t *testing.T) {
	if got := renderText(map[string]any{"simpleText": "x"}); got != "x" {
		t.Fatalf("simpleText = %q", got)
	}
	runs := map[string]any{"runs": []any{map[string]any{"text": "a"}, map[string]any{"text": "b"}}}
	if got := renderText(runs); got != "ab" {
		t.Fatalf("runs = %q", got)
	}
	if got := renderText("string solta"); got != "" {
		t.Fatalf("tipo inesperado devia dar vazio, deu %q", got)
	}
}
