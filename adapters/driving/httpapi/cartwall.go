package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"autodj/domain/cartwall"
)

// getCartwall returns the operator's cartwall board (cartwall.Default() when the
// store is empty, resolved in the app layer).
func (rt *Router) getCartwall(w http.ResponseWriter, r *http.Request) {
	b, err := rt.cartwall.Board(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// putCartwall validates and persists the whole board, returning the stored
// value. A malformed body → 400; a hotkey clash → 409; a non-mixable track or
// malformed button → 422 (via writeErr).
func (rt *Router) putCartwall(w http.ResponseWriter, r *http.Request) {
	var b cartwall.Board
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.cartwall.SaveBoard(r.Context(), b)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// postCartButton upserts a single button into the board and returns the whole
// updated board. A malformed body → 400; a hotkey clash → 409; a non-mixable
// track or malformed button → 422 (via writeErr).
func (rt *Router) postCartButton(w http.ResponseWriter, r *http.Request) {
	var btn cartwall.CartButton
	if err := json.NewDecoder(r.Body).Decode(&btn); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.cartwall.AssignButton(r.Context(), btn)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteCartButton removes a button by id. Unknown id → 404 (via writeErr).
func (rt *Router) deleteCartButton(w http.ResponseWriter, r *http.Request) {
	if err := rt.cartwall.ClearButton(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
