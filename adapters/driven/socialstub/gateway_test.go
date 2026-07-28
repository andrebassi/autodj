package socialstub

import (
	"context"
	"testing"
	"time"

	"autodj/domain/social"
)

func TestPublishSynthesizesReceipt(t *testing.T) {
	at := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	post := social.Post{Platform: social.PlatformX, Text: "🎵 Agora na rádio: Cometa", CreatedAt: at}

	rc, err := New().Publish(context.Background(), post)
	if err != nil {
		t.Fatal(err)
	}
	wantID := "x-" + itoa(at.UnixNano())
	if rc.Platform != social.PlatformX {
		t.Fatalf("platform=%s", rc.Platform)
	}
	if rc.ExternalID != wantID {
		t.Fatalf("externalID=%q want %q", rc.ExternalID, wantID)
	}
	if rc.URL != "stub://x/"+wantID {
		t.Fatalf("url=%q", rc.URL)
	}
	if !rc.PublishedAt.Equal(at) {
		t.Fatalf("publishedAt=%v", rc.PublishedAt)
	}
}

func TestPublishDeterministic(t *testing.T) {
	at := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	post := social.Post{Platform: social.PlatformWhatsApp, Text: "oi", CreatedAt: at}
	g := New()
	a, _ := g.Publish(context.Background(), post)
	b, _ := g.Publish(context.Background(), post)
	if a != b {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
