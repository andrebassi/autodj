package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func testEvent(id string) schedule.ScheduledEvent {
	return schedule.ScheduledEvent{
		ID:       id,
		Kind:     schedule.EventComercial,
		Days:     []time.Weekday{time.Monday},
		Hour:     8,
		Minute:   0,
		Priority: 50,
		MediaRef: "commercial",
	}
}

func TestGetEvents(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{events: []schedule.ScheduledEvent{testEvent("voz")}}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []schedule.ScheduledEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "voz" {
		t.Fatalf("body = %+v", got)
	}
}

func TestGetEventsEmptyIsArray(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("empty events body = %q, want []", body)
	}
}

func TestGetEventsError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{eventsErr: errBoom}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestPutEventOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"kind":"comercial","days":[1],"hour":8,"minute":0,"priority":50,"mediaRef":"commercial"}`
	rec := do(t, h, http.MethodPut, "/api/v1/events/voz", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got schedule.ScheduledEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// path id is authoritative, overriding any body id.
	if got.ID != "voz" {
		t.Fatalf("id = %q, want voz", got.ID)
	}
}

func TestPutEventPathIDOverridesBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"id":"WRONG","kind":"comercial","days":[1],"hour":8,"minute":0,"priority":50}`
	rec := do(t, h, http.MethodPut, "/api/v1/events/right", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got schedule.ScheduledEvent
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != "right" {
		t.Fatalf("id = %q, want right", got.ID)
	}
}

func TestPutEventBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/events/x", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPutEventInvalid(t *testing.T) {
	// SaveEvent rejects with a schedule validation error → 422.
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{saveEvErr: schedule.ErrEventHour}, nil)
	body := `{"kind":"comercial","days":[1],"hour":99,"minute":0,"priority":50}`
	rec := do(t, h, http.MethodPut, "/api/v1/events/x", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}

func TestDeleteEventOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/events/voz", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestDeleteEventNotFound(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{delEvErr: outbound.ErrEventNotFound}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/events/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestEventsDueOK(t *testing.T) {
	tr, err := library.NewTrack("c1", "local", "spot", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Category = string(library.CategoryCommercial)
	tr.Path = "/spots/c1.flac"
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{injTracks: []library.Track{*tr}}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events/due?at=2024-01-01T08:00:00Z", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got []trackDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c1" {
		t.Fatalf("body = %+v", got)
	}
}

func TestEventsDueEmptyIsArray(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events/due?at=2024-01-01T09:00:00Z", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("empty due body = %q, want []", body)
	}
}

func TestEventsDueMissingAt(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events/due", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestEventsDueInvalidAt(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events/due?at=not-a-date", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestEventsDueError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{injErr: errBoom}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/events/due?at=2024-01-01T08:00:00Z", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}
