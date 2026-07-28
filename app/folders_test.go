package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/shared"
)

/* Pastas na camada de aplicação. O que estes testes travam:

   - a listagem é UNIÃO (catálogo vazio + em uso + automáticas de fonte) e a ORDEM é
     fixa — a tela não pode dançar entre cargas;
   - excluir pasta NUNCA apaga faixa (a faixa cai em "Sem pasta");
   - streaming não se move de pasta (a dele é derivada da fonte). */

// catálogo de pastas em memória, com gatilho de erro por operação — os caminhos de
// falha são metade da regra (a listagem não pode devolver meia-verdade).
type fakeFolders struct {
	names    []string
	errList  error
	errCreat error
	errDel   error
	created  []string
	deleted []string
}

func (f *fakeFolders) List(context.Context) ([]string, error) { return f.names, f.errList }
func (f *fakeFolders) Create(_ context.Context, n string) error {
	if f.errCreat != nil {
		return f.errCreat
	}
	f.created = append(f.created, n)
	for _, e := range f.names {
		if library.SameFolder(e, n) {
			return nil // idempotente
		}
	}
	f.names = append(f.names, n)
	return nil
}
func (f *fakeFolders) Delete(_ context.Context, n string) error {
	if f.errDel != nil {
		return f.errDel
	}
	f.deleted = append(f.deleted, n)
	out := f.names[:0]
	for _, e := range f.names {
		if !library.SameFolder(e, n) {
			out = append(out, e)
		}
	}
	f.names = out
	return nil
}

func trackWithFolders(t *testing.T, id string, src shared.SourceKind, pastas ...string) *library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, src, "T "+id, time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	switch src {
	case shared.SourceSpotify:
		tr.SpotifyURI = "spotify:track:" + id
	case shared.SourceYouTube:
		tr.YouTubeURI = "yt:" + id
	default:
		tr.Path = "/m/" + id + ".mp3"
	}
	tr.Folders = pastas
	return tr
}

func newFolderService(t *testing.T) (*LibraryService, *memory.TrackRepo, *fakeFolders) {
	t.Helper()
	repo := memory.New()
	cat := &fakeFolders{}
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil).WithFolders(cat)
	return svc, repo, cat
}

func TestFolders_UnionAndOrder(t *testing.T) {
	ctx := context.Background()
	svc, repo, cat := newFolderService(t)
	// duas locais em pastas (uma com caixa diferente: tem que somar junto), uma
	// local solta, uma do Spotify e uma do YouTube.
	for _, tr := range []*library.Track{
		trackWithFolders(t, "1", shared.SourceLocal, "Rock"),
		trackWithFolders(t, "2", shared.SourceLocal, "rock", "anos 90"),
		trackWithFolders(t, "3", shared.SourceLocal),
		trackWithFolders(t, "4", shared.SourceSpotify),
		trackWithFolders(t, "5", shared.SourceYouTube),
	} {
		if err := repo.Upsert(ctx, tr); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	cat.names = []string{"vazia"} // pasta criada e ainda sem faixa

	got, err := svc.Folders(ctx)
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	names := make([]string, len(got))
	for i, f := range got {
		names[i] = f.Name
	}
	// automáticas primeiro, depois "Sem pasta", depois as do operador em ordem
	want := []string{library.FolderSpotify, library.FolderYouTube, FolderUnfiled, "anos 90", "Rock", "vazia"}
	if len(names) != len(want) {
		t.Fatalf("ordem/qtd: got %v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("posição %d: got %q want %q (todas: %v)", i, names[i], want[i], names)
		}
	}
	byName := map[string]library.FolderCount{}
	for _, f := range got {
		byName[f.Name] = f
	}
	if c := byName["Rock"].Count; c != 2 {
		t.Fatalf("Rock deve somar caixa diferente: got %d want 2", c)
	}
	if c := byName["vazia"].Count; c != 0 {
		t.Fatalf("pasta do catálogo sem faixa deve contar 0, got %d", c)
	}
	if c := byName[FolderUnfiled].Count; c != 1 {
		t.Fatalf("Sem pasta deve contar só a local solta, got %d", c)
	}
	if !byName[library.FolderSpotify].Auto || !byName[FolderUnfiled].Auto || byName["Rock"].Auto {
		t.Fatalf("flag Auto errada: %+v", got)
	}
}

func TestFolders_EmptyServiceFolderIsHidden(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newFolderService(t)
	_ = repo.Upsert(ctx, trackWithFolders(t, "1", shared.SourceLocal, "Rock"))
	got, err := svc.Folders(ctx)
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	for _, f := range got {
		if f.Name == library.FolderSpotify || f.Name == library.FolderYouTube {
			t.Fatalf("pasta de serviço sem faixa não deve aparecer: %v", got)
		}
	}
}

func TestFolders_CatalogErrorPropagates(t *testing.T) {
	ctx := context.Background()
	svc, _, cat := newFolderService(t)
	cat.errList = errors.New("catálogo fora")
	if _, err := svc.Folders(ctx); err == nil {
		t.Fatal("erro do catálogo deve propagar (listagem meio-certa engana mais que falhar)")
	}
}

func TestFolders_ListsWithoutCatalog(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil) // sem WithFolders
	_ = repo.Upsert(ctx, trackWithFolders(t, "1", shared.SourceLocal, "Rock"))
	got, err := svc.Folders(ctx)
	if err != nil || len(got) != 1 || got[0].Name != "Rock" {
		t.Fatalf("sem catálogo as pastas em uso ainda valem: %v err=%v", got, err)
	}
	// mas criar pasta vazia não tem onde guardar
	if err := svc.CreateFolder(ctx, "nova"); err == nil {
		t.Fatal("CreateFolder sem catálogo deve falhar")
	}
}

