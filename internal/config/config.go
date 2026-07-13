// Package config loads process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HenrikAPIKey       string
	HenrikBaseURL      string
	Port               string
	PBDataDir          string
	RateLimitPerMinute int
	RateLimitBurst     int
	APIAuthToken       string
	SuperuserEmail     string
	SuperuserPassword  string
	// Debug enables verbose (slog.Debug-level) logging: full SQL query
	// text, request/response payloads, and tool-call arguments. Off by
	// default since those can be large and noisy for routine operation.
	Debug bool
}

func Load() (Config, error) {
	cfg := Config{
		HenrikAPIKey:       os.Getenv("HENRIK_API_KEY"),
		HenrikBaseURL:      getEnvDefault("HENRIK_BASE_URL", "https://api.henrikdev.xyz"),
		Port:               getEnvDefault("PORT", "8090"),
		PBDataDir:          getEnvDefault("PB_DATA_DIR", "pb_data"),
		APIAuthToken:       os.Getenv("API_AUTH_TOKEN"),
		SuperuserEmail:     os.Getenv("SUPERUSER_EMAIL"),
		SuperuserPassword:  os.Getenv("SUPERUSER_PASSWORD"),
		Debug:              getEnvBool("DEBUG", false),
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
	if (cfg.SuperuserEmail == "") != (cfg.SuperuserPassword == "") {
		return Config{}, fmt.Errorf("SUPERUSER_EMAIL and SUPERUSER_PASSWORD must be set together")
	}

	return cfg, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}
