package app

import (
	"context"
	"sync"

	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

// PlaybackService implements inbound.PlaybackService. It owns the in-memory
// playout queue mirror and pushes items to the engine.
type PlaybackService struct {
	repo   outbound.TrackRepo
	engine outbound.PlaybackEngine

	mu    sync.Mutex
	queue *playback.PlayQueue
}

// NewPlaybackService wires playback use cases with a playout queue.
func NewPlaybackService(repo outbound.TrackRepo, engine outbound.PlaybackEngine) *PlaybackService {
	return &PlaybackService{
		repo:   repo,
		engine: engine,
		queue:  playback.NewPlayQueue(),
	}
}

// LoadFromLibrary enqueues every mixable local track, crossfaded. Spotify
// tracks are silently ignored by the queue invariant. Returns count enqueued.
//
// Idempotent: PLAY is a master-start. If the queue is already loaded/playing,
// a second call is a no-op that returns the current length — it must NOT wipe
// and rebuild the running queue (clicking PLAY twice used to Clear() + reload,
// zeroing the fila and stopping playout). Only a truly empty queue gets filled.
func (s *PlaybackService) LoadFromLibrary(ctx context.Context) (int, error) {
	s.mu.Lock()
	if s.queue.Len() > 0 {
		n := s.queue.Len()
		s.mu.Unlock()
		return n, nil
	}
	s.mu.Unlock()
	tracks, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// re-check under the lock: another loader may have filled it meanwhile.
	if s.queue.Len() > 0 {
		return s.queue.Len(), nil
	}
	n := 0
	for _, t := range tracks {
		if !t.Mixable {
			continue
		}
		p := toPlayable(t)
		if err := s.queue.Enqueue(p, shared.TransitionCrossfade); err != nil {
			continue
		}
		if err := s.engine.Push(ctx, playback.QueueItem{Playable: p, Transition: shared.TransitionCrossfade}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Enqueue resolves trackID from the catalog and inserts it into the playout
// queue at pos (pos<0 appends). The entry transition follows the source: PCM
// sources crossfade, Spotify (non-mixable) hard-cuts — it plays out-of-band via
// the browser SDK, then swaps. It mirrors the item to the engine and returns the
// queue/engine errors unchanged. If playNow is set, the current track is skipped
// so the inserted item plays.
func (s *PlaybackService) Enqueue(ctx context.Context, trackID string, pos int, playNow bool, origin string) error {
	t, err := s.repo.Get(ctx, trackID)
	if err != nil {
		return err
	}
	p := toPlayable(t)
	p.Origin = origin
	tr := shared.DefaultTransition(p.Source)
	item := playback.QueueItem{Playable: p, Transition: tr}

	// The mirror keeps the now-playing at index 0, the próxima at 1, the tail at
	// 2+. The engine keeps the now-playing in `cur` and the pending list in
	// `items` (mirror[1:]) — so mirror index k maps to engine pending index k-1.
	// Both must stay in lockstep or the engine plays a different "next" than the
	// UI shows (the bug: a cued/cut item became the mirror's próxima but the
	// engine had appended it to the back, so it played the old next instead).
	s.mu.Lock()
	before := s.queue.Len()
	hadHead := before > 0
	if pos < 0 {
		// pos<0 means "append to the back".
		pos = before
	}
	// Resolve where the item lands in the mirror:
	//  • playNow (cut / "tocar agora") → it becomes the next item and is then
	//    promoted onto the now-playing, dropping the interrupted head. With no
	//    head yet it simply becomes the now-playing.
	//  • pos 0 without playNow ("inserir após atual") must never silently displace
	//    the on-air head → it goes right after it (próxima).
	insertPos := pos
	switch {
	case playNow:
		if hadHead {
			insertPos = 1
		} else {
			insertPos = 0
		}
	case pos == 0 && hadHead:
		insertPos = 1
	}
	if err := s.queue.InsertAt(insertPos, p, tr); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	// Mirror the insert into the engine. The engine only appends, so push then
	// slide the just-pushed item (pending tail) into its mirror position. With no
	// prior head the push itself becomes the now-playing — nothing to slide.
	if err := s.engine.Push(ctx, item); err != nil {
		return err
	}
	if hadHead {
		tail := before - 1      // pending index of the just-pushed item
		target := insertPos - 1 // engine pending index that mirrors insertPos
		if target >= 0 && target != tail {
			if err := s.engine.MoveAt(ctx, tail, target); err != nil {
				return err
			}
		}
	}
	if playNow && hadHead {
		// Promote the inserted próxima onto the now-playing, dropping the old head
		// on both sides so mirror[0] and engine.cur stay equal.
		s.mu.Lock()
		_, _ = s.queue.Next()
		s.mu.Unlock()
		return s.engine.Skip(ctx)
	}
	return nil
}

// Dequeue removes the pending item at queue index pos from both the mirror and
// the engine so a future skip doesn't resurrect it. pos 0 is the now-playing
// item — refuse it (skipping the current track is Next's job). The engine's
// pending list excludes the current track, so its index is pos-1.
func (s *PlaybackService) Dequeue(ctx context.Context, pos int) error {
	if pos <= 0 {
		return playback.ErrEmptyQueue
	}
	s.mu.Lock()
	if _, err := s.queue.RemoveAt(pos); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.engine.RemoveAt(ctx, pos-1)
}

// Move reorders the running order, moving the pending item at index from to
// index to in both the mirror and the engine. Index 0 is the now-playing item:
// it cannot move and nothing may move onto it (the current track keeps playing),
// so both ends must be >= 1. The engine's pending list excludes the current
// track, so its indices are from-1 / to-1.
func (s *PlaybackService) Move(ctx context.Context, from, to int) error {
	if from <= 0 || to <= 0 {
		return playback.ErrEmptyQueue
	}
	s.mu.Lock()
	if _, err := s.queue.MoveAt(from, to); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.engine.MoveAt(ctx, from-1, to-1)
}

// Next skips to the next track.
func (s *PlaybackService) Next(ctx context.Context) error {
	s.mu.Lock()
	_, _ = s.queue.Next()
	s.mu.Unlock()
	return s.engine.Skip(ctx)
}

// Clear wipes the whole playout so the operator starts from scratch: it empties
// the mirror queue, drops every pending item from the engine (RemoveAt(0) as the
// list collapses; out-of-range is a no-op per the port contract), then skips the
// current track so nothing keeps playing.
func (s *PlaybackService) Clear(ctx context.Context) error {
	s.mu.Lock()
	pending := s.queue.Len()
	s.queue.Clear()
	s.mu.Unlock()
	for i := 0; i < pending; i++ {
		if err := s.engine.RemoveAt(ctx, 0); err != nil {
			return err
		}
	}
	return s.engine.Skip(ctx)
}

// NowPlaying returns the engine's current track and the pending queue.
func (s *PlaybackService) NowPlaying(ctx context.Context) (playback.Playable, []playback.QueueItem, error) {
	np, err := s.engine.NowPlaying(ctx)
	if err != nil {
		return playback.Playable{}, nil, err
	}
	return np, s.Queue(ctx), nil
}

// Queue returns a snapshot of pending items.
func (s *PlaybackService) Queue(_ context.Context) []playback.QueueItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Items()
}

func toPlayable(t *library.Track) playback.Playable {
	uri := t.Path
	switch t.Source {
	case shared.SourceSpotify:
		uri = t.SpotifyURI
	case shared.SourceYouTube:
		uri = t.YouTubeURI
	}
	return playback.Playable{
		TrackID:    t.ID,
		Source:     t.Source,
		Title:      t.Title,
		Artist:     t.Artist,
		URI:        uri,
		Duration:   t.Duration,
		ArtworkURL: t.ArtworkURL,
		Category:   t.Category,
		// Dicas de reprodução. Este é o ÚNICO ponto de construção do Playable, então
		// é aqui que elas entram — a fila, o WS e o REST herdam sem saber da regra.
		// Os cues vêm dos MARCADORES (não de colunas próprias): assim a marca que o
		// operador arrasta no editor dirige o deck exatamente como a do analisador.
		CueInMs:      t.CueInMs(),
		CueOutMs:     t.CueOutMs(),
		VocalMs:      t.VocalMs(),
		BPM:          t.BPM,
		LoudnessLUFS: t.LoudnessLUFS,
		// BOOLEANO, não o dB cru: a régua de "dá pra tirar o vocal" é do domínio.
		// Mandar o número convidaria o browser a reimplementar o limiar e a divergir
		// dele em silêncio.
	}
}
