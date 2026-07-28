// Package publicsite builds the station's read-only public snapshot — the
// "tocando agora / a tocar / já tocou" view plus the daily fact sheet. Like the
// reports package it owns no state: a Snapshot is a pure projection over the
// current now-playing/queue (domain/playback) and the recent history
// (domain/playlog), so regenerating always agrees with the sources.
//
// The public site shows what actually played, including studio Spotify — unlike
// the ECAD/proof reports it does not filter on Broadcasted. It only reads the
// source kind so the UI can label each item; it never alters an entry.
package publicsite

import (
	"time"

	"autodj/domain/dailyinfo"
	"autodj/domain/playback"
	"autodj/domain/playlog"
	"autodj/domain/shared"
)

// NowPlaying is the track on the air right now. OnAir is false when nothing is
// playing (empty TrackID); Mixable reflects the two-engine capability matrix
// (DRM Spotify is not mixable) so the UI can badge it.
type NowPlaying struct {
	TrackID string
	Title   string
	Source  string
	OnAir   bool
	Mixable bool
}

// UpcomingItem is one queued track that has not played yet.
type UpcomingItem struct {
	TrackID string
	Title   string
	Source  string
}

// PlayedItem is one track from the recent history, newest first in the snapshot.
type PlayedItem struct {
	TrackID    string
	Title      string
	Artist     string
	Source     string
	PlayedAt   time.Time
	DurationMs int64
}

// Snapshot is the whole public view at GeneratedAt: what is on the air, what is
// coming up, what recently played, and the daily fact sheet.
type Snapshot struct {
	GeneratedAt time.Time
	NowPlaying  NowPlaying
	Upcoming    []UpcomingItem
	Recent      []PlayedItem
	Daily       dailyinfo.Info
}

// BuildSnapshot projects the live playout and recent history into a public
// snapshot. now becomes NowPlaying (OnAir when it has a TrackID); upcoming is the
// queue mapped and capped at upcomingLimit; recent is the history reversed to
// newest-first and capped at recentLimit. A limit <= 0 means no cap. Entries are
// never mutated; daily is carried through as-is.
func BuildSnapshot(
	now playback.Playable,
	upcoming []playback.QueueItem,
	recent []playlog.Entry,
	daily dailyinfo.Info,
	upcomingLimit, recentLimit int,
	generatedAt time.Time,
) Snapshot {
	return Snapshot{
		GeneratedAt: generatedAt,
		NowPlaying:  mapNow(now),
		Upcoming:    mapUpcoming(upcoming, upcomingLimit),
		Recent:      mapRecent(recent, recentLimit),
		Daily:       daily,
	}
}

func mapNow(now playback.Playable) NowPlaying {
	if now.TrackID == "" {
		return NowPlaying{}
	}
	return NowPlaying{
		TrackID: now.TrackID,
		Title:   now.Title,
		Source:  string(now.Source),
		OnAir:   true,
		Mixable: shared.Capabilities(now.Source).Mixable,
	}
}

func mapUpcoming(items []playback.QueueItem, limit int) []UpcomingItem {
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]UpcomingItem, 0, len(items))
	for _, it := range items {
		out = append(out, UpcomingItem{
			TrackID: it.Playable.TrackID,
			Title:   it.Playable.Title,
			Source:  string(it.Playable.Source),
		})
	}
	return out
}

// mapRecent reverses the oldest-first history into newest-first and caps it at
// limit (limit <= 0 keeps all).
func mapRecent(entries []playlog.Entry, limit int) []PlayedItem {
	out := make([]PlayedItem, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		e := entries[i]
		out = append(out, PlayedItem{
			TrackID:    e.TrackID,
			Title:      e.Title,
			Artist:     e.Artist,
			Source:     string(e.Source),
			PlayedAt:   e.PlayedAt,
			DurationMs: e.DurationMs,
		})
	}
	return out
}
