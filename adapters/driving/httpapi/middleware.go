package httpapi

import (
	"context"
	"net/http"
	"strings"

	"autodj/domain/identity"
)

// ctxKey is the private type for request-context keys so no other package can
// collide with or read the injected user.
type ctxKey int

const userKey ctxKey = 0

// bearer extracts the token from an "Authorization: Bearer <token>" header, or
// "" when absent/malformed. The scheme match is case-insensitive per RFC 7235.
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// requirePerm returns middleware that authorizes the request against perm. On
// success the authenticated user is injected into the context; otherwise it
// short-circuits with 401 (bad/missing token) or 403 (role lacks the permission).
// It is only mounted when an AuthService is wired (see the router), so rt.auth is
// never nil here.
func (rt *Router) requirePerm(perm identity.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, err := rt.auth.Authorize(r.Context(), bearer(r), perm)
			if err != nil {
				writeErr(w, err)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// userFrom returns the authenticated user injected by requirePerm, or the zero
// user (and false) when the request was not gated (unconfigured build).
func userFrom(ctx context.Context) (identity.User, bool) {
	u, ok := ctx.Value(userKey).(identity.User)
	return u, ok
}
