package library

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"autodj/domain/shared"
)

/*
Pastas do operador — "rock", "anos 90", "vinhetas do João".

Por que NÃO reaproveitar `Category`: aquela é um enum FIXO (music, jingle,
commercial…) que alimenta o rodízio da rotação. Uma pasta livre ali quebraria a
programação. São eixos diferentes: categoria diz O QUE a faixa é; pasta diz onde o
operador guardou.

Por que texto livre e não uma entidade com id: o operador cria e renomeia pasta o
tempo todo, e nenhuma regra do domínio depende do nome. Id daria trabalho de
migração sem comprar nada.

Spotify e YouTube não gravam pasta: a deles é DERIVADA de Source (ver
SourceFolder). Assim a faixa nasce na pasta certa e nunca desalinha.
*/

const (
	// MaxFolderName limita o nome pra caber na UI sem truncar no meio.
	MaxFolderName = 40
	// MaxFoldersPerTrack evita que uma faixa vire uma lista de etiquetas.
	MaxFoldersPerTrack = 12
)

var (
	// ErrFolderVazia é nome só com espaços (ou vazio).
	ErrFolderEmpty = errors.New("library: nome de pasta vazio")
	// ErrFolderLonga é nome acima de MaxFolderName.
	ErrFolderTooLong = errors.New("library: nome de pasta muito longo")
	// ErrFoldersDemais é passar de MaxFoldersPerTrack numa faixa.
	ErrTooManyFolders = errors.New("library: faixa em pastas demais")
	// ErrFolderStreaming é tentar mover faixa de Spotify/YouTube de pasta: a pasta
	// delas é derivada da fonte (pasta única por serviço), não editável.
	ErrFolderStreaming = errors.New("library: faixa de streaming tem pasta fixa da fonte")
)

// NormalizeFolder limpa o nome como o operador digitou: tira espaços das pontas e
// colapsa os do meio. NÃO muda a caixa — "Anos 90" é como ele quer ver escrito; a
// comparação é que ignora caixa (ver SameFolder).
func NormalizeFolder(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrFolderEmpty
	}
	if utf8.RuneCountInString(s) > MaxFolderName {
		return "", ErrFolderTooLong
	}
	return s, nil
}

// SameFolder compara dois nomes ignorando caixa e espaços — "Anos 90", "anos 90" e
// " ANOS  90 " são a mesma pasta. Sem isto o operador criaria duplicatas sem ver.
func SameFolder(a, b string) bool {
	na := strings.ToLower(strings.Join(strings.Fields(a), " "))
	nb := strings.ToLower(strings.Join(strings.Fields(b), " "))
	return na == nb
}

// NormalizeFolders limpa a lista inteira: normaliza cada nome, descarta vazios,
// remove duplicatas (ignorando caixa, preservando a PRIMEIRA grafia digitada) e
// ordena pra listagem ficar estável. Estoura em nome inválido ou lista longa demais.
func NormalizeFolders(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		if strings.TrimSpace(raw) == "" {
			continue // entrada vazia é descarte silencioso, não erro
		}
		n, err := NormalizeFolder(raw)
		if err != nil {
			return nil, err
		}
		dup := false
		for _, j := range out {
			if SameFolder(j, n) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, n)
		}
	}
	if len(out) > MaxFoldersPerTrack {
		return nil, ErrTooManyFolders
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out, nil
}

// Nomes das pastas automáticas por fonte. Ficam aqui (e não na UI) pra que a
// listagem e o filtro concordem sempre sobre o mesmo rótulo.
const (
	FolderSpotify = "Spotify"
	FolderYouTube = "YouTube"
)

// SourceFolder devolve a pasta automática de uma fonte, ou "" quando a fonte não
// tem uma (local, line_in, tts — essas usam as pastas do operador).
func SourceFolder(s shared.SourceKind) string {
	switch s {
	case shared.SourceSpotify:
		return FolderSpotify
	case shared.SourceYouTube:
		return FolderYouTube
	default:
		return ""
	}
}

// TrackFolders são as pastas EFETIVAS de uma faixa: a automática da fonte quando
// existe, senão as do operador. Uma faixa de streaming não mistura os dois eixos —
// decisão do produto: pasta única por serviço.
func TrackFolders(t Track) []string {
	if f := SourceFolder(t.Source); f != "" {
		return []string{f}
	}
	return t.Folders
}

// FolderCount é uma pasta na listagem, com quantas faixas tem dentro.
type FolderCount struct {
	Name string
	// Count é quantas faixas estão na pasta.
	Count int
	// Auto marca a pasta DERIVADA de uma fonte (Spotify/YouTube): não é criada nem
	// removida pelo operador, e a UI não deve oferecer renomear/excluir.
	Auto bool
}
