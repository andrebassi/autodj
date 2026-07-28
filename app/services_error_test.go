package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/shared"
)

type failRepo struct{ listErr error }

func (f failRepo) Upsert(context.Context, *library.Track) error { return nil }
func (f failRepo) List(context.Context) ([]*library.Track, error) {
	return nil, f.listErr
}
func (f failRepo) Get(context.Context, string) (*library.Track, error) { return nil, nil }
func (f failRepo) DeleteAll(context.Context) error                     { return nil }
func (f failRepo) Delete(context.Context, string) error                { return nil }
func (f failRepo) Count(context.Context) (int, error)                  { return 0, nil }

type okRepoOneLocal struct{}

func (okRepoOneLocal) Upsert(context.Context, *library.Track) error { return nil }
func (okRepoOneLocal) List(context.Context) ([]*library.Track, error) {
	t, _ := library.NewTrack("1", shared.SourceLocal, "L", time.Minute)
	t.Path = "/l.mp3"
	return []*library.Track{t}, nil
}
func (okRepoOneLocal) Get(context.Context, string) (*library.Track, error) { return nil, nil }
func (okRepoOneLocal) DeleteAll(context.Context) error                     { return nil }
func (okRepoOneLocal) Delete(context.Context, string) error                {return nil }
func (okRepoOneLocal) Count(context.Context) (int, error)                  { return 1, nil }

type failEngine struct{ pushErr, npErr error }

func (f failEngine) Push(context.Context, playback.QueueItem) error { return f.pushErr }
func (f failEngine) Skip(context.Context) error                     { return nil }
func (f failEngine) RemoveAt(context.Context, int) error            { return nil }
func (f failEngine) MoveAt(context.Context, int, int) error         { return nil }
func (f failEngine) NowPlaying(context.Context) (playback.Playable, error) {
	return playback.Playable{}, f.npErr
}
func (f failEngine) Healthy(context.Context) bool { return true }

func TestLoadFromLibraryRepoError(t *testing.T) {
	svc := NewPlaybackService(failRepo{listErr: errors.New("db down")}, failEngine{})
	if _, err := svc.LoadFromLibrary(context.Background()); err == nil {
		t.Fatal("repo error must propagate")
	}
}

func TestLoadFromLibraryEnginePushError(t *testing.T) {
	svc := NewPlaybackService(okRepoOneLocal{}, failEngine{pushErr: errors.New("socket")})
	if _, err := svc.LoadFromLibrary(context.Background()); err == nil {
		t.Fatal("engine push error must propagate")
	}
}

func TestNowPlayingEngineError(t *testing.T) {
	svc := NewPlaybackService(okRepoOneLocal{}, failEngine{npErr: errors.New("no")})
	if _, _, err := svc.NowPlaying(context.Background()); err == nil {
		t.Fatal("engine nowplaying error must propagate")
	}
}

func TestToPlayableSpotifyUsesURI(t *testing.T) {
	tr := &library.Track{ID: "s", Source: shared.SourceSpotify, Title: "Sp", SpotifyURI: "spotify:track:z", Duration: time.Minute}
	p := toPlayable(tr)
	if p.URI != "spotify:track:z" {
		t.Fatalf("spotify playable must use spotify uri, got %q", p.URI)
	}
}

func TestToPlayableYouTubeUsesURI(t *testing.T) {
	tr := &library.Track{ID: "y", Source: shared.SourceYouTube, Title: "Yt", YouTubeURI: "n61ULEU7CO0", Duration: time.Minute}
	p := toPlayable(tr)
	if p.URI != "n61ULEU7CO0" {
		t.Fatalf("youtube playable must use youtube uri, got %q", p.URI)
	}
}
