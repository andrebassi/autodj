package schedule

import (
	"errors"
	"math/rand"

	"autodj/domain/library"
)

// Rotation validation sentinels.
var (
	// ErrBadArtistGap is returned when MinArtistGap is negative.
	ErrBadArtistGap = errors.New("schedule: artist gap must be >= 0")
	// ErrBadTitleGap is returned when MinTitleGap is negative.
	ErrBadTitleGap = errors.New("schedule: title gap must be >= 0")
	// ErrBadBPMJump is returned when MaxBPMJump is negative.
	ErrBadBPMJump = errors.New("schedule: bpm jump must be >= 0")
	// ErrEmptyCategory is returned by Generate when the whole clock produces no
	// item — i.e. none of its slots' categories has a single candidate.
	ErrEmptyCategory = errors.New("schedule: clock produced no items (empty catalog)")
)

// RotationRules parameterizes playlist generation. Gaps are measured in items
// (how many other items must sit between two plays of the same artist/title).
// MaxBPMJump is a soft preference for music slots (0 disables it). Seed makes
// generation deterministic and therefore testable.
type RotationRules struct {
	MinArtistGap int     `json:"minArtistGap"`
	MinTitleGap  int     `json:"minTitleGap"`
	MaxBPMJump   float64 `json:"maxBpmJump"`
	Seed         int64   `json:"seed"`
}

// DefaultRotation returns sensible home-radio rotation defaults: don't replay an
// artist within 5 items, a title within 10, and keep consecutive music within a
// 12 BPM jump.
func DefaultRotation() RotationRules {
	return RotationRules{MinArtistGap: 5, MinTitleGap: 10, MaxBPMJump: 12, Seed: 1}
}

// Validate checks the rules are well-formed: non-negative gaps and jump.
func (r RotationRules) Validate() error {
	if r.MinArtistGap < 0 {
		return ErrBadArtistGap
	}
	if r.MinTitleGap < 0 {
		return ErrBadTitleGap
	}
	if r.MaxBPMJump < 0 {
		return ErrBadBPMJump
	}
	return nil
}

// Candidate is the minimal projection of a catalog track that generation needs.
// Projecting to Candidate keeps domain/schedule decoupled from the full
// library.Track aggregate (it only depends on the Category enum). The app maps
// eligible Tracks (Mixable && PCMAvailable — Spotify never enters generation)
// into Candidates before calling Generate.
type Candidate struct {
	TrackID  string
	Artist   string
	Title    string
	BPM      float64
	Category library.Category
}

// Generate materializes a clock into an ordered sequence of TrackIDs honoring
// the rotation rules. It is pure and deterministic for a given Seed (uses a
// seeded rand, never time.Now or the global source).
//
// For each slot, for each of its Count items, it draws a candidate from the
// slot's category that respects MinArtistGap/MinTitleGap against the window
// (recent history plus items already chosen in this generation) and, for music
// slots, prefers a candidate within MaxBPMJump of the previous track. When no
// candidate satisfies every constraint it relaxes progressively — first drops
// the BPM preference, then the title gap, then the artist gap — and, in the last
// resort, repeats: a slot whose category has >= 1 candidate never comes out
// empty. A category with no candidate is skipped (best-effort). If the whole
// clock yields nothing, it returns ErrEmptyCategory.
func Generate(clock Clock, pool map[library.Category][]Candidate, recent []Candidate, rules RotationRules) ([]string, error) {
	rng := rand.New(rand.NewSource(rules.Seed)) //nolint:gosec // deterministic scheduling, not crypto
	history := make([]Candidate, len(recent))
	copy(history, recent)

	var out []string
	for _, slot := range clock.Slots {
		cands := pool[slot.Category]
		if len(cands) == 0 {
			continue // best-effort: skip a category with no media
		}
		for i := 0; i < slot.Count; i++ {
			pick := selectCandidate(cands, history, rules, slot.Kind, rng)
			out = append(out, pick.TrackID)
			history = append(history, pick)
		}
	}
	if len(out) == 0 {
		return nil, ErrEmptyCategory
	}
	return out, nil
}

// selectCandidate picks one candidate from cands, relaxing constraints level by
// level until a non-empty match set exists. Level 3 accepts anything, so the
// function always returns a candidate (cands is guaranteed non-empty by caller).
func selectCandidate(cands, history []Candidate, rules RotationRules, kind SlotKind, rng *rand.Rand) Candidate {
	prevBPM := lastBPM(history)

	okArtist := func(c Candidate) bool {
		if rules.MinArtistGap <= 0 || c.Artist == "" {
			return true
		}
		return !withinGap(history, rules.MinArtistGap, func(h Candidate) bool {
			return h.Artist != "" && h.Artist == c.Artist
		})
	}
	okTitle := func(c Candidate) bool {
		if rules.MinTitleGap <= 0 || c.Title == "" {
			return true
		}
		return !withinGap(history, rules.MinTitleGap, func(h Candidate) bool {
			return h.Title != "" && h.Title == c.Title
		})
	}
	okBPM := func(c Candidate) bool {
		if kind != SlotMusic || rules.MaxBPMJump <= 0 || prevBPM <= 0 || c.BPM <= 0 {
			return true
		}
		return abs(c.BPM-prevBPM) <= rules.MaxBPMJump
	}

	levels := []func(Candidate) bool{
		func(c Candidate) bool { return okArtist(c) && okTitle(c) && okBPM(c) }, // full
		func(c Candidate) bool { return okArtist(c) && okTitle(c) },             // drop BPM
		func(c Candidate) bool { return okArtist(c) },                           // drop title
	}
	for _, pass := range levels {
		if match := filterCandidates(cands, pass); len(match) > 0 {
			return match[rng.Intn(len(match))]
		}
	}
	// last resort: artist gap unsatisfiable too — repeat, drawing any candidate.
	return cands[rng.Intn(len(cands))]
}

// withinGap reports whether any of the last gap items of history matches — i.e.
// choosing such a candidate would violate the gap rule.
func withinGap(history []Candidate, gap int, match func(Candidate) bool) bool {
	start := len(history) - gap
	if start < 0 {
		start = 0
	}
	for _, h := range history[start:] {
		if match(h) {
			return true
		}
	}
	return false
}

// lastBPM returns the BPM of the most recent history item with a known BPM (>0),
// or 0 when there is none — used as the anchor for the BPM-jump preference.
func lastBPM(history []Candidate) float64 {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].BPM > 0 {
			return history[i].BPM
		}
	}
	return 0
}

// filterCandidates returns the subset of cands satisfying keep, preserving order
// so selection stays deterministic given the seeded rng.
func filterCandidates(cands []Candidate, keep func(Candidate) bool) []Candidate {
	var out []Candidate
	for _, c := range cands {
		if keep(c) {
			out = append(out, c)
		}
	}
	return out
}

// abs returns the absolute value of x.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
