package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/social"
	"autodj/ports/inbound"
)

// fakeSocial is a configurable SocialService stub.
type fakeSocial struct {
	receipts   []social.Receipt
	receipt    social.Receipt
	announceEr error
	postEr     error
}

func (f *fakeSocial) Platforms() []social.Platform { return social.Platforms() }
func (f *fakeSocial) AnnounceNowPlaying(context.Context, []social.Platform) ([]social.Receipt, error) {
	return f.receipts, f.announceEr
}
func (f *fakeSocial) PostCustom(context.Context, social.Platform, string) (social.Receipt, error) {
	return f.receipt, f.postEr
}

func withSocial(s inbound.SocialService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithSocial(s))
}

func TestGetSocialPlatforms(t *testing.T) {
	h := withSocial(&fakeSocial{})
	rec := do(t, h, http.MethodGet, "/api/v1/social/platforms", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got []socPlatformDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Key != "whatsapp" || got[0].Label != "WhatsApp" || got[1].MaxLen != 280 {
		t.Fatalf("platforms=%+v", got)
	}
}

func TestPostSocialAnnounceOK(t *testing.T) {
	s := &fakeSocial{receipts: []social.Receipt{
		{Platform: social.PlatformWhatsApp, ExternalID: "whatsapp-1", URL: "stub://whatsapp/whatsapp-1"},
		{Platform: social.PlatformX, ExternalID: "x-1", URL: "stub://x/x-1"},
	}}
	h := withSocial(s)
	rec := do(t, h, http.MethodPost, "/api/v1/social/announce", `{"platforms":["whatsapp","x"]}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got socAnnounceDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Receipts) != 2 || got.Receipts[0].Platform != "whatsapp" || got.Receipts[1].ExternalID != "x-1" {
		t.Fatalf("receipts=%+v", got.Receipts)
	}
}

func TestPostSocialAnnounceNothingPlaying(t *testing.T) {
	h := withSocial(&fakeSocial{announceEr: inbound.ErrNothingPlaying})
	rec := do(t, h, http.MethodPost, "/api/v1/social/announce", `{"platforms":["x"]}`)
	if rec.Code != 409 {
		t.Fatalf("nothing playing should 409, got %d", rec.Code)
	}
}

func TestPostSocialAnnounceBadPlatform(t *testing.T) {
	h := withSocial(&fakeSocial{announceEr: social.ErrBadPlatform})
	rec := do(t, h, http.MethodPost, "/api/v1/social/announce", `{"platforms":["myspace"]}`)
	if rec.Code != 422 {
		t.Fatalf("bad platform should 422, got %d", rec.Code)
	}
}

func TestPostSocialAnnounceBadBody(t *testing.T) {
	h := withSocial(&fakeSocial{})
	rec := do(t, h, http.MethodPost, "/api/v1/social/announce", `not json`)
	if rec.Code != 400 {
		t.Fatalf("bad body should 400, got %d", rec.Code)
	}
}

func TestPostSocialCustomOK(t *testing.T) {
	s := &fakeSocial{receipt: social.Receipt{Platform: social.PlatformFacebook, ExternalID: "facebook-1", URL: "stub://facebook/facebook-1"}}
	h := withSocial(s)
	rec := do(t, h, http.MethodPost, "/api/v1/social/post", `{"platform":"facebook","text":"Recado"}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got socReceiptDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Platform != "facebook" || got.ExternalID != "facebook-1" {
		t.Fatalf("receipt=%+v", got)
	}
}

func TestPostSocialCustomEmptyText(t *testing.T) {
	h := withSocial(&fakeSocial{postEr: social.ErrEmptyText})
	rec := do(t, h, http.MethodPost, "/api/v1/social/post", `{"platform":"x","text":"  "}`)
	if rec.Code != 422 {
		t.Fatalf("empty text should 422, got %d", rec.Code)
	}
}

func TestPostSocialCustomBadBody(t *testing.T) {
	h := withSocial(&fakeSocial{})
	rec := do(t, h, http.MethodPost, "/api/v1/social/post", `nope`)
	if rec.Code != 400 {
		t.Fatalf("bad body should 400, got %d", rec.Code)
	}
}

func TestSocialRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, path := range []string{"/api/v1/social/platforms"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 404 {
			t.Fatalf("%s should 404 without WithSocial, got %d", path, rec.Code)
		}
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/social/announce", `{"platforms":["x"]}`); rec.Code != 404 {
		t.Fatalf("announce should 404 without WithSocial, got %d", rec.Code)
	}
}
