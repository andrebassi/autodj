package memory

import (
	"context"
	"testing"

	"autodj/domain/mixing"
)

func TestMixRepoDefaultThenSave(t *testing.T) {
	ctx := context.Background()
	r := NewMixRepo()

	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != mixing.Default() {
		t.Fatalf("initial = %+v, want default", got)
	}

	want := mixing.Default()
	want.MusicaOut = 4000
	want.OverlapVol = 15
	if err := r.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ = r.Get(ctx)
	if got != want {
		t.Fatalf("after Save = %+v, want %+v", got, want)
	}
}
