package app

import (
	"context"
	"sort"
	"strings"

	"autodj/domain/library"
	"autodj/ports/outbound"
)

/*
Pastas do operador na camada de aplicação.

A listagem é a UNIÃO de três origens, e cada uma existe por um motivo:

  - catálogo (tabela `folders`) — pasta criada e ainda VAZIA. Sem isso, criar uma
    pasta e recarregar a página faria ela sumir.
  - pastas em uso (JSON em `tracks.folders`) — a verdade sobre onde as faixas
    estão. Uma pasta pode existir aqui sem estar no catálogo (import, edição).
  - automáticas por fonte (Spotify/YouTube) — derivadas de `Source`, nunca
    gravadas: assim a faixa nasce na pasta certa e nada desalinha.

Mais "Sem pasta", que conta as faixas locais fora de qualquer pasta — decisão do
produto: elas aparecem na aba, não somem.
*/

// FolderSemPasta é o rótulo da pasta virtual que junta as faixas locais que não
// estão em nenhuma pasta. Não é criável nem removível.
const FolderUnfiled = "Sem pasta"

// WithFolders liga o catálogo de pastas ao serviço. Opcional: sem ele as pastas
// ainda funcionam (derivadas das faixas), só não é possível criar uma vazia.
func (s *LibraryService) WithFolders(c outbound.FolderCatalog) *LibraryService {
	s.folders = c
	return s
}

// Folders lista as pastas com a contagem de faixas de cada uma, em ordem: as
// automáticas de fonte primeiro (Spotify, YouTube), depois "Sem pasta", depois as
// do operador em ordem alfabética. A ordem é fixa pra tela não dançar entre cargas.
func (s *LibraryService) Folders(ctx context.Context) ([]library.FolderCount, error) {
	tracks, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	// contagem por pasta, com a chave em minúsculas pra "Rock"/"rock" somarem juntas
	type acc struct {
		name string
		n    int
	}
	used := map[string]*acc{}
	auto := map[string]int{}
	unfiled := 0

	for _, t := range tracks {
		if t == nil {
			continue
		}
		if a := library.SourceFolder(t.Source); a != "" {
			auto[a]++
			continue // streaming não entra nas pastas do operador (pasta única por serviço)
		}
		fs := t.Folders
		if len(fs) == 0 {
			unfiled++
			continue
		}
		for _, f := range fs {
			k := strings.ToLower(f)
			if cur, ok := used[k]; ok {
				cur.n++
			} else {
				used[k] = &acc{name: f, n: 1}
			}
		}
	}

	// catálogo entra com contagem 0 quando ninguém usa a pasta ainda
	if s.folders != nil {
		names, err := s.folders.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			k := strings.ToLower(n)
			if _, ok := used[k]; !ok {
				used[k] = &acc{name: n, n: 0}
			}
		}
	}

	out := make([]library.FolderCount, 0, len(used)+3)
	// automáticas primeiro, e só quando têm conteúdo — pasta de serviço vazia só
	// ocuparia espaço com o que o operador nem usa.
	for _, n := range []string{library.FolderSpotify, library.FolderYouTube} {
		if c := auto[n]; c > 0 {
			out = append(out, library.FolderCount{Name: n, Count: c, Auto: true})
		}
	}
	if unfiled > 0 {
		out = append(out, library.FolderCount{Name: FolderUnfiled, Count: unfiled, Auto: true})
	}

	operatorFolders := make([]library.FolderCount, 0, len(used))
	for _, a := range used {
		operatorFolders = append(operatorFolders, library.FolderCount{Name: a.name, Count: a.n})
	}
	sort.Slice(operatorFolders, func(i, j int) bool {
		return strings.ToLower(operatorFolders[i].Name) < strings.ToLower(operatorFolders[j].Name)
	})
	return append(out, operatorFolders...), nil
}

// CreateFolder registra uma pasta vazia. Idempotente.
func (s *LibraryService) CreateFolder(ctx context.Context, name string) error {
	if s.folders == nil {
		return library.ErrFolderEmpty // sem catálogo não há onde guardar pasta vazia
	}
	n, err := library.NormalizeFolder(name)
	if err != nil {
		return err
	}
	return s.folders.Create(ctx, n)
}

// DeleteFolder remove a pasta do catálogo E de todas as faixas que a usavam. As
// FAIXAS FICAM — excluir pasta nunca apaga música (decisão do produto). Faixa que
// ficar sem nenhuma pasta passa a contar em "Sem pasta".
func (s *LibraryService) DeleteFolder(ctx context.Context, name string) error {
	tracks, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, t := range tracks {
		if t == nil || len(t.Folders) == 0 {
			continue
		}
		remaining := make([]string, 0, len(t.Folders))
		for _, f := range t.Folders {
			if !library.SameFolder(f, name) {
				remaining = append(remaining, f)
			}
		}
		if len(remaining) == len(t.Folders) {
			continue // esta faixa não estava na pasta
		}
		t.Folders = remaining
		if err := s.repo.Upsert(ctx, t); err != nil {
			return err
		}
	}
	if s.folders == nil {
		return nil
	}
	return s.folders.Delete(ctx, name)
}

// SetTrackFolders troca as pastas de UMA faixa. Streaming é recusado: a pasta
// dele é derivada da fonte e não se move (pasta única por serviço).
func (s *LibraryService) SetTrackFolders(ctx context.Context, id string, folders []string) (*library.Track, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, library.ErrTrackNotFound
	}
	if library.SourceFolder(t.Source) != "" {
		return nil, library.ErrFolderStreaming
	}
	fs, err := library.NormalizeFolders(folders)
	if err != nil {
		return nil, err
	}
	t.Folders = fs
	if err := s.repo.Upsert(ctx, t); err != nil {
		return nil, err
	}
	// pastas novas entram no catálogo, pra continuarem existindo se a faixa sair
	if s.folders != nil {
		for _, f := range fs {
			_ = s.folders.Create(ctx, f)
		}
	}
	return t, nil
}
