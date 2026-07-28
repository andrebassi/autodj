package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/cartwall"
)

// fakeCartwall is a configurable CartwallService stub.
type fakeCartwall struct {
	board      cartwall.Board
	boardErr   error
	saveErr    error
	assignErr  error
	clearErr   error
	lastButton cartwall.CartButton // last AssignButton arg
	clearedID  string              // last ClearButton arg
}

func (f *fakeCartwall) Board(context.Context) (cartwall.Board, error) {
	return f.board, f.boardErr
}
func (f *fakeCartwall) SaveBoard(_ context.Context, b cartwall.Board) (cartwall.Board, error) {
	if f.saveErr != nil {
		return cartwall.Board{}, f.saveErr
	}
	f.board = b
	return b, nil
}
func (f *fakeCartwall) AssignButton(_ context.Context, btn cartwall.CartButton) (cartwall.Board, error) {
	f.lastButton = btn
	if f.assignErr != nil {
		return cartwall.Board{}, f.assignErr
	}
	f.board.Buttons = append(f.board.Buttons, btn)
	return f.board, nil
}
func (f *fakeCartwall) ClearButton(_ context.Context, id string) error {
	f.clearedID = id
	return f.clearErr
}

func newCartH(cw *fakeCartwall) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithCartwall(cw))
}

func TestGetCartwall(t *testing.T) {
	cw := &fakeCartwall{board: cartwall.Board{Buttons: []cartwall.CartButton{
		{ID: "a", Label: "Vinheta", TrackID: "demo-01", Hotkey: "q", Page: 1},
	}}}
	rec := do(t, newCartH(cw), http.MethodGet, "/api/v1/cartwall", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got cartwall.Board
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Buttons) != 1 || got.Buttons[0].ID != "a" {
		t.Fatalf("body = %+v", got)
	}
}

func TestGetCartwallError(t *testing.T) {
	cw := &fakeCartwall{boardErr: errBoom}
	rec := do(t, newCartH(cw), http.MethodGet, "/api/v1/cartwall", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestPutCartwallOK(t *testing.T) {
	cw := &fakeCartwall{}
	body := `{"buttons":[{"id":"a","label":"Vinheta","trackId":"demo-01","hotkey":"q","page":1}]}`
	rec := do(t, newCartH(cw), http.MethodPut, "/api/v1/cartwall", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got cartwall.Board
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Buttons) != 1 || got.Buttons[0].ID != "a" {
		t.Fatalf("body = %+v", got)
	}
}

func TestPutCartwallBadBody(t *testing.T) {
	rec := do(t, newCartH(&fakeCartwall{}), http.MethodPut, "/api/v1/cartwall", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPutCartwallHotkeyConflict(t *testing.T) {
	cw := &fakeCartwall{saveErr: cartwall.ErrHotkeyTaken}
	body := `{"buttons":[{"id":"a","label":"X","trackId":"demo-01","hotkey":"q","page":1}]}`
	rec := do(t, newCartH(cw), http.MethodPut, "/api/v1/cartwall", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestPutCartwallNonMixable(t *testing.T) {
	cw := &fakeCartwall{saveErr: cartwall.ErrNotMixable}
	body := `{"buttons":[{"id":"a","label":"X","trackId":"drm","hotkey":"q","page":1}]}`
	rec := do(t, newCartH(cw), http.MethodPut, "/api/v1/cartwall", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}

func TestPostCartButtonOK(t *testing.T) {
	cw := &fakeCartwall{}
	body := `{"id":"a","label":"Vinheta","trackId":"demo-01","hotkey":"q","page":1}`
	rec := do(t, newCartH(cw), http.MethodPost, "/api/v1/cartwall/buttons", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if cw.lastButton.ID != "a" {
		t.Fatalf("button not forwarded: %+v", cw.lastButton)
	}
}

func TestPostCartButtonBadBody(t *testing.T) {
	rec := do(t, newCartH(&fakeCartwall{}), http.MethodPost, "/api/v1/cartwall/buttons", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPostCartButtonHotkeyConflict(t *testing.T) {
	cw := &fakeCartwall{assignErr: cartwall.ErrHotkeyTaken}
	body := `{"id":"a","label":"X","trackId":"demo-01","hotkey":"q","page":1}`
	rec := do(t, newCartH(cw), http.MethodPost, "/api/v1/cartwall/buttons", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestPostCartButtonNonMixable(t *testing.T) {
	cw := &fakeCartwall{assignErr: cartwall.ErrNotMixable}
	body := `{"id":"a","label":"X","trackId":"drm","hotkey":"q","page":1}`
	rec := do(t, newCartH(cw), http.MethodPost, "/api/v1/cartwall/buttons", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}

func TestPostCartButtonEmptyLabel(t *testing.T) {
	cw := &fakeCartwall{assignErr: cartwall.ErrEmptyLabel}
	body := `{"id":"a","label":"","trackId":"demo-01","hotkey":"q","page":1}`
	rec := do(t, newCartH(cw), http.MethodPost, "/api/v1/cartwall/buttons", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422", rec.Code)
	}
}

func TestDeleteCartButtonOK(t *testing.T) {
	cw := &fakeCartwall{}
	rec := do(t, newCartH(cw), http.MethodDelete, "/api/v1/cartwall/buttons/a", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if cw.clearedID != "a" {
		t.Fatalf("cleared id = %q, want a", cw.clearedID)
	}
}

func TestDeleteCartButtonNotFound(t *testing.T) {
	cw := &fakeCartwall{clearErr: cartwall.ErrNotFound}
	rec := do(t, newCartH(cw), http.MethodDelete, "/api/v1/cartwall/buttons/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestCartwallRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/cartwall", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET should 404 without WithCartwall, got %d", rec.Code)
	}
}
