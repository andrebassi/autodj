// Package schedule holds the station's programming model: clocks (hour
// templates), the weekly grid, rotation-based playlist generation, scheduled
// events and the autopilot policy. It is pure — only stdlib plus
// autodj/domain/library for the media Category enum. The domain never touches
// concrete tracks here: a Clock is a *template* of an hour (ordered slots that
// draw from categories); resolving it to real tracks with rotation rules lives
// in the generation step, and mapping clocks onto weekdays/hours in the grid.
package schedule

import (
	"errors"

	"autodj/domain/library"
)

// Clock validation sentinels.
var (
	// ErrClockEmptyID is returned when a Clock has no id.
	ErrClockEmptyID = errors.New("schedule: clock id is empty")
	// ErrClockNoName is returned when a Clock has no name.
	ErrClockNoName = errors.New("schedule: clock name is empty")
	// ErrClockNoSlots is returned when a Clock has no slots.
	ErrClockNoSlots = errors.New("schedule: clock has no slots")
	// ErrBadSlotKind is returned for a slot whose kind is not a valid SlotKind.
	ErrBadSlotKind = errors.New("schedule: invalid slot kind")
	// ErrBadSlotCategory is returned for a slot whose category is not valid.
	ErrBadSlotCategory = errors.New("schedule: invalid slot category")
	// ErrSlotCount is returned when a slot's item count is out of [1,60].
	ErrSlotCount = errors.New("schedule: slot count out of range")
	// ErrKindCategoryMismatch is returned when a slot's kind and category are
	// incoherent (e.g. a jingle slot pointing at the music category).
	ErrKindCategoryMismatch = errors.New("schedule: slot kind/category mismatch")
)

// maxSlotCount caps how many items a single slot may yield.
const maxSlotCount = 60

// SlotKind classifies what a clock slot draws from. It is a fixed, closed set.
type SlotKind string

const (
	// SlotMusic draws a rotation music track (from the music/soundtrack pool).
	SlotMusic SlotKind = "music"
	// SlotJingle draws a station ID / vinheta.
	SlotJingle SlotKind = "jingle"
	// SlotCommercial draws an ad played as media.
	SlotCommercial SlotKind = "commercial"
	// SlotEvent reserves a position for a scheduled event (resolved later); its
	// category is free because the concrete event is bound in the event layer.
	SlotEvent SlotKind = "event"
)

// validSlotKind reports whether k is one of the fixed slot kinds.
func validSlotKind(k SlotKind) bool {
	switch k {
	case SlotMusic, SlotJingle, SlotCommercial, SlotEvent:
		return true
	default:
		return false
	}
}

// Slot is one entry of a clock: Count items drawn from Category, classified by
// Kind. Kind and Category must stay coherent (see Clock.Validate).
type Slot struct {
	Kind     SlotKind         `json:"kind"`
	Category library.Category `json:"category"`
	Count    int              `json:"count"`
}

// kindCategoryOK checks that a slot's kind and category are coherent.
// music → music/soundtrack; jingle → jingle; commercial → commercial;
// event → any valid category (the event binds the real media later).
func kindCategoryOK(k SlotKind, c library.Category) bool {
	switch k {
	case SlotMusic:
		return c == library.CategoryMusic || c == library.CategorySoundtrack
	case SlotJingle:
		return c == library.CategoryJingle
	case SlotCommercial:
		return c == library.CategoryCommercial
	}
	// SlotEvent (the only remaining kind, since kind is pre-validated): the
	// concrete event binds its media later, so any valid category is allowed.
	return true
}

// Clock is an hour template: an ordered list of slots. It is reusable across the
// weekly grid; it does not reference concrete tracks.
type Clock struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Slots []Slot `json:"slots"`
}

// ItemCount returns the total number of items this clock yields (sum of slot
// counts). Feeds the per-hour grade-load histogram.
func (c Clock) ItemCount() int {
	n := 0
	for _, s := range c.Slots {
		n += s.Count
	}
	return n
}

// Validate checks the clock is well-formed: non-empty id/name, at least one
// slot, and each slot has a valid kind, a valid category, a count in [1,60] and
// a coherent kind/category pairing.
func (c Clock) Validate() error {
	if c.ID == "" {
		return ErrClockEmptyID
	}
	if c.Name == "" {
		return ErrClockNoName
	}
	if len(c.Slots) == 0 {
		return ErrClockNoSlots
	}
	for _, s := range c.Slots {
		if !validSlotKind(s.Kind) {
			return ErrBadSlotKind
		}
		if !library.ValidCategory(s.Category) {
			return ErrBadSlotCategory
		}
		if s.Count < 1 || s.Count > maxSlotCount {
			return ErrSlotCount
		}
		if !kindCategoryOK(s.Kind, s.Category) {
			return ErrKindCategoryMismatch
		}
	}
	return nil
}

// DefaultClocks returns the seed set of hour templates (fictional, home-radio
// profiles). The wiring seeds these into an empty repo on boot. Every entry
// validates.
func DefaultClocks() []Clock {
	return []Clock{
		{
			ID:   "manha",
			Name: "Manhã",
			Slots: []Slot{
				{Kind: SlotJingle, Category: library.CategoryJingle, Count: 1},
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 3},
				{Kind: SlotCommercial, Category: library.CategoryCommercial, Count: 1},
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 3},
				{Kind: SlotEvent, Category: library.CategoryMusic, Count: 1},
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 4},
			},
		},
		{
			ID:   "tarde",
			Name: "Tarde",
			Slots: []Slot{
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 4},
				{Kind: SlotJingle, Category: library.CategoryJingle, Count: 1},
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 4},
				{Kind: SlotCommercial, Category: library.CategoryCommercial, Count: 2},
				{Kind: SlotMusic, Category: library.CategoryMusic, Count: 4},
			},
		},
		{
			ID:   "madrugada",
			Name: "Madrugada",
			Slots: []Slot{
				{Kind: SlotMusic, Category: library.CategorySoundtrack, Count: 6},
				{Kind: SlotJingle, Category: library.CategoryJingle, Count: 1},
				{Kind: SlotMusic, Category: library.CategorySoundtrack, Count: 6},
			},
		},
	}
}
