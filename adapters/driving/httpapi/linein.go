package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"autodj/domain/linein"
)

// getLineIn lists the configured live-capture inputs (mic/aux/soundcard) with
// their armed state and gain/duck. GET /linein
func (rt *Router) getLineIn(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.linein.List(r.Context()))
}

// postLineInArm routes an input onto the local bus, ducking the music.
// Unknown id → 404. POST /linein/{id}/arm
func (rt *Router) postLineInArm(w http.ResponseWriter, r *http.Request) {
	in, err := rt.linein.Arm(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// postLineInDisarm removes an input from the bus (idempotent).
// Unknown id → 404. POST /linein/{id}/disarm
func (rt *Router) postLineInDisarm(w http.ResponseWriter, r *http.Request) {
	in, err := rt.linein.Disarm(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// putLineInLevels sets the input's pass-through gain and/or music-duck level.
// Body {gain?,duck?} — only the fields present are applied (gain then duck); an
// armed input has its levels pushed to the engine. Malformed body → 400; no field
// → 400; out-of-range → 422; unknown id → 404. PUT /linein/{id}/levels
func (rt *Router) putLineInLevels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Gain *float64 `json:"gain"`
		Duck *float64 `json:"duck"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Gain == nil && body.Duck == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gain or duck required"})
		return
	}
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	var in linein.Input
	var err error
	if body.Gain != nil {
		if in, err = rt.linein.SetGain(ctx, id, *body.Gain); err != nil {
			writeErr(w, err)
			return
		}
	}
	if body.Duck != nil {
		if in, err = rt.linein.SetDuck(ctx, id, *body.Duck); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, in)
}
