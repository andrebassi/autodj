package httpapi

import (
	"net/http"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

func TestYouTubeVideoID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"youtube:video:2bS-rippO6I", "2bS-rippO6I"}, // canonical
		{"2bS-rippO6I", "2bS-rippO6I"},               // bare id (raw-URI drop)
		{"youtube:playlist:PL123", ""},               // playlist has no single thumb
		{"", ""},                                     // non-youtube track
	}
	for _, c := range cases {
		if got := youtubeVideoID(c.in); got != c.want {
			t.Errorf("youtubeVideoID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A YouTube track whose stored artwork_url is empty (older import) still yields a
// cover: the handler derives the public ytimg URL from the video id and 302s.
func TestArtworkYouTubeFallback(t *testing.T) {
	tr, _ := library.NewTrack("yt1", shared.SourceYouTube, "Song", time.Minute)
	tr.ArtworkURL = ""
	tr.YouTubeURI = "youtube:video:2bS-rippO6I"
	h := New(fakeLib{artworkT: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)

	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/yt1/artwork", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://i.ytimg.com/vi/2bS-rippO6I/mqdefault.jpg" {
		t.Fatalf("bad Location: %s", loc)
	}
}

// An explicit ArtworkURL still wins (Spotify album image, or a YouTube thumbnail
// that WAS stored) — the fallback only fires when it's empty.
func TestArtworkURLWins(t *testing.T) {
	tr, _ := library.NewTrack("yt2", shared.SourceYouTube, "Song", time.Minute)
	tr.ArtworkURL = "https://i.scdn.co/image/abc"
	tr.YouTubeURI = "youtube:video:zzz"
	h := New(fakeLib{artworkT: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)

	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/yt2/artwork", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://i.scdn.co/image/abc" {
		t.Fatalf("bad Location: %s", loc)
	}
}
