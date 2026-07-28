package stubengine

import (
	"context"
	"testing"
	"time"

	"autodj/domain/playback"
	"autodj/domain/shared"
)

func item(id string, src shared.SourceKind) playback.QueueItem {
	return playback.QueueItem{
		Playable:   playback.Playable{TrackID: id, Source: src, Title: id, Duration: time.Minute},
		Transition: shared.TransitionCrossfade,
	}
}

func TestStubPushSkipNowPlaying(t *testing.T) {
	ctx := context.Background()
	e := New()
	if !e.Healthy(ctx) {
		t.Fatal("stub always healthy")
	}
	np, _ := e.NowPlaying(ctx)
	if np.TrackID != "" {
		t.Fatal("empty now-playing initially")
	}
	// first push becomes now-playing
	if err := e.Push(ctx, item("a", shared.SourceLocal)); err != nil {
		t.Fatal(err)
	}
	np, _ = e.NowPlaying(ctx)
	if np.TrackID != "a" {
		t.Fatal("first push should play")
	}
	// second push queues
	_ = e.Push(ctx, item("b", shared.SourceLocal))
	_ = e.Skip(ctx)
	np, _ = e.NowPlaying(ctx)
	if np.TrackID != "b" {
		t.Fatal("skip should advance to b")
	}
	// skip past end → empty
	_ = e.Skip(ctx)
	np, _ = e.NowPlaying(ctx)
	if np.TrackID != "" {
		t.Fatal("skip past end clears now-playing")
	}
}

func TestStubRemoveAt(t *testing.T) {
	ctx := context.Background()
	e := New()
	_ = e.Push(ctx, item("a", shared.SourceLocal)) // becomes cur
	_ = e.Push(ctx, item("b", shared.SourceLocal)) // pending[0]
	_ = e.Push(ctx, item("c", shared.SourceLocal)) // pending[1]
	// out-of-range = no-op, no error
	if err := e.RemoveAt(ctx, -1); err != nil {
		t.Fatalf("remove -1 = %v", err)
	}
	if err := e.RemoveAt(ctx, 5); err != nil {
		t.Fatalf("remove 5 = %v", err)
	}
	// remove pending[0] (b) → skip should now land on c
	if err := e.RemoveAt(ctx, 0); err != nil {
		t.Fatal(err)
	}
	_ = e.Skip(ctx)
	np, _ := e.NowPlaying(ctx)
	if np.TrackID != "c" {
		t.Fatalf("after removing b, skip → %q, want c", np.TrackID)
	}
}

func TestStubMoveAt(t *testing.T) {
	ctx := context.Background()
	e := New()
	_ = e.Push(ctx, item("a", shared.SourceLocal)) // becomes cur
	_ = e.Push(ctx, item("b", shared.SourceLocal)) // pending[0]
	_ = e.Push(ctx, item("c", shared.SourceLocal)) // pending[1]
	_ = e.Push(ctx, item("d", shared.SourceLocal)) // pending[2]
	// out-of-range / equal = no-op, no error
	for _, mv := range [][2]int{{-1, 0}, {0, 5}, {1, 1}} {
		if err := e.MoveAt(ctx, mv[0], mv[1]); err != nil {
			t.Fatalf("move %v = %v", mv, err)
		}
	}
	// pending is b,c,d; move pending[2] (d) → pending[0] ⇒ d,b,c
	if err := e.MoveAt(ctx, 2, 0); err != nil {
		t.Fatal(err)
	}
	// skip lands on d, then b, then c
	for _, want := range []string{"d", "b", "c"} {
		_ = e.Skip(ctx)
		np, _ := e.NowPlaying(ctx)
		if np.TrackID != want {
			t.Fatalf("skip → %q, want %q", np.TrackID, want)
		}
	}
}

func TestStubAcceptsSpotify(t *testing.T) {
	ctx := context.Background()
	e := New()
	// stub is the browser-driven playout mirror: a non-mixable Spotify item is
	// scheduled and becomes now-playing (browser plays it via the SDK).
	if err := e.Push(ctx, item("x", shared.SourceSpotify)); err != nil {
		t.Fatalf("stub must accept spotify, got %v", err)
	}
	np, _ := e.NowPlaying(ctx)
	if np.TrackID != "x" || np.Source != shared.SourceSpotify {
		t.Fatalf("spotify should be now-playing, got %+v", np)
	}
}
