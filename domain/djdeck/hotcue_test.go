package djdeck

import (
	"testing"

	"autodj/domain/library"
)

func TestHotCueBankSetGet(t *testing.T) {
	var b HotCueBank
	if err := b.Set(1, 5000, 200000); err != nil {
		t.Fatal(err)
	}
	c, ok := b.Get(1)
	if !ok || c.Slot != 1 || c.PosMs != 5000 {
		t.Fatalf("get = %+v ok=%v", c, ok)
	}
}

func TestHotCueBankBadSlot(t *testing.T) {
	var b HotCueBank
	for _, s := range []int{0, 9, -1} {
		if err := b.Set(s, 0, 1000); err != ErrBadSlot {
			t.Fatalf("Set slot %d: want ErrBadSlot, got %v", s, err)
		}
		if err := b.Clear(s); err != ErrBadSlot {
			t.Fatalf("Clear slot %d: want ErrBadSlot, got %v", s, err)
		}
	}
	if _, ok := b.Get(0); ok {
		t.Fatal("Get(0) should be ok=false")
	}
	if _, ok := b.Get(9); ok {
		t.Fatal("Get(9) should be ok=false")
	}
}

func TestHotCueClamp(t *testing.T) {
	var b HotCueBank
	// past the end → clamped to durMs
	_ = b.Set(2, 999999, 10000)
	if c, _ := b.Get(2); c.PosMs != 10000 {
		t.Fatalf("clamp high: PosMs = %d, want 10000", c.PosMs)
	}
	// negative → clamped to 0
	_ = b.Set(3, -50, 10000)
	if c, _ := b.Get(3); c.PosMs != 0 {
		t.Fatalf("clamp low: PosMs = %d, want 0", c.PosMs)
	}
	// negative durMs → collapses to 0
	_ = b.Set(4, 500, -1)
	if c, _ := b.Get(4); c.PosMs != 0 {
		t.Fatalf("neg dur: PosMs = %d, want 0", c.PosMs)
	}
}

func TestHotCueClearAndList(t *testing.T) {
	var b HotCueBank
	_ = b.Set(3, 300, 10000)
	_ = b.Set(1, 100, 10000)
	_ = b.Set(2, 200, 10000)
	if err := b.Clear(2); err != nil {
		t.Fatal(err)
	}
	if err := b.Clear(7); err != nil { // clearing empty slot = no-op
		t.Fatalf("clear empty: %v", err)
	}
	list := b.List()
	if len(list) != 2 || list[0].Slot != 1 || list[1].Slot != 3 {
		t.Fatalf("list = %+v", list)
	}
}

func TestHotCueGetUnset(t *testing.T) {
	var b HotCueBank
	_ = b.Set(1, 100, 1000)
	if _, ok := b.Get(5); ok {
		t.Fatal("unset slot should be ok=false")
	}
}

func TestFromMarkers(t *testing.T) {
	markers := []library.Marker{
		{Kind: library.MarkerHook, PosMs: 30000},
		{Kind: library.MarkerStart, PosMs: 0},
		{Kind: library.MarkerIntro, PosMs: 8000},
		{Kind: library.MarkerMixOut, PosMs: 190000}, // ignored
		{Kind: library.MarkerStamp, PosMs: 15000},   // ignored
	}
	b := FromMarkers(markers, 200000)
	list := b.List()
	if len(list) != 3 {
		t.Fatalf("want 3 hot cues from jump markers, got %d: %+v", len(list), list)
	}
	// ordered by position → slots 1,2,3 = start(0), intro(8000), hook(30000)
	want := []int{0, 8000, 30000}
	for i, c := range list {
		if c.Slot != i+1 || c.PosMs != want[i] {
			t.Fatalf("cue %d = %+v, want slot %d pos %d", i, c, i+1, want[i])
		}
	}
}

func TestFromMarkersOverflowAndClamp(t *testing.T) {
	// 9 jump markers → only first 8 slots; positions clamped to durMs
	var markers []library.Marker
	for i := 0; i < 9; i++ {
		markers = append(markers, library.Marker{Kind: library.MarkerHook, PosMs: int64(i * 1000)})
	}
	b := FromMarkers(markers, 3000) // clamp everything past 3000
	list := b.List()
	if len(list) != SlotCount {
		t.Fatalf("want %d cues (overflow dropped), got %d", SlotCount, len(list))
	}
	for _, c := range list {
		if c.PosMs > 3000 {
			t.Fatalf("cue past dur not clamped: %+v", c)
		}
	}
}
