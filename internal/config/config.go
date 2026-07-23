// Package config loads process configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port              string
	PBDataDir         string
	APIAuthToken      string
	SuperuserEmail    string
	SuperuserPassword string
	// VaultAddr/VaultToken/VaultUnsealKey configure polyglot's OpenBao
	// client (internal/vault) - required, like API_AUTH_TOKEN, since every
	// onboarded datasource's secrets live there, not in PocketBase's own
	// persisted config. VaultUnsealKey is required (not just VaultAddr/
	// VaultToken) because OpenBao's file backend starts sealed on every
	// real restart and this project auto-unseals rather than requiring a
	// manual `bao operator unseal` step each time - see internal/vault's
	// New for the operational trade-off this implies.
	VaultAddr      string
	VaultToken     string
	VaultUnsealKey string
	// Debug enables verbose (slog.Debug-level) logging: full SQL query
	// text, request/response payloads, and tool-call arguments. Off by
	// default since those can be large and noisy for routine operation.
	Debug bool
}

func Load() (Config, error) {
	cfg := Config{
		Port:              getEnvDefault("PORT", "8090"),
		PBDataDir:         getEnvDefault("PB_DATA_DIR", "pb_data"),
		APIAuthToken:      os.Getenv("API_AUTH_TOKEN"),
		SuperuserEmail:    os.Getenv("SUPERUSER_EMAIL"),
		SuperuserPassword: os.Getenv("SUPERUSER_PASSWORD"),
		VaultAddr:         os.Getenv("VAULT_ADDR"),
		VaultToken:        os.Getenv("VAULT_TOKEN"),
		VaultUnsealKey:    os.Getenv("VAULT_UNSEAL_KEY"),
		Debug:             getEnvBool("DEBUG", false),
	}

	if cfg.APIAuthToken == "" {
		return Config{}, fmt.Errorf("API_AUTH_TOKEN is required")
	}
	if cfg.VaultAddr == "" {
		return Config{}, fmt.Errorf("VAULT_ADDR is required")
	}
	if cfg.VaultToken == "" {
		return Config{}, fmt.Errorf("VAULT_TOKEN is required")
	}
	if cfg.VaultUnsealKey == "" {
		return Config{}, fmt.Errorf("VAULT_UNSEAL_KEY is required")
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
