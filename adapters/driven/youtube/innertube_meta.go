package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	domyoutube "autodj/domain/youtube"
)

/*
Resolução de METADADOS sem credencial nenhuma — o que faltava pro YouTube funcionar
com zero configuração além de existir internet.

A busca já era grátis (lib ytsearch, ver innertube.go). Faltavam os dois caminhos que
o import usa, e que até aqui só a Data API respondia:

  - id avulso (colar link)  → POST /youtubei/v1/player  {videoId}
  - conteúdo de playlist    → POST /youtubei/v1/next    {playlistId}

Ambos são os mesmos endpoints que o site do YouTube chama, sem API key e sem OAuth.

ARMADILHA MEDIDA (27/07/2026), a mais importante daqui: o `playabilityStatus` que o
`player` devolve NÃO diz se o vídeo toca no browser do operador. Voltou `UNPLAYABLE`
para vídeo que toca normalmente — é detecção de bot (requisição de servidor, sem
sessão/PO-token), não bloqueio de incorporação. Usar esse campo como filtro rejeitaria
quase tudo. Ele é IGNORADO de propósito; `videoDetails` vem correto mesmo com o status
ruim. Quem decide de verdade é o IFrame no browser, que já sabe pular faixa bloqueada.

Clients testados no `player`: WEB e MWEB devolvem `videoDetails`; ANDROID, IOS e
TVHTML5 voltam vazios (exigem auth mais nova). Por isso WEB fixo.

Limitação conhecida da playlist: o painel do `next` devolve a janela corrente, não
necessariamente a playlist inteira — medido 19 de 20 itens (o que falta costuma ser
vídeo privado/removido). Sem paginação por continuation, que é frágil e muda de forma
com o layout. Para playlist grande, a Data API (quando há key) continua mais completa.
*/

// innerTubeBase é o host dos endpoints InnerTube. Variável (não const) só para os
// testes apontarem para um httptest.Server.
var innerTubeBase = "https://www.youtube.com/youtubei/v1"

// innerTubeClient é o contexto de client exigido em toda chamada InnerTube.
var innerTubeClient = map[string]any{
	"client": map[string]any{
		"clientName":    "WEB",
		"clientVersion": "2.20240722.01.00",
		"hl":            "pt",
		"gl":            "BR",
	},
}

// innerTubeMetaConcurrency limita as chamadas simultâneas do resolve de ids: um lote
// de 50 links viraria 50 requisições instantâneas ao YouTube, que é exatamente o
// padrão que dispara bloqueio por bot.
const innerTubeMetaConcurrency = 4

// videosFree resolve ids em metadados pelo endpoint `player`, um id por chamada.
// Devolve só o que respondeu — id inválido/privado some do lote, mesmo comportamento
// do filtro de não-embeddable da Data API (o chamador compara len() se quiser avisar).
func (i *InnerTube) videosFree(ctx context.Context, ids []string) ([]domyoutube.CatalogItem, error) {
	ids = dedupeNonEmpty(ids)
	if len(ids) == 0 {
		return []domyoutube.CatalogItem{}, nil
	}
	out := make([]domyoutube.CatalogItem, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, innerTubeMetaConcurrency)
	for n, id := range ids {
		wg.Add(1)
		go func(n int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, err := i.playerItem(ctx, id); err == nil {
				out[n] = it
			}
		}(n, id)
	}
	wg.Wait()

	res := make([]domyoutube.CatalogItem, 0, len(ids))
	for _, it := range out {
		if it.URI != "" {
			res = append(res, it)
		}
	}
	if len(res) == 0 {
		// nenhum id respondeu: rede fora, YouTube bloqueando ou ids todos inválidos.
		// Erro (em vez de lista vazia) pro chamador poder cair na Data API.
		return nil, domyoutube.ErrCatalogUnavailable
	}
	return res, nil
}

