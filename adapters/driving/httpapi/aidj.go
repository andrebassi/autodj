package httpapi

import (
	"encoding/json"
	"net/http"

	"autodj/ports/inbound"
)

// getAIDJConfig reports whether spoken segues are enabled and the chosen voice.
// GET /aidj/config
func (rt *Router) getAIDJConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.aidj.Config(r.Context()))
}

// putAIDJConfig updates the enabled flag and voice. Body {enabled,voice}. A
// malformed body → 400; an unknown voice → 422 (aidj.ErrBadVoice via writeErr).
// PUT /aidj/config
func (rt *Router) putAIDJConfig(w http.ResponseWriter, r *http.Request) {
	var cfg inbound.AIDJConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.aidj.SetConfig(r.Context(), cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// postAIDJVoiceover renders + synthesizes the spoken segue for the next track and
// returns the source=tts mixable Track to enqueue ahead of it. Body {trackId}.
// Missing trackId → 400; segues off → 409; no gateway → 503; unknown track → 404.
// POST /aidj/voiceover
func (rt *Router) postAIDJVoiceover(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TrackID string `json:"trackId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trackId required"})
		return
	}
	t, err := rt.aidj.Voiceover(r.Context(), body.TrackID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTrackDTO(&t))
}
