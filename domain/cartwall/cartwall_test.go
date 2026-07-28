package cartwall

import (
	"reflect"
	"testing"
)

func btn(id, hotkey string, page int) CartButton {
	return CartButton{ID: id, Label: "L-" + id, TrackID: "t-" + id, Hotkey: hotkey, Page: page}
}

func TestAssignOK(t *testing.T) {
	var b Board
	if err := b.Assign(btn("a", "q", 1), true); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if len(b.Buttons) != 1 {
		t.Fatalf("want 1 button, got %d", len(b.Buttons))
	}
}

func TestAssignUpsert(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "q", 1), true)
	up := btn("a", "q", 1)
	up.Label = "Renamed"
	if err := b.Assign(up, true); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(b.Buttons) != 1 || b.Buttons[0].Label != "Renamed" {
		t.Fatalf("upsert did not replace: %+v", b.Buttons)
	}
}

func TestAssignEmptyID(t *testing.T) {
	var b Board
	if err := b.Assign(btn("", "q", 1), true); err != ErrEmptyID {
		t.Fatalf("want ErrEmptyID, got %v", err)
	}
}

func TestAssignEmptyLabel(t *testing.T) {
	var b Board
	bad := CartButton{ID: "a", Label: "", TrackID: "t", Page: 1}
	if err := b.Assign(bad, true); err != ErrEmptyLabel {
		t.Fatalf("want ErrEmptyLabel, got %v", err)
	}
}

func TestAssignBadPage(t *testing.T) {
	var b Board
	if err := b.Assign(btn("a", "q", 0), true); err != ErrBadPage {
		t.Fatalf("want ErrBadPage, got %v", err)
	}
}

func TestAssignNotMixable(t *testing.T) {
	var b Board
	if err := b.Assign(btn("a", "q", 1), false); err != ErrNotMixable {
		t.Fatalf("want ErrNotMixable, got %v", err)
	}
}

func TestAssignHotkeyTakenSamePage(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "q", 1), true)
	if err := b.Assign(btn("b", "q", 1), true); err != ErrHotkeyTaken {
		t.Fatalf("want ErrHotkeyTaken, got %v", err)
	}
}

func TestAssignHotkeyDifferentPageOK(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "q", 1), true)
	if err := b.Assign(btn("b", "q", 2), true); err != nil {
		t.Fatalf("hotkey reuse across pages should be ok: %v", err)
	}
}

func TestAssignEmptyHotkeyNotUnique(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "", 1), true)
	if err := b.Assign(btn("b", "", 1), true); err != nil {
		t.Fatalf("empty hotkeys may repeat: %v", err)
	}
}

func TestAssignUpsertKeepsOwnHotkey(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "q", 1), true)
	// re-assigning the same id with the same hotkey must not self-collide
	if err := b.Assign(btn("a", "q", 1), true); err != nil {
		t.Fatalf("self upsert collided: %v", err)
	}
}

func TestClear(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "q", 1), true)
	if err := b.Clear("a"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(b.Buttons) != 0 {
		t.Fatalf("want empty, got %d", len(b.Buttons))
	}
}

func TestClearNotFound(t *testing.T) {
	var b Board
	if err := b.Clear("nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestByPageOrdering(t *testing.T) {
	var b Board
	_ = b.Assign(CartButton{ID: "z", Label: "Zeta", TrackID: "t", Page: 1}, true)     // no hotkey → last
	_ = b.Assign(CartButton{ID: "m", Label: "M", TrackID: "t", Hotkey: "s", Page: 1}, true)
	_ = b.Assign(CartButton{ID: "n", Label: "N", TrackID: "t", Hotkey: "a", Page: 1}, true)
	_ = b.Assign(CartButton{ID: "x", Label: "X", TrackID: "t", Page: 2}, true) // other page
	got := b.ByPage(1)
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	want := []string{"n", "m", "z"} // hotkey a, hotkey s, then no-hotkey
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ByPage order = %v, want %v", ids, want)
	}
	if len(b.ByPage(2)) != 1 {
		t.Fatalf("page 2 should have 1 button")
	}
}

func TestByPageHotkeyFirstThenEmpty(t *testing.T) {
	// insert the hotkeyed button first so the sort compares the no-hotkey
	// element as the left operand (exercises the "a has no hotkey" branch).
	var b Board
	_ = b.Assign(CartButton{ID: "h", Label: "H", TrackID: "t", Hotkey: "a", Page: 1}, true)
	_ = b.Assign(CartButton{ID: "e", Label: "E", TrackID: "t", Page: 1}, true) // no hotkey
	got := b.ByPage(1)
	if got[0].ID != "h" || got[1].ID != "e" {
		t.Fatalf("hotkeyed button must sort before no-hotkey: %v", got)
	}
}

func TestByPageLabelTieBreak(t *testing.T) {
	var b Board
	_ = b.Assign(CartButton{ID: "b2", Label: "Beta", TrackID: "t", Page: 1}, true)
	_ = b.Assign(CartButton{ID: "a1", Label: "Alfa", TrackID: "t", Page: 1}, true)
	got := b.ByPage(1) // both no hotkey → label order Alfa, Beta
	if got[0].ID != "a1" || got[1].ID != "b2" {
		t.Fatalf("label tie-break failed: %v", got)
	}
}

func TestByPageIDTieBreak(t *testing.T) {
	var b Board
	_ = b.Assign(CartButton{ID: "id2", Label: "Same", TrackID: "t", Page: 1}, true)
	_ = b.Assign(CartButton{ID: "id1", Label: "Same", TrackID: "t", Page: 1}, true)
	got := b.ByPage(1) // same label → id order
	if got[0].ID != "id1" || got[1].ID != "id2" {
		t.Fatalf("id tie-break failed: %v", got)
	}
}

func TestPages(t *testing.T) {
	var b Board
	_ = b.Assign(btn("a", "", 3), true)
	_ = b.Assign(btn("b", "", 1), true)
	_ = b.Assign(btn("c", "", 3), true)
	got := b.Pages()
	if !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("Pages = %v, want [1 3]", got)
	}
}

func TestDefaultPopulated(t *testing.T) {
	b := Default()
	if len(b.Buttons) == 0 {
		t.Fatal("Default must seed >= 1 button")
	}
	if got := b.Pages(); len(got) == 0 || got[0] != 1 {
		t.Fatalf("Default should populate page 1, pages=%v", got)
	}
	// every seed must be a valid assignable button (re-assign into a fresh board)
	var fresh Board
	for _, s := range b.Buttons {
		if err := fresh.Assign(s, true); err != nil {
			t.Fatalf("seed %q invalid: %v", s.ID, err)
		}
	}
}