func TestCreateFolder_NormalizesAndValidates(t *testing.T) {
	ctx := context.Background()
	svc, _, cat := newFolderService(t)
	if err := svc.CreateFolder(ctx, "  anos   90 "); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if len(cat.created) != 1 || cat.created[0] != "anos 90" {
		t.Fatalf("nome deve chegar normalizado ao catálogo: %v", cat.created)
	}
	if err := svc.CreateFolder(ctx, "   "); err == nil {
		t.Fatal("nome vazio deve ser recusado")
	}
	cat.errCreat = errors.New("disco cheio")
	if err := svc.CreateFolder(ctx, "rock"); err == nil {
		t.Fatal("erro do catálogo deve propagar")
	}
}

func TestDeleteFolder_RemovesFromTracksWithoutDeletingAny(t *testing.T) {
	ctx := context.Background()
	svc, repo, cat := newFolderService(t)
	_ = repo.Upsert(ctx, trackWithFolders(t, "1", shared.SourceLocal, "Rock", "anos 90"))
	_ = repo.Upsert(ctx, trackWithFolders(t, "2", shared.SourceLocal, "ROCK")) // caixa diferente
	_ = repo.Upsert(ctx, trackWithFolders(t, "3", shared.SourceLocal, "sertanejo"))
	cat.names = []string{"Rock"}

	if err := svc.DeleteFolder(ctx, "rock"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	all, _ := repo.List(ctx)
	if len(all) != 3 {
		t.Fatalf("excluir pasta NUNCA apaga faixa: sobraram %d de 3", len(all))
	}
	t1, _ := repo.Get(ctx, "1")
	if len(t1.Folders) != 1 || t1.Folders[0] != "anos 90" {
		t.Fatalf("faixa 1 deve manter as outras pastas: %v", t1.Folders)
	}
	t2, _ := repo.Get(ctx, "2")
	if len(t2.Folders) != 0 {
		t.Fatalf("faixa 2 deve cair em Sem pasta (caixa não importa): %v", t2.Folders)
	}
	t3, _ := repo.Get(ctx, "3")
	if len(t3.Folders) != 1 {
		t.Fatalf("faixa de outra pasta não podia ser tocada: %v", t3.Folders)
	}
	if len(cat.deleted) != 1 {
		t.Fatalf("catálogo deve receber o delete: %v", cat.deleted)
	}
}

func TestDeleteFolder_NoCatalogAndCatalogError(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil)
	_ = repo.Upsert(ctx, trackWithFolders(t, "1", shared.SourceLocal, "Rock"))
	if err := svc.DeleteFolder(ctx, "Rock"); err != nil {
		t.Fatalf("sem catálogo o delete ainda limpa as faixas: %v", err)
	}
	t1, _ := repo.Get(ctx, "1")
	if len(t1.Folders) != 0 {
		t.Fatalf("pasta devia sair da faixa: %v", t1.Folders)
	}

	svc2, repo2, cat := newFolderService(t)
	_ = repo2.Upsert(ctx, trackWithFolders(t, "9", shared.SourceLocal, "x"))
	cat.errDel = errors.New("catálogo fora")
	if err := svc2.DeleteFolder(ctx, "x"); err == nil {
		t.Fatal("erro do catálogo deve propagar")
	}
}

func TestSetTrackFolders(t *testing.T) {
	ctx := context.Background()
	svc, repo, cat := newFolderService(t)
	_ = repo.Upsert(ctx, trackWithFolders(t, "1", shared.SourceLocal))
	_ = repo.Upsert(ctx, trackWithFolders(t, "2", shared.SourceSpotify))

	got, err := svc.SetTrackFolders(ctx, "1", []string{" Rock ", "rock", "anos 90"})
	if err != nil {
		t.Fatalf("SetTrackFolders: %v", err)
	}
	// dedupe ignora caixa/espaço e o domínio devolve ORDENADO (a tela lê daqui).
	if len(got.Folders) != 2 || got.Folders[0] != "anos 90" || got.Folders[1] != "Rock" {
		t.Fatalf("normalização/dedupe/ordem falhou: %v", got.Folders)
	}
	// pastas novas entram no catálogo pra não sumirem se a faixa sair delas
	if len(cat.created) != 2 {
		t.Fatalf("pastas novas deviam entrar no catálogo: %v", cat.created)
	}
	if _, err := svc.SetTrackFolders(ctx, "2", []string{"Rock"}); !errors.Is(err, library.ErrFolderStreaming) {
		t.Fatalf("streaming não se move de pasta, got %v", err)
	}
	if _, err := svc.SetTrackFolders(ctx, "inexistente", nil); !errors.Is(err, library.ErrTrackNotFound) {
		t.Fatalf("faixa inexistente → ErrTrackNotFound, got %v", err)
	}
	names := make([]string, 0, library.MaxFoldersPerTrack+1)
	for i := 0; i <= library.MaxFoldersPerTrack; i++ {
		names = append(names, string(rune('a'+i))+"pasta")
	}
	if _, err := svc.SetTrackFolders(ctx, "1", names); err == nil {
		t.Fatal("acima do teto de pastas por faixa deve falhar")
	}
}
