// Package socialstub is the home-build SocialGateway (Story 8.5): it does NOT
// call any external network or use credentials. It turns a domain/social.Post
// into a synthetic, deterministic Receipt so the whole social flow works locally
// without WhatsApp/X/Facebook API access. Swapping in a real gateway (WhatsApp
// Cloud API / X / Graph) is just another adapter — the domain and app do not
// change.
package socialstub

import (
	"context"
	"fmt"

	"autodj/domain/social"
	"autodj/ports/outbound"
)

// Gateway is a stateless, credential-free SocialGateway.
type Gateway struct{}

// New builds the stub gateway.
func New() *Gateway { return &Gateway{} }

// Publish synthesizes a Receipt from the post: ExternalID is
// "{platform}-{CreatedAt UnixNano}", URL is "stub://{platform}/{ExternalID}",
// and PublishedAt echoes the post's CreatedAt. Deterministic, no side effects.
func (g *Gateway) Publish(_ context.Context, post social.Post) (social.Receipt, error) {
	id := fmt.Sprintf("%s-%d", post.Platform, post.CreatedAt.UnixNano())
	return social.Receipt{
		Platform:    post.Platform,
		ExternalID:  id,
		URL:         fmt.Sprintf("stub://%s/%s", post.Platform, id),
		PublishedAt: post.CreatedAt,
	}, nil
}

// compile-time: Gateway satisfies the outbound port.
var _ outbound.SocialGateway = (*Gateway)(nil)
