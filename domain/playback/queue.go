// Package playback holds the transient play model: Playable (one thing to
// play, carrying its SourceKind capabilities) and PlayQueue (ordered items the
// engine consumes). The queue enforces the two-engine transition rules.
package playback

import (
	"errors"
	"time"

	"autodj/domain/shared"
)

var (
	// ErrEmptyQueue is returned when advancing an empty queue.
	ErrEmptyQueue = errors.New("queue: empty")
	// ErrBadTransition is returned when a transition is illegal for the target
	// source (e.g. crossfade into a Spotify item).
	ErrBadTransition = errors.New("queue: transition not allowed for source")
)

// Playable is one unit the engine can play. It references a catalog TrackID and
// carries the SourceKind so the queue/engine can consult the capability matrix
// without loading the full Track.
type Playable struct {
	TrackID  string
	Source   shared.SourceKind
	Title    string
	Artist   string // performer, snapshotted from the catalog Track for display
	URI      string // file path or spotify uri — resolved by the engine adapter
	Duration time.Duration
	// ArtworkURL is the album-cover URL snapshotted from the catalog Track (e.g. a
	// Spotify CDN image). Carried on the wire so the UI renders the cover directly
	// from the public URL — no dependency on the session-scoped /artwork endpoint,
	// which 404s for tracks not held by the caller's tenant. Empty for local files
	// (UI falls back to the artwork endpoint, which extracts the embedded picture).
	ArtworkURL string
	// Origin is the enqueue-time provenance label — the playlist/grid/event this
	// item came from (e.g. a Spotify playlist name, "Grade", "Acervo"). Empty
	// for a bare/manual enqueue. Purely informational (shown in the queue column);
	// it never affects transition/mix rules.
	Origin string

	// Category is the catalog rotation bucket ("music", "jingle", …) snapshotted
	// from the Track. The playout needs it to honour per-type settings: trimming
	// músicas and trimming vinhetas are two separate switches for the operator.
	Category string

	// CueInMs / CueOutMs / VocalMs / LoudnessLUFS are the analysis hints the
	// browser playout consumes: where the audio really starts and ends, where the
	// voice comes in, and how loud the master is. They travel ON THE PLAYABLE (not
	// via a separate library fetch) for one concrete reason: the snapshot is
	// already mirrored to localStorage, so the hints keep working with the server
	// off — a fetch in the deck path would not.
	//
	// Zero means "unknown / nothing to do" in all four: no trim, no vocal mark, no
	// gain correction. Non-PCM sources (Spotify/YouTube) are always zero here —
	// there is no PCM on this side to have measured.
	// BPM medido na análise; 0 = não medido. Viaja no Playable porque é ESTÁTICO:
	// uma vez medido não muda, então o retrato do enfileiramento continua verdade.
	// (Diferente do preparo do SEM-VOZ, que termina minutos depois e por isso NÃO
	// pode viajar aqui — ver o endpoint de estados.)
	BPM float64

	CueInMs      int64
	CueOutMs     int64
	VocalMs      int64
	LoudnessLUFS float64
}

// Mixable reports whether this Playable may enter the local playout/mix bus.
func (p Playable) Mixable() bool {
	return shared.Capabilities(p.Source).Mixable
}

// QueueItem is a Playable plus the transition used to enter it.
type QueueItem struct {
	Playable   Playable
	Transition shared.Transition
}

// PlayQueue is an ordered list of items the engine pops from the front. The
// queue is the *schedule* (what plays next, in order) — not the mix bus. A
// non-mixable source (Spotify/DRM) is welcome in the schedule: it plays
// out-of-band via the browser SDK and swaps by hard_cut. The mix-bus invariant
// (spotify never enters the local PCM stream) is enforced downstream by the
// Liquidsoap engine's Push, not here. The only queue-level rule is that the
// entry transition must be legal for the source (no crossfade into Spotify).
type PlayQueue struct {
	items []QueueItem
}

// NewPlayQueue creates an empty queue.
func NewPlayQueue() *PlayQueue {
	return &PlayQueue{}
}

// Len returns the number of queued items.
func (q *PlayQueue) Len() int { return len(q.items) }

// Items returns a copy of the current items (safe to read).
func (q *PlayQueue) Items() []QueueItem {
	out := make([]QueueItem, len(q.items))
	copy(out, q.items)
	return out
}

// Enqueue appends p with transition t, validating the transition against the
// target source (crossfade into a non-PCM source is rejected).
func (q *PlayQueue) Enqueue(p Playable, t shared.Transition) error {
	if !shared.TransitionAllowed(p.Source, t) {
		return ErrBadTransition
	}
	q.items = append(q.items, QueueItem{Playable: p, Transition: t})
	return nil
}

// InsertAt inserts p at index i, applying the same validation as Enqueue. The
// index is clamped to [0, Len]: i<=0 pushes to the front (plays next), i>=Len
// appends to the back. Returns the same errors as Enqueue.
func (q *PlayQueue) InsertAt(i int, p Playable, t shared.Transition) error {
	if !shared.TransitionAllowed(p.Source, t) {
		return ErrBadTransition
	}
	if i < 0 {
		i = 0
	}
	if i > len(q.items) {
		i = len(q.items)
	}
	it := QueueItem{Playable: p, Transition: t}
	q.items = append(q.items, QueueItem{})
	copy(q.items[i+1:], q.items[i:])
	q.items[i] = it
	return nil
}

// Peek returns the front item without removing it.
func (q *PlayQueue) Peek() (QueueItem, error) {
	if len(q.items) == 0 {
		return QueueItem{}, ErrEmptyQueue
	}
	return q.items[0], nil
}

// Next pops and returns the front item.
func (q *PlayQueue) Next() (QueueItem, error) {
	if len(q.items) == 0 {
		return QueueItem{}, ErrEmptyQueue
	}
	it := q.items[0]
	q.items = q.items[1:]
	return it, nil
}

// RemoveAt drops the item at index i and returns it. Out-of-range i yields
// ErrEmptyQueue (nothing to remove there). Used to pull a queued item the
// operator no longer wants (right-click → remover).
func (q *PlayQueue) RemoveAt(i int) (QueueItem, error) {
	if i < 0 || i >= len(q.items) {
		return QueueItem{}, ErrEmptyQueue
	}
	it := q.items[i]
	q.items = append(q.items[:i], q.items[i+1:]...)
	return it, nil
}

// MoveAt moves the item at index from to index to, shifting the rest, and
// returns the moved item. Both indices are clamped to [0, Len-1]. Reordering
// never changes an item's transition, so no source/transition revalidation is
// needed (the entry rule was already satisfied at enqueue time). Out-of-range
// from on an empty queue yields ErrEmptyQueue. Used by the operator to drag /
// nudge a queued item up or down in the running order.
func (q *PlayQueue) MoveAt(from, to int) (QueueItem, error) {
	n := len(q.items)
	if n == 0 {
		return QueueItem{}, ErrEmptyQueue
	}
	clamp := func(i int) int {
		if i < 0 {
			return 0
		}
		if i >= n {
			return n - 1
		}
		return i
	}
	from, to = clamp(from), clamp(to)
	it := q.items[from]
	if from == to {
		return it, nil
	}
	q.items = append(q.items[:from], q.items[from+1:]...)
	q.items = append(q.items, QueueItem{})
	copy(q.items[to+1:], q.items[to:])
	q.items[to] = it
	return it, nil
}

// Clear empties the queue.
func (q *PlayQueue) Clear() { q.items = nil }
