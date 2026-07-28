package djdeck

import (
	"math"
	"testing"
)

func TestCrossfadeAnchors(t *testing.T) {
	cases := []struct {
		pos            float64
		wantA, wantB   float64
	}{
		{0, 1, 0},
		{1, 0, 1},
		{0.5, math.Sqrt2 / 2, math.Sqrt2 / 2},
	}
	for _, c := range cases {
		a, b := Crossfade(c.pos)
		if math.Abs(a-c.wantA) > 1e-9 || math.Abs(b-c.wantB) > 1e-9 {
			t.Errorf("Crossfade(%v)=(%v,%v) want (%v,%v)", c.pos, a, b, c.wantA, c.wantB)
		}
	}
}

func TestCrossfadeConstantPower(t *testing.T) {
	for pos := 0.0; pos <= 1.0; pos += 0.05 {
		a, b := Crossfade(pos)
		if p := a*a + b*b; math.Abs(p-1) > 1e-9 {
			t.Errorf("power at pos=%v = %v, want 1", pos, p)
		}
	}
}

func TestCrossfadeClamp(t *testing.T) {
	a, b := Crossfade(-2)
	if a != 1 || b != 0 {
		t.Errorf("Crossfade(-2)=(%v,%v) want (1,0)", a, b)
	}
	a, b = Crossfade(3)
	if math.Abs(a) > 1e-9 || math.Abs(b-1) > 1e-9 {
		t.Errorf("Crossfade(3)=(%v,%v) want (0,1)", a, b)
	}
}

func TestDeckZeroValueEmpty(t *testing.T) {
	var d Deck
	if d.State() != StateEmpty {
		t.Fatalf("zero deck state = %q, want empty", d.State())
	}
}

func TestDeckHappyPath(t *testing.T) {
	var d Deck
	must := func(err error, step string) {
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	must(d.Load(), "load")
	if d.State() != StateLoaded {
		t.Fatalf("after load = %q", d.State())
	}
	must(d.Cue(), "cue")
	if d.State() != StateCued {
		t.Fatalf("after cue = %q", d.State())
	}
	must(d.Play(), "play")
	if d.State() != StatePlaying {
		t.Fatalf("after play = %q", d.State())
	}
	must(d.Pause(), "pause")
	if d.State() != StatePaused {
		t.Fatalf("after pause = %q", d.State())
	}
	must(d.Play(), "resume")
	must(d.End(), "end")
	if d.State() != StateEnded {
		t.Fatalf("after end = %q", d.State())
	}
	must(d.Eject(), "eject")
	if d.State() != StateEmpty {
		t.Fatalf("after eject = %q", d.State())
	}
}

func TestDeckInvalidTransitions(t *testing.T) {
	t.Run("play on empty", func(t *testing.T) {
		var d Deck
		if err := d.Play(); err != ErrBadTransition {
			t.Fatalf("Play empty = %v", err)
		}
	})
	t.Run("cue on empty", func(t *testing.T) {
		var d Deck
		if err := d.Cue(); err != ErrBadTransition {
			t.Fatalf("Cue empty = %v", err)
		}
	})
	t.Run("pause when not playing", func(t *testing.T) {
		var d Deck
		_ = d.Load()
		if err := d.Pause(); err != ErrBadTransition {
			t.Fatalf("Pause loaded = %v", err)
		}
	})
	t.Run("end when not playing", func(t *testing.T) {
		var d Deck
		_ = d.Load()
		if err := d.End(); err != ErrBadTransition {
			t.Fatalf("End loaded = %v", err)
		}
	})
	t.Run("play on ended needs cue", func(t *testing.T) {
		var d Deck
		_ = d.Load()
		_ = d.Play()
		_ = d.End()
		if err := d.Play(); err != ErrBadTransition {
			t.Fatalf("Play ended = %v", err)
		}
		if err := d.Cue(); err != nil {
			t.Fatalf("Cue ended = %v", err)
		}
		if err := d.Play(); err != nil {
			t.Fatalf("Play after re-cue = %v", err)
		}
	})
}

func TestDeckLoadFromAnyState(t *testing.T) {
	var d Deck
	_ = d.Load()
	_ = d.Play()
	if err := d.Load(); err != nil {
		t.Fatalf("Load while playing = %v", err)
	}
	if d.State() != StateLoaded {
		t.Fatalf("reload state = %q", d.State())
	}
}

func TestDeckPlayWhilePlaying(t *testing.T) {
	var d Deck
	_ = d.Load()
	_ = d.Play()
	if err := d.Play(); err != nil {
		t.Fatalf("Play while playing = %v", err)
	}
}
