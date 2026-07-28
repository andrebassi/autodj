package library

import (
	"testing"

	"autodj/domain/shared"
)

func TestDemoTracks(t *testing.T) {
	ts := DemoTracks()
	if len(ts) != len(demoSeeds)*demoCopies {
		t.Fatalf("want %d demo tracks, got %d", len(demoSeeds)*demoCopies, len(ts))
	}
	seen := map[string]bool{}
	for _, tr := range ts {
		if err := tr.Validate(); err != nil {
			t.Fatalf("demo track %s invalid: %v", tr.ID, err)
		}
		if tr.Source != shared.SourceLocal || !tr.Mixable || !tr.PCMAvailable {
			t.Fatalf("demo track %s should be mixable local PCM: %+v", tr.ID, tr)
		}
		if tr.Artist == "" || tr.Title == "" || tr.Path == "" {
			t.Fatalf("demo track %s missing fields: %+v", tr.ID, tr)
		}
		if seen[tr.ID] {
			t.Fatalf("duplicate demo id %s", tr.ID)
		}
		seen[tr.ID] = true
	}
	if ts[0].ID != "demo-01" {
		t.Fatalf("first demo id should be demo-01, got %s", ts[0].ID)
	}
}
