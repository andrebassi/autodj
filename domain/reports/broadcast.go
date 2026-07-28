package reports

import (
	"sort"
	"time"

	"autodj/domain/playlog"
	"autodj/domain/shared"
)

// ProofLine is one aired play in the broadcast proof — one line per execution,
// never aggregated. Source is exposed for transparency (local/tts/line_in); a
// DRM Spotify play never appears here because its Broadcasted flag is false.
type ProofLine struct {
	EntryID    string
	PlayedAt   time.Time
	TrackID    string
	Title      string
	Artist     string
	Source     shared.SourceKind
	DurationMs int64
}

// BroadcastProof is the chronological proof of what actually aired over a window:
// every Broadcasted play, in play order. Unlike the ECAD report it does NOT
// aggregate — each play is its own line, because the point is auditable evidence
// of the broadcast minute by minute, not a per-work rollup.
type BroadcastProof struct {
	From            time.Time
	To              time.Time
	GeneratedAt     time.Time
	Lines           []ProofLine
	TotalEntries    int
	TotalDurationMs int64
}

// BuildBroadcastProof projects the play log into a chronological broadcast proof
// over [from, to). It keeps only entries whose Broadcasted flag is true (the
// two-engine invariant at the report level: DRM Spotify, Broadcasted=false, is
// excluded), sorts them by PlayedAt ascending with a stable EntryID tie-break,
// and emits one line per entry — no aggregation. Totals are the line count and
// the summed durations. generatedAt is supplied by the caller (the domain never
// reads the wall clock); entries are not mutated.
func BuildBroadcastProof(entries []playlog.Entry, from, to, generatedAt time.Time) BroadcastProof {
	proof := BroadcastProof{From: from, To: to, GeneratedAt: generatedAt}
	for _, e := range entries {
		if !e.Broadcasted {
			continue
		}
		if e.PlayedAt.Before(from) || !e.PlayedAt.Before(to) {
			continue
		}
		proof.Lines = append(proof.Lines, ProofLine{
			EntryID:    e.ID,
			PlayedAt:   e.PlayedAt,
			TrackID:    e.TrackID,
			Title:      e.Title,
			Artist:     e.Artist,
			Source:     e.Source,
			DurationMs: e.DurationMs,
		})
	}
	sort.SliceStable(proof.Lines, func(i, j int) bool {
		if !proof.Lines[i].PlayedAt.Equal(proof.Lines[j].PlayedAt) {
			return proof.Lines[i].PlayedAt.Before(proof.Lines[j].PlayedAt)
		}
		return proof.Lines[i].EntryID < proof.Lines[j].EntryID
	})
	proof.TotalEntries = len(proof.Lines)
	for _, l := range proof.Lines {
		proof.TotalDurationMs += l.DurationMs
	}
	return proof
}
