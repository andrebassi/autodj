package httpapi

import (
	"net/http"
	"time"

	"autodj/domain/reports"
)

// ecadLineDTO is the wire shape of one aggregated work in the ECAD report.
type ecadLineDTO struct {
	TrackID         string `json:"track_id"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Plays           int    `json:"plays"`
	TotalDurationMs int64  `json:"total_duration_ms"`
}

// ecadReportDTO is the wire shape of an ECAD execution report over [from, to).
type ecadReportDTO struct {
	From            string        `json:"from"`
	To              string        `json:"to"`
	GeneratedAt     string        `json:"generated_at"`
	TotalPlays      int           `json:"total_plays"`
	TotalDurationMs int64         `json:"total_duration_ms"`
	Lines           []ecadLineDTO `json:"lines"`
}

func toECADReportDTO(rep reports.ECADReport) ecadReportDTO {
	lines := make([]ecadLineDTO, 0, len(rep.Lines))
	for _, l := range rep.Lines {
		lines = append(lines, ecadLineDTO{
			TrackID:         l.TrackID,
			Title:           l.Title,
			Artist:          l.Artist,
			Plays:           l.Plays,
			TotalDurationMs: l.TotalDurationMs,
		})
	}
	return ecadReportDTO{
		From:            rep.From.UTC().Format(time.RFC3339Nano),
		To:              rep.To.UTC().Format(time.RFC3339Nano),
		GeneratedAt:     rep.GeneratedAt.UTC().Format(time.RFC3339Nano),
		TotalPlays:      rep.TotalPlays,
		TotalDurationMs: rep.TotalDurationMs,
		Lines:           lines,
	}
}

// getECADReport returns the ECAD execution report over [from, to): the
// ECAD-relevant plays aggregated by track. from and to are RFC3339 query params;
// both are required (missing/invalid → 400). An empty/inverted window → 422 (via
// writeErr / inbound.ErrBadRange). GET /reports/ecad?from=<RFC3339>&to=<RFC3339>.
func (rt *Router) getECADReport(w http.ResponseWriter, r *http.Request) {
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
	rep, err := rt.reports.ECAD(r.Context(), from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toECADReportDTO(rep))
}

// proofLineDTO is the wire shape of one aired play in the broadcast proof.
type proofLineDTO struct {
	EntryID    string `json:"entry_id"`
	PlayedAt   string `json:"played_at"`
	TrackID    string `json:"track_id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Source     string `json:"source"`
	DurationMs int64  `json:"duration_ms"`
}

// broadcastProofDTO is the wire shape of a chronological broadcast proof over
// [from, to): one line per aired play, in play order.
type broadcastProofDTO struct {
	From            string         `json:"from"`
	To              string         `json:"to"`
	GeneratedAt     string         `json:"generated_at"`
	TotalEntries    int            `json:"total_entries"`
	TotalDurationMs int64          `json:"total_duration_ms"`
	Lines           []proofLineDTO `json:"lines"`
}

func toBroadcastProofDTO(p reports.BroadcastProof) broadcastProofDTO {
	lines := make([]proofLineDTO, 0, len(p.Lines))
	for _, l := range p.Lines {
		lines = append(lines, proofLineDTO{
			EntryID:    l.EntryID,
			PlayedAt:   l.PlayedAt.UTC().Format(time.RFC3339Nano),
			TrackID:    l.TrackID,
			Title:      l.Title,
			Artist:     l.Artist,
			Source:     string(l.Source),
			DurationMs: l.DurationMs,
		})
	}
	return broadcastProofDTO{
		From:            p.From.UTC().Format(time.RFC3339Nano),
		To:              p.To.UTC().Format(time.RFC3339Nano),
		GeneratedAt:     p.GeneratedAt.UTC().Format(time.RFC3339Nano),
		TotalEntries:    p.TotalEntries,
		TotalDurationMs: p.TotalDurationMs,
		Lines:           lines,
	}
}

// getBroadcastProof returns the chronological broadcast proof over [from, to):
// every aired play (Broadcasted=true), one line per play, in play order. from and
// to are RFC3339 query params; both required (missing/invalid → 400). An
// empty/inverted window → 422 (via writeErr / inbound.ErrBadRange).
// GET /reports/broadcast?from=<RFC3339>&to=<RFC3339>.
func (rt *Router) getBroadcastProof(w http.ResponseWriter, r *http.Request) {
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
	proof, err := rt.reports.BroadcastProof(r.Context(), from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBroadcastProofDTO(proof))
}
