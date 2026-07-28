package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"autodj/domain/identity"
)

// userDTO is the JSON shape for an account. It never carries the password hash
// (identity.User hides it) and adds the resolved permission list so the frontend
// can gate menus/actions without re-deriving the role→permission matrix.
type userDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// toUserDTO resolves a user's permissions from the fixed matrix.
func toUserDTO(u identity.User) userDTO {
	perms := make([]string, 0, len(identity.Permissions))
	for _, p := range identity.Permissions {
		if u.Can(p) {
			perms = append(perms, string(p))
		}
	}
	return userDTO{ID: u.ID, Name: u.Name, Role: string(u.Role), Permissions: perms}
}

func toUserDTOs(us []identity.User) []userDTO {
	out := make([]userDTO, 0, len(us))
	for _, u := range us {
		out = append(out, toUserDTO(u))
	}
	return out
}

// login exchanges name+password for a token + the user. Bad credentials → 401
// (via writeErr). POST /auth/login  {name,password} → {token,user}.
func (rt *Router) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	token, u, err := rt.auth.Login(r.Context(), body.Name, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": toUserDTO(u)})
}

// me returns the authenticated user for the bearer token. Missing/invalid token
// → 401 (via writeErr). GET /auth/me.
func (rt *Router) me(w http.ResponseWriter, r *http.Request) {
	u, err := rt.auth.Me(r.Context(), bearer(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

// listUsers returns every account. Gated to users.manage by requirePerm, so the
// actor is present in the context. GET /users.
func (rt *Router) listUsers(w http.ResponseWriter, r *http.Request) {
	actor, _ := userFrom(r.Context())
	us, err := rt.auth.ListUsers(r.Context(), actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserDTOs(us))
}

// createUser adds an account. Gated to users.manage. Malformed body → 400;
// validation error → 422; duplicate name → 409 (via writeErr).
// POST /users  {name,role,password}.
func (rt *Router) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	actor, _ := userFrom(r.Context())
	u, err := rt.auth.CreateUser(r.Context(), actor, body.Name, identity.Role(body.Role), body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserDTO(u))
}

// updateUser changes a user's role and (when password non-empty) its password.
// Gated to users.manage. Unknown id → 404; validation → 422 (via writeErr).
// PATCH /users/{id}  {role,password?}.
func (rt *Router) updateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	actor, _ := userFrom(r.Context())
	u, err := rt.auth.UpdateUser(r.Context(), actor, chi.URLParam(r, "id"), identity.Role(body.Role), body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

// deleteUser removes an account. Gated to users.manage. Unknown id → 404.
// DELETE /users/{id}.
func (rt *Router) deleteUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := userFrom(r.Context())
	if err := rt.auth.DeleteUser(r.Context(), actor, chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
