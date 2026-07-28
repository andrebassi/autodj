// Package playlog is the station's append-only record of what actually played.
// It is the single source of truth for ECAD royalty reporting, the irradiation
// proof, and the "most played" ranking. Entries are never mutated or deleted:
// history is immutable by design so the reports it feeds cannot be forged.
//
// Each Entry derives Broadcasted/ECADRelevant from the two-engine capability
// matrix (domain/shared): Spotify audio is DRM (never entered the local playout
// bus) so it is neither broadcast nor ECAD-relevant, and reports must skip it.
package playlog

import (
	"errors"
	"sort"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

// ErrBadEntry is returned when an Entry lacks the minimum identity needed to be
// a trustworthy history record: a track id and the instant it played.
var ErrBadEntry = errors.New("playlog: entry requires track id and played-at time")

// Entry is one immutable line of play history. Title/Artist are snapshotted at
// play time (the catalog may change later; the log must not). Broadcasted and
// ECADRelevant are derived from the source at construction — never set by hand —
// so the two-engine invariant holds in the record itself.
type Entry struct {
	ID           string
	TrackID      string
	Title        string
	Artist       string
	Source       shared.SourceKind
	PlayedAt     time.Time
	DurationMs   int64
	Broadcasted  bool
	ECADRelevant bool
}

// NewEntry snapshots a track that just played at playedAt into an immutable
// Entry. Broadcasted mirrors the source's Mixable capability (only PCM audio
// that could enter the local playout bus counts as broadcast) and ECADRelevant
// mirrors the source's ECAD capability — so Spotify yields false for both.
// Returns ErrBadEntry when the track has no id or playedAt is the zero time.
func NewEntry(t library.Track, playedAt time.Time) (Entry, error) {
	if t.ID == "" || playedAt.IsZero() {
		return Entry{}, ErrBadEntry
	}
	cap := shared.Capabilities(t.Source)
	return Entry{
		ID:           t.ID + "@" + playedAt.UTC().Format(time.RFC3339Nano),
		TrackID:      t.ID,
		Title:        t.Title,
		Artist:       t.Artist,
		Source:       t.Source,
		PlayedAt:     playedAt,
		DurationMs:   t.Duration.Milliseconds(),
		Broadcasted:  cap.Mixable,
		ECADRelevant: cap.ECAD,
	}, nil
}

// TopItem is one row of the "most played" ranking: a track and how many times
// it appears in the aggregated entries.
type TopItem struct {
	TrackID string
	Title   string
	Artist  string
	Count   int
}

// TopN aggregates entries by TrackID and returns the n most played, ordered by
// count descending with first-seen order as a stable tie-break. Title/Artist
// come from the first entry seen for each track. n <= 0 or no entries yields
// nil. A larger n than distinct tracks returns every track.
func TopN(entries []Entry, n int) []TopItem {
	if n <= 0 || len(entries) == 0 {
		return nil
	}
	idx := make(map[string]int, len(entries))
	items := make([]TopItem, 0, len(entries))
	for _, e := range entries {
		if i, ok := idx[e.TrackID]; ok {
			items[i].Count++
			continue
		}
		idx[e.TrackID] = len(items)
		items = append(items, TopItem{
			TrackID: e.TrackID,
			Title:   e.Title,
			Artist:  e.Artist,
			Count:   1,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})
	if n > len(items) {
		n = len(items)
	}
	return items[:n]
}
