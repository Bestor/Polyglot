package polyglot

import (
	"crypto/subtle"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// requireAuthToken guards every polyglot route with a static shared-secret
// bearer token (same scheme as internal/api's identical middleware,
// duplicated here rather than imported since internal/api belongs to the
// old cmd/server HTTP layer this service is replacing).
func requireAuthToken(token string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		got := strings.TrimPrefix(e.Request.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return e.UnauthorizedError("invalid or missing API token", nil)
		}
		return e.Next()
	}
}
