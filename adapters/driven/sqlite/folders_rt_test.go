package sqlite

import (
	"context"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

// Pastas do operador precisam sobreviver ao round-trip; sem isto a organização
// some a cada Upsert, em silêncio.
func TestFolders_roundTrip(t *testing.T) {
	r, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	ctx := context.Background()

	tr := &library.Track{
		ID: "t1", Title: "X", Source: shared.SourceLocal, Path: "/a.mp3",
		Duration: time.Minute, Folders: []string{"rock", "anos 90"},
	}
	if err := r.Upsert(ctx, tr); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Folders) != 2 || got.Folders[0] != "anos 90" || got.Folders[1] != "rock" {
		t.Fatalf("Folders = %v", got.Folders)
	}
	// faixa sem pasta volta vazia, nunca nil-vs-[] inconsistente
	semPasta := &library.Track{ID: "t2", Title: "Y", Source: shared.SourceLocal, Path: "/b.mp3", Duration: time.Minute}
	if err := r.Upsert(ctx, semPasta); err != nil {
		t.Fatal(err)
	}
	g2, _ := r.Get(ctx, "t2")
	if len(g2.Folders) != 0 {
		t.Fatalf("sem pasta devia voltar vazio, veio %v", g2.Folders)
	}
}
