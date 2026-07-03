package api

import (
	"crypto/subtle"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// requireAuthToken guards every /api/* route with a static shared-secret
// token, since this service will eventually run in the cloud and make
// outbound HenrikDev API calls on behalf of whoever can reach it.
func requireAuthToken(token string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		got := strings.TrimPrefix(e.Request.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return e.UnauthorizedError("invalid or missing API token", nil)
		}
		return e.Next()
	}
}
