package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/requests"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// fakeRequests is a configurable RequestService stub. Each method returns the
// preset out/err; the moderation methods record the last id so path parsing can
// be asserted.
type fakeRequests struct {
	out       requests.Request
	list      []requests.Request
	err       error
	gotStatus string
	gotID     string
	gotSubmit [3]string
}

func (f *fakeRequests) Submit(_ context.Context, listener, songHint, dedication string) (requests.Request, error) {
	f.gotSubmit = [3]string{listener, songHint, dedication}
	return f.out, f.err
}

func (f *fakeRequests) List(_ context.Context, status string) ([]requests.Request, error) {
	f.gotStatus = status
	return f.list, f.err
}

func (f *fakeRequests) Approve(_ context.Context, id string) (requests.Request, error) {
	f.gotID = id
	return f.out, f.err
}

func (f *fakeRequests) Reject(_ context.Context, id string) (requests.Request, error) {
	f.gotID = id
	return f.out, f.err
}

func (f *fakeRequests) MarkPlayed(_ context.Context, id string) (requests.Request, error) {
	f.gotID = id
	return f.out, f.err
}

func withRequests(rq inbound.RequestService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithRequests(rq))
}

func sampleRequest() requests.Request {
	created := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	return requests.Request{
		ID:           "req-1",
		ListenerName: "Ana",
		SongHint:     "Aurora - Nébula",
		Dedication:   "pra Bia",
		Status:       requests.StatusPending,
		CreatedAt:    created,
	}
}

func TestPostRequestOK(t *testing.T) {
	rq := &fakeRequests{out: sampleRequest()}
	h := withRequests(rq)
	rec := do(t, h, http.MethodPost, "/api/v1/requests",
		`{"listener_name":"Ana","song_hint":"Aurora - Nébula","dedication":"pra Bia"}`)
	if rec.Code != 201 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if rq.gotSubmit != [3]string{"Ana", "Aurora - Nébula", "pra Bia"} {
		t.Fatalf("submit args=%v", rq.gotSubmit)
	}
	var got requestDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "req-1" || got.Status != "pending" || got.HandledAt != "" {
		t.Fatalf("dto=%+v", got)
	}
}

func TestPostRequestBadBody(t *testing.T) {
	h := withRequests(&fakeRequests{})
	rec := do(t, h, http.MethodPost, "/api/v1/requests", `{bad`)
	if rec.Code != 400 {
		t.Fatalf("bad body should 400, got %d", rec.Code)
	}
}

func TestPostRequestValidationError(t *testing.T) {
	h := withRequests(&fakeRequests{err: requests.ErrEmptyListener})
	rec := do(t, h, http.MethodPost, "/api/v1/requests",
		`{"listener_name":"  ","song_hint":"x"}`)
	if rec.Code != 422 {
		t.Fatalf("empty listener should 422, got %d", rec.Code)
	}
}

func TestGetRequestsOK(t *testing.T) {
	handled := time.Date(2026, 7, 24, 13, 0, 0, 0, time.UTC)
	r := sampleRequest()
	r.Status = requests.StatusApproved
	r.HandledAt = handled
	rq := &fakeRequests{list: []requests.Request{r}}
	h := withRequests(rq)
	rec := do(t, h, http.MethodGet, "/api/v1/requests?status=approved", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if rq.gotStatus != "approved" {
		t.Fatalf("status filter=%q", rq.gotStatus)
	}
	var got []requestDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != "approved" || got[0].HandledAt == "" {
		t.Fatalf("dto=%+v", got)
	}
}

func TestGetRequestsBadStatus(t *testing.T) {
	h := withRequests(&fakeRequests{err: inbound.ErrBadStatus})
	rec := do(t, h, http.MethodGet, "/api/v1/requests?status=bogus", "")
	if rec.Code != 422 {
		t.Fatalf("bad status should 422, got %d", rec.Code)
	}
}

func TestModerateRequestOK(t *testing.T) {
	for _, tc := range []struct {
		verb string
		path string
	}{
		{"approve", "/api/v1/requests/req-9/approve"},
		{"reject", "/api/v1/requests/req-9/reject"},
		{"played", "/api/v1/requests/req-9/played"},
	} {
		out := sampleRequest()
		out.ID = "req-9"
		rq := &fakeRequests{out: out}
		h := withRequests(rq)
		rec := do(t, h, http.MethodPost, tc.path, "")
		if rec.Code != 200 {
			t.Fatalf("%s code=%d body=%s", tc.verb, rec.Code, rec.Body)
		}
		if rq.gotID != "req-9" {
			t.Fatalf("%s id=%q", tc.verb, rq.gotID)
		}
	}
}

func TestModerateRequestNotFound(t *testing.T) {
	h := withRequests(&fakeRequests{err: outbound.ErrRequestNotFound})
	rec := do(t, h, http.MethodPost, "/api/v1/requests/nope/approve", "")
	if rec.Code != 404 {
		t.Fatalf("unknown id should 404, got %d", rec.Code)
	}
}

func TestModerateRequestInvalidTransition(t *testing.T) {
	h := withRequests(&fakeRequests{err: requests.ErrInvalidTransition})
	rec := do(t, h, http.MethodPost, "/api/v1/requests/req-1/played", "")
	if rec.Code != 409 {
		t.Fatalf("bad transition should 409, got %d", rec.Code)
	}
}

func TestRequestsRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, path := range []string{"/api/v1/requests", "/api/v1/requests/x/approve"} {
		if rec := do(t, h, http.MethodPost, path, "{}"); rec.Code != 404 {
			t.Fatalf("%s should 404 without WithRequests, got %d", path, rec.Code)
		}
	}
}
