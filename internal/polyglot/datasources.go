package polyglot

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

type ConfigFieldDescription struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
}

type ProviderTypeDescription struct {
	Type   string                   `json:"type"`
	Config []ConfigFieldDescription `json:"config"`
}

type ActiveDatasourceDescription struct {
	Type      string   `json:"type"`
	Tables    []string `json:"tables"`
	Functions []string `json:"functions"`
}

type DatasourcesResponse struct {
	AvailableTypes []ProviderTypeDescription     `json:"available_types"`
	Active         []ActiveDatasourceDescription `json:"active"`
}

type OnboardRequest struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type OnboardResponse struct {
	Type      string         `json:"type"`
	Tables    []string       `json:"tables"`
	Functions []string       `json:"functions"`
	Config    map[string]any `json:"config"`
}

// handleListDatasources implements GET /datasources: every compiled-in
// provider type (with its config schema) plus every currently active
// datasource instance.
func handleListDatasources(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error { return e.JSON(http.StatusOK, reg.List()) }
}

// handleOnboardDatasource implements POST /datasources: onboard (or
// idempotently reconfigure) a datasource instance of a compiled-in
// provider type.
func handleOnboardDatasource(reg *Registry) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req OnboardRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if req.Type == "" {
			return e.BadRequestError("type is required", nil)
		}

		result, err := reg.Onboard(e.App, req.Type, req.Config)
		switch {
		case err == nil:
			return e.JSON(http.StatusOK, result)
		case errors.Is(err, errUnknownProviderType), errors.Is(err, errInvalidConfig):
			return e.BadRequestError(err.Error(), nil)
		case errors.Is(err, errTableCollision):
			return e.Error(http.StatusConflict, err.Error(), nil)
		default:
			return e.InternalServerError("onboarding failed", err)
		}
	}
}
