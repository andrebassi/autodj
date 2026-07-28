package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/linein"
	"autodj/ports/inbound"
)

// fakeLineIn is a configurable LineInService stub.
type fakeLineIn struct {
	inputs   []linein.Input
	armErr   error
	disErr   error
	gainErr  error
	duckErr  error
	last     linein.Input
	armID    string
	disID    string
	gainVal  float64
	duckVal  float64
	gainCall int
	duckCall int
}

func (f *fakeLineIn) List(context.Context) []linein.Input { return f.inputs }
func (f *fakeLineIn) Arm(_ context.Context, id string) (linein.Input, error) {
	f.armID = id
	if f.armErr != nil {
		return linein.Input{}, f.armErr
	}
	return linein.Input{ID: id, Label: "Mic", Armed: true, Gain: 1, Duck: 0.6}, nil
}
func (f *fakeLineIn) Disarm(_ context.Context, id string) (linein.Input, error) {
	f.disID = id
	if f.disErr != nil {
		return linein.Input{}, f.disErr
	}
	return linein.Input{ID: id, Label: "Mic", Armed: false, Gain: 1, Duck: 0.6}, nil
}
func (f *fakeLineIn) SetGain(_ context.Context, id string, v float64) (linein.Input, error) {
	f.gainCall++
	f.gainVal = v
	if f.gainErr != nil {
		return linein.Input{}, f.gainErr
	}
	f.last = linein.Input{ID: id, Label: "Mic", Armed: true, Gain: v, Duck: 0.6}
	return f.last, nil
}
func (f *fakeLineIn) SetDuck(_ context.Context, id string, v float64) (linein.Input, error) {
	f.duckCall++
	f.duckVal = v
	if f.duckErr != nil {
		return linein.Input{}, f.duckErr
	}
	f.last = linein.Input{ID: id, Label: "Mic", Armed: true, Gain: 1, Duck: v}
	return f.last, nil
}

func TestGetLineIn(t *testing.T) {
	svc := &fakeLineIn{inputs: []linein.Input{
		{ID: "mic0", Label: "Microfone", Gain: 1, Duck: 0.6},
		{ID: "aux0", Label: "Auxiliar", Gain: 1, Duck: 0.6},
	}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodGet, "/api/v1/linein", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got []linein.Input
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "mic0" {
		t.Fatalf("list=%+v", got)
	}
}

func TestPostLineInArm(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/linein/mic0/arm", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.armID != "mic0" {
		t.Fatalf("armID=%q", svc.armID)
	}
	var got linein.Input
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Armed {
		t.Fatal("armed input expected")
	}
}

func TestPostLineInArmNotFound(t *testing.T) {
	svc := &fakeLineIn{armErr: inbound.ErrLineInNotFound}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/linein/ghost/arm", "")
	if rec.Code != 404 {
		t.Fatalf("unknown id should 404, got %d", rec.Code)
	}
}

func TestPostLineInDisarm(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/linein/mic0/disarm", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.disID != "mic0" {
		t.Fatalf("disID=%q", svc.disID)
	}
}

func TestPutLineInLevelsGain(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/mic0/levels", `{"gain":0.4}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.gainCall != 1 || svc.gainVal != 0.4 || svc.duckCall != 0 {
		t.Fatalf("gain=%d/%v duck=%d", svc.gainCall, svc.gainVal, svc.duckCall)
	}
}

func TestPutLineInLevelsBoth(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/mic0/levels", `{"gain":0.5,"duck":0.2}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.gainCall != 1 || svc.duckCall != 1 || svc.duckVal != 0.2 {
		t.Fatalf("gain=%d duck=%d/%v", svc.gainCall, svc.duckCall, svc.duckVal)
	}
	var got linein.Input
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Duck != 0.2 {
		t.Fatalf("last should reflect duck: %+v", got)
	}
}

func TestPutLineInLevelsBadBody(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/mic0/levels", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should 400, got %d", rec.Code)
	}
	if svc.gainCall != 0 || svc.duckCall != 0 {
		t.Fatal("bad body must not reach service")
	}
}

func TestPutLineInLevelsEmpty(t *testing.T) {
	svc := &fakeLineIn{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/mic0/levels", `{}`)
	if rec.Code != 400 {
		t.Fatalf("no field should 400, got %d", rec.Code)
	}
	if svc.gainCall != 0 || svc.duckCall != 0 {
		t.Fatal("empty body must not reach service")
	}
}

func TestPutLineInLevelsInvalid(t *testing.T) {
	svc := &fakeLineIn{gainErr: linein.ErrBadLevel}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/mic0/levels", `{"gain":2}`)
	if rec.Code != 422 {
		t.Fatalf("out-of-range should 422, got %d", rec.Code)
	}
}

func TestPutLineInLevelsNotFound(t *testing.T) {
	svc := &fakeLineIn{gainErr: inbound.ErrLineInNotFound}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithLineIn(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/linein/ghost/levels", `{"gain":0.5}`)
	if rec.Code != 404 {
		t.Fatalf("unknown id should 404, got %d", rec.Code)
	}
}

func TestLineInRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/linein", ""},
		{http.MethodPost, "/api/v1/linein/mic0/arm", ""},
		{http.MethodPost, "/api/v1/linein/mic0/disarm", ""},
		{http.MethodPut, "/api/v1/linein/mic0/levels", `{"gain":0.5}`},
	} {
		if rec := do(t, h, tc.method, tc.path, tc.body); rec.Code != 404 {
			t.Fatalf("%s %s should 404 without WithLineIn, got %d", tc.method, tc.path, rec.Code)
		}
	}
}
