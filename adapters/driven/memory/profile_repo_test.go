package memory

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/operatorprofile"
	"autodj/ports/outbound"
)

func TestProfileRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewProfileRepo()

	if _, err := r.Get(ctx, "usr_x"); !errors.Is(err, outbound.ErrProfileNotFound) {
		t.Fatalf("Get missing = %v, want ErrProfileNotFound", err)
	}

	p := operatorprofile.Default("usr_1")
	p.Theme = operatorprofile.ThemeLight
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := r.Get(ctx, "usr_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Theme != operatorprofile.ThemeLight {
		t.Fatalf("theme = %v, want claro", got.Theme)
	}

	// update path
	p.Theme = operatorprofile.ThemeDark
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	got, _ = r.Get(ctx, "usr_1")
	if got.Theme != operatorprofile.ThemeDark {
		t.Fatalf("update theme = %v", got.Theme)
	}
}
