package sqlite

import (
	"context"
	"testing"
)

/* Catálogo de pastas no SQLite. O que estes testes travam:

   - criar é IDEMPOTENTE e ignora caixa — clicar duas vezes em "Criar" não pode
     estourar erro nem duplicar a pasta na tela;
   - apagar pasta inexistente também não é erro (a UI apaga otimista);
   - a ordem sai alfabética ignorando caixa, senão "Rock" e "anos 90" alternariam
     de lugar conforme quem foi criada primeiro. */

func TestFolderCatalog_CreateListDelete(t *testing.T) {
	ctx := context.Background()
	cat := openTmp(t).Folders()

	if got, err := cat.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("catálogo novo deve vir vazio: %v err=%v", got, err)
	}
	for _, n := range []string{"Rock", "anos 90", "  sertanejo  "} {
		if err := cat.Create(ctx, n); err != nil {
			t.Fatalf("Create(%q): %v", n, err)
		}
	}
	got, err := cat.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"anos 90", "Rock", "sertanejo"} // ordenado ignorando caixa, e com trim
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("posição %d: got %q want %q (todas: %v)", i, got[i], want[i], got)
		}
	}

	// idempotência: mesma pasta em outra caixa não entra de novo
	if err := cat.Create(ctx, "ROCK"); err != nil {
		t.Fatalf("recriar não pode dar erro: %v", err)
	}
	if got, _ := cat.List(ctx); len(got) != 3 {
		t.Fatalf("recriar duplicou: %v", got)
	}

	if err := cat.Delete(ctx, "rock"); err != nil { // caixa diferente da criada
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := cat.List(ctx); len(got) != 2 {
		t.Fatalf("delete devia remover ignorando caixa: %v", got)
	}
	if err := cat.Delete(ctx, "nao-existe"); err != nil {
		t.Fatalf("apagar pasta inexistente não é erro: %v", err)
	}
}

func TestFolderCatalog_RejectsInvalidName(t *testing.T) {
	ctx := context.Background()
	cat := openTmp(t).Folders()
	if err := cat.Create(ctx, "   "); err == nil {
		t.Fatal("nome vazio deve ser recusado antes do banco")
	}
	longo := make([]byte, 200)
	for i := range longo {
		longo[i] = 'x'
	}
	if err := cat.Create(ctx, string(longo)); err == nil {
		t.Fatal("nome acima do teto deve ser recusado")
	}
}

func TestFolderCatalog_ClosedDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	r, closeFn, err := Open(t.TempDir() + "/f.db")
	if err != nil {
		t.Fatal(err)
	}
	cat := r.Folders()
	_ = closeFn() // banco fechado: as três operações têm de devolver erro, não pânico
	if err := cat.Create(ctx, "rock"); err == nil {
		t.Fatal("Create com banco fechado deve falhar")
	}
	if _, err := cat.List(ctx); err == nil {
		t.Fatal("List com banco fechado deve falhar")
	}
	if err := cat.Delete(ctx, "rock"); err == nil {
		t.Fatal("Delete com banco fechado deve falhar")
	}
}
