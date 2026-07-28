package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/staffschedule"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// fakeStaff is a configurable StaffScheduleService stub.
type fakeStaff struct {
	shift     staffschedule.Shift
	roster    staffschedule.Roster
	onAir     staffschedule.Shift
	onAirOK   bool
	addErr    error
	removeErr error
	rosterErr error
	onAirErr  error
}

func (f *fakeStaff) AddShift(context.Context, string, string, time.Weekday, int, int) (staffschedule.Shift, error) {
	return f.shift, f.addErr
}
func (f *fakeStaff) RemoveShift(context.Context, string) error { return f.removeErr }
func (f *fakeStaff) Roster(context.Context) (staffschedule.Roster, error) {
	return f.roster, f.rosterErr
}
func (f *fakeStaff) OnAir(context.Context, time.Time) (staffschedule.Shift, bool, error) {
	return f.onAir, f.onAirOK, f.onAirErr
}

func withStaff(s inbound.StaffScheduleService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithStaffSchedule(s))
}

func mustRoster(t *testing.T, shifts ...staffschedule.Shift) staffschedule.Roster {
	t.Helper()
	r, err := staffschedule.BuildRoster(shifts)
	if err != nil {
		t.Fatalf("build roster: %v", err)
	}
	return r
}

func mkShiftDTO(t *testing.T, id, name string, weekday time.Weekday, start, end int) staffschedule.Shift {
	t.Helper()
	s, err := staffschedule.NewShift(id, "aid-"+id, name, weekday, start, end)
	if err != nil {
		t.Fatalf("new shift: %v", err)
	}
	return s
}

func TestGetStaffShiftsOK(t *testing.T) {
	roster := mustRoster(t,
		mkShiftDTO(t, "s1", "Bruna", time.Monday, 8*60, 10*60),
		mkShiftDTO(t, "s2", "Caio", time.Monday, 10*60, 12*60),
	)
	h := withStaff(&fakeStaff{roster: roster})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/shifts", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got []staffShiftDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].AnnouncerName != "Bruna" || got[1].StartMin != 600 {
		t.Fatalf("shifts=%+v", got)
	}
}

func TestGetStaffShiftsError(t *testing.T) {
	h := withStaff(&fakeStaff{rosterErr: outbound.ErrShiftNotFound})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/shifts", "")
	if rec.Code != 404 {
		t.Fatalf("roster error should 404, got %d", rec.Code)
	}
}

func TestPostStaffShiftOK(t *testing.T) {
	s := mkShiftDTO(t, "staff-1-1", "Duda", time.Tuesday, 14*60, 16*60)
	h := withStaff(&fakeStaff{shift: s})
	rec := do(t, h, http.MethodPost, "/api/v1/staff/shifts",
		`{"announcer_id":"aid-x","announcer_name":"Duda","weekday":2,"start_min":840,"end_min":960}`)
	if rec.Code != 201 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got staffShiftDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "staff-1-1" || got.AnnouncerName != "Duda" || got.Weekday != 2 {
		t.Fatalf("shift=%+v", got)
	}
}

func TestPostStaffShiftValidation(t *testing.T) {
	h := withStaff(&fakeStaff{addErr: staffschedule.ErrBadRange})
	rec := do(t, h, http.MethodPost, "/api/v1/staff/shifts",
		`{"announcer_name":"Duda","weekday":2,"start_min":960,"end_min":840}`)
	if rec.Code != 422 {
		t.Fatalf("bad range should 422, got %d", rec.Code)
	}
}

func TestPostStaffShiftOverlap(t *testing.T) {
	h := withStaff(&fakeStaff{addErr: staffschedule.ErrOverlap})
	rec := do(t, h, http.MethodPost, "/api/v1/staff/shifts",
		`{"announcer_name":"Duda","weekday":1,"start_min":480,"end_min":600}`)
	if rec.Code != 409 {
		t.Fatalf("overlap should 409, got %d", rec.Code)
	}
}

func TestPostStaffShiftBadBody(t *testing.T) {
	h := withStaff(&fakeStaff{})
	rec := do(t, h, http.MethodPost, "/api/v1/staff/shifts", `not json`)
	if rec.Code != 400 {
		t.Fatalf("bad body should 400, got %d", rec.Code)
	}
}

func TestDeleteStaffShiftOK(t *testing.T) {
	h := withStaff(&fakeStaff{})
	rec := do(t, h, http.MethodDelete, "/api/v1/staff/shifts/s1", "")
	if rec.Code != 204 {
		t.Fatalf("delete should 204, got %d", rec.Code)
	}
}

func TestDeleteStaffShiftNotFound(t *testing.T) {
	h := withStaff(&fakeStaff{removeErr: outbound.ErrShiftNotFound})
	rec := do(t, h, http.MethodDelete, "/api/v1/staff/shifts/nope", "")
	if rec.Code != 404 {
		t.Fatalf("unknown shift should 404, got %d", rec.Code)
	}
}

func TestGetStaffOnAirCovered(t *testing.T) {
	s := mkShiftDTO(t, "s1", "Bruna", time.Monday, 8*60, 12*60)
	h := withStaff(&fakeStaff{onAir: s, onAirOK: true})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/onair", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got staffOnAirDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OnAir || got.Shift == nil || got.Shift.AnnouncerName != "Bruna" {
		t.Fatalf("onair=%+v", got)
	}
}

func TestGetStaffOnAirNobody(t *testing.T) {
	h := withStaff(&fakeStaff{onAirOK: false})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/onair", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got staffOnAirDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OnAir || got.Shift != nil {
		t.Fatalf("nobody on air but got %+v", got)
	}
}

func TestGetStaffOnAirAtParam(t *testing.T) {
	s := mkShiftDTO(t, "s1", "Bruna", time.Monday, 8*60, 12*60)
	h := withStaff(&fakeStaff{onAir: s, onAirOK: true})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/onair?at=2026-07-20T09:00:00Z", "")
	if rec.Code != 200 {
		t.Fatalf("valid at should 200, got %d body=%s", rec.Code, rec.Body)
	}
}

func TestGetStaffOnAirBadAt(t *testing.T) {
	h := withStaff(&fakeStaff{})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/onair?at=nope", "")
	if rec.Code != 400 {
		t.Fatalf("bad at should 400, got %d", rec.Code)
	}
}

func TestGetStaffOnAirError(t *testing.T) {
	h := withStaff(&fakeStaff{onAirErr: outbound.ErrShiftNotFound})
	rec := do(t, h, http.MethodGet, "/api/v1/staff/onair", "")
	if rec.Code != 404 {
		t.Fatalf("onair error should 404, got %d", rec.Code)
	}
}

func TestStaffRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, path := range []string{"/api/v1/staff/shifts", "/api/v1/staff/onair"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 404 {
			t.Fatalf("%s should 404 without WithStaffSchedule, got %d", path, rec.Code)
		}
	}
}
