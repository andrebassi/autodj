package memory

import (
	"context"
	"testing"

	"autodj/domain/cartwall"
)

func TestCartwallRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewCartwallRepo()

	// starts empty.
	b, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Buttons) != 0 {
		t.Fatalf("fresh board buttons = %d, want 0", len(b.Buttons))
	}

	// save then read back.
	want := cartwall.Board{Buttons: []cartwall.CartButton{
		{ID: "a", Label: "Vinheta", TrackID: "demo-01", Hotkey: "q", Page: 1},
	}}
	if err := r.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(got.Buttons) != 1 || got.Buttons[0].ID != "a" {
		t.Fatalf("round-trip board = %+v", got)
	}
}
