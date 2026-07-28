package playlog

import (
	"errors"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

func trk(id, title, artist string, src shared.SourceKind, dur time.Duration) library.Track {
	return library.Track{ID: id, Title: title, Artist: artist, Source: src, Duration: dur}
}

func TestNewEntryLocalIsBroadcastAndECAD(t *testing.T) {
	at := time.Date(2026, 7, 24, 10, 30, 0, 0, time.UTC)
	e, err := NewEntry(trk("t1", "Aurora Fria", "Nébula Sul", shared.SourceLocal, 3*time.Minute), at)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !e.Broadcasted {
		t.Error("local must be broadcasted")
	}
	if !e.ECADRelevant {
		t.Error("local must be ECAD relevant")
	}
	if e.DurationMs != 180000 {
		t.Errorf("DurationMs = %d, want 180000", e.DurationMs)
	}
	if e.ID != "t1@2026-07-24T10:30:00Z" {
		t.Errorf("ID = %q", e.ID)
	}
	if e.Title != "Aurora Fria" || e.Artist != "Nébula Sul" || e.TrackID != "t1" {
		t.Errorf("snapshot mismatch: %+v", e)
	}
	if e.Source != shared.SourceLocal {
		t.Errorf("Source = %q", e.Source)
	}
	if !e.PlayedAt.Equal(at) {
		t.Errorf("PlayedAt = %v", e.PlayedAt)
	}
}

func TestNewEntryDerivesFromMatrix(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		src              shared.SourceKind
		wantBroad, wantE bool
	}{
		{shared.SourceLocal, true, true},
		{shared.SourceSpotify, false, false},
		{shared.SourceLineIn, true, false},
		{shared.SourceTTS, true, false},
	}
	for _, c := range cases {
		e, err := NewEntry(trk("x", "T", "A", c.src, time.Minute), at)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if e.Broadcasted != c.wantBroad {
			t.Errorf("%s Broadcasted = %v, want %v", c.src, e.Broadcasted, c.wantBroad)
		}
		if e.ECADRelevant != c.wantE {
			t.Errorf("%s ECADRelevant = %v, want %v", c.src, e.ECADRelevant, c.wantE)
		}
	}
}

func TestNewEntryRejectsBad(t *testing.T) {
	at := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	if _, err := NewEntry(trk("", "T", "A", shared.SourceLocal, time.Minute), at); !errors.Is(err, ErrBadEntry) {
		t.Errorf("empty id: err = %v, want ErrBadEntry", err)
	}
	if _, err := NewEntry(trk("t1", "T", "A", shared.SourceLocal, time.Minute), time.Time{}); !errors.Is(err, ErrBadEntry) {
		t.Errorf("zero time: err = %v, want ErrBadEntry", err)
	}
}

func TestTopNAggregatesAndOrders(t *testing.T) {
	at := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	mk := func(id string) Entry {
		e, _ := NewEntry(trk(id, "Title-"+id, "Artist-"+id, shared.SourceLocal, time.Minute), at.Add(time.Duration(len(id))*time.Second))
		return e
	}
	// b played 3x, a 2x, c 1x; a appears before b in first-seen order.
	entries := []Entry{mk("a"), mk("b"), mk("a"), mk("b"), mk("c"), mk("b")}
	// give each play a distinct ID so nothing collapses accidentally
	for i := range entries {
		entries[i].ID = entries[i].TrackID + "#" + time.Duration(i).String()
	}
	top := TopN(entries, 2)
	if len(top) != 2 {
		t.Fatalf("len = %d, want 2", len(top))
	}
	if top[0].TrackID != "b" || top[0].Count != 3 {
		t.Errorf("top[0] = %+v, want b/3", top[0])
	}
	if top[1].TrackID != "a" || top[1].Count != 2 {
		t.Errorf("top[1] = %+v, want a/2", top[1])
	}
	if top[0].Title != "Title-b" || top[0].Artist != "Artist-b" {
		t.Errorf("meta from first entry mismatch: %+v", top[0])
	}
}

func TestTopNTieBreakFirstSeen(t *testing.T) {
	at := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	e := func(id string) Entry {
		v, _ := NewEntry(trk(id, id, id, shared.SourceLocal, time.Minute), at)
		return v
	}
	// x and y both once; x seen first must rank first.
	entries := []Entry{e("x"), e("y")}
	entries[1].ID = "y-distinct"
	top := TopN(entries, 5)
	if len(top) != 2 {
		t.Fatalf("len = %d, want 2 (n>distinct returns all)", len(top))
	}
	if top[0].TrackID != "x" || top[1].TrackID != "y" {
		t.Errorf("tie order = %s,%s want x,y", top[0].TrackID, top[1].TrackID)
	}
}

func TestTopNEdgeCases(t *testing.T) {
	at := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	e, _ := NewEntry(trk("a", "A", "A", shared.SourceLocal, time.Minute), at)
	if TopN(nil, 3) != nil {
		t.Error("nil entries must yield nil")
	}
	if TopN([]Entry{e}, 0) != nil {
		t.Error("n=0 must yield nil")
	}
	if TopN([]Entry{e}, -1) != nil {
		t.Error("n<0 must yield nil")
	}
}
