package playback

import (
	"testing"
	"time"

	"autodj/domain/shared"
)

func local(id string) Playable {
	return Playable{TrackID: id, Source: shared.SourceLocal, Title: id, URI: "/" + id, Duration: time.Minute}
}
func spotify(id string) Playable {
	return Playable{TrackID: id, Source: shared.SourceSpotify, Title: id, URI: "spotify:" + id, Duration: time.Minute}
}

func TestMixable(t *testing.T) {
	if !local("a").Mixable() {
		t.Fatal("local mixable")
	}
	if spotify("b").Mixable() {
		t.Fatal("spotify not mixable")
	}
}

// The schedule welcomes Spotify with a non-mix transition (hard_cut/fade_tts):
// it plays out-of-band via the browser SDK, then swaps to the next item. Only
// crossfade (a PCM overlap) is illegal for a DRM source.
func TestQueueAcceptsSpotifyHardCut(t *testing.T) {
	q := NewPlayQueue()
	if err := q.Enqueue(spotify("x"), shared.TransitionHardCut); err != nil {
		t.Fatalf("queue must accept spotify hard_cut, got %v", err)
	}
	if err := q.Enqueue(spotify("y"), shared.TransitionFadeTTS); err != nil {
		t.Fatalf("queue must accept spotify fade_tts, got %v", err)
	}
	if err := q.Enqueue(local("z"), shared.TransitionCrossfade); err != nil {
		t.Fatalf("queue accepts local crossfade, got %v", err)
	}
	if q.Len() != 3 {
		t.Fatalf("len = %d, want 3", q.Len())
	}
	// crossfade into a spotify item stays illegal (no PCM overlap possible)
	if err := q.Enqueue(spotify("bad"), shared.TransitionCrossfade); err != ErrBadTransition {
		t.Fatalf("crossfade into spotify must fail, got %v", err)
	}
}

func TestEnqueueTransitionValidation(t *testing.T) {
	q := NewPlayQueue()
	if err := q.Enqueue(local("a"), shared.Transition("nonsense")); err != ErrBadTransition {
		t.Fatalf("bad transition must fail, got %v", err)
	}
}

func TestInsertAtClampsAndOrders(t *testing.T) {
	q := NewPlayQueue()
	_ = q.Enqueue(local("a"), shared.TransitionCrossfade)
	_ = q.Enqueue(local("b"), shared.TransitionCrossfade)

	// i<=0 pushes to front (plays next)
	if err := q.InsertAt(0, local("front"), shared.TransitionCrossfade); err != nil {
		t.Fatalf("insert front: %v", err)
	}
	// i>=Len appends to back
	if err := q.InsertAt(999, local("back"), shared.TransitionCrossfade); err != nil {
		t.Fatalf("insert back: %v", err)
	}
	// middle
	if err := q.InsertAt(2, local("mid"), shared.TransitionCrossfade); err != nil {
		t.Fatalf("insert mid: %v", err)
	}
	// negative clamps to front
	if err := q.InsertAt(-5, local("neg"), shared.TransitionCrossfade); err != nil {
		t.Fatalf("insert neg: %v", err)
	}

	want := []string{"neg", "front", "a", "mid", "b", "back"}
	got := q.Items()
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].Playable.TrackID != id {
			t.Fatalf("pos %d = %q want %q", i, got[i].Playable.TrackID, id)
		}
	}
}

func TestInsertAtAcceptsSpotify(t *testing.T) {
	q := NewPlayQueue()
	if err := q.InsertAt(0, spotify("s"), shared.TransitionFadeTTS); err != nil {
		t.Fatalf("insert spotify fade_tts = %v, want nil", err)
	}
	if err := q.InsertAt(0, spotify("h"), shared.TransitionHardCut); err != nil {
		t.Fatalf("insert spotify hard_cut = %v, want nil", err)
	}
}

func TestInsertAtRejectsBadTransition(t *testing.T) {
	q := NewPlayQueue()
	if err := q.InsertAt(0, spotify("s"), shared.TransitionCrossfade); err != ErrBadTransition {
		t.Fatalf("spotify crossfade = %v, want ErrBadTransition", err)
	}
}

