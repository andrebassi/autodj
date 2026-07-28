package memory

import (
	"context"
	"testing"
	"time"

	"autodj/domain/spotify"
)

func TestSpotifyAccountRepo(t *testing.T) {
	ctx := context.Background()
	r := NewSpotifyAccountRepo()

	// Empty => ErrNoAccount.
	if _, err := r.Load(ctx); err != spotify.ErrNoAccount {
		t.Fatalf("empty Load = %v, want ErrNoAccount", err)
	}

	acc := spotify.Account{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Unix(100, 0), Product: "premium"}
	if err := r.Save(ctx, acc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := r.Load(ctx)
	if err != nil || got.AccessToken != "a" || !got.Premium() {
		t.Fatalf("Load = %+v err %v", got, err)
	}

	if err := r.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Load(ctx); err != spotify.ErrNoAccount {
		t.Fatalf("after Delete Load = %v, want ErrNoAccount", err)
	}
	// Delete idempotent.
	if err := r.Delete(ctx); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}
