package schedule

import (
	"errors"
	"testing"

	"autodj/domain/library"
)

func TestDefaultRotationValidates(t *testing.T) {
	if err := DefaultRotation().Validate(); err != nil {
		t.Fatalf("DefaultRotation invalid: %v", err)
	}
}

func TestRotationRulesValidate(t *testing.T) {
	cases := []struct {
		name string
		r    RotationRules
		want error
	}{
		{"ok", RotationRules{MinArtistGap: 1, MinTitleGap: 2, MaxBPMJump: 5}, nil},
		{"zeros ok", RotationRules{}, nil},
		{"bad artist", RotationRules{MinArtistGap: -1}, ErrBadArtistGap},
		{"bad title", RotationRules{MinTitleGap: -1}, ErrBadTitleGap},
		{"bad bpm", RotationRules{MaxBPMJump: -0.5}, ErrBadBPMJump},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.r.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

// musicClock builds a clock of a single music slot of n items.
func musicClock(n int) Clock {
	return Clock{
		ID:    "c",
		Name:  "C",
		Slots: []Slot{{Kind: SlotMusic, Category: library.CategoryMusic, Count: n}},
	}
}

func cand(id, artist, title string, bpm float64) Candidate {
	return Candidate{TrackID: id, Artist: artist, Title: title, BPM: bpm, Category: library.CategoryMusic}
}

func TestGenerateArtistSeparation(t *testing.T) {
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("t1", "A", "s1", 120),
			cand("t2", "B", "s2", 122),
			cand("t3", "C", "s3", 121),
		},
	}
	rules := RotationRules{MinArtistGap: 2, MinTitleGap: 0, MaxBPMJump: 0, Seed: 7}
	got, err := Generate(musicClock(6), pool, nil, rules)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("len = %d, want 6", len(got))
	}
	// with 3 distinct artists and gap 2, no artist may appear in any 2
	// consecutive positions.
	byID := map[string]string{"t1": "A", "t2": "B", "t3": "C"}
	for i := 1; i < len(got); i++ {
		if byID[got[i]] == byID[got[i-1]] {
			t.Fatalf("artist repeat within gap at %d: %v", i, got)
		}
	}
}

func TestGenerateDeterministicBySeed(t *testing.T) {
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("t1", "A", "s1", 120),
			cand("t2", "B", "s2", 122),
			cand("t3", "C", "s3", 121),
			cand("t4", "D", "s4", 119),
		},
	}
	rules := RotationRules{MinArtistGap: 1, MinTitleGap: 1, MaxBPMJump: 0, Seed: 42}
	a, _ := Generate(musicClock(8), pool, nil, rules)
	b, _ := Generate(musicClock(8), pool, nil, rules)
	if len(a) != 8 {
		t.Fatalf("len = %d, want 8", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d: %v vs %v", i, a, b)
		}
	}
	// a different seed should be free to diverge (not asserted equal); just
	// ensure it still produces a full playlist.
	c, _ := Generate(musicClock(8), pool, nil, RotationRules{MinArtistGap: 1, Seed: 99})
	if len(c) != 8 {
		t.Fatalf("seed 99 len = %d, want 8", len(c))
	}
}

func TestGenerateSingleArtistRelaxesAndFills(t *testing.T) {
	// one artist, one title: every gap rule is impossible to satisfy, so the
	// algorithm must relax all the way down and still fill every slot.
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {cand("only", "A", "s", 120)},
	}
	rules := RotationRules{MinArtistGap: 5, MinTitleGap: 5, MaxBPMJump: 4, Seed: 1}
	got, err := Generate(musicClock(4), pool, nil, rules)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := []string{"only", "only", "only", "only"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestGenerateBPMPreference(t *testing.T) {
	// after a 120 BPM anchor, with jump 5 and gap 0, only the 123 neighbour is
	// within range on the first pick; the 150 outlier must be avoided while a
	// within-range candidate exists.
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("near", "A", "s1", 123),
			cand("far", "B", "s2", 150),
		},
	}
	recent := []Candidate{cand("anchor", "Z", "s0", 120)}
	rules := RotationRules{MinArtistGap: 0, MinTitleGap: 0, MaxBPMJump: 5, Seed: 3}
	got, err := Generate(musicClock(1), pool, recent, rules)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got[0] != "near" {
		t.Fatalf("first pick = %q, want near (within BPM jump)", got[0])
	}
}

func TestGenerateBPMPreferenceBelowAnchor(t *testing.T) {
	// anchor 120, jump 5: the 117 candidate is within range (abs of a negative
	// delta), the 150 outlier is not — exercises the below-anchor BPM branch.
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("below", "A", "s1", 117),
			cand("far", "B", "s2", 150),
		},
	}
	recent := []Candidate{cand("anchor", "Z", "s0", 120)}
	got, err := Generate(musicClock(1), pool, recent, RotationRules{MaxBPMJump: 5, Seed: 4})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got[0] != "below" {
		t.Fatalf("first pick = %q, want below (within BPM jump)", got[0])
	}
}

func TestGenerateBPMRelaxesWhenNoNeighbour(t *testing.T) {
	// anchor 120, jump 2, both candidates far away → BPM preference cannot be
	// met, algorithm relaxes and still picks one (non-empty).
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("f1", "A", "s1", 150),
			cand("f2", "B", "s2", 160),
		},
	}
	recent := []Candidate{cand("anchor", "Z", "s0", 120)}
	got, err := Generate(musicClock(1), pool, recent, RotationRules{MaxBPMJump: 2, Seed: 1})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got[0] != "f1" && got[0] != "f2" {
		t.Fatalf("unexpected pick %q", got[0])
	}
}

func TestGenerateSkipsEmptyCategoryBestEffort(t *testing.T) {
	// a clock with a jingle slot (no jingle candidates) plus a music slot: the
	// jingle slot is skipped, music still generated.
	clock := Clock{
		ID:   "c", Name: "C",
		Slots: []Slot{
			{Kind: SlotJingle, Category: library.CategoryJingle, Count: 2},
			{Kind: SlotMusic, Category: library.CategoryMusic, Count: 3},
		},
	}
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {cand("m1", "A", "s1", 120), cand("m2", "B", "s2", 121)},
	}
	got, err := Generate(clock, pool, nil, RotationRules{MinArtistGap: 1, Seed: 5})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (jingle slot skipped)", len(got))
	}
}

func TestGenerateEmptyClockErrors(t *testing.T) {
	// clock draws only from a category with no candidates → whole clock empty.
	clock := Clock{
		ID: "c", Name: "C",
		Slots: []Slot{{Kind: SlotJingle, Category: library.CategoryJingle, Count: 1}},
	}
	_, err := Generate(clock, map[library.Category][]Candidate{}, nil, DefaultRotation())
	if !errors.Is(err, ErrEmptyCategory) {
		t.Fatalf("err = %v, want ErrEmptyCategory", err)
	}
}

func TestGenerateTitleGapWithSharedArtist(t *testing.T) {
	// two titles by the same artist, artist gap disabled but title gap 1: the
	// two must alternate, never the same title twice in a row.
	pool := map[library.Category][]Candidate{
		library.CategoryMusic: {
			cand("x", "A", "one", 100),
			cand("y", "A", "two", 100),
		},
	}
	got, err := Generate(musicClock(4), pool, nil, RotationRules{MinTitleGap: 1, Seed: 2})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatalf("same title repeated at %d: %v", i, got)
		}
	}
}
