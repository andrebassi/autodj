// Package cartwall models the operator's wall of instant-fire buttons (jingles,
// station IDs, sound effects). It is a pure aggregate — no external deps. A
// Board is a set of CartButtons spread across pages; each button points at a
// library track by id, carries a colour hue, an icon, an optional keyboard
// hotkey and a page. JSON tags are the wire contract for GET/PUT /cartwall and
// mirror the frontend shape.
//
// Two-engine invariant, 6th guard: a button may only reference a *mixable* track
// (local PCM). Assign takes the resolved capability and rejects a non-mixable
// reference — DRM/Spotify audio never fires from the cartwall.
package cartwall

import (
	"errors"
	"sort"
)

// Validation sentinels.
var (
	// ErrEmptyID is returned when a button has no id (the caller assigns it).
	ErrEmptyID = errors.New("cartwall: button id required")
	// ErrEmptyLabel is returned when a button has an empty label.
	ErrEmptyLabel = errors.New("cartwall: button label required")
	// ErrBadPage is returned when a button's page is below 1.
	ErrBadPage = errors.New("cartwall: page must be >= 1")
	// ErrHotkeyTaken is returned when a non-empty hotkey is already used by
	// another button on the same page.
	ErrHotkeyTaken = errors.New("cartwall: hotkey already used on this page")
	// ErrNotMixable is returned when a button references a non-mixable track —
	// the two-engine invariant's 6th guard (DRM/Spotify never fires).
	ErrNotMixable = errors.New("cartwall: track not mixable")
	// ErrNotFound is returned by Clear for an unknown button id.
	ErrNotFound = errors.New("cartwall: button not found")
)

// CartButton is one instant-fire pad. TrackID references a library track; Hue is
// a 0..360 colour hint for the UI; Icon is an emoji/glyph; Hotkey is a single
// key (empty = no hotkey); Page groups buttons into tabs (>= 1).
type CartButton struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	TrackID string `json:"trackId"`
	Hue     int    `json:"hue"`
	Icon    string `json:"icon"`
	Hotkey  string `json:"hotkey"`
	Page    int    `json:"page"`
}

// Board is the whole cartwall: a flat set of buttons partitioned by page. It is
// a value object; the service loads/saves it as one blob per user.
type Board struct {
	Buttons []CartButton `json:"buttons"`
}

// Assign upserts a button (matched by id), enforcing the invariants: id and
// label non-empty, page >= 1, the referenced track mixable (capability passed
// in), and — for a non-empty hotkey — uniqueness within the button's page
// (ignoring the button being replaced). A validation failure leaves the board
// untouched.
func (b *Board) Assign(btn CartButton, mixable bool) error {
	if btn.ID == "" {
		return ErrEmptyID
	}
	if btn.Label == "" {
		return ErrEmptyLabel
	}
	if btn.Page < 1 {
		return ErrBadPage
	}
	if !mixable {
		return ErrNotMixable
	}
	if btn.Hotkey != "" {
		for _, e := range b.Buttons {
			if e.ID != btn.ID && e.Page == btn.Page && e.Hotkey == btn.Hotkey {
				return ErrHotkeyTaken
			}
		}
	}
	for i := range b.Buttons {
		if b.Buttons[i].ID == btn.ID {
			b.Buttons[i] = btn
			return nil
		}
	}
	b.Buttons = append(b.Buttons, btn)
	return nil
}

// Clear removes a button by id. Returns ErrNotFound when the id is unknown.
func (b *Board) Clear(id string) error {
	for i := range b.Buttons {
		if b.Buttons[i].ID == id {
			b.Buttons = append(b.Buttons[:i], b.Buttons[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// ByPage returns the buttons on a page in a stable order: buttons with a hotkey
// first (sorted by hotkey), then the rest by label, with id as the final
// tie-break so the order is deterministic.
func (b Board) ByPage(page int) []CartButton {
	var out []CartButton
	for _, e := range b.Buttons {
		if e.Page == page {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.Hotkey != c.Hotkey {
			if a.Hotkey == "" {
				return false
			}
			if c.Hotkey == "" {
				return true
			}
			return a.Hotkey < c.Hotkey
		}
		if a.Label != c.Label {
			return a.Label < c.Label
		}
		return a.ID < c.ID
	})
	return out
}

// Pages returns the distinct pages that hold at least one button, ascending.
func (b Board) Pages() []int {
	seen := make(map[int]bool, len(b.Buttons))
	var out []int
	for _, e := range b.Buttons {
		if !seen[e.Page] {
			seen[e.Page] = true
			out = append(out, e.Page)
		}
	}
	sort.Ints(out)
	return out
}

// Default returns the factory cartwall: fictional jingles/effects (anti-copyright)
// on page 1, each mapped to a seeded demo track id (demo-NN) and a home-row
// hotkey. The wiring seeds it into an empty repo on first boot.
func Default() Board {
	seeds := []CartButton{
		{ID: "seed-1", Label: "Vinheta Manhã", TrackID: "demo-01", Hue: 15, Icon: "🌅", Hotkey: "a", Page: 1},
		{ID: "seed-2", Label: "ID Estação", TrackID: "demo-02", Hue: 210, Icon: "📻", Hotkey: "s", Page: 1},
		{ID: "seed-3", Label: "Efeito Riser", TrackID: "demo-03", Hue: 280, Icon: "🚀", Hotkey: "d", Page: 1},
		{ID: "seed-4", Label: "Aplausos", TrackID: "demo-04", Hue: 45, Icon: "👏", Hotkey: "f", Page: 1},
		{ID: "seed-5", Label: "Risada", TrackID: "demo-05", Hue: 130, Icon: "😂", Hotkey: "g", Page: 1},
		{ID: "seed-6", Label: "Sweep", TrackID: "demo-06", Hue: 190, Icon: "🌀", Hotkey: "h", Page: 1},
		{ID: "seed-7", Label: "Buzina", TrackID: "demo-07", Hue: 0, Icon: "📣", Hotkey: "j", Page: 1},
		{ID: "seed-8", Label: "Beep", TrackID: "demo-08", Hue: 320, Icon: "🔔", Hotkey: "k", Page: 1},
	}
	return Board{Buttons: seeds}
}