// playerItem resolve UM id.
func (i *InnerTube) playerItem(ctx context.Context, id string) (domyoutube.CatalogItem, error) {
	var body struct {
		VideoDetails struct {
			VideoID       string `json:"videoId"`
			Title         string `json:"title"`
			Author        string `json:"author"`
			LengthSeconds string `json:"lengthSeconds"` // string no payload
		} `json:"videoDetails"`
	}
	if err := i.post(ctx, "player", map[string]any{"videoId": id}, &body); err != nil {
		return domyoutube.CatalogItem{}, err
	}
	d := body.VideoDetails
	if d.VideoID == "" || d.Title == "" {
		return domyoutube.CatalogItem{}, domyoutube.ErrCatalogUnavailable
	}
	secs, _ := strconv.Atoi(d.LengthSeconds)
	if secs <= 0 {
		// live/premiere não tem duração; o import rejeita dur<=0 de qualquer forma.
		return domyoutube.CatalogItem{}, domyoutube.ErrCatalogUnavailable
	}
	return domyoutube.CatalogItem{
		URI:        d.VideoID,
		Title:      d.Title,
		Artist:     d.Author,
		ArtworkURL: ytThumbURL(d.VideoID),
		DurationMs: secs * 1000,
		Kind:       domyoutube.KindVideo,
	}, nil
}

// playlistItemsFree lê o conteúdo de uma playlist pelo endpoint `next`.
func (i *InnerTube) playlistItemsFree(ctx context.Context, playlistID string) ([]domyoutube.CatalogItem, error) {
	playlistID = strings.TrimSpace(playlistID)
	if playlistID == "" {
		return nil, domyoutube.ErrCatalogUnavailable
	}
	var raw map[string]any
	if err := i.post(ctx, "next", map[string]any{"playlistId": playlistID}, &raw); err != nil {
		return nil, err
	}
	nodes := findRenderers(raw, "playlistPanelVideoRenderer")
	out := make([]domyoutube.CatalogItem, 0, len(nodes))
	for _, n := range nodes {
		id, _ := n["videoId"].(string)
		title := renderText(n["title"])
		if id == "" || title == "" {
			continue
		}
		ms := parseClock(renderText(n["lengthText"]))
		if ms <= 0 {
			continue
		}
		out = append(out, domyoutube.CatalogItem{
			URI:        id,
			Title:      title,
			Artist:     renderText(n["shortBylineText"]),
			ArtworkURL: ytThumbURL(id),
			DurationMs: ms,
			Kind:       domyoutube.KindVideo,
		})
	}
	if len(out) == 0 {
		return nil, domyoutube.ErrCatalogUnavailable
	}
	return out, nil
}

// post faz uma chamada InnerTube e decodifica a resposta em v.
func (i *InnerTube) post(ctx context.Context, endpoint string, payload map[string]any, v any) error {
	body := map[string]any{"context": innerTubeClient}
	for k, val := range payload {
		body[k] = val
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.baseURL()+"/"+endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	// sem UA de browser o endpoint responde vazio com mais frequência.
	req.Header.Set("user-agent", "Mozilla/5.0")
	res, err := (&http.Client{Timeout: i.timeout()}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("innertube %s: status %d: %w", endpoint, res.StatusCode, domyoutube.ErrCatalogUnavailable)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

func (i *InnerTube) baseURL() string {
	if i.BaseURL != "" {
		return i.BaseURL
	}
	return innerTubeBase
}

// ytThumbURL deriva a miniatura pública a partir do id (mesma forma que o
// router usa como fallback de artwork). Não custa requisição.
func ytThumbURL(id string) string {
	return "https://i.ytimg.com/vi/" + id + "/mqdefault.jpg"
}

// findRenderers varre a árvore procurando todas as ocorrências de uma chave. O
// payload do InnerTube é uma árvore de renderers sem esquema publicado; tipar tudo
// seria pior (quebra a cada campo novo) do que procurar exatamente o nó que interessa.
func findRenderers(node any, key string) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			for k, v := range t {
				if k == key {
					if m, ok := v.(map[string]any); ok {
						out = append(out, m)
						continue
					}
				}
				walk(v)
			}
		case []any:
			for _, v := range t {
				walk(v)
			}
		}
	}
	walk(node)
	return out
}

// renderText extrai texto de um nó do InnerTube, que aparece em duas formas:
// {"simpleText":"x"} e {"runs":[{"text":"x"},…]}.
func renderText(n any) string {
	m, ok := n.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["simpleText"].(string); ok && s != "" {
		return s
	}
	runs, ok := m["runs"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, r := range runs {
		if rm, ok := r.(map[string]any); ok {
			if s, ok := rm["text"].(string); ok {
				b.WriteString(s)
			}
		}
	}
	return b.String()
}

// parseClock converte "H:MM:SS" ou "M:SS" em milissegundos. Formato desconhecido → 0.
func parseClock(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return 0
		}
		total = total*60 + n
	}
	return total * 1000
}

// dedupeNonEmpty limpa a lista de ids preservando a ordem de entrada.
func dedupeNonEmpty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
