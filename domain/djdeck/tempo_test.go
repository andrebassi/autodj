package djdeck

import (
	"math"
	"testing"
)

const eps = 1e-9

func TestTempoRatio(t *testing.T) {
	r, err := TempoRatio(128, 130)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r-130.0/128.0) > eps {
		t.Fatalf("ratio = %v, want %v", r, 130.0/128.0)
	}
}

func TestTempoRatioNoBPM(t *testing.T) {
	for _, c := range [][2]float64{{0, 130}, {128, 0}, {-1, 130}, {128, -5}} {
		if _, err := TempoRatio(c[0], c[1]); err != ErrNoBPM {
			t.Fatalf("TempoRatio(%v,%v): want ErrNoBPM, got %v", c[0], c[1], err)
		}
	}
}

func TestPitchToRate(t *testing.T) {
	cases := []struct {
		pct, want float64
	}{
		{0, 1.0},
		{8, 1.08},
		{-8, 0.92},
		{20, 1.08},  // clamp high
		{-20, 0.92}, // clamp low
	}
	for _, c := range cases {
		if got := PitchToRate(c.pct); math.Abs(got-c.want) > eps {
			t.Fatalf("PitchToRate(%v) = %v, want %v", c.pct, got, c.want)
		}
	}
}

func TestRateToBPM(t *testing.T) {
	if got := RateToBPM(128, 1.08); math.Abs(got-138.24) > eps {
		t.Fatalf("RateToBPM = %v, want 138.24", got)
	}
}

func TestSync(t *testing.T) {
	ratio, pct, err := Sync(128, 130)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ratio-130.0/128.0) > eps {
		t.Fatalf("ratio = %v", ratio)
	}
	if math.Abs(pct-1.5625) > eps {
		t.Fatalf("pitchPct = %v, want 1.5625", pct)
	}
}

func TestSyncClamp(t *testing.T) {
	// 100 → 130 is +30%, past the ±8 fader → clamped
	_, pct, err := Sync(100, 130)
	if err != nil {
		t.Fatal(err)
	}
	if pct != PitchRange {
		t.Fatalf("pitchPct = %v, want clamp %v", pct, PitchRange)
	}
	// 130 → 100 is negative, past -8 → clamped low
	_, pct2, _ := Sync(130, 100)
	if pct2 != -PitchRange {
		t.Fatalf("pitchPct = %v, want clamp %v", pct2, -PitchRange)
	}
}

func TestSyncNoBPM(t *testing.T) {
	if _, _, err := Sync(0, 130); err != ErrNoBPM {
		t.Fatalf("want ErrNoBPM, got %v", err)
	}
}
