package djdeck

import (
	"errors"
	"sort"

	"autodj/domain/library"
)

// ErrBadSlot is returned when a hot-cue slot is outside the 1..8 range.
var ErrBadSlot = errors.New("djdeck: hot-cue slot out of range (1..8)")

// SlotCount is the number of hot-cue slots per deck (DJ-controller convention).
const SlotCount = 8

// HotCue is a saved jump point on a deck: a slot (1..8) at a millisecond
// position. The deck seeks to PosMs when the operator fires the slot.
type HotCue struct {
	Slot  int
	PosMs int
}

// HotCueBank holds up to SlotCount hot cues for one deck, keyed by slot. The
// zero value is an empty, usable bank. It is pure state — the Web Audio seek
// lives in the browser hook; the bank only validates slots and clamps positions.
type HotCueBank struct {
	cues map[int]int // slot -> posMs
}

// Set stores a hot cue at slot with posMs clamped to [0, durMs]. Returns
// ErrBadSlot for a slot outside 1..8. A negative durMs is treated as 0 (clamp
// collapses to 0), so callers never store a position past the track.
func (b *HotCueBank) Set(slot, posMs, durMs int) error {
	if slot < 1 || slot > SlotCount {
		return ErrBadSlot
	}
	if durMs < 0 {
		durMs = 0
	}
	if posMs < 0 {
		posMs = 0
	}
	if posMs > durMs {
		posMs = durMs
	}
	if b.cues == nil {
		b.cues = make(map[int]int, SlotCount)
	}
	b.cues[slot] = posMs
	return nil
}

// Clear removes the hot cue at slot. Returns ErrBadSlot for an invalid slot;
// clearing an empty slot is a no-op (no error).
func (b *HotCueBank) Clear(slot int) error {
	if slot < 1 || slot > SlotCount {
		return ErrBadSlot
	}
	delete(b.cues, slot)
	return nil
}

// Get returns the hot cue at slot and whether it is set. Returns ok=false for
// an unset or out-of-range slot.
func (b *HotCueBank) Get(slot int) (HotCue, bool) {
	if slot < 1 || slot > SlotCount {
		return HotCue{}, false
	}
	pos, ok := b.cues[slot]
	if !ok {
		return HotCue{}, false
	}
	return HotCue{Slot: slot, PosMs: pos}, true
}

// List returns the set hot cues ordered by slot.
func (b *HotCueBank) List() []HotCue {
	out := make([]HotCue, 0, len(b.cues))
	for slot, pos := range b.cues {
		out = append(out, HotCue{Slot: slot, PosMs: pos})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// FromMarkers seeds a bank from a track's cue markers (Épico 3). It picks the
// jump-worthy kinds (start, intro, hook/refrão), orders them by position, and
// assigns them to the first slots (1..SlotCount), clamping each position to
// [0, durMs]. Markers beyond the slot count are dropped. Other kinds
// (mixin/mixout/stamp/end) are ignored — they are not deck jump points.
func FromMarkers(markers []library.Marker, durMs int) HotCueBank {
	var pts []int
	for _, m := range markers {
		switch m.Kind {
		case library.MarkerStart, library.MarkerIntro, library.MarkerHook:
			pts = append(pts, int(m.PosMs))
		}
	}
	sort.Ints(pts)
	var b HotCueBank
	for i, pos := range pts {
		if i >= SlotCount {
			break
		}
		_ = b.Set(i+1, pos, durMs)
	}
	return b
}
