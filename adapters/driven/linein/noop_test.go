package linein

import (
	"context"
	"testing"

	dlinein "autodj/domain/linein"
	"autodj/ports/outbound"
)

// compile-time: NoopController satisfies the port.
var _ outbound.LineInController = (*NoopController)(nil)

func TestNoopArmSetDisarm(t *testing.T) {
	c := NewNoopController()
	ctx := context.Background()
	in, err := dlinein.NewInput("mic0", "Microfone")
	if err != nil {
		t.Fatal(err)
	}
	in = in.Arm()

	if err := c.Arm(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Armed("mic0")
	if !ok || !got.Armed {
		t.Fatalf("armed=%+v ok=%v", got, ok)
	}

	lvl, _ := in.SetGain(0.4)
	if err := c.SetLevels(ctx, lvl); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Armed("mic0"); got.Gain != 0.4 {
		t.Fatalf("gain=%v", got.Gain)
	}

	if err := c.Disarm(ctx, "mic0"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Armed("mic0"); ok {
		t.Fatal("disarm should drop the input")
	}
	if c.commands != 3 {
		t.Fatalf("commands=%d", c.commands)
	}
}
