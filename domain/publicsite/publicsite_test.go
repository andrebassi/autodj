package publicsite

import (
	"testing"
	"time"

	"autodj/domain/dailyinfo"
	"autodj/domain/playback"
	"autodj/domain/playlog"
	"autodj/domain/shared"
)

func local(id string) playback.Playable {
	return playback.Playable{TrackID: id, Source: shared.SourceLocal, Title: "T-" + id, URI: "/" + id}
}

func spotify(id string) playback.Playable {
	return playback.Playable{TrackID: id, Source: shared.SourceSpotify, Title: "T-" + id, URI: "spotify:" + id}
}

func qi(p playback.Playable) playback.QueueItem {
	return playback.QueueItem{Playable: p, Transition: shared.TransitionCrossfade}
}

func entry(id string, at time.Time) playlog.Entry {
	return playlog.Entry{
		TrackID: id, Title: "T-" + id, Artist: "A-" + id,
		Source: shared.SourceLocal, PlayedAt: at, DurationMs: 1000,
	}
}

func TestBuildSnapshotNowPlayingLocal(t *testing.T) {
	gen := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	daily := dailyinfo.Compute(gen)
	snap := BuildSnapshot(local("a"), nil, nil, daily, 20, 20, gen)

	if !snap.NowPlaying.OnAir || snap.NowPlaying.TrackID != "a" {
		t.Fatalf("now playing: %+v", snap.NowPlaying)
	}
	if snap.NowPlaying.Source != "local" || !snap.NowPlaying.Mixable {
		t.Fatalf("local must be mixable: %+v", snap.NowPlaying)
	}
	if !snap.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt: %v", snap.GeneratedAt)
	}
	if snap.Daily.Zodiac != daily.Zodiac {
		t.Fatalf("daily not carried: %+v", snap.Daily)
	}
}

func TestBuildSnapshotSpotifyNotMixable(t *testing.T) {
	snap := BuildSnapshot(spotify("x"), nil, nil, dailyinfo.Info{}, 0, 0, time.Now())
	if !snap.NowPlaying.OnAir || snap.NowPlaying.Source != "spotify" {
		t.Fatalf("now: %+v", snap.NowPlaying)
	}
	if snap.NowPlaying.Mixable {
		t.Fatalf("spotify must not be mixable")
	}
}

func TestBuildSnapshotOffAir(t *testing.T) {
	snap := BuildSnapshot(playback.Playable{}, nil, nil, dailyinfo.Info{}, 0, 0, time.Now())
	if snap.NowPlaying.OnAir || snap.NowPlaying.TrackID != "" {
		t.Fatalf("expected off-air empty now: %+v", snap.NowPlaying)
	}
}

func TestBuildSnapshotUpcomingLimit(t *testing.T) {
	up := []playback.QueueItem{qi(local("a")), qi(local("b")), qi(local("c"))}
	snap := BuildSnapshot(playback.Playable{}, up, nil, dailyinfo.Info{}, 2, 0, time.Now())
	if len(snap.Upcoming) != 2 || snap.Upcoming[0].TrackID != "a" || snap.Upcoming[1].TrackID != "b" {
		t.Fatalf("upcoming cap: %+v", snap.Upcoming)
	}
	if snap.Upcoming[0].Source != "local" || snap.Upcoming[0].Title != "T-a" {
		t.Fatalf("upcoming fields: %+v", snap.Upcoming[0])
	}
	// limit <= 0 keeps everything
	all := BuildSnapshot(playback.Playable{}, up, nil, dailyinfo.Info{}, 0, 0, time.Now())
	if len(all.Upcoming) != 3 {
		t.Fatalf("no-cap upcoming: %d", len(all.Upcoming))
	}
}

func TestBuildSnapshotRecentReversedAndCapped(t *testing.T) {
	base := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	// oldest-first as PlayLogRepo.Since returns
	recent := []playlog.Entry{
		entry("old", base),
		entry("mid", base.Add(time.Hour)),
		entry("new", base.Add(2*time.Hour)),
	}
	snap := BuildSnapshot(playback.Playable{}, nil, recent, dailyinfo.Info{}, 0, 2, time.Now())
	if len(snap.Recent) != 2 {
		t.Fatalf("recent cap: %d", len(snap.Recent))
	}
	// newest first
	if snap.Recent[0].TrackID != "new" || snap.Recent[1].TrackID != "mid" {
		t.Fatalf("recent order: %+v", snap.Recent)
	}
	if snap.Recent[0].Artist != "A-new" || snap.Recent[0].Source != "local" || snap.Recent[0].DurationMs != 1000 {
		t.Fatalf("recent fields: %+v", snap.Recent[0])
	}
	// limit <= 0 keeps all, still newest-first
	all := BuildSnapshot(playback.Playable{}, nil, recent, dailyinfo.Info{}, 0, 0, time.Now())
	if len(all.Recent) != 3 || all.Recent[0].TrackID != "new" || all.Recent[2].TrackID != "old" {
		t.Fatalf("no-cap recent: %+v", all.Recent)
	}
}

func TestBuildSnapshotEmpty(t *testing.T) {
	snap := BuildSnapshot(playback.Playable{}, nil, nil, dailyinfo.Info{}, 20, 20, time.Now())
	if len(snap.Upcoming) != 0 || len(snap.Recent) != 0 || snap.NowPlaying.OnAir {
		t.Fatalf("empty snapshot: %+v", snap)
	}
}
