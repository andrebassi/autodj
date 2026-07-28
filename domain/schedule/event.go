package schedule

import (
	"errors"
	"time"

	"autodj/domain/library"
)

// Event validation sentinels.
var (
	// ErrEventEmptyID is returned when a ScheduledEvent has no id.
	ErrEventEmptyID = errors.New("schedule: event id is empty")
	// ErrBadEventKind is returned for an event whose kind is not a valid EventKind.
	ErrBadEventKind = errors.New("schedule: invalid event kind")
	// ErrEventHour is returned when an event hour is out of [0,23].
	ErrEventHour = errors.New("schedule: event hour out of range")
	// ErrEventMinute is returned when an event minute is out of [0,59].
	ErrEventMinute = errors.New("schedule: event minute out of range")
	// ErrEventNoDays is returned when an event has no weekdays set.
	ErrEventNoDays = errors.New("schedule: event has no days")
	// ErrEventBadDay is returned for a weekday outside [Sunday,Saturday] or a
	// duplicate weekday in the list.
	ErrEventBadDay = errors.New("schedule: event day invalid or duplicated")
	// ErrEventPriority is returned when an event priority is out of [0,100].
	ErrEventPriority = errors.New("schedule: event priority out of range")
)

// maxEventPriority caps an event's preemption priority.
const maxEventPriority = 100

// EventKind classifies a scheduled event. It is a fixed, closed set: the
// fixed-slot programming a station overlays on top of the music rotation.
type EventKind string

const (
	// EventVozDoBrasil is the mandatory Brazilian government broadcast block.
	EventVozDoBrasil EventKind = "voz_do_brasil"
	// EventHoraCerta is the on-the-hour time announcement.
	EventHoraCerta EventKind = "hora_certa"
	// EventTemperatura is the weather/temperature announcement.
	EventTemperatura EventKind = "temperatura"
	// EventComercial is a scheduled ad break played as media.
	EventComercial EventKind = "comercial"
	// EventProgramete is a short scheduled program insert.
	EventProgramete EventKind = "programete"
)

// validEventKind reports whether k is one of the fixed event kinds.
func validEventKind(k EventKind) bool {
	switch k {
	case EventVozDoBrasil, EventHoraCerta, EventTemperatura, EventComercial, EventProgramete:
		return true
	default:
		return false
	}
}

// ScheduledEvent is a fixed, recurring programming item that preempts the music
// rotation at a given weekday+hour+minute. It is cron-lite: structured recurring
// fields (Days+Hour+Minute), not a cron expression. Priority resolves collisions
// (higher wins). MediaRef points at the media to play (a category or track id);
// empty means the media is generated later (e.g. hora certa via TTS in Épico 6).
type ScheduledEvent struct {
	ID       string         `json:"id"`
	Kind     EventKind      `json:"kind"`
	Days     []time.Weekday `json:"days"`
	Hour     int            `json:"hour"`
	Minute   int            `json:"minute"`
	Priority int            `json:"priority"`
	MediaRef string         `json:"mediaRef"`
}

// Validate checks the event is well-formed: non-empty id, valid kind, hour in
// [0,23], minute in [0,59], at least one weekday (all in range, no duplicates)
// and priority in [0,100].
func (e ScheduledEvent) Validate() error {
	if e.ID == "" {
		return ErrEventEmptyID
	}
	if !validEventKind(e.Kind) {
		return ErrBadEventKind
	}
	if e.Hour < 0 || e.Hour > 23 {
		return ErrEventHour
	}
	if e.Minute < 0 || e.Minute > 59 {
		return ErrEventMinute
	}
	if len(e.Days) == 0 {
		return ErrEventNoDays
	}
	seen := make(map[time.Weekday]bool, len(e.Days))
	for _, d := range e.Days {
		if d < time.Sunday || d > time.Saturday || seen[d] {
			return ErrEventBadDay
		}
		seen[d] = true
	}
	if e.Priority < 0 || e.Priority > maxEventPriority {
		return ErrEventPriority
	}
	return nil
}

// DueAt reports whether the event is due at instant t: t's weekday is in Days
// and t's hour/minute match (a 1-minute window — seconds are ignored).
func (e ScheduledEvent) DueAt(t time.Time) bool {
	if t.Hour() != e.Hour || t.Minute() != e.Minute {
		return false
	}
	wd := t.Weekday()
	for _, d := range e.Days {
		if d == wd {
			return true
		}
	}
	return false
}

// DueEvents filters the events due at t, ordered by priority descending (highest
// first). Ties keep the input order (stable). Pure — the caller injects t.
func DueEvents(events []ScheduledEvent, t time.Time) []ScheduledEvent {
	var due []ScheduledEvent
	for _, e := range events {
		if e.DueAt(t) {
			due = append(due, e)
		}
	}
	// stable insertion sort by priority desc keeps ties in input order.
	for i := 1; i < len(due); i++ {
		for j := i; j > 0 && due[j].Priority > due[j-1].Priority; j-- {
			due[j], due[j-1] = due[j-1], due[j]
		}
	}
	return due
}

// DefaultEvents returns the seed set of scheduled events (fictional home-radio
// profile). The wiring seeds these into an empty repo on boot. Every entry
// validates. MediaRefs point at seed categories, not concrete tracks.
func DefaultEvents() []ScheduledEvent {
	weekdays := []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday,
	}
	everyday := []time.Weekday{
		time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday,
		time.Friday, time.Saturday,
	}
	return []ScheduledEvent{
		{
			ID:       "voz-do-brasil",
			Kind:     EventVozDoBrasil,
			Days:     weekdays,
			Hour:     19,
			Minute:   0,
			Priority: 100,
			MediaRef: string(library.CategoryCommercial),
		},
		{
			ID:       "hora-certa",
			Kind:     EventHoraCerta,
			Days:     everyday,
			Hour:     12,
			Minute:   0,
			Priority: 80,
			MediaRef: "", // generated (TTS) later; skipped until then
		},
		{
			ID:       "break-comercial-manha",
			Kind:     EventComercial,
			Days:     weekdays,
			Hour:     8,
			Minute:   30,
			Priority: 50,
			MediaRef: string(library.CategoryCommercial),
		},
	}
}
