package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/schedule"
)

func TestGetGridEmptyIsArray(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/grid", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got schedule.Grid
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Cells == nil {
		t.Fatal("cells must serialize as [] not null")
	}
}

func TestGetGrid(t *testing.T) {
	g := schedule.Grid{Cells: []schedule.Cell{{Weekday: time.Monday, Hour: 6, ClockID: "manha"}}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{grid: g}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/grid", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got schedule.Grid
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Cells) != 1 || got.Cells[0].ClockID != "manha" {
		t.Fatalf("body = %+v", got)
	}
}

func TestGetGridError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{gridErr: errBoom}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/grid", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestPutGridOK(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	body := `{"cells":[{"weekday":1,"hour":6,"clockId":"manha"}]}`
	rec := do(t, h, http.MethodPut, "/api/v1/grid", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got schedule.Grid
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Cells) != 1 || got.Cells[0].ClockID != "manha" {
		t.Fatalf("body = %+v", got)
	}
}

func TestPutGridBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPut, "/api/v1/grid", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPutGridInvalid(t *testing.T) {
	// SaveGrid rejects with a schedule validation error → 422.
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{},
		fakeSched{gridSvErr: schedule.ErrUnknownClock}, nil)
	body := `{"cells":[{"weekday":1,"hour":6,"clockId":"ghost"}]}`
	rec := do(t, h, http.MethodPut, "/api/v1/grid", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}
