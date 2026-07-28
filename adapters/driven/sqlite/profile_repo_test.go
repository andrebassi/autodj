package sqlite

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/operatorprofile"
	"autodj/ports/outbound"
)

func newProfileRepo(t *testing.T) *ProfileRepo {
	t.Helper()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })
	return tr.Profiles()
}

func TestProfileRepoSQLite(t *testing.T) {
	ctx := context.Background()
	r := newProfileRepo(t)

	// absent → ErrProfileNotFound
	if _, err := r.Get(ctx, "usr_x"); !errors.Is(err, outbound.ErrProfileNotFound) {
		t.Fatalf("Get missing = %v, want ErrProfileNotFound", err)
	}

	// upsert a validated default with a tweak
	p := operatorprofile.Default("usr_1")
	p.Theme = operatorprofile.ThemeLight
	p.Density = operatorprofile.DensityCompact
	p.Toggle(operatorprofile.PanelLibrary)
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := r.Get(ctx, "usr_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Theme != operatorprofile.ThemeLight || got.Density != operatorprofile.DensityCompact {
		t.Fatalf("round-trip theme/density = %v/%v", got.Theme, got.Density)
	}
	if len(got.Panels) != len(operatorprofile.Panels) {
		t.Fatalf("panels = %d, want %d", len(got.Panels), len(operatorprofile.Panels))
	}
	// library toggled off round-trips
	for _, pref := range got.Panels {
		if pref.Key == operatorprofile.PanelLibrary && pref.Visible {
			t.Fatalf("library should be hidden after round-trip")
		}
	}

	// upsert again (update path) with a different theme
	p.Theme = operatorprofile.ThemeDark
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	got, _ = r.Get(ctx, "usr_1")
	if got.Theme != operatorprofile.ThemeDark {
		t.Fatalf("update theme = %v, want escuro", got.Theme)
	}

	// Story 9.2: new row defaults touch=off/screens=1; then touch/screens/screen round-trip.
	if got.Touch || got.Screens != 1 {
		t.Fatalf("row defaults touch=%v screens=%d, want false/1", got.Touch, got.Screens)
	}
	p2 := operatorprofile.Default("usr_2")
	p2.SetTouch(true)
	p2.SetScreens(2)
	p2.AssignScreen(operatorprofile.PanelCartwall, 1)
	if err := p2.Validate(); err != nil {
		t.Fatalf("validate p2: %v", err)
	}
	if err := r.Upsert(ctx, p2); err != nil {
		t.Fatalf("Upsert p2: %v", err)
	}
	got2, err := r.Get(ctx, "usr_2")
	if err != nil {
		t.Fatalf("Get p2: %v", err)
	}
	if !got2.Touch || got2.Screens != 2 {
		t.Fatalf("round-trip touch/screens = %v/%d, want true/2", got2.Touch, got2.Screens)
	}
	var cartScreen = -1
	for _, pref := range got2.Panels {
		if pref.Key == operatorprofile.PanelCartwall {
			cartScreen = pref.Screen
		}
	}
	if cartScreen != 1 {
		t.Fatalf("cartwall screen round-trip = %d, want 1", cartScreen)
	}
}
