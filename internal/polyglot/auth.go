package polyglot

import (
	"crypto/subtle"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// requireAuthToken guards every polyglot route with a static shared-secret
// bearer token, constant-time compared server-side.
func requireAuthToken(token string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		got := strings.TrimPrefix(e.Request.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return e.UnauthorizedError("invalid or missing API token", nil)
		}
		return e.Next()
	}
}
