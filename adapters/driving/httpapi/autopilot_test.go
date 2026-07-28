package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"autodj/ports/inbound"
)

// fakeAutopilot is a configurable AutopilotService stub.
type fakeAutopilot struct {
	status  inbound.AutopilotStatus
	setErr  error
	enabled bool // last SetEnabled arg
	ticked  bool // SetEnabled was called
}

func (f *fakeAutopilot) Status(context.Context) inbound.AutopilotStatus { return f.status }
func (f *fakeAutopilot) SetEnabled(_ context.Context, enabled bool) (inbound.AutopilotStatus, error) {
	f.enabled = enabled
	f.ticked = true
	if f.setErr != nil {
		return inbound.AutopilotStatus{}, f.setErr
	}
	f.status.Enabled = enabled
	return f.status, nil
}
func (f *fakeAutopilot) Tick(context.Context) error { return nil }

func TestGetAutopilot(t *testing.T) {
	ap := &fakeAutopilot{status: inbound.AutopilotStatus{Enabled: true, QueueLen: 7, LastTick: time.Unix(1000, 0).UTC()}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAutopilot(ap))
	rec := do(t, h, http.MethodGet, "/api/v1/autopilot", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var st inbound.AutopilotStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.QueueLen != 7 {
		t.Fatalf("status=%+v", st)
	}
}

func TestPostAutopilotEnables(t *testing.T) {
	ap := &fakeAutopilot{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAutopilot(ap))
	rec := do(t, h, http.MethodPost, "/api/v1/autopilot", `{"enabled":true}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if !ap.ticked || !ap.enabled {
		t.Fatal("POST should call SetEnabled(true) — ticking immediately")
	}
	var st inbound.AutopilotStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Enabled {
		t.Fatalf("expected enabled status, got %+v", st)
	}
}

func TestPostAutopilotDisables(t *testing.T) {
	ap := &fakeAutopilot{status: inbound.AutopilotStatus{Enabled: true}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAutopilot(ap))
	rec := do(t, h, http.MethodPost, "/api/v1/autopilot", `{"enabled":false}`)
	if rec.Code != 200 || ap.enabled {
		t.Fatalf("expected disable, code=%d enabled=%v", rec.Code, ap.enabled)
	}
}

func TestPostAutopilotBadBody(t *testing.T) {
	ap := &fakeAutopilot{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAutopilot(ap))
	rec := do(t, h, http.MethodPost, "/api/v1/autopilot", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should be 400, got %d", rec.Code)
	}
	if ap.ticked {
		t.Fatal("bad body must not toggle supervision")
	}
}

func TestPostAutopilotTickError(t *testing.T) {
	ap := &fakeAutopilot{setErr: errors.New("boom")}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAutopilot(ap))
	rec := do(t, h, http.MethodPost, "/api/v1/autopilot", `{"enabled":true}`)
	if rec.Code != 500 {
		t.Fatalf("tick failure should be 500, got %d", rec.Code)
	}
}

func TestAutopilotRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/autopilot", ""); rec.Code != 404 {
		t.Fatalf("GET should 404 without WithAutopilot, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/autopilot", `{"enabled":true}`); rec.Code != 404 {
		t.Fatalf("POST should 404 without WithAutopilot, got %d", rec.Code)
	}
}
