package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"autodj/domain/staffschedule"
)

// staffShiftDTO is the wire shape of one announcer shift (snake_case).
type staffShiftDTO struct {
	ID            string `json:"id"`
	AnnouncerID   string `json:"announcer_id"`
	AnnouncerName string `json:"announcer_name"`
	Weekday       int    `json:"weekday"`
	StartMin      int    `json:"start_min"`
	EndMin        int    `json:"end_min"`
}

// staffOnAirDTO is the wire shape of the on-air answer: on_air=false means
// nobody is scheduled right now (shift is then absent).
type staffOnAirDTO struct {
	OnAir bool           `json:"on_air"`
	Shift *staffShiftDTO `json:"shift,omitempty"`
}

func toStaffShiftDTO(s staffschedule.Shift) staffShiftDTO {
	return staffShiftDTO{
		ID:            s.ID,
		AnnouncerID:   s.AnnouncerID,
		AnnouncerName: s.AnnouncerName,
		Weekday:       int(s.Weekday),
		StartMin:      s.StartMin,
		EndMin:        s.EndMin,
	}
}

// getStaffShifts lists the roster ordered by (weekday, start). GET /staff/shifts.
func (rt *Router) getStaffShifts(w http.ResponseWriter, r *http.Request) {
	roster, err := rt.staff.Roster(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	shifts := roster.Shifts()
	out := make([]staffShiftDTO, 0, len(shifts))
	for _, s := range shifts {
		out = append(out, toStaffShiftDTO(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// postStaffShift registers a new shift. Body snake_case. Validation → 422,
// overlap → 409. POST /staff/shifts.
func (rt *Router) postStaffShift(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AnnouncerID   string `json:"announcer_id"`
		AnnouncerName string `json:"announcer_name"`
		Weekday       int    `json:"weekday"`
		StartMin      int    `json:"start_min"`
		EndMin        int    `json:"end_min"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	s, err := rt.staff.AddShift(r.Context(), body.AnnouncerID, body.AnnouncerName,
		time.Weekday(body.Weekday), body.StartMin, body.EndMin)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toStaffShiftDTO(s))
}

// deleteStaffShift removes a shift by id. 204 on success, 404 if unknown.
// DELETE /staff/shifts/{id}.
func (rt *Router) deleteStaffShift(w http.ResponseWriter, r *http.Request) {
	if err := rt.staff.RemoveShift(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getStaffOnAir returns the announcer on air right now, or on_air=false when
// nobody is scheduled. An optional ?at=RFC3339 query param overrides "now"
// (used for testing / previewing another instant). GET /staff/onair.
func (rt *Router) getStaffOnAir(w http.ResponseWriter, r *http.Request) {
	at := time.Now()
	if raw := r.URL.Query().Get("at"); raw != "" {
		parsed, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid at"})
			return
		}
		at = parsed
	}
	s, ok, err := rt.staff.OnAir(r.Context(), at)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, staffOnAirDTO{OnAir: false})
		return
	}
	dto := toStaffShiftDTO(s)
	writeJSON(w, http.StatusOK, staffOnAirDTO{OnAir: true, Shift: &dto})
}
