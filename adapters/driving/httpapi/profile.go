package httpapi

import (
	"encoding/json"
	"net/http"

	"autodj/domain/operatorprofile"
)

// getProfile returns the authenticated operator's saved layout, or the shipped
// default when none was ever saved. The user is resolved from the bearer token
// (Me), so a bad/missing token → 401. GET /profile/me.
func (rt *Router) getProfile(w http.ResponseWriter, r *http.Request) {
	u, err := rt.auth.Me(r.Context(), bearer(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := rt.profile.Get(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// putProfile saves the authenticated operator's layout. The owner is taken from
// the token, not the body, so nobody can write another user's profile: the body's
// userId is overridden with the token's id before Save. A malformed body → 400; an
// invalid layout → 422; a mismatched owner (defense-in-depth) → 403 (via writeErr).
// PUT /profile/me  {theme,density,panels:[{key,visible,order}]}.
func (rt *Router) putProfile(w http.ResponseWriter, r *http.Request) {
	u, err := rt.auth.Me(r.Context(), bearer(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	var p operatorprofile.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	p.UserID = u.ID // token is authoritative — ignore any body userId
	out, err := rt.profile.Save(r.Context(), u, p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
