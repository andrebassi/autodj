package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"autodj/domain/dailyinfo"
	"autodj/domain/publicsite"
	"autodj/ports/inbound"
)

// fakePublicSite is a configurable PublicSiteService stub.
type fakePublicSite struct {
	out publicsite.Snapshot
	err error
}

func (f *fakePublicSite) Snapshot(context.Context) (publicsite.Snapshot, error) {
	return f.out, f.err
}

func withPublicSite(ps inbound.PublicSiteService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithPublicSite(ps))
}

func TestGetPublicSiteOK(t *testing.T) {
	gen := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	ps := &fakePublicSite{out: publicsite.Snapshot{
		GeneratedAt: gen,
		NowPlaying:  publicsite.NowPlaying{TrackID: "cur", Title: "Aurora", Source: "local", OnAir: true, Mixable: true},
		Upcoming:    []publicsite.UpcomingItem{{TrackID: "nxt", Title: "Nébula", Source: "local"}},
		Recent: []publicsite.PlayedItem{{
			TrackID: "old", Title: "Cometa", Artist: "Vega", Source: "local",
			PlayedAt: gen.Add(-1 * time.Hour), DurationMs: 180000,
		}},
		Daily: dailyinfo.Compute(gen),
	}}
	h := withPublicSite(ps)
	rec := do(t, h, http.MethodGet, "/api/v1/public/site", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got pubSiteDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.NowPlaying.OnAir || got.NowPlaying.TrackID != "cur" || !got.NowPlaying.Mixable {
		t.Fatalf("now=%+v", got.NowPlaying)
	}
	if len(got.Upcoming) != 1 || got.Upcoming[0].TrackID != "nxt" {
		t.Fatalf("upcoming=%+v", got.Upcoming)
	}
	if len(got.Recent) != 1 || got.Recent[0].TrackID != "old" || got.Recent[0].DurationMs != 180000 {
		t.Fatalf("recent=%+v", got.Recent)
	}
	if got.Daily.DayOfYear != gen.YearDay() || got.Daily.Weekday == "" {
		t.Fatalf("daily=%+v", got.Daily)
	}
}

func TestGetPublicSiteError(t *testing.T) {
	h := withPublicSite(&fakePublicSite{err: errors.New("boom")})
	rec := do(t, h, http.MethodGet, "/api/v1/public/site", "")
	if rec.Code != 500 {
		t.Fatalf("repo error should 500, got %d", rec.Code)
	}
}

func TestPublicSiteRouteAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/public/site", ""); rec.Code != 404 {
		t.Fatalf("should 404 without WithPublicSite, got %d", rec.Code)
	}
}
