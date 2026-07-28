package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"autodj/domain/mixing"
)

var errBoom = errors.New("boom")

func TestGetMix(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/mix", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var s mixing.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s != mixing.Default() {
		t.Fatalf("body = %+v, want default", s)
	}
}

func TestGetMixError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{getErr: errBoom}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/mix", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestPutMixOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"musicaOut":3500,"normalizeVol":90}`
	rec := do(t, h, http.MethodPut, "/api/v1/mix", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var s mixing.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.MusicaOut != 3500 {
		t.Fatalf("MusicaOut = %d, want 3500", s.MusicaOut)
	}
}

func TestPutMixBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/mix", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPutMixInvalid(t *testing.T) {
	// Save rejects with a mixing validation error → 422.
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{saveErr: mixing.ErrVolPercent}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/mix", `{"normalizeVol":500}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}
