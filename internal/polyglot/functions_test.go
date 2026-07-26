package polyglot

import (
	"errors"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"val-analyzer/internal/dataprovider"
)

// TestFunctionErrorResponse_Classification proves functionErrorResponse
// maps each error kind to the right HTTP status, in particular that a
// dataprovider.RemoteError's exact status code passes through unchanged
// (not collapsed to a generic 400/500) - critical for GET /jobs?datasource=
// to correctly surface a remote 404 for an unknown job id, not a 400.
// Constructing a bare &core.RequestEvent{} is safe here: BadRequestError/
// Error/InternalServerError are pure delegating wrappers around
// router.NewXxxError that never dereference the event itself (verified
// against the vendored pocketbase module).
func TestFunctionErrorResponse_Classification(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"unknown datasource", errUnknownDatasource, http.StatusBadRequest},
		{"datasource not function-capable", errFunctionsNotSupported, http.StatusBadRequest},
		{"remote 404", &dataprovider.RemoteError{StatusCode: http.StatusNotFound, Message: "no job with that id"}, http.StatusNotFound},
		{"remote 400", &dataprovider.RemoteError{StatusCode: http.StatusBadRequest, Message: "unknown function"}, http.StatusBadRequest},
		{"remote 502", &dataprovider.RemoteError{StatusCode: http.StatusBadGateway, Message: "upstream unreachable"}, http.StatusBadGateway},
		{"plain error", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := functionErrorResponse(&core.RequestEvent{}, tt.err)
			var apiErr *router.ApiError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected a *router.ApiError, got %T: %v", err, err)
			}
			if apiErr.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.wantStatus)
			}
		})
	}
}
