// Package config loads process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	HenrikAPIKey       string
	HenrikBaseURL      string
	Port               string
	PBDataDir          string
	RateLimitPerMinute int
	RateLimitBurst     int
	AIProvider         string
	AnthropicAPIKey    string
	APIAuthToken       string
}

func Load() (Config, error) {
	cfg := Config{
		HenrikAPIKey:       os.Getenv("HENRIK_API_KEY"),
		HenrikBaseURL:      getEnvDefault("HENRIK_BASE_URL", "https://api.henrikdev.xyz"),
		Port:               getEnvDefault("PORT", "8090"),
		PBDataDir:          getEnvDefault("PB_DATA_DIR", "pb_data"),
		AIProvider:         getEnvDefault("AI_PROVIDER", "mock"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		APIAuthToken:       os.Getenv("API_AUTH_TOKEN"),
		RateLimitPerMinute: 30,
		RateLimitBurst:     30,
	}

	if v := os.Getenv("HENRIK_RATE_LIMIT_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid HENRIK_RATE_LIMIT_PER_MINUTE: %w", err)
		}
		cfg.RateLimitPerMinute = n
		cfg.RateLimitBurst = n
	}

	if cfg.HenrikAPIKey == "" {
		return Config{}, fmt.Errorf("HENRIK_API_KEY is required")
	}
	if cfg.APIAuthToken == "" {
		return Config{}, fmt.Errorf("API_AUTH_TOKEN is required")
	}
	if cfg.AIProvider == "claude" && cfg.AnthropicAPIKey == "" {
		return Config{}, fmt.Errorf("ANTHROPIC_API_KEY is required when AI_PROVIDER=claude")
	}

	return cfg, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
