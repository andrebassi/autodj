package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/ports/inbound"
)

func TestCategoriesEndpoint(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/categories", "")
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var got []categoryDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != len(library.Categories()) {
		t.Fatalf("want %d categories, got %d", len(library.Categories()), len(got))
	}
	if got[0].Key != "music" || got[0].Label != "Músicas" {
		t.Fatalf("first category should be music/Músicas: %+v", got[0])
	}
}

// uploadReq builds a multipart request with a "file" part plus form fields.
func uploadReq(t *testing.T, fields map[string]string, withFile bool) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if withFile {
		fw, _ := mw.CreateFormFile("file", "track.mp3")
		_, _ = fw.Write([]byte("fake-audio-bytes"))
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/library/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestUploadEndpointHappy(t *testing.T) {
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "Uploaded", time.Minute)
	tr.Category = "jingle"
	h := New(fakeLib{uploaded: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, uploadReq(t, map[string]string{"category": "jingle"}, true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var got trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Title != "Uploaded" || got.Category != "jingle" || !got.Mixable {
		t.Fatalf("bad upload dto: %+v", got)
	}
}

func TestUploadEndpointNoFile(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, uploadReq(t, map[string]string{"category": "music"}, false))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no file should be 400, got %d", rec.Code)
	}
}

func TestUploadEndpointBadCategory(t *testing.T) {
	h := New(fakeLib{upErr: library.ErrBadCategory}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, uploadReq(t, map[string]string{"category": "bogus"}, true))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad category should be 422, got %d", rec.Code)
	}
}

func TestUpdateTrackEndpoint(t *testing.T) {
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "New Title", time.Minute)
	tr.Category = "soundtrack"
	h := New(fakeLib{updated: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPatch, "/api/v1/library/tracks/id1",
		`{"title":"New Title","category":"soundtrack"}`)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Title != "New Title" || got.Category != "soundtrack" {
		t.Fatalf("bad update dto: %+v", got)
	}
}

func TestUpdateTrackNotFound(t *testing.T) {
	h := New(fakeLib{updErr: library.ErrTrackNotFound}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPatch, "/api/v1/library/tracks/nope", `{"title":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestUpdateTrackBadCategory(t *testing.T) {
	h := New(fakeLib{updErr: library.ErrBadCategory}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPatch, "/api/v1/library/tracks/id1", `{"title":"x","category":"bogus"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}

func TestUpdateTrackBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPatch, "/api/v1/library/tracks/id1", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json should be 400, got %d", rec.Code)
	}
}

func TestDeleteTrackEndpoint(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/library/tracks/id1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

func TestDeleteTrackNotFound(t *testing.T) {
	h := New(fakeLib{delErr: library.ErrTrackNotFound}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodDelete, "/api/v1/library/tracks/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestSetMarkersEndpoint(t *testing.T) {
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "Song", 5*time.Minute)
	_ = tr.SetMarkers([]library.Marker{
		{Kind: library.MarkerIntro, PosMs: 8000},
		{Kind: library.MarkerHook, PosMs: 45000, Label: "refrão"},
	})
	h := New(fakeLib{marked: tr}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/library/tracks/id1/markers",
		`{"markers":[{"kind":"intro","pos_ms":8000},{"kind":"hook","pos_ms":45000,"label":"refrão"}]}`)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Markers) != 2 || got.Markers[0].Kind != "intro" || got.Markers[1].PosMs != 45000 {
		t.Fatalf("bad markers dto: %+v", got.Markers)
	}
}

func TestSetMarkersNotFound(t *testing.T) {
	h := New(fakeLib{markErr: library.ErrTrackNotFound}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/library/tracks/nope/markers", `{"markers":[]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestSetMarkersNotPCM(t *testing.T) {
	h := New(fakeLib{markErr: library.ErrMarkersNotPCM}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/library/tracks/spot/markers",
		`{"markers":[{"kind":"intro","pos_ms":1000}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-PCM markers should be 422, got %d", rec.Code)
	}
}

func TestSetMarkersBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/library/tracks/id1/markers", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json should be 400, got %d", rec.Code)
	}
}

func TestWaveformEndpoint(t *testing.T) {
	h := New(fakeLib{wf: inbound.WaveformData{DurationMs: 300000, Peaks: []float64{0.1, 0.5, 0.9}}}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/id1/waveform?bars=3", "")
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got waveformDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.DurationMs != 300000 || len(got.Peaks) != 3 || got.Peaks[2] != 0.9 {
		t.Fatalf("bad waveform dto: %+v", got)
	}
}

func TestWaveformNotFound(t *testing.T) {
	h := New(fakeLib{wfErr: library.ErrTrackNotFound}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/nope/waveform", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestWaveformNotPCM(t *testing.T) {
	h := New(fakeLib{wfErr: library.ErrWaveformNotPCM}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks/spot/waveform", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-PCM waveform should be 422, got %d", rec.Code)
	}
}

// TestClampBars covers the bars query parsing: default, non-numeric, and both
// clamp bounds.
func TestClampBars(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 120}, {"abc", 120}, {"3", 16}, {"5000", 2000}, {"200", 200}, {"16", 16}, {"2000", 2000},
	}
	for _, c := range cases {
		if got := clampBars(c.in); got != c.want {
			t.Fatalf("clampBars(%q)=%d, want %d", c.in, got, c.want)
		}
	}
}

// TestCategoryLabelParity guards against drift: the pt-BR label map must cover
// exactly the domain's fixed category set — no missing, no extra.
func TestCategoryLabelParity(t *testing.T) {
	cats := library.Categories()
	if len(categoryLabels) != len(cats) {
		t.Fatalf("label map size %d != domain categories %d", len(categoryLabels), len(cats))
	}
	for _, c := range cats {
		d, ok := categoryLabels[c]
		if !ok {
			t.Fatalf("category %q has no pt-BR label", c)
		}
		if d.Key != string(c) || d.Label == "" || d.Ico == "" {
			t.Fatalf("category %q label incomplete: %+v", c, d)
		}
	}
}
