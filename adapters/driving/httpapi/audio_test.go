package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"autodj/domain/library"
)

// writeTempAudio drops a small fake audio file and returns its path.
func writeTempAudio(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "track.mp3")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestAudioServesBytes(t *testing.T) {
	path := writeTempAudio(t, "FAKEPCMDATA")
	tr := &library.Track{ID: "id1", Path: path, Mixable: true, PCMAvailable: true}
	h := New(fakeLib{audioT: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/id1/audio", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "FAKEPCMDATA" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if ar := rec.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", ar)
	}
}

func TestAudioRangeRequest(t *testing.T) {
	path := writeTempAudio(t, "0123456789")
	tr := &library.Track{ID: "id1", Path: path, Mixable: true, PCMAvailable: true}
	h := New(fakeLib{audioT: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/library/tracks/id1/audio", nil)
	req.Header.Set("Range", "bytes=0-3")
	rec := doReq(t, h, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d", rec.Code)
	}
	if rec.Body.String() != "0123" {
		t.Fatalf("range body = %q, want 0123", rec.Body.String())
	}
	if cr := rec.Header().Get("Content-Range"); cr == "" {
		t.Fatal("missing Content-Range")
	}
}

func TestAudioSpotifyForbidden(t *testing.T) {
	h := New(fakeLib{audioErr: library.ErrTrackNotMixable}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/spot/audio", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("spotify audio should be 403, got %d", rec.Code)
	}
}

func TestAudioNotFound(t *testing.T) {
	h := New(fakeLib{audioErr: library.ErrTrackNotFound}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/nope/audio", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestAudioNoPath(t *testing.T) {
	h := New(fakeLib{audioErr: library.ErrNoAudioPath}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/id1/audio", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-path audio should be 404, got %d", rec.Code)
	}
}
