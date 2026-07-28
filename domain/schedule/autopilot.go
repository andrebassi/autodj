package schedule

import (
	"sort"
	"time"
)

// AutopilotDecision is the pure output of the autopilot policy for one tick:
// which due events to inject in front of the queue (priority-first) and whether
// to refill the queue from the current grid hour. It carries no I/O — the app
// layer executes it (resolving media, enqueuing, replenish fallback).
type AutopilotDecision struct {
	// Inject is the due events to push to the front of the queue, priority desc.
	Inject []ScheduledEvent
	// Refill signals the queue should be topped up from the current grid hour.
	Refill bool
}

// Decide is the autopilot policy: given the instant now, the current queue
// length, the low-water mark, the events due now and whether the grid programs
// the current hour, it decides what to inject and whether to refill. Pure and
// deterministic — the app injects now/queueLen/dueEvents/hasGrade so the
// supervision loop stays testable without a wall clock or real I/O.
//
//   - Inject = the due events ordered by priority desc (ties keep input order).
//     Always a non-nil slice (empty when nothing is due).
//   - Refill = the grid programs the current hour AND the queue has dropped
//     below lowWater. Without grid programming the autopilot never refills from
//     the grid; the app still guards against silence via the replenish pool.
//
// now is part of the policy signature (the app always ticks at a concrete
// instant) even though the current rules key off the resolved inputs.
func Decide(now time.Time, queueLen, lowWater int, dueEvents []ScheduledEvent, hasGrade bool) AutopilotDecision {
	_ = now // reserved: current rules derive from the resolved inputs
	inject := make([]ScheduledEvent, len(dueEvents))
	copy(inject, dueEvents)
	sort.SliceStable(inject, func(i, j int) bool {
		return inject[i].Priority > inject[j].Priority
	})
	return AutopilotDecision{
		Inject: inject,
		Refill: hasGrade && queueLen < lowWater,
	}
}
