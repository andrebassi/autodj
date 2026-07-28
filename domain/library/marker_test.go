package library

import (
	"errors"
	"testing"
	"time"

	"autodj/domain/shared"
)

func pcmTrack(t *testing.T) *Track {
	t.Helper()
	tr, err := NewTrack("m1", shared.SourceLocal, "Song", 200*time.Second)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.Path = "/tmp/song.flac"
	if err := tr.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return tr
}

func TestMarkerKindValid(t *testing.T) {
	for _, k := range []MarkerKind{MarkerStart, MarkerIntro, MarkerHook, MarkerStamp, MarkerMixIn, MarkerMixOut, MarkerEnd} {
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	if MarkerKind("bogus").Valid() {
		t.Error("bogus should be invalid")
	}
}

func TestSetMarkersSortsByPosition(t *testing.T) {
	tr := pcmTrack(t)
	err := tr.SetMarkers([]Marker{
		{Kind: MarkerEnd, PosMs: 190000},
		{Kind: MarkerIntro, PosMs: 8000, Label: "vocal"},
		{Kind: MarkerStart, PosMs: 0},
	})
	if err != nil {
		t.Fatalf("SetMarkers: %v", err)
	}
	if len(tr.Markers) != 3 {
		t.Fatalf("want 3 markers, got %d", len(tr.Markers))
	}
	if tr.Markers[0].Kind != MarkerStart || tr.Markers[2].Kind != MarkerEnd {
		t.Fatalf("markers not sorted: %+v", tr.Markers)
	}
}

func TestSetMarkersRepeatableKinds(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{
		{Kind: MarkerStamp, PosMs: 1000},
		{Kind: MarkerStamp, PosMs: 2000},
		{Kind: MarkerHook, PosMs: 30000},
		{Kind: MarkerHook, PosMs: 90000},
	}); err != nil {
		t.Fatalf("repeatable kinds should be allowed: %v", err)
	}
}

func TestSetMarkersEmptyClears(t *testing.T) {
	tr := pcmTrack(t)
	_ = tr.SetMarkers([]Marker{{Kind: MarkerStart, PosMs: 0}})
	if err := tr.SetMarkers(nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(tr.Markers) != 0 {
		t.Fatalf("markers should be cleared, got %+v", tr.Markers)
	}
}

func TestSetMarkersRejectsNonPCM(t *testing.T) {
	tr, _ := NewTrack("s1", shared.SourceSpotify, "DRM", 100*time.Second)
	tr.SpotifyURI = "spotify:track:x"
	if err := tr.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := tr.SetMarkers([]Marker{{Kind: MarkerStart, PosMs: 0}}); !errors.Is(err, ErrMarkersNotPCM) {
		t.Fatalf("want ErrMarkersNotPCM, got %v", err)
	}
}

func TestSetMarkersRejectsBadKind(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{{Kind: "chorus", PosMs: 0}}); !errors.Is(err, ErrBadMarkerKind) {
		t.Fatalf("want ErrBadMarkerKind, got %v", err)
	}
}

func TestSetMarkersRejectsOutOfRange(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{{Kind: MarkerStart, PosMs: -1}}); !errors.Is(err, ErrMarkerOutOfRange) {
		t.Fatalf("negative: want ErrMarkerOutOfRange, got %v", err)
	}
	if err := tr.SetMarkers([]Marker{{Kind: MarkerEnd, PosMs: 200001}}); !errors.Is(err, ErrMarkerOutOfRange) {
		t.Fatalf("past duration: want ErrMarkerOutOfRange, got %v", err)
	}
}

func TestSetMarkersRejectsDuplicateSingleton(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{
		{Kind: MarkerIntro, PosMs: 5000},
		{Kind: MarkerIntro, PosMs: 6000},
	}); !errors.Is(err, ErrDuplicateMarker) {
		t.Fatalf("want ErrDuplicateMarker, got %v", err)
	}
}

func TestSetMarkersRejectsBadOrder(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{
		{Kind: MarkerStart, PosMs: 100000},
		{Kind: MarkerEnd, PosMs: 50000},
	}); !errors.Is(err, ErrMarkerOrder) {
		t.Fatalf("start>end: want ErrMarkerOrder, got %v", err)
	}
	if err := tr.SetMarkers([]Marker{
		{Kind: MarkerMixIn, PosMs: 120000},
		{Kind: MarkerMixOut, PosMs: 100000},
	}); !errors.Is(err, ErrMarkerOrder) {
		t.Fatalf("mixin>mixout: want ErrMarkerOrder, got %v", err)
	}
}

func TestSetMarkersAllowsValidOrder(t *testing.T) {
	tr := pcmTrack(t)
	if err := tr.SetMarkers([]Marker{
		{Kind: MarkerStart, PosMs: 0},
		{Kind: MarkerMixIn, PosMs: 4000},
		{Kind: MarkerMixOut, PosMs: 180000},
		{Kind: MarkerEnd, PosMs: 195000},
	}); err != nil {
		t.Fatalf("valid order rejected: %v", err)
	}
}
