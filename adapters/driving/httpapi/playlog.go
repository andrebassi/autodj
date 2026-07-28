package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"autodj/domain/playlog"
)

// playLogEntryDTO is the wire shape of one append-only history row. Broadcasted
// and ecad_relevant are snapshotted from the two-engine capability matrix at play
// time (Spotify → both false), so the reports/UI filter on them.
type playLogEntryDTO struct {
	ID           string `json:"id"`
	TrackID      string `json:"track_id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Source       string `json:"source"`
	PlayedAt     string `json:"played_at"`
	DurationMs   int64  `json:"duration_ms"`
	Broadcasted  bool   `json:"broadcasted"`
	ECADRelevant bool   `json:"ecad_relevant"`
}

// topItemDTO is one row of the "most played" ranking.
type topItemDTO struct {
	TrackID string `json:"track_id"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Count   int    `json:"count"`
}

func toPlayLogEntryDTO(e playlog.Entry) playLogEntryDTO {
	return playLogEntryDTO{
		ID:           e.ID,
		TrackID:      e.TrackID,
		Title:        e.Title,
		Artist:       e.Artist,
		Source:       string(e.Source),
		PlayedAt:     e.PlayedAt.UTC().Format(time.RFC3339Nano),
		DurationMs:   e.DurationMs,
		Broadcasted:  e.Broadcasted,
		ECADRelevant: e.ECADRelevant,
	}
}

// getPlayLog returns the play history in [from, to), oldest first. from and to
// are RFC3339 query params; both are required (missing/invalid → 400).
// GET /playlog?from=<RFC3339>&to=<RFC3339>.
func (rt *Router) getPlayLog(w http.ResponseWriter, r *http.Request) {
	from, ok := parseRFC3339Param(r, "from")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from (RFC3339) required"})
		return
	}
	to, ok := parseRFC3339Param(r, "to")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to (RFC3339) required"})
		return
	}
	entries, err := rt.playlog.History(r.Context(), from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]playLogEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toPlayLogEntryDTO(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// getPlayLogTop returns the n most played tracks over the named rolling window.
// period is dia|semana|mes (invalid → 422 via writeErr); n defaults to 10 and is
// clamped to [1,100]. GET /playlog/top?period=dia&n=10.
func (rt *Router) getPlayLogTop(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	n := clampTopN(r.URL.Query().Get("n"))
	items, err := rt.playlog.Top(r.Context(), period, n)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]topItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, topItemDTO{TrackID: it.TrackID, Title: it.Title, Artist: it.Artist, Count: it.Count})
	}
	writeJSON(w, http.StatusOK, out)
}

// parseRFC3339Param reads an RFC3339 query param, reporting ok=false when it is
// absent or malformed.
func parseRFC3339Param(r *http.Request, name string) (time.Time, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// clampTopN parses the n query param, defaulting to 10 and clamping to [1,100].
func clampTopN(s string) int {
	const def, min, max = 10, 1, 100
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
