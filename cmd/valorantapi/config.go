package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Port           string
	PBDataDir      string
	APIAuthToken   string
	HenrikAPIKey   string
	HenrikBaseURL  string
	HenrikRatePerM int
	Debug          bool
}

const (
	defaultHenrikBaseURL  = "https://api.henrikdev.xyz"
	defaultHenrikRatePerM = 30
)

// loadConfig reads valorantapi's own, small set of env vars - not
// val-analyzer/internal/config, since the two binaries' required
// configuration barely overlaps anymore (core polyglot needs vault
// settings it has no use for; valorantapi needs HenrikDev settings core
// polyglot has no use for).
func loadConfig() (config, error) {
	cfg := config{
		Port:           getEnvDefault("PORT", "8093"),
		PBDataDir:      getEnvDefault("PB_DATA_DIR", "pb_data"),
		APIAuthToken:   os.Getenv("API_AUTH_TOKEN"),
		HenrikAPIKey:   os.Getenv("HENRIK_API_KEY"),
		HenrikBaseURL:  getEnvDefault("HENRIK_BASE_URL", defaultHenrikBaseURL),
		HenrikRatePerM: defaultHenrikRatePerM,
		Debug:          getEnvBool("DEBUG", false),
	}

	if cfg.APIAuthToken == "" {
		return config{}, fmt.Errorf("API_AUTH_TOKEN is required")
	}
	if cfg.HenrikAPIKey == "" {
		return config{}, fmt.Errorf("HENRIK_API_KEY is required")
	}
	if v := os.Getenv("HENRIK_RATE_LIMIT_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid HENRIK_RATE_LIMIT_PER_MINUTE: %w", err)
		}
		cfg.HenrikRatePerM = n
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
