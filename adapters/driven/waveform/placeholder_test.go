package waveform

import (
	"context"
	"testing"

	"autodj/domain/library"
	"autodj/domain/shared"
)

func pcmTrack(id string) *library.Track {
	return &library.Track{ID: id, Source: shared.SourceLocal, Title: id, PCMAvailable: true}
}

func TestPeaksDeterministic(t *testing.T) {
	ctx := context.Background()
	g := New()
	a, _ := g.Peaks(ctx, pcmTrack("song-42"), 64)
	b, _ := g.Peaks(ctx, pcmTrack("song-42"), 64)
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("want 64 peaks, got %d/%d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("peaks not deterministic at %d: %v != %v", i, a[i], b[i])
		}
	}
	// Different id → different shape (guards against a constant output).
	c, _ := g.Peaks(ctx, pcmTrack("other"), 64)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different ids must yield different waves")
	}
}

func TestPeaksRange(t *testing.T) {
	got, _ := New().Peaks(context.Background(), pcmTrack("x"), 200)
	for i, v := range got {
		if v < 0 || v > 1 {
			t.Fatalf("peak %d=%v out of [0,1]", i, v)
		}
	}
}

func TestPeaksDefaultBars(t *testing.T) {
	for _, bars := range []int{0, -5} {
		got, _ := New().Peaks(context.Background(), pcmTrack("x"), bars)
		if len(got) != defaultBars {
			t.Fatalf("bars=%d should fall back to %d, got %d", bars, defaultBars, len(got))
		}
	}
}
