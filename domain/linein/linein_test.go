package linein

import (
	"errors"
	"testing"

	"autodj/domain/shared"
)

func TestNewInputDefaults(t *testing.T) {
	in, err := NewInput("mic0", "Microfone")
	if err != nil {
		t.Fatal(err)
	}
	if in.Armed {
		t.Fatal("new input must be disarmed")
	}
	if in.Gain != DefaultGain || in.Duck != DefaultDuck {
		t.Fatalf("defaults wrong: %+v", in)
	}
	if in.ID != "mic0" || in.Label != "Microfone" {
		t.Fatalf("identity wrong: %+v", in)
	}
}

func TestNewInputEmptyLabel(t *testing.T) {
	if _, err := NewInput("mic0", "   "); !errors.Is(err, ErrEmptyLabel) {
		t.Fatalf("got %v, want ErrEmptyLabel", err)
	}
}

func TestArmDisarm(t *testing.T) {
	in, _ := NewInput("aux", "Auxiliar")
	armed := in.Arm()
	if !armed.Armed {
		t.Fatal("Arm should set Armed")
	}
	if in.Armed {
		t.Fatal("Arm must not mutate the receiver")
	}
	if armed.Disarm().Armed {
		t.Fatal("Disarm should clear Armed")
	}
}

func TestSetGain(t *testing.T) {
	in, _ := NewInput("aux", "Auxiliar")
	got, err := in.SetGain(0.5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Gain != 0.5 {
		t.Fatalf("gain=%v", got.Gain)
	}
	for _, bad := range []float64{-0.1, 1.1} {
		if _, err := in.SetGain(bad); !errors.Is(err, ErrBadLevel) {
			t.Fatalf("gain %v: got %v, want ErrBadLevel", bad, err)
		}
	}
}

func TestSetDuck(t *testing.T) {
	in, _ := NewInput("aux", "Auxiliar")
	got, err := in.SetDuck(0.0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Duck != 0.0 {
		t.Fatalf("duck=%v", got.Duck)
	}
	for _, bad := range []float64{-1, 2} {
		if _, err := in.SetDuck(bad); !errors.Is(err, ErrBadLevel) {
			t.Fatalf("duck %v: got %v, want ErrBadLevel", bad, err)
		}
	}
}

func TestValidate(t *testing.T) {
	ok, _ := NewInput("id", "Label")
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if err := (Input{Label: "", Gain: 1, Duck: 0.5}).Validate(); !errors.Is(err, ErrEmptyLabel) {
		t.Fatalf("empty label: got %v", err)
	}
	if err := (Input{Label: "x", Gain: 2, Duck: 0.5}).Validate(); !errors.Is(err, ErrBadLevel) {
		t.Fatalf("bad gain: got %v", err)
	}
	if err := (Input{Label: "x", Gain: 1, Duck: -1}).Validate(); !errors.Is(err, ErrBadLevel) {
		t.Fatalf("bad duck: got %v", err)
	}
}

func TestMixableMatchesMatrix(t *testing.T) {
	in, _ := NewInput("id", "Label")
	if !in.Mixable() {
		t.Fatal("line-in must be mixable (PCM live source)")
	}
	if Source != shared.SourceLineIn {
		t.Fatalf("Source=%v, want SourceLineIn", Source)
	}
	cap := shared.Capabilities(Source)
	if !cap.PCM || !cap.Mixable || cap.Beatmatch || cap.ECAD {
		t.Fatalf("line-in matrix wrong: %+v", cap)
	}
}
