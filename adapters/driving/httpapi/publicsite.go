package httpapi

import (
	"net/http"
	"time"

	"autodj/domain/dailyinfo"
	"autodj/domain/publicsite"
)

// pubNowDTO is the wire shape of the track currently on the air.
type pubNowDTO struct {
	TrackID string `json:"track_id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	OnAir   bool   `json:"on_air"`
	Mixable bool   `json:"mixable"`
}

// pubUpcomingDTO is the wire shape of one queued track.
type pubUpcomingDTO struct {
	TrackID string `json:"track_id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
}

// pubPlayedDTO is the wire shape of one recently played track (newest first).
type pubPlayedDTO struct {
	TrackID    string `json:"track_id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Source     string `json:"source"`
	PlayedAt   string `json:"played_at"`
	DurationMs int64  `json:"duration_ms"`
}

// pubDailyDTO is the wire shape of the daily fact sheet.
type pubDailyDTO struct {
	Date      string   `json:"date"`
	Weekday   string   `json:"weekday"`
	MoonPhase string   `json:"moon_phase"`
	MoonIllum int      `json:"moon_illum"`
	Zodiac    string   `json:"zodiac"`
	Season    string   `json:"season"`
	DayOfYear int      `json:"day_of_year"`
	Holidays  []string `json:"holidays"`
}

// pubSiteDTO is the wire shape of the whole public snapshot.
type pubSiteDTO struct {
	GeneratedAt string            `json:"generated_at"`
	NowPlaying  pubNowDTO     `json:"now_playing"`
	Upcoming    []pubUpcomingDTO `json:"upcoming"`
	Recent      []pubPlayedDTO   `json:"recent"`
	Daily       pubDailyDTO      `json:"daily"`
}

func toPubDailyDTO(d dailyinfo.Info) pubDailyDTO {
	hs := d.Holidays
	if hs == nil {
		hs = []string{}
	}
	return pubDailyDTO{
		Date:      d.Date.UTC().Format(time.RFC3339Nano),
		Weekday:   d.Weekday,
		MoonPhase: d.MoonPhase,
		MoonIllum: d.MoonIllum,
		Zodiac:    d.Zodiac,
		Season:    d.Season,
		DayOfYear: d.DayOfYear,
		Holidays:  hs,
	}
}

func toPubSiteDTO(s publicsite.Snapshot) pubSiteDTO {
	up := make([]pubUpcomingDTO, 0, len(s.Upcoming))
	for _, u := range s.Upcoming {
		up = append(up, pubUpcomingDTO{TrackID: u.TrackID, Title: u.Title, Source: u.Source})
	}
	rec := make([]pubPlayedDTO, 0, len(s.Recent))
	for _, p := range s.Recent {
		rec = append(rec, pubPlayedDTO{
			TrackID:    p.TrackID,
			Title:      p.Title,
			Artist:     p.Artist,
			Source:     p.Source,
			PlayedAt:   p.PlayedAt.UTC().Format(time.RFC3339Nano),
			DurationMs: p.DurationMs,
		})
	}
	return pubSiteDTO{
		GeneratedAt: s.GeneratedAt.UTC().Format(time.RFC3339Nano),
		NowPlaying: pubNowDTO{
			TrackID: s.NowPlaying.TrackID,
			Title:   s.NowPlaying.Title,
			Source:  s.NowPlaying.Source,
			OnAir:   s.NowPlaying.OnAir,
			Mixable: s.NowPlaying.Mixable,
		},
		Upcoming: up,
		Recent:   rec,
		Daily:    toPubDailyDTO(s.Daily),
	}
}

// getPublicSite returns the read-only public snapshot: now-playing, the upcoming
// queue, the recent history (newest first) and the daily fact sheet. Public (no
// auth). A repo/playback error → 500 via writeErr. GET /public/site.
func (rt *Router) getPublicSite(w http.ResponseWriter, r *http.Request) {
	snap, err := rt.public.Snapshot(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPubSiteDTO(snap))
}