func TestPeekNextClear(t *testing.T) {
	q := NewPlayQueue()
	if _, err := q.Peek(); err != ErrEmptyQueue {
		t.Fatalf("peek empty, got %v", err)
	}
	if _, err := q.Next(); err != ErrEmptyQueue {
		t.Fatalf("next empty, got %v", err)
	}
	_ = q.Enqueue(local("a"), shared.TransitionCrossfade)
	_ = q.Enqueue(local("b"), shared.TransitionHardCut)
	if it, _ := q.Peek(); it.Playable.TrackID != "a" {
		t.Fatal("peek should be a")
	}
	if q.Len() != 2 {
		t.Fatal("peek must not consume")
	}
	it, _ := q.Next()
	if it.Playable.TrackID != "a" || it.Transition != shared.TransitionCrossfade {
		t.Fatal("next should pop a/crossfade")
	}
	if q.Len() != 1 {
		t.Fatal("next consumes")
	}
	items := q.Items()
	if len(items) != 1 || items[0].Playable.TrackID != "b" {
		t.Fatal("items snapshot wrong")
	}
	q.Clear()
	if q.Len() != 0 {
		t.Fatal("clear empties")
	}
}

func TestRemoveAt(t *testing.T) {
	q := NewPlayQueue()
	// out-of-range on empty
	if _, err := q.RemoveAt(0); err != ErrEmptyQueue {
		t.Fatalf("remove empty = %v, want ErrEmptyQueue", err)
	}
	_ = q.Enqueue(local("a"), shared.TransitionCrossfade)
	_ = q.Enqueue(local("b"), shared.TransitionCrossfade)
	_ = q.Enqueue(local("c"), shared.TransitionCrossfade)
	// negative + past-end are no-op errors
	if _, err := q.RemoveAt(-1); err != ErrEmptyQueue {
		t.Fatalf("remove -1 = %v, want ErrEmptyQueue", err)
	}
	if _, err := q.RemoveAt(3); err != ErrEmptyQueue {
		t.Fatalf("remove 3 = %v, want ErrEmptyQueue", err)
	}
	// remove the middle → a,c remain
	it, err := q.RemoveAt(1)
	if err != nil || it.Playable.TrackID != "b" {
		t.Fatalf("remove 1 = %v,%v want b,nil", it.Playable.TrackID, err)
	}
	items := q.Items()
	if len(items) != 2 || items[0].Playable.TrackID != "a" || items[1].Playable.TrackID != "c" {
		t.Fatalf("after remove got %v, want [a c]", items)
	}
}

func ids(items []QueueItem) string {
	s := ""
	for _, it := range items {
		s += it.Playable.TrackID
	}
	return s
}

func TestMoveAt(t *testing.T) {
	q := NewPlayQueue()
	// out-of-range on empty → ErrEmptyQueue
	if _, err := q.MoveAt(0, 1); err != ErrEmptyQueue {
		t.Fatalf("move empty = %v, want ErrEmptyQueue", err)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		_ = q.Enqueue(local(id), shared.TransitionCrossfade)
	}
	// move down: b (idx 1) → idx 2 ⇒ a c b d
	it, err := q.MoveAt(1, 2)
	if err != nil || it.Playable.TrackID != "b" {
		t.Fatalf("move 1→2 = %v,%v want b,nil", it.Playable.TrackID, err)
	}
	if got := ids(q.Items()); got != "acbd" {
		t.Fatalf("after move 1→2 = %s, want acbd", got)
	}
	// move up: b (now idx 2) → idx 0 ⇒ b a c d
	if _, err := q.MoveAt(2, 0); err != nil {
		t.Fatalf("move 2→0 err = %v", err)
	}
	if got := ids(q.Items()); got != "bacd" {
		t.Fatalf("after move 2→0 = %s, want bacd", got)
	}
	// from==to is a no-op, returns the item there
	it, err = q.MoveAt(1, 1)
	if err != nil || it.Playable.TrackID != "a" {
		t.Fatalf("move 1→1 = %v,%v want a,nil", it.Playable.TrackID, err)
	}
	if got := ids(q.Items()); got != "bacd" {
		t.Fatalf("no-op move changed order = %s, want bacd", got)
	}
	// indices clamp into range: from>=len clamps to last (d), to<0 clamps to 0
	if _, err := q.MoveAt(99, -5); err != nil {
		t.Fatalf("move 99→-5 err = %v", err)
	}
	if got := ids(q.Items()); got != "dbac" {
		t.Fatalf("after clamp move = %s, want dbac", got)
	}
}
