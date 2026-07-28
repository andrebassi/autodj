package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/rds"
	"autodj/ports/inbound"
)

// fakeRDS is a configurable RDSService stub.
type fakeRDS struct {
	cfg        inbound.RDSConfig
	setErr     error
	updErr     error
	state      rds.State
	current    rds.State
	lastTitle  string
	lastArtist string
	setCalls   int
	updCalls   int
}

func (f *fakeRDS) Config(context.Context) inbound.RDSConfig { return f.cfg }
func (f *fakeRDS) SetConfig(_ context.Context, cfg inbound.RDSConfig) (inbound.RDSConfig, error) {
	f.setCalls++
	if f.setErr != nil {
		return inbound.RDSConfig{}, f.setErr
	}
	f.cfg = cfg
	return cfg, nil
}
func (f *fakeRDS) UpdateNowPlaying(_ context.Context, title, artist string) (rds.State, error) {
	f.updCalls++
	f.lastTitle, f.lastArtist = title, artist
	if f.updErr != nil {
		return rds.State{}, f.updErr
	}
	return f.state, nil
}
func (f *fakeRDS) Current(context.Context) rds.State { return f.current }

func enabledDTO() inbound.RDSConfig {
	return inbound.RDSConfig{Enabled: true, PI: 0x1001, PS: "AUTODJ", PTY: 10}
}

func TestGetRDSConfig(t *testing.T) {
	svc := &fakeRDS{cfg: enabledDTO()}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodGet, "/api/v1/rds/config", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got inbound.RDSConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.PS != "AUTODJ" {
		t.Fatalf("cfg=%+v", got)
	}
}

func TestPutRDSConfig(t *testing.T) {
	svc := &fakeRDS{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/rds/config", `{"enabled":true,"pi":4097,"ps":"AUTODJ","pty":10}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.setCalls != 1 || !svc.cfg.Enabled || svc.cfg.PS != "AUTODJ" {
		t.Fatalf("stored=%+v calls=%d", svc.cfg, svc.setCalls)
	}
}

func TestPutRDSConfigBadBody(t *testing.T) {
	svc := &fakeRDS{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/rds/config", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should be 400, got %d", rec.Code)
	}
	if svc.setCalls != 0 {
		t.Fatal("bad body must not reach service")
	}
}

func TestPutRDSConfigInvalid(t *testing.T) {
	svc := &fakeRDS{setErr: rds.ErrBadPS}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/rds/config", `{"enabled":true,"pi":1,"ps":"TOOLONGPS","pty":0}`)
	if rec.Code != 422 {
		t.Fatalf("bad identity should be 422, got %d", rec.Code)
	}
}

func TestGetRDSState(t *testing.T) {
	svc := &fakeRDS{current: rds.State{PI: 0x1001, PS: "AUTODJ", RT: "♪ Trim - Song"}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodGet, "/api/v1/rds/state", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got rds.State
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RT != "♪ Trim - Song" {
		t.Fatalf("state=%+v", got)
	}
}

func TestPostRDSRefresh(t *testing.T) {
	svc := &fakeRDS{state: rds.State{PI: 0x1001, PS: "AUTODJ", RT: "♪ Os Ventania - Chuva de Neon"}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/rds/refresh", `{"title":"Chuva de Neon","artist":"Os Ventania"}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.lastTitle != "Chuva de Neon" || svc.lastArtist != "Os Ventania" {
		t.Fatalf("args title=%q artist=%q", svc.lastTitle, svc.lastArtist)
	}
	var got rds.State
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RT != "♪ Os Ventania - Chuva de Neon" {
		t.Fatalf("state=%+v", got)
	}
}

func TestPostRDSRefreshBadBody(t *testing.T) {
	svc := &fakeRDS{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/rds/refresh", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should be 400, got %d", rec.Code)
	}
	if svc.updCalls != 0 {
		t.Fatal("bad body must not reach service")
	}
}

func TestPostRDSRefreshDisabled(t *testing.T) {
	svc := &fakeRDS{updErr: rds.ErrDisabled}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/rds/refresh", `{"title":"x","artist":"y"}`)
	if rec.Code != 409 {
		t.Fatalf("disabled should be 409, got %d", rec.Code)
	}
}

func TestPostRDSRefreshNoEncoder(t *testing.T) {
	svc := &fakeRDS{updErr: inbound.ErrEncoderUnavailable}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/rds/refresh", `{"title":"x","artist":"y"}`)
	if rec.Code != 503 {
		t.Fatalf("no encoder should be 503, got %d", rec.Code)
	}
}

func TestPostRDSRefreshInvalidIdentity(t *testing.T) {
	svc := &fakeRDS{updErr: rds.ErrBadPI}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRDS(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/rds/refresh", `{"title":"x","artist":"y"}`)
	if rec.Code != 422 {
		t.Fatalf("bad identity should be 422, got %d", rec.Code)
	}
}

func TestRDSRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/rds/config", ""},
		{http.MethodPut, "/api/v1/rds/config", `{}`},
		{http.MethodGet, "/api/v1/rds/state", ""},
		{http.MethodPost, "/api/v1/rds/refresh", `{}`},
	} {
		if rec := do(t, h, tc.method, tc.path, tc.body); rec.Code != 404 {
			t.Fatalf("%s %s should 404 without WithRDS, got %d", tc.method, tc.path, rec.Code)
		}
	}
}
