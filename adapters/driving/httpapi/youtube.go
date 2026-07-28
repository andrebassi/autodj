package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"autodj/domain/library"
	"autodj/domain/youtube"
)

// getYouTubeStatus reports whether a Google/YouTube account is linked and (best
// effort) Premium. GET /api/v1/youtube/status → {connected, premium}.
func (rt *Router) getYouTubeStatus(w http.ResponseWriter, r *http.Request) {
	st, err := rt.youtube.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// postYouTubeConnect starts a Google auth-code OAuth flow and returns the
// authorize URL the browser should navigate to. POST /api/v1/youtube/connect →
// {authURL}. Server without OAuth client (só API key) → 503: conectar é
// impossível, mas o resto da aba (colar links) segue funcionando.
func (rt *Router) postYouTubeConnect(w http.ResponseWriter, r *http.Request) {
	authURL, err := rt.youtube.Begin(r.Context())
	if err != nil {
		writeYouTubeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authURL": authURL})
}

// getYouTubeCallback finishes the OAuth round-trip: validates state, exchanges the
// code for tokens and persists the account, then redirects (302) back to the UI. A
// state mismatch (ErrStateMismatch) or expired pending auth (ErrAuthExpired) → 400.
// GET /api/v1/youtube/callback?code&state.
func (rt *Router) getYouTubeCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code, state := q.Get("code"), q.Get("state")
	if err := rt.youtube.Complete(r.Context(), state, code); err != nil {
		writeErr(w, err)
		return
	}
	http.Redirect(w, r, rt.youtubeReturn, http.StatusFound)
}

// postYouTubeDisconnect unlinks the account. Idempotent → 204.
// POST /api/v1/youtube/disconnect.
func (rt *Router) postYouTubeDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := rt.youtube.Disconnect(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getYouTubeToken returns the current access token for the IFrame Player (Story
// 6.7). The browser fetches it here; the backend refreshes transparently.
// GET /api/v1/youtube/token → {accessToken}. No linked account → 409 (connect
// first). The token authorizes read-only catalog access; playout uses the free
// IFrame API and never mixes into the local PCM bus.
func (rt *Router) getYouTubeToken(w http.ResponseWriter, r *http.Request) {
	tok, err := rt.youtube.AccessToken(r.Context())
	if err != nil {
		writeYouTubeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": tok})
}

// getYouTubeSearch queries the YouTube catalog through the backend the operator
// picked in the panel.
// GET /api/v1/youtube/search?q&type&backend (type = video|playlist, default video;
// backend = free|api, default free — an older client that omits it gets the free
// path, which needs no account) →
// [{uri,title,artist,album,artworkUrl,durationMs,kind}].
func (rt *Router) getYouTubeSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, kind := q.Get("q"), q.Get("type")
	if kind == "" {
		kind = youtube.KindVideo
	}
	items, err := rt.youtube.Search(r.Context(), query, kind, q.Get("backend"))
	if err != nil {
		writeYouTubeErr(w, err)
		return
	}
	if items == nil {
		items = []youtube.CatalogItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// youtubeImportRequest is the /import body: either explicit video URIs/ids or a
// playlistID whose videos are imported wholesale.
type youtubeImportRequest struct {
	URIs       []string `json:"uris"`
	PlaylistID string   `json:"playlistID"`
}

// postYouTubeImport imports videos (or a whole playlist) into the catalog as
// non-mixable YouTube tracks. POST /api/v1/youtube/import → created tracks.
func (rt *Router) postYouTubeImport(w http.ResponseWriter, r *http.Request) {
	var req youtubeImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.PlaylistID == "" && len(req.URIs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "uris or playlistID required"})
		return
	}
	tracks, err := rt.importYouTube(r, req)
	if err != nil {
		writeYouTubeErr(w, err)
		return
	}
	out := make([]trackDTO, 0, len(tracks))
	for i := range tracks {
		out = append(out, toTrackDTO(&tracks[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// youtubeLinksRequest is the /import-links body: one blob of pasted text.
type youtubeLinksRequest struct {
	Text string `json:"text"`
}

// postYouTubeImportLinks imports videos from pasted links (separated by ";",
// newlines or commas) WITHOUT requiring a linked Google account — the operator
// already has the links, so a login would be friction for nothing, and resolving
// ids costs 1 Data API unit per batch (search, which does cost ~100, stays
// gated). POST /api/v1/youtube/import-links {text} → created tracks. Text with
// no recognizable link → 400. Non-embeddable videos are dropped by the catalog,
// so the response may be shorter than what was pasted.
func (rt *Router) postYouTubeImportLinks(w http.ResponseWriter, r *http.Request) {
	var req youtubeLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	tracks, err := rt.youtube.ImportLinks(r.Context(), req.Text)
	if err != nil {
		writeYouTubeErr(w, err)
		return
	}
	out := make([]trackDTO, 0, len(tracks))
	for i := range tracks {
		out = append(out, toTrackDTO(&tracks[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) importYouTube(r *http.Request, req youtubeImportRequest) ([]library.Track, error) {
	if req.PlaylistID != "" {
		return rt.youtube.ImportPlaylist(r.Context(), req.PlaylistID)
	}
	return rt.youtube.ImportTracks(r.Context(), req.URIs)
}

// writeYouTubeErr maps catalog errors: no account → 409 (connect first), nothing
// parseable in the paste → 400, daily quota spent → 429 (try again tomorrow),
// gateway/oauth off → 503, otherwise 500.
func writeYouTubeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, youtube.ErrNoLinks):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, youtube.ErrOAuthUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case errors.Is(err, youtube.ErrNoAccount):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
	case errors.Is(err, youtube.ErrCatalogUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}
