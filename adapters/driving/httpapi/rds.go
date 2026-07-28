package httpapi

import (
	"encoding/json"
	"net/http"

	"autodj/ports/inbound"
)

// getRDSConfig reports the station-fixed RDS identity (enabled + PI/PS/PTY).
// GET /rds/config
func (rt *Router) getRDSConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.rds.Config(r.Context()))
}

// putRDSConfig stores the RDS identity. Body {enabled,pi,ps,pty}. A malformed
// body → 400; a bad identity → 422 (rds validation error via writeErr).
// PUT /rds/config
func (rt *Router) putRDSConfig(w http.ResponseWriter, r *http.Request) {
	var cfg inbound.RDSConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.rds.SetConfig(r.Context(), cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// getRDSState returns the last RDS state pushed to the encoder (zero before the
// first push). GET /rds/state
func (rt *Router) getRDSState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.rds.Current(r.Context()))
}

// postRDSRefresh rebuilds the RDS state from the given now-playing metadata and
// pushes it to the encoder, returning the pushed state. Body {title,artist}.
// Malformed body → 400; RDS off → 409; bad identity → 422; no encoder → 503.
// POST /rds/refresh
func (rt *Router) postRDSRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title  string `json:"title"`
		Artist string `json:"artist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	st, err := rt.rds.UpdateNowPlaying(r.Context(), body.Title, body.Artist)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
