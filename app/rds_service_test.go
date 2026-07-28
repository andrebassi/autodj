package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/rds"
	"autodj/ports/inbound"
)

// fakeEncoder is an in-memory RDSEncoder: records the last pushed state, counts
// calls, and can force a push failure.
type fakeEncoder struct {
	last    rds.State
	calls   int
	pushErr error
}

func (f *fakeEncoder) Push(_ context.Context, s rds.State) error {
	f.calls++
	if f.pushErr != nil {
		return f.pushErr
	}
	f.last = s
	return nil
}

func enabledCfg() inbound.RDSConfig {
	return inbound.RDSConfig{Enabled: true, PI: 0x1001, PS: "AUTODJ", PTY: 10}
}

func TestRDSConfigDefault(t *testing.T) {
	svc := NewRDSService(&fakeEncoder{})
	cfg := svc.Config(context.Background())
	if cfg.Enabled {
		t.Fatal("default config must be disabled")
	}
	if cfg.PS != "AUTODJ" || cfg.PI != 0x1001 || cfg.PTY != 10 {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}

func TestRDSSetConfig(t *testing.T) {
	svc := NewRDSService(&fakeEncoder{})
	got, err := svc.SetConfig(context.Background(), enabledCfg())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.PS != "AUTODJ" {
		t.Fatalf("stored config wrong: %+v", got)
	}
	if svc.Config(context.Background()) != got {
		t.Fatal("Config should return the stored value")
	}
}

func TestRDSSetConfigInvalid(t *testing.T) {
	svc := NewRDSService(&fakeEncoder{})
	// PI==0 is invalid → ErrBadPI; store stays at default.
	_, err := svc.SetConfig(context.Background(), inbound.RDSConfig{Enabled: true, PI: 0, PS: "X", PTY: 0})
	if !errors.Is(err, rds.ErrBadPI) {
		t.Fatalf("got %v, want ErrBadPI", err)
	}
	if svc.Config(context.Background()).Enabled {
		t.Fatal("invalid SetConfig must not mutate the store")
	}
}

func TestRDSUpdateNowPlaying(t *testing.T) {
	enc := &fakeEncoder{}
	svc := NewRDSService(enc)
	if _, err := svc.SetConfig(context.Background(), enabledCfg()); err != nil {
		t.Fatal(err)
	}
	st, err := svc.UpdateNowPlaying(context.Background(), "Chuva de Neon", "Os Ventania")
	if err != nil {
		t.Fatal(err)
	}
	if st.RT != "♪ Os Ventania - Chuva de Neon" {
		t.Fatalf("RT=%q", st.RT)
	}
	if enc.calls != 1 || enc.last.RT != st.RT {
		t.Fatalf("encoder not pushed: calls=%d last=%+v", enc.calls, enc.last)
	}
	if svc.Current(context.Background()).RT != st.RT {
		t.Fatal("Current should reflect the last push")
	}
}

func TestRDSUpdateDisabled(t *testing.T) {
	enc := &fakeEncoder{}
	svc := NewRDSService(enc) // default = disabled
	_, err := svc.UpdateNowPlaying(context.Background(), "t", "a")
	if !errors.Is(err, rds.ErrDisabled) {
		t.Fatalf("got %v, want ErrDisabled", err)
	}
	if enc.calls != 0 {
		t.Fatal("disabled RDS must not push")
	}
}

func TestRDSUpdateNoEncoder(t *testing.T) {
	svc := NewRDSService(nil)
	if _, err := svc.SetConfig(context.Background(), enabledCfg()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UpdateNowPlaying(context.Background(), "t", "a")
	if !errors.Is(err, inbound.ErrEncoderUnavailable) {
		t.Fatalf("got %v, want ErrEncoderUnavailable", err)
	}
}

func TestRDSUpdatePushError(t *testing.T) {
	enc := &fakeEncoder{pushErr: errors.New("boom")}
	svc := NewRDSService(enc)
	if _, err := svc.SetConfig(context.Background(), enabledCfg()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateNowPlaying(context.Background(), "t", "a"); err == nil {
		t.Fatal("push error should surface")
	}
	// last state stays zero after a failed push.
	if svc.Current(context.Background()).RT != "" {
		t.Fatal("failed push must not update Current")
	}
}

func TestRDSCurrentZeroBeforePush(t *testing.T) {
	svc := NewRDSService(&fakeEncoder{})
	if svc.Current(context.Background()) != (rds.State{}) {
		t.Fatal("Current must be zero before first push")
	}
}
