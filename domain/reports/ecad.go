// Package reports derives read-only royalty/execution reports from the
// append-only play log (domain/playlog). Reports own no state: a report is a
// pure projection over immutable Entries, so regenerating always yields the same
// result — which is exactly what makes an ECAD execution report trustworthy.
//
// The ECAD report honours the two-engine invariant at the report level: it
// aggregates only entries whose ECADRelevant flag is true (already derived from
// the capability matrix at play time), so DRM Spotify plays never appear and
// never count. The report never inspects the source string — only the boolean.
package reports

import (
	"sort"
	"time"

	"autodj/domain/playlog"
)

// ECADLine is one aggregated work in the report: a track with how many times it
// aired in the window and the total aired duration.
type ECADLine struct {
	TrackID         string
	Title           string
	Artist          string
	Plays           int
	TotalDurationMs int64
}

// ECADReport is a period execution report for royalty reporting (ECAD/Tec-Rádio).
// It covers [From, To); Lines are ordered by Plays descending with a stable
// Title tie-break; totals are the sum over Lines. GeneratedAt is supplied by the
// caller (an injected clock) so the report is deterministic in tests.
type ECADReport struct {
	From            time.Time
	To              time.Time
	GeneratedAt     time.Time
	Lines           []ECADLine
	TotalPlays      int
	TotalDurationMs int64
}

// BuildECAD projects play-log entries into an ECAD execution report over the
// window [from, to). It keeps only entries that are ECAD-relevant and fall
// inside the window, aggregates them by TrackID (summing plays and duration),
// and orders the result by plays descending with a stable Title tie-break.
// Entries outside the window or with ECADRelevant=false (e.g. DRM Spotify) are
// ignored and contribute nothing to the totals. Entries are never mutated.
func BuildECAD(entries []playlog.Entry, from, to, generatedAt time.Time) ECADReport {
	rep := ECADReport{From: from, To: to, GeneratedAt: generatedAt}
	idx := make(map[string]int, len(entries))
	for _, e := range entries {
		if !e.ECADRelevant {
			continue
		}
		if e.PlayedAt.Before(from) || !e.PlayedAt.Before(to) {
			continue
		}
		if i, ok := idx[e.TrackID]; ok {
			rep.Lines[i].Plays++
			rep.Lines[i].TotalDurationMs += e.DurationMs
			continue
		}
		idx[e.TrackID] = len(rep.Lines)
		rep.Lines = append(rep.Lines, ECADLine{
			TrackID:         e.TrackID,
			Title:           e.Title,
			Artist:          e.Artist,
			Plays:           1,
			TotalDurationMs: e.DurationMs,
		})
	}
	sort.SliceStable(rep.Lines, func(i, j int) bool {
		if rep.Lines[i].Plays != rep.Lines[j].Plays {
			return rep.Lines[i].Plays > rep.Lines[j].Plays
		}
		return rep.Lines[i].Title < rep.Lines[j].Title
	})
	for _, l := range rep.Lines {
		rep.TotalPlays += l.Plays
		rep.TotalDurationMs += l.TotalDurationMs
	}
	return rep
}
