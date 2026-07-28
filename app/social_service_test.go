package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/playback"
	"autodj/domain/social"
	"autodj/ports/inbound"
)

// --- fake SocialGateway ---

type fakeGateway struct {
	posts  []social.Post
	failOn social.Platform // publish fails for this platform
}

var errGwBoom = errors.New("gateway boom")

func (g *fakeGateway) Publish(_ context.Context, post social.Post) (social.Receipt, error) {
	if post.Platform == g.failOn {
		return social.Receipt{}, errGwBoom
	}
	g.posts = append(g.posts, post)
	return social.Receipt{
		Platform:    post.Platform,
		ExternalID:  string(post.Platform) + "-1",
		URL:         "stub://" + string(post.Platform) + "/1",
		PublishedAt: post.CreatedAt,
	}, nil
}

var socClock = fixedClock{time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)}

func TestSocialPlatforms(t *testing.T) {
	svc := NewSocialService(&fakePlayback{}, &fakeGateway{}, socClock)
	if got := svc.Platforms(); len(got) != 3 || got[0] != social.PlatformWhatsApp {
		t.Fatalf("platforms=%v", got)
	}
}

func TestAnnounceNowPlayingOK(t *testing.T) {
	play := &fakePlayback{now: psLocal("cur")}
	gw := &fakeGateway{}
	svc := NewSocialService(play, gw, socClock)

	rcs, err := svc.AnnounceNowPlaying(context.Background(), []social.Platform{social.PlatformWhatsApp, social.PlatformX})
	if err != nil {
		t.Fatal(err)
	}
	if len(rcs) != 2 || rcs[0].Platform != social.PlatformWhatsApp || rcs[1].Platform != social.PlatformX {
		t.Fatalf("receipts=%+v", rcs)
	}
	if len(gw.posts) != 2 {
		t.Fatalf("posts=%d", len(gw.posts))
	}
	if !gw.posts[0].CreatedAt.Equal(socClock.t) {
		t.Fatalf("createdAt=%v", gw.posts[0].CreatedAt)
	}
}

func TestAnnounceNothingPlaying(t *testing.T) {
	play := &fakePlayback{now: playback.Playable{}} // empty TrackID
	svc := NewSocialService(play, &fakeGateway{}, socClock)
	_, err := svc.AnnounceNowPlaying(context.Background(), social.Platforms())
	if !errors.Is(err, inbound.ErrNothingPlaying) {
		t.Fatalf("want ErrNothingPlaying, got %v", err)
	}
}

func TestAnnouncePlaybackError(t *testing.T) {
	play := &fakePlayback{err: errPlBoom}
	svc := NewSocialService(play, &fakeGateway{}, socClock)
	if _, err := svc.AnnounceNowPlaying(context.Background(), social.Platforms()); !errors.Is(err, errPlBoom) {
		t.Fatalf("want errPlBoom, got %v", err)
	}
}

func TestAnnounceBadPlatform(t *testing.T) {
	play := &fakePlayback{now: psLocal("cur")}
	svc := NewSocialService(play, &fakeGateway{}, socClock)
	_, err := svc.AnnounceNowPlaying(context.Background(), []social.Platform{"myspace"})
	if !errors.Is(err, social.ErrBadPlatform) {
		t.Fatalf("want ErrBadPlatform, got %v", err)
	}
}

func TestAnnounceGatewayError(t *testing.T) {
	play := &fakePlayback{now: psLocal("cur")}
	gw := &fakeGateway{failOn: social.PlatformX}
	svc := NewSocialService(play, gw, socClock)
	_, err := svc.AnnounceNowPlaying(context.Background(), []social.Platform{social.PlatformWhatsApp, social.PlatformX})
	if !errors.Is(err, errGwBoom) {
		t.Fatalf("want errGwBoom, got %v", err)
	}
	// whatsapp published before X failed (fail-fast, partial dropped)
	if len(gw.posts) != 1 {
		t.Fatalf("posts=%d", len(gw.posts))
	}
}

func TestPostCustomOK(t *testing.T) {
	gw := &fakeGateway{}
	svc := NewSocialService(&fakePlayback{}, gw, socClock)
	rc, err := svc.PostCustom(context.Background(), social.PlatformFacebook, "  Recado  ")
	if err != nil {
		t.Fatal(err)
	}
	if rc.Platform != social.PlatformFacebook {
		t.Fatalf("receipt=%+v", rc)
	}
	if len(gw.posts) != 1 || gw.posts[0].Text != "Recado" {
		t.Fatalf("posts=%+v", gw.posts)
	}
}

func TestPostCustomComposeError(t *testing.T) {
	svc := NewSocialService(&fakePlayback{}, &fakeGateway{}, socClock)
	if _, err := svc.PostCustom(context.Background(), social.PlatformX, "   "); !errors.Is(err, social.ErrEmptyText) {
		t.Fatalf("want ErrEmptyText, got %v", err)
	}
	if _, err := svc.PostCustom(context.Background(), "myspace", "hi"); !errors.Is(err, social.ErrBadPlatform) {
		t.Fatalf("want ErrBadPlatform, got %v", err)
	}
}

func TestPostCustomGatewayError(t *testing.T) {
	gw := &fakeGateway{failOn: social.PlatformWhatsApp}
	svc := NewSocialService(&fakePlayback{}, gw, socClock)
	if _, err := svc.PostCustom(context.Background(), social.PlatformWhatsApp, "hi"); !errors.Is(err, errGwBoom) {
		t.Fatalf("want errGwBoom, got %v", err)
	}
}
