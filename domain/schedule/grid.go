package schedule

import (
	"errors"
	"time"
)

// Grid validation sentinels.
var (
	// ErrBadHour is returned for a cell whose hour is outside [0,23].
	ErrBadHour = errors.New("schedule: cell hour out of range")
	// ErrBadWeekday is returned for a cell whose weekday is outside [0,6].
	ErrBadWeekday = errors.New("schedule: cell weekday out of range")
	// ErrDuplicateCell is returned when two cells share the same (weekday,hour).
	ErrDuplicateCell = errors.New("schedule: duplicate grid cell")
	// ErrUnknownClock is returned when a cell references an unknown clock id.
	ErrUnknownClock = errors.New("schedule: cell references unknown clock")
)

// hoursPerDay is the number of hour slots in a grid day.
const hoursPerDay = 24

// Cell assigns a clock (by id) to one (weekday, hour) position of the week.
// Only assigned positions are stored; an absent position means "no programming".
type Cell struct {
	Weekday time.Weekday `json:"weekday"` // 0=Sunday .. 6=Saturday
	Hour    int          `json:"hour"`    // 0..23
	ClockID string       `json:"clockId"`
}

// Grid is the station's weekly programming grid: a sparse set of cells mapping
// (weekday, hour) → clock id. It is a template layer above clocks; it never
// references concrete tracks.
type Grid struct {
	Cells []Cell `json:"cells"`
}

// Validate checks the grid is well-formed against the set of known clock ids:
// every cell has an in-range weekday/hour, no two cells share a position, and
// every referenced clock id exists in known.
func (g Grid) Validate(known map[string]bool) error {
	seen := make(map[[2]int]bool, len(g.Cells))
	for _, c := range g.Cells {
		if c.Weekday < time.Sunday || c.Weekday > time.Saturday {
			return ErrBadWeekday
		}
		if c.Hour < 0 || c.Hour >= hoursPerDay {
			return ErrBadHour
		}
		key := [2]int{int(c.Weekday), c.Hour}
		if seen[key] {
			return ErrDuplicateCell
		}
		seen[key] = true
		if !known[c.ClockID] {
			return ErrUnknownClock
		}
	}
	return nil
}

// ClockAt returns the clock id assigned to (wd, hour) and whether a cell exists.
func (g Grid) ClockAt(wd time.Weekday, hour int) (string, bool) {
	for _, c := range g.Cells {
		if c.Weekday == wd && c.Hour == hour {
			return c.ClockID, true
		}
	}
	return "", false
}

// HourlyLoad returns, for each hour of day wd, the item count of the clock
// assigned to that (wd, hour) — 0 when the cell is empty or its clock is unknown.
// Pure: the caller resolves the current weekday and the clock set. Feeds the
// Player's 24h grade-load histogram.
func HourlyLoad(g Grid, clocks map[string]Clock, wd time.Weekday) [hoursPerDay]int {
	var out [hoursPerDay]int
	for h := 0; h < hoursPerDay; h++ {
		id, ok := g.ClockAt(wd, h)
		if !ok {
			continue
		}
		if c, known := clocks[id]; known {
			out[h] = c.ItemCount()
		}
	}
	return out
}

// DefaultGrid returns the seed weekly grid (fictional home-radio profile): every
// day maps morning hours to "manha", afternoon/evening to "tarde", and the rest
// to "madrugada" — referencing the ids from DefaultClocks. The wiring seeds it
// into an empty repo on boot. It validates against DefaultClocks.
func DefaultGrid() Grid {
	var cells []Cell
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		for h := 0; h < hoursPerDay; h++ {
			var id string
			switch {
			case h >= 6 && h < 12:
				id = "manha"
			case h >= 12 && h < 20:
				id = "tarde"
			default:
				id = "madrugada"
			}
			cells = append(cells, Cell{Weekday: wd, Hour: h, ClockID: id})
		}
	}
	return Grid{Cells: cells}
}
