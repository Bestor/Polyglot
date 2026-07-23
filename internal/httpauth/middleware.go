// Package httpauth is a tiny static bearer-token middleware for
// PocketBase-embedded HTTP APIs, shared by cmd/polyglot and
// cmd/valorantapi so both binaries' HTTP surfaces are gated the same way.
package httpauth

import (
	"crypto/subtle"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// RequireToken guards every route it's bound to with a static shared-secret
// bearer token, constant-time compared server-side.
func RequireToken(token string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		got := strings.TrimPrefix(e.Request.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return e.UnauthorizedError("invalid or missing API token", nil)
		}
		return e.Next()
	}
}
