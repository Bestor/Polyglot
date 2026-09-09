package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Port              string
	PBDataDir         string
	APIAuthToken      string
	ChesscomBaseURL   string
	ChesscomUserAgent string
	ChesscomRatePerM  int
	Debug             bool
}

const (
	defaultChesscomBaseURL = "https://api.chess.com"
	// defaultChesscomRatePerM is a courtesy limit, not an enforced upstream
	// cap - chess.com documents no per-minute rate limit for serial
	// (one-at-a-time) requests, only warning about bursts of parallel ones.
	defaultChesscomRatePerM = 60
)

// loadConfig reads chesscomapi's own, small set of env vars - not
// val-analyzer/internal/config, mirroring cmd/valorantapi/config.go's own
// precedent (each standalone Data API's required configuration barely
// overlaps with any other binary's).
func loadConfig() (config, error) {
	cfg := config{
		Port:              getEnvDefault("PORT", "8094"),
		PBDataDir:         getEnvDefault("PB_DATA_DIR", "pb_data"),
		APIAuthToken:      os.Getenv("API_AUTH_TOKEN"),
		ChesscomBaseURL:   getEnvDefault("CHESSCOM_BASE_URL", defaultChesscomBaseURL),
		ChesscomUserAgent: os.Getenv("CHESSCOM_USER_AGENT"),
		ChesscomRatePerM:  defaultChesscomRatePerM,
		Debug:             getEnvBool("DEBUG", false),
	}

	if cfg.APIAuthToken == "" {
		return config{}, fmt.Errorf("API_AUTH_TOKEN is required")
	}
	// Required, not just a courtesy nicety: chess.com's own published
	// guidance asks for contact info in the User-Agent so they can warn
	// before blocking - there's no API key here to enforce the same
	// "identify yourself" discipline HENRIK_API_KEY's own requiredness does.
	if cfg.ChesscomUserAgent == "" {
		return config{}, fmt.Errorf("CHESSCOM_USER_AGENT is required")
	}
	if v := os.Getenv("CHESSCOM_RATE_LIMIT_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return config{}, fmt.Errorf("invalid CHESSCOM_RATE_LIMIT_PER_MINUTE: %w", err)
		}
		cfg.ChesscomRatePerM = n
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
