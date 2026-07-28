package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/cartwall"
	"autodj/domain/library"
	"autodj/domain/shared"
)

var errBoom = errors.New("boom")

// cartTracks seeds a track repo with one mixable (local PCM) track and one
// non-mixable (Spotify DRM) track so the service's mixable guard is exercised
// both ways.
func cartTracks(t *testing.T) *memory.TrackRepo {
	t.Helper()
	ctx := context.Background()
	tr := memory.New()

	loc, err := library.NewTrack("mix-1", shared.SourceLocal, "Jingle", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	loc.Path = "/media/mix-1.flac"
	if err := tr.Upsert(ctx, loc); err != nil {
		t.Fatal(err)
	}

	sp, err := library.NewTrack("drm-1", shared.SourceSpotify, "DRM", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.SpotifyURI = "spotify:track:x"
	if err := tr.Upsert(ctx, sp); err != nil {
		t.Fatal(err)
	}
	return tr
}

func cartButton(id, hotkey, trackID string) cartwall.CartButton {
	return cartwall.CartButton{ID: id, Label: "L-" + id, TrackID: trackID, Page: 1, Hotkey: hotkey}
}

func TestCartwallBoardEmptyReturnsDefault(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	b, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if len(b.Buttons) == 0 {
		t.Fatal("empty store should fall back to Default()")
	}
}

func TestCartwallAssignAndPersist(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCartwallRepo()
	svc := NewCartwallService(repo, cartTracks(t))

	b, err := svc.AssignButton(ctx, cartButton("a", "q", "mix-1"))
	if err != nil {
		t.Fatalf("AssignButton: %v", err)
	}
	// upsert into the Default board → button "a" plus the seeded defaults.
	found := false
	for _, btn := range b.Buttons {
		if btn.ID == "a" {
			found = true
		}
	}
	if !found {
		t.Fatal("assigned button missing from board")
	}

	// persisted: a fresh Board read must contain "a".
	got, err := svc.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, btn := range got.Buttons {
		if btn.ID == "a" {
			found = true
		}
	}
	if !found {
		t.Fatal("assigned button not persisted")
	}
}

func TestCartwallAssignNonMixableRejected(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	_, err := svc.AssignButton(context.Background(), cartButton("a", "q", "drm-1"))
	if err != cartwall.ErrNotMixable {
		t.Fatalf("want ErrNotMixable, got %v", err)
	}
}

func TestCartwallAssignUnknownTrackRejected(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	_, err := svc.AssignButton(context.Background(), cartButton("a", "q", "nope"))
	if err != cartwall.ErrNotMixable {
		t.Fatalf("unknown track should be non-mixable, got %v", err)
	}
}

func TestCartwallAssignEmptyTrackRejected(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	_, err := svc.AssignButton(context.Background(), cartButton("a", "q", ""))
	if err != cartwall.ErrNotMixable {
		t.Fatalf("empty track should be non-mixable, got %v", err)
	}
}

func TestCartwallSaveBoard(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCartwallRepo()
	svc := NewCartwallService(repo, cartTracks(t))

	in := cartwall.Board{Buttons: []cartwall.CartButton{
		cartButton("a", "q", "mix-1"),
		cartButton("b", "w", "mix-1"),
	}}
	out, err := svc.SaveBoard(ctx, in)
	if err != nil {
		t.Fatalf("SaveBoard: %v", err)
	}
	if len(out.Buttons) != 2 {
		t.Fatalf("saved board len = %d, want 2", len(out.Buttons))
	}
	// round-trip: a fresh Board read returns exactly the saved two (not Default).
	got, err := svc.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Buttons) != 2 {
		t.Fatalf("persisted board len = %d, want 2", len(got.Buttons))
	}
}

func TestCartwallSaveBoardRejectsNonMixable(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	in := cartwall.Board{Buttons: []cartwall.CartButton{cartButton("a", "q", "drm-1")}}
	if _, err := svc.SaveBoard(context.Background(), in); err != cartwall.ErrNotMixable {
		t.Fatalf("want ErrNotMixable, got %v", err)
	}
}

func TestCartwallSaveBoardRejectsHotkeyClash(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	in := cartwall.Board{Buttons: []cartwall.CartButton{
		cartButton("a", "q", "mix-1"),
		cartButton("b", "q", "mix-1"), // same hotkey same page
	}}
	if _, err := svc.SaveBoard(context.Background(), in); err != cartwall.ErrHotkeyTaken {
		t.Fatalf("want ErrHotkeyTaken, got %v", err)
	}
}

func TestCartwallClearButton(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCartwallRepo()
	svc := NewCartwallService(repo, cartTracks(t))

	if _, err := svc.AssignButton(ctx, cartButton("a", "q", "mix-1")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearButton(ctx, "a"); err != nil {
		t.Fatalf("ClearButton: %v", err)
	}
	got, err := svc.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, btn := range got.Buttons {
		if btn.ID == "a" {
			t.Fatal("cleared button still present")
		}
	}
}

func TestCartwallClearButtonNotFound(t *testing.T) {
	svc := NewCartwallService(memory.NewCartwallRepo(), cartTracks(t))
	if err := svc.ClearButton(context.Background(), "nope"); err != cartwall.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// failCartwallRepo returns errLoad on Load and errSave on Save, to exercise the
// service's repo-error branches.
type failCartwallRepo struct {
	loadErr, saveErr error
	board            cartwall.Board
}

func (r failCartwallRepo) Load(context.Context) (cartwall.Board, error) {
	return r.board, r.loadErr
}
func (r failCartwallRepo) Save(context.Context, cartwall.Board) error { return r.saveErr }

func TestCartwallBoardLoadError(t *testing.T) {
	svc := NewCartwallService(failCartwallRepo{loadErr: errBoom}, cartTracks(t))
	if _, err := svc.Board(context.Background()); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCartwallSaveBoardSaveError(t *testing.T) {
	svc := NewCartwallService(failCartwallRepo{saveErr: errBoom}, cartTracks(t))
	in := cartwall.Board{Buttons: []cartwall.CartButton{cartButton("a", "q", "mix-1")}}
	if _, err := svc.SaveBoard(context.Background(), in); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCartwallAssignSaveError(t *testing.T) {
	// non-empty board so Board() does not fall back to Default, then Save fails.
	seed := cartwall.Board{Buttons: []cartwall.CartButton{cartButton("z", "p", "mix-1")}}
	svc := NewCartwallService(failCartwallRepo{saveErr: errBoom, board: seed}, cartTracks(t))
	if _, err := svc.AssignButton(context.Background(), cartButton("a", "q", "mix-1")); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCartwallClearSaveError(t *testing.T) {
	seed := cartwall.Board{Buttons: []cartwall.CartButton{cartButton("a", "q", "mix-1")}}
	svc := NewCartwallService(failCartwallRepo{saveErr: errBoom, board: seed}, cartTracks(t))
	if err := svc.ClearButton(context.Background(), "a"); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCartwallAssignBoardError(t *testing.T) {
	svc := NewCartwallService(failCartwallRepo{loadErr: errBoom}, cartTracks(t))
	if _, err := svc.AssignButton(context.Background(), cartButton("a", "q", "mix-1")); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}

func TestCartwallClearBoardError(t *testing.T) {
	svc := NewCartwallService(failCartwallRepo{loadErr: errBoom}, cartTracks(t))
	if err := svc.ClearButton(context.Background(), "a"); err != errBoom {
		t.Fatalf("want errBoom, got %v", err)
	}
}
