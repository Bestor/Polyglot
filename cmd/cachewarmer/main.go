// Command cachewarmer periodically calls POST /warm for every player
// listed in a players file, so caches stay fresh without waiting on a
// live question to trigger a sync. It never blocks on a warm job's
// completion - polyglot's /warm is asynchronous - it only fires the
// requests and logs each job id.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"val-analyzer/internal/cachewarmer"
	"val-analyzer/internal/logging"
)

func main() {
	debug := getEnvBool("DEBUG", false)
	logging.Init(debug)

	// POLYGLOT_URL/POLYGLOT_AUTH_TOKEN name is unchanged from before the
	// two-binary split, but now points at cmd/valorantapi - the only
	// service with a /warm endpoint (see internal/polyglot/routes.go's
	// doc comment on why core polyglot itself no longer has one).
	valorantAPIURL := mustEnv("POLYGLOT_URL")
	authToken := mustEnv("POLYGLOT_AUTH_TOKEN")
	playersFile := getEnvDefault("PLAYERS_FILE", "cmd/cachewarmer/players.txt")
	function := getEnvDefault("WARM_FUNCTION", "sync_matches")

	interval, err := time.ParseDuration(getEnvDefault("WARM_INTERVAL", "1h"))
	if err != nil {
		log.Fatalf("invalid WARM_INTERVAL: %v", err)
	}

	client := cachewarmer.NewClient(valorantAPIURL, authToken)
	ctx := context.Background()

	log.Printf("cachewarmer: starting, valorant_api_url=%s players_file=%s function=%s interval=%s",
		valorantAPIURL, playersFile, function, interval)

	cachewarmer.RunPass(ctx, client, playersFile, function)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			cachewarmer.RunPass(ctx, client, playersFile, function)
		case <-stop:
			log.Print("cachewarmer: shutting down")
			return
		}
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
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
	return v == "1" || v == "true"
}
