package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func testClock(id string) schedule.Clock {
	return schedule.Clock{
		ID:   id,
		Name: id,
		Slots: []schedule.Slot{
			{Kind: schedule.SlotMusic, Category: library.CategoryMusic, Count: 3},
		},
	}
}

func TestGetClocks(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{clocks: []schedule.Clock{testClock("manha")}}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/clocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []schedule.Clock
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "manha" {
		t.Fatalf("body = %+v", got)
	}
}

func TestGetClocksEmptyIsArray(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/clocks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("empty clocks body = %q, want []", body)
	}
}

func TestGetClocksError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{clocksErr: errBoom}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/clocks", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestPutClockOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"name":"Manhã","slots":[{"kind":"music","category":"music","count":3}]}`
	rec := do(t, h, http.MethodPut, "/api/v1/clocks/manha", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got schedule.Clock
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// path id is authoritative, overriding any body id.
	if got.ID != "manha" || got.Name != "Manhã" {
		t.Fatalf("body = %+v", got)
	}
}

func TestPutClockPathIDOverridesBody(t *testing.T) {
	// fakeSched echoes the clock it received; the path id must win.
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"id":"WRONG","name":"x","slots":[{"kind":"music","category":"music","count":1}]}`
	rec := do(t, h, http.MethodPut, "/api/v1/clocks/right", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got schedule.Clock
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != "right" {
		t.Fatalf("id = %q, want right", got.ID)
	}
}

func TestPutClockBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/clocks/x", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPutClockInvalid(t *testing.T) {
	// SaveClock rejects with a schedule validation error → 422.
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{saveErr: schedule.ErrSlotCount}, nil)
	body := `{"name":"x","slots":[{"kind":"music","category":"music","count":0}]}`
	rec := do(t, h, http.MethodPut, "/api/v1/clocks/x", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}

func TestDeleteClockOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/clocks/manha", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestDeleteClockNotFound(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{delErr: outbound.ErrClockNotFound}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/clocks/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}
