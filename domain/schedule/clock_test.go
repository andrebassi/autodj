package schedule

import (
	"errors"
	"testing"

	"autodj/domain/library"
)

func TestDefaultClocksValidate(t *testing.T) {
	clocks := DefaultClocks()
	if len(clocks) == 0 {
		t.Fatal("DefaultClocks empty")
	}
	for _, c := range clocks {
		if err := c.Validate(); err != nil {
			t.Errorf("default clock %q invalid: %v", c.ID, err)
		}
	}
}

func TestItemCount(t *testing.T) {
	c := Clock{
		ID:   "x",
		Name: "X",
		Slots: []Slot{
			{Kind: SlotMusic, Category: library.CategoryMusic, Count: 3},
			{Kind: SlotJingle, Category: library.CategoryJingle, Count: 2},
		},
	}
	if got := c.ItemCount(); got != 5 {
		t.Fatalf("ItemCount = %d, want 5", got)
	}
	if got := (Clock{}).ItemCount(); got != 0 {
		t.Fatalf("empty ItemCount = %d, want 0", got)
	}
}

func TestClockValidate(t *testing.T) {
	ok := []Slot{{Kind: SlotMusic, Category: library.CategoryMusic, Count: 1}}
	tests := []struct {
		name  string
		clock Clock
		want  error
	}{
		{"happy", Clock{ID: "a", Name: "A", Slots: ok}, nil},
		{"empty id", Clock{ID: "", Name: "A", Slots: ok}, ErrClockEmptyID},
		{"no name", Clock{ID: "a", Name: "", Slots: ok}, ErrClockNoName},
		{"no slots", Clock{ID: "a", Name: "A", Slots: nil}, ErrClockNoSlots},
		{"bad kind", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: "bogus", Category: library.CategoryMusic, Count: 1}}}, ErrBadSlotKind},
		{"bad category", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotMusic, Category: "bogus", Count: 1}}}, ErrBadSlotCategory},
		{"count zero", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotMusic, Category: library.CategoryMusic, Count: 0}}}, ErrSlotCount},
		{"count too big", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotMusic, Category: library.CategoryMusic, Count: 61}}}, ErrSlotCount},
		{"mismatch jingle", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotJingle, Category: library.CategoryMusic, Count: 1}}}, ErrKindCategoryMismatch},
		{"mismatch commercial", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotCommercial, Category: library.CategoryMusic, Count: 1}}}, ErrKindCategoryMismatch},
		{"music soundtrack ok", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotMusic, Category: library.CategorySoundtrack, Count: 1}}}, nil},
		{"event any category ok", Clock{ID: "a", Name: "A", Slots: []Slot{
			{Kind: SlotEvent, Category: library.CategoryExtra, Count: 1}}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.clock.Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate = %v, want %v", err, tt.want)
			}
		})
	}
}
