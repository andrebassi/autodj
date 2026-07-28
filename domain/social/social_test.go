package social

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

func TestPlatformMeta(t *testing.T) {
	cases := []struct {
		p     Platform
		label string
		max   int
	}{
		{PlatformWhatsApp, "WhatsApp", 65536},
		{PlatformX, "X", 280},
		{PlatformFacebook, "Facebook", 63206},
	}
	for _, c := range cases {
		if !c.p.Valid() {
			t.Fatalf("%s should be valid", c.p)
		}
		if c.p.Label() != c.label {
			t.Fatalf("%s label=%q want %q", c.p, c.p.Label(), c.label)
		}
		if c.p.MaxLen() != c.max {
			t.Fatalf("%s max=%d want %d", c.p, c.p.MaxLen(), c.max)
		}
	}
	var bad Platform = "myspace"
	if bad.Valid() || bad.MaxLen() != 0 || bad.Label() != "" {
		t.Fatalf("unknown platform should be zero-valued")
	}
}

func TestPlatformsCanonical(t *testing.T) {
	got := Platforms()
	if len(got) != 3 || got[0] != PlatformWhatsApp || got[1] != PlatformX || got[2] != PlatformFacebook {
		t.Fatalf("platforms=%v", got)
	}
}

func TestComposeNowPlayingOK(t *testing.T) {
	post, err := ComposeNowPlaying(PlatformWhatsApp, NowInfo{Title: "Aurora Boreal", Source: "local"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if post.Platform != PlatformWhatsApp || !strings.Contains(post.Text, "Aurora Boreal") {
		t.Fatalf("post=%+v", post)
	}
	if !post.CreatedAt.Equal(at) {
		t.Fatalf("createdAt=%v", post.CreatedAt)
	}
	if strings.Contains(post.Text, "#") {
		t.Fatalf("whatsapp should have no hashtag: %q", post.Text)
	}
}

func TestComposeNowPlayingXHashtag(t *testing.T) {
	post, err := ComposeNowPlaying(PlatformX, NowInfo{Title: "Cometa"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(post.Text, "#NaRádio") {
		t.Fatalf("x should have hashtag: %q", post.Text)
	}
}

func TestComposeNowPlayingErrors(t *testing.T) {
	if _, err := ComposeNowPlaying("myspace", NowInfo{Title: "x"}, at); !errors.Is(err, ErrBadPlatform) {
		t.Fatalf("want ErrBadPlatform, got %v", err)
	}
	if _, err := ComposeNowPlaying(PlatformX, NowInfo{Title: "   "}, at); !errors.Is(err, ErrNoTrack) {
		t.Fatalf("want ErrNoTrack, got %v", err)
	}
}

func TestComposeNowPlayingTruncatesX(t *testing.T) {
	long := strings.Repeat("á", 400) // 400 runes, over the 280 X limit
	post, err := ComposeNowPlaying(PlatformX, NowInfo{Title: long}, at)
	if err != nil {
		t.Fatal(err)
	}
	r := []rune(post.Text)
	if len(r) != 280 {
		t.Fatalf("len=%d want 280", len(r))
	}
	if r[len(r)-1] != '…' {
		t.Fatalf("should end with ellipsis: %q", string(r[len(r)-3:]))
	}
}

func TestComposeCustomOK(t *testing.T) {
	post, err := ComposeCustom(PlatformFacebook, "  Recado do locutor  ", at)
	if err != nil {
		t.Fatal(err)
	}
	if post.Text != "Recado do locutor" {
		t.Fatalf("text=%q (should be trimmed)", post.Text)
	}
}

func TestComposeCustomErrors(t *testing.T) {
	if _, err := ComposeCustom("myspace", "hi", at); !errors.Is(err, ErrBadPlatform) {
		t.Fatalf("want ErrBadPlatform, got %v", err)
	}
	if _, err := ComposeCustom(PlatformX, "   ", at); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("want ErrEmptyText, got %v", err)
	}
}

func TestComposeCustomTruncates(t *testing.T) {
	post, err := ComposeCustom(PlatformX, strings.Repeat("z", 500), at)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(post.Text)) != 280 {
		t.Fatalf("len=%d want 280", len([]rune(post.Text)))
	}
}

func TestTruncateNoLimit(t *testing.T) {
	// max<=0 path: unlimited.
	if got := truncate("hello", 0); got != "hello" {
		t.Fatalf("got %q", got)
	}
	// exact fit (len == max) is not truncated.
	if got := truncate("hello", 5); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
