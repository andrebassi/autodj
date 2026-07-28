package app

import (
	"context"

	"autodj/domain/social"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// SocialService composes and publishes social posts (Story 8.5). It reads the
// current track from the PlaybackService, builds a platform-shaped post through
// domain/social, and publishes it via the outbound SocialGateway. It owns no
// state — the clock stamps each post. This is metadata, never audio: the
// two-engine (spotify ⇒ !mixable) invariant does not apply here.
type SocialService struct {
	play    inbound.PlaybackService
	gateway outbound.SocialGateway
	clock   outbound.Clock
}

// NewSocialService wires the service over the playback service, the social
// gateway and a clock.
func NewSocialService(play inbound.PlaybackService, gateway outbound.SocialGateway, clock outbound.Clock) *SocialService {
	return &SocialService{play: play, gateway: gateway, clock: clock}
}

// Platforms lists the platforms the UI can target (canonical order).
func (s *SocialService) Platforms() []social.Platform {
	return social.Platforms()
}

// AnnounceNowPlaying posts the current track to each platform. It reads the
// now-playing track once; an empty TrackID means nothing is on the air
// (ErrNothingPlaying). For each platform it composes a now-playing post and
// publishes it, collecting the receipts. Composition or gateway errors propagate
// fail-fast (partial receipts are dropped).
func (s *SocialService) AnnounceNowPlaying(ctx context.Context, platforms []social.Platform) ([]social.Receipt, error) {
	now, _, err := s.play.NowPlaying(ctx)
	if err != nil {
		return nil, err
	}
	if now.TrackID == "" {
		return nil, inbound.ErrNothingPlaying
	}
	info := social.NowInfo{Title: now.Title, Source: string(now.Source)}
	at := s.clock.Now()
	receipts := make([]social.Receipt, 0, len(platforms))
	for _, p := range platforms {
		post, err := social.ComposeNowPlaying(p, info, at)
		if err != nil {
			return nil, err
		}
		rc, err := s.gateway.Publish(ctx, post)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, rc)
	}
	return receipts, nil
}

// PostCustom composes a free-text note for one platform and publishes it.
func (s *SocialService) PostCustom(ctx context.Context, p social.Platform, text string) (social.Receipt, error) {
	post, err := social.ComposeCustom(p, text, s.clock.Now())
	if err != nil {
		return social.Receipt{}, err
	}
	return s.gateway.Publish(ctx, post)
}

// compile-time: SocialService satisfies the inbound port.
var _ inbound.SocialService = (*SocialService)(nil)
