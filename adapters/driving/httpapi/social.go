package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"autodj/domain/social"
)

// socPlatformDTO is the wire shape of one targetable platform.
type socPlatformDTO struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	MaxLen int    `json:"max_len"`
}

// socReceiptDTO is the wire shape of one publish receipt.
type socReceiptDTO struct {
	Platform    string `json:"platform"`
	ExternalID  string `json:"external_id"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
}

// socAnnounceDTO wraps the receipts returned by an announce.
type socAnnounceDTO struct {
	Receipts []socReceiptDTO `json:"receipts"`
}

func toSocReceiptDTO(rc social.Receipt) socReceiptDTO {
	return socReceiptDTO{
		Platform:    string(rc.Platform),
		ExternalID:  rc.ExternalID,
		URL:         rc.URL,
		PublishedAt: rc.PublishedAt.UTC().Format(time.RFC3339Nano),
	}
}

// getSocialPlatforms lists the platforms the UI can target (from the domain, no
// hardcoded list). GET /social/platforms.
func (rt *Router) getSocialPlatforms(w http.ResponseWriter, r *http.Request) {
	ps := rt.social.Platforms()
	out := make([]socPlatformDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, socPlatformDTO{Key: string(p), Label: p.Label(), MaxLen: p.MaxLen()})
	}
	writeJSON(w, http.StatusOK, out)
}

// postSocialAnnounce publishes the current now-playing track to the given
// platforms. Body: {platforms:[...]}. Nothing on air → 409; unknown platform →
// 422. POST /social/announce.
func (rt *Router) postSocialAnnounce(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Platforms []string `json:"platforms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	ps := make([]social.Platform, 0, len(body.Platforms))
	for _, p := range body.Platforms {
		ps = append(ps, social.Platform(p))
	}
	rcs, err := rt.social.AnnounceNowPlaying(r.Context(), ps)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]socReceiptDTO, 0, len(rcs))
	for _, rc := range rcs {
		out = append(out, toSocReceiptDTO(rc))
	}
	writeJSON(w, http.StatusOK, socAnnounceDTO{Receipts: out})
}

// postSocialCustom publishes a free-text note to one platform. Body:
// {platform, text}. Blank text → 422; unknown platform → 422. POST /social/post.
func (rt *Router) postSocialCustom(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Platform string `json:"platform"`
		Text     string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	rc, err := rt.social.PostCustom(r.Context(), social.Platform(body.Platform), body.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSocReceiptDTO(rc))
}
