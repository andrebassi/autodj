package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"autodj/domain/library"
	"autodj/domain/spotify"
)

// getSpotifyStatus reports whether an account is linked and Premium.
// GET /api/v1/spotify/status → {connected, premium}.
func (rt *Router) getSpotifyStatus(w http.ResponseWriter, r *http.Request) {
	st, err := rt.spotify.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// postSpotifyRevalidate força a renovação do token e devolve o estado REAL da
// conta. Chamado pelo front quando o playout trava (device conectado, faixa
// carregada, comandos aceitos e áudio nenhum) — é o que distingue "o Spotify
// recusou o token" de "é outra coisa". Falha na renovação → 409 com o motivo,
// que a UI traduz em "reconecte a conta".
// POST /api/v1/spotify/revalidate → {connected, premium}
func (rt *Router) postSpotifyRevalidate(w http.ResponseWriter, r *http.Request) {
	st, err := rt.spotify.Revalidate(r.Context())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// getSpotifyDiagnose roda a cadeia de verificação e devolve o passo a passo. Só
// leitura — nunca inicia reprodução. GET /api/v1/spotify/diagnose
func (rt *Router) getSpotifyDiagnose(w http.ResponseWriter, r *http.Request) {
	d, err := rt.spotify.Diagnose(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// 200 mesmo quando o diagnóstico acusa problema: a requisição funcionou; quem
	// falhou foi o Spotify, e isso está descrito no corpo.
	writeJSON(w, http.StatusOK, d)
}

// postSpotifyConnect starts a PKCE OAuth flow and returns the authorize URL the
// browser should navigate to. POST /api/v1/spotify/connect → {authURL}.
func (rt *Router) postSpotifyConnect(w http.ResponseWriter, r *http.Request) {
	authURL, err := rt.spotify.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authURL": authURL})
}

// getSpotifyCallback finishes the OAuth round-trip: validates state, exchanges
// the code for tokens and persists the account, then redirects (302) back to the
// UI. A state mismatch (ErrStateMismatch) or expired pending auth (ErrAuthExpired)
// → 400. GET /api/v1/spotify/callback?code&state.
func (rt *Router) getSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code, state := q.Get("code"), q.Get("state")
	if err := rt.spotify.Complete(r.Context(), state, code); err != nil {
		writeErr(w, err)
		return
	}
	http.Redirect(w, r, rt.spotifyReturn, http.StatusFound)
}

// deleteSpotifyDisconnect unlinks the account. Idempotent → 204.
// POST /api/v1/spotify/disconnect.
func (rt *Router) postSpotifyDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := rt.spotify.Disconnect(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getSpotifyToken returns the current access token for the Web Playback SDK
// (Story 6.3). The browser SDK's getOAuthToken callback fetches it here; the
// backend refreshes transparently. GET /api/v1/spotify/token → {accessToken}.
// No linked account → 409 (connect first). The token is short-lived and only
// authorizes Spotify's own SDK playback — no PCM ever crosses to the local bus.
func (rt *Router) getSpotifyToken(w http.ResponseWriter, r *http.Request) {
	tok, err := rt.spotify.AccessToken(r.Context())
	if err != nil {
		writeSpotifyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"accessToken": tok})
}

// getSpotifySearch queries the Spotify catalog. GET /api/v1/spotify/search?q&type
// (type = track|playlist, default track) → [{uri,title,artist,album,durationMs,kind}].
func (rt *Router) getSpotifySearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, kind := q.Get("q"), q.Get("type")
	if kind == "" {
		kind = spotify.KindTrack
	}
	items, err := rt.spotify.Search(r.Context(), query, kind)
	if err != nil {
		writeSpotifyErr(w, err)
		return
	}
	if items == nil {
		items = []spotify.CatalogItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// spotifyImportRequest is the /import body: either explicit track URIs or a
// playlistID whose tracks are imported wholesale.
type spotifyImportRequest struct {
	URIs       []string `json:"uris"`
	PlaylistID string   `json:"playlistID"`
}

// postSpotifyImport imports tracks (or a whole playlist) into the catalog as
// non-mixable Spotify tracks. POST /api/v1/spotify/import → created tracks.
func (rt *Router) postSpotifyImport(w http.ResponseWriter, r *http.Request) {
	var req spotifyImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.PlaylistID == "" && len(req.URIs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "uris or playlistID required"})
		return
	}
	tracks, err := rt.importSpotify(r, req)
	if err != nil {
		writeSpotifyErr(w, err)
		return
	}
	out := make([]trackDTO, 0, len(tracks))
	for i := range tracks {
		out = append(out, toTrackDTO(&tracks[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) importSpotify(r *http.Request, req spotifyImportRequest) ([]library.Track, error) {
	if req.PlaylistID != "" {
		return rt.spotify.ImportPlaylist(r.Context(), req.PlaylistID)
	}
	return rt.spotify.ImportTracks(r.Context(), req.URIs)
}

// writeSpotifyErr maps catalog errors: no account → 409 (connect first),
// gateway off → 503, otherwise 500.
func writeSpotifyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, spotify.ErrNoAccount):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, spotify.ErrCatalogUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}
