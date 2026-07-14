// Package valorant is the DataProvider implementation for Valorant esports
// data, backed by the HenrikDev API. It wraps data_sources/ingest/store -
// the same caching/sync logic val-analyzer has always used - behind the
// generic internal/dataprovider.Provider/Instance interfaces so polyglot
// can host it (and, in the future, other domains) uniformly.
package valorant

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/providers/valorant/data_sources"
	"val-analyzer/internal/providers/valorant/data_sources/henrik"
	"val-analyzer/internal/providers/valorant/ingest"
	"val-analyzer/internal/providers/valorant/store"
	"val-analyzer/internal/ratelimit"
)

// Type is this provider's stable slug, used as the registry key and the
// datasource id.
const Type = "valorant"

const (
	defaultBaseURL         = "https://api.henrikdev.xyz"
	defaultRateLimitPerMin = 30
)

type Provider struct{}

var _ dataprovider.Provider = Provider{}

func (Provider) Type() string { return Type }

func (Provider) ConfigSchema() []dataprovider.ConfigField {
	return []dataprovider.ConfigField{
		{Name: "henrik_api_key", Type: "string", Required: true, Secret: true,
			Description: "HenrikDev API key."},
		{Name: "henrik_base_url", Type: "string",
			Description: "Base URL of the HenrikDev API. Defaults to " + defaultBaseURL + "."},
		{Name: "rate_limit_per_minute", Type: "integer",
			Description: "Outbound request budget to HenrikDev, per minute. Defaults to 30."},
	}
}

func (Provider) Tables() []dataprovider.TableSpec { return tables }

func (Provider) New(config map[string]any) (dataprovider.Instance, error) {
	apiKey, _ := config["henrik_api_key"].(string)
	if apiKey == "" {
		return nil, fmt.Errorf("valorant: henrik_api_key is required")
	}
	baseURL, _ := config["henrik_base_url"].(string)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	rpm := defaultRateLimitPerMin
	if n, ok := asPositiveInt(config["rate_limit_per_minute"]); ok {
		rpm = n
	}

	limiter := ratelimit.NewLimiter(rpm, rpm)
	return &Instance{source: henrik.NewClient(baseURL, apiKey, limiter)}, nil
}

// asPositiveInt handles both float64 (JSON-decoded config, e.g. from an
// onboarding HTTP request) and int (a Go-literal config, e.g. from
// cmd/polyglot's env-var auto-onboard bridge).
func asPositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n > 0
	case int:
		return n, n > 0
	default:
		return 0, false
	}
}

// Instance is a configured-but-not-yet-bound Valorant provider instance.
type Instance struct {
	source data_sources.Source
	ing    *ingest.Service
}

var _ dataprovider.Instance = (*Instance)(nil)

func (i *Instance) Bind(app core.App) error {
	i.ing = ingest.NewService(i.source,
		store.NewPlayerStore(app), store.NewMatchStore(app), store.NewSeasonStore(app))
	return nil
}

func (i *Instance) Functions() []dataprovider.Function {
	return []dataprovider.Function{resolvePlayerFunction(i.ing), syncMatchesFunction(i.ing), syncSeasonsFunction(i.ing)}
}
