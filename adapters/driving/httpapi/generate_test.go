package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

// genTrack builds a resolved local music track for the fake generator.
func genTrack(id, title string) library.Track {
	tr, err := library.NewTrack(id, shared.SourceLocal, title, time.Minute)
	if err != nil {
		panic(err)
	}
	tr.Category = string(library.CategoryMusic)
	return *tr
}

func TestGeneratePlaylistOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{genTracks: []library.Track{genTrack("t1", "s1"), genTrack("t2", "s2")}}, nil)
	rec := do(t, h, http.MethodPost, "/api/v1/clocks/manha/generate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got []trackDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].ID != "t1" || got[1].ID != "t2" {
		t.Fatalf("body = %+v", got)
	}
}

func TestGeneratePlaylistEmptyIsArray(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPost, "/api/v1/clocks/manha/generate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("empty generate body = %q, want []", body)
	}
}

func TestGeneratePlaylistUnknownClock404(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{genErr: outbound.ErrClockNotFound}, nil)
	rec := do(t, h, http.MethodPost, "/api/v1/clocks/nope/generate", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestGeneratePlaylistEmptyCatalog422(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{genErr: schedule.ErrEmptyCategory}, nil)
	rec := do(t, h, http.MethodPost, "/api/v1/clocks/manha/generate", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}
