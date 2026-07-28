package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"autodj/domain/requests"
)

// requestDTO is the wire shape of a listener request / dedication. handled_at is
// empty (omitted) until the first moderation transition.
type requestDTO struct {
	ID           string `json:"id"`
	ListenerName string `json:"listener_name"`
	SongHint     string `json:"song_hint"`
	Dedication   string `json:"dedication"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	HandledAt    string `json:"handled_at,omitempty"`
}

func toRequestDTO(r requests.Request) requestDTO {
	handled := ""
	if !r.HandledAt.IsZero() {
		handled = r.HandledAt.UTC().Format(time.RFC3339Nano)
	}
	return requestDTO{
		ID:           r.ID,
		ListenerName: r.ListenerName,
		SongHint:     r.SongHint,
		Dedication:   r.Dedication,
		Status:       string(r.Status),
		CreatedAt:    r.CreatedAt.UTC().Format(time.RFC3339Nano),
		HandledAt:    handled,
	}
}

func toRequestDTOs(rs []requests.Request) []requestDTO {
	out := make([]requestDTO, 0, len(rs))
	for _, r := range rs {
		out = append(out, toRequestDTO(r))
	}
	return out
}

// postRequest records a new pending listener request. Body:
// {listener_name, song_hint, dedication}. Blank listener/song → 422 (via
// writeErr / requests.ErrEmptyListener|ErrEmptyRequest). POST /requests.
func (rt *Router) postRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ListenerName string `json:"listener_name"`
		SongHint     string `json:"song_hint"`
		Dedication   string `json:"dedication"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	req, err := rt.requests.Submit(r.Context(), body.ListenerName, body.SongHint, body.Dedication)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRequestDTO(req))
}

// getRequests lists requests, optionally filtered by ?status=. Unknown status →
// 422 (via writeErr / inbound.ErrBadStatus). GET /requests.
func (rt *Router) getRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, err := rt.requests.List(r.Context(), status)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRequestDTOs(list))
}

// postRequestApprove approves the request with {id}. POST /requests/{id}/approve.
func (rt *Router) postRequestApprove(w http.ResponseWriter, r *http.Request) {
	rt.moderateRequest(w, r, rt.requests.Approve)
}

// postRequestReject rejects the request with {id}. POST /requests/{id}/reject.
func (rt *Router) postRequestReject(w http.ResponseWriter, r *http.Request) {
	rt.moderateRequest(w, r, rt.requests.Reject)
}

// postRequestPlayed marks the request with {id} as played. POST /requests/{id}/played.
func (rt *Router) postRequestPlayed(w http.ResponseWriter, r *http.Request) {
	rt.moderateRequest(w, r, rt.requests.MarkPlayed)
}

// moderateRequest applies a service moderation func to the {id} path param.
// Unknown id → 404; forbidden transition → 409 (via writeErr).
func (rt *Router) moderateRequest(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id string) (requests.Request, error)) {
	req, err := fn(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRequestDTO(req))
}
