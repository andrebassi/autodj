package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/linein"
	"autodj/ports/inbound"
)

// fakeController records arm/disarm/level commands and can force failures.
type fakeController struct {
	armed    map[string]linein.Input
	disarms  []string
	levels   []linein.Input
	armErr   error
	disErr   error
	levelErr error
}

func newFakeController() *fakeController {
	return &fakeController{armed: make(map[string]linein.Input)}
}

func (f *fakeController) Arm(_ context.Context, in linein.Input) error {
	if f.armErr != nil {
		return f.armErr
	}
	f.armed[in.ID] = in
	return nil
}
func (f *fakeController) Disarm(_ context.Context, id string) error {
	if f.disErr != nil {
		return f.disErr
	}
	f.disarms = append(f.disarms, id)
	delete(f.armed, id)
	return nil
}
func (f *fakeController) SetLevels(_ context.Context, in linein.Input) error {
	if f.levelErr != nil {
		return f.levelErr
	}
	f.levels = append(f.levels, in)
	return nil
}

func seedInputs(t *testing.T) []linein.Input {
	t.Helper()
	mic, err := linein.NewInput("mic0", "Microfone")
	if err != nil {
		t.Fatal(err)
	}
	aux, err := linein.NewInput("aux0", "Auxiliar")
	if err != nil {
		t.Fatal(err)
	}
	return []linein.Input{mic, aux}
}

func TestLineInListOrder(t *testing.T) {
	svc := NewLineInService(newFakeController(), seedInputs(t)...)
	got := svc.List(context.Background())
	if len(got) != 2 || got[0].ID != "mic0" || got[1].ID != "aux0" {
		t.Fatalf("list order wrong: %+v", got)
	}
}

func TestLineInDedupSeed(t *testing.T) {
	dup, _ := linein.NewInput("mic0", "Outro")
	svc := NewLineInService(newFakeController(), append(seedInputs(t), dup)...)
	if n := len(svc.List(context.Background())); n != 2 {
		t.Fatalf("dup id should collapse, got %d", n)
	}
}

func TestLineInArm(t *testing.T) {
	ctrl := newFakeController()
	svc := NewLineInService(ctrl, seedInputs(t)...)
	in, err := svc.Arm(context.Background(), "mic0")
	if err != nil {
		t.Fatal(err)
	}
	if !in.Armed {
		t.Fatal("Arm should return an armed input")
	}
	if _, ok := ctrl.armed["mic0"]; !ok {
		t.Fatal("controller not commanded to arm")
	}
}

func TestLineInArmNotFound(t *testing.T) {
	svc := NewLineInService(newFakeController(), seedInputs(t)...)
	if _, err := svc.Arm(context.Background(), "ghost"); !errors.Is(err, inbound.ErrLineInNotFound) {
		t.Fatalf("got %v, want ErrLineInNotFound", err)
	}
}

func TestLineInArmError(t *testing.T) {
	ctrl := newFakeController()
	ctrl.armErr = errors.New("socket down")
	svc := NewLineInService(ctrl, seedInputs(t)...)
	if _, err := svc.Arm(context.Background(), "mic0"); err == nil {
		t.Fatal("arm error should surface")
	}
	// state must not flip to armed on a failed command.
	if svc.List(context.Background())[0].Armed {
		t.Fatal("failed arm must not store Armed")
	}
}

func TestLineInDisarm(t *testing.T) {
	ctrl := newFakeController()
	svc := NewLineInService(ctrl, seedInputs(t)...)
	if _, err := svc.Arm(context.Background(), "mic0"); err != nil {
		t.Fatal(err)
	}
	in, err := svc.Disarm(context.Background(), "mic0")
	if err != nil {
		t.Fatal(err)
	}
	if in.Armed {
		t.Fatal("Disarm should clear Armed")
	}
	if len(ctrl.disarms) != 1 || ctrl.disarms[0] != "mic0" {
		t.Fatalf("controller disarms=%v", ctrl.disarms)
	}
}

func TestLineInDisarmNotFound(t *testing.T) {
	svc := NewLineInService(newFakeController(), seedInputs(t)...)
	if _, err := svc.Disarm(context.Background(), "ghost"); !errors.Is(err, inbound.ErrLineInNotFound) {
		t.Fatalf("got %v, want ErrLineInNotFound", err)
	}
}

func TestLineInDisarmError(t *testing.T) {
	ctrl := newFakeController()
	ctrl.disErr = errors.New("boom")
	svc := NewLineInService(ctrl, seedInputs(t)...)
	if _, err := svc.Disarm(context.Background(), "mic0"); err == nil {
		t.Fatal("disarm error should surface")
	}
}

func TestLineInSetGainArmedPushes(t *testing.T) {
	ctrl := newFakeController()
	svc := NewLineInService(ctrl, seedInputs(t)...)
	if _, err := svc.Arm(context.Background(), "mic0"); err != nil {
		t.Fatal(err)
	}
	in, err := svc.SetGain(context.Background(), "mic0", 0.4)
	if err != nil {
		t.Fatal(err)
	}
	if in.Gain != 0.4 {
		t.Fatalf("gain=%v", in.Gain)
	}
	if len(ctrl.levels) != 1 || ctrl.levels[0].Gain != 0.4 {
		t.Fatalf("armed level change must push: %+v", ctrl.levels)
	}
}

func TestLineInSetDuckDisarmedNoPush(t *testing.T) {
	ctrl := newFakeController()
	svc := NewLineInService(ctrl, seedInputs(t)...)
	in, err := svc.SetDuck(context.Background(), "aux0", 0.2)
	if err != nil {
		t.Fatal(err)
	}
	if in.Duck != 0.2 {
		t.Fatalf("duck=%v", in.Duck)
	}
	if len(ctrl.levels) != 0 {
		t.Fatal("disarmed level change must not touch the bus")
	}
}

func TestLineInSetGainInvalid(t *testing.T) {
	svc := NewLineInService(newFakeController(), seedInputs(t)...)
	if _, err := svc.SetGain(context.Background(), "mic0", 2); !errors.Is(err, linein.ErrBadLevel) {
		t.Fatalf("got %v, want ErrBadLevel", err)
	}
}

func TestLineInSetGainNotFound(t *testing.T) {
	svc := NewLineInService(newFakeController(), seedInputs(t)...)
	if _, err := svc.SetGain(context.Background(), "ghost", 0.5); !errors.Is(err, inbound.ErrLineInNotFound) {
		t.Fatalf("got %v, want ErrLineInNotFound", err)
	}
}

func TestLineInSetLevelsPushError(t *testing.T) {
	ctrl := newFakeController()
	svc := NewLineInService(ctrl, seedInputs(t)...)
	if _, err := svc.Arm(context.Background(), "mic0"); err != nil {
		t.Fatal(err)
	}
	ctrl.levelErr = errors.New("boom")
	if _, err := svc.SetGain(context.Background(), "mic0", 0.3); err == nil {
		t.Fatal("SetLevels error should surface")
	}
}
