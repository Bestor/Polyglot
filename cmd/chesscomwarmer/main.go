// Command chesscomwarmer periodically calls POST /warm for every username
// listed in a watchlist file, so caches stay fresh without waiting on a
// live question to trigger a sync. It never blocks on a warm job's
// completion - chesscomapi's /warm is asynchronous - it only fires the
// requests and logs each job id. Mirrors cmd/cachewarmer exactly, built on
// the same shared internal/warmer package - the only domain-specific bit
// either binary needs is its own argsFor closure.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"val-analyzer/internal/logging"
	"val-analyzer/internal/warmer"
)

// usernameArgs maps one watchlist line (a chess.com username) to the args
// shape sync_games/resolve_player expect.
func usernameArgs(line string) map[string]any {
	return map[string]any{"username": line}
}

func main() {
	debug := getEnvBool("DEBUG", false)
	logging.Init(debug)

	chesscomAPIURL := mustEnv("CHESSCOM_API_URL")
	authToken := mustEnv("CHESSCOM_API_AUTH_TOKEN")
	usernamesFile := getEnvDefault("USERNAMES_FILE", "cmd/chesscomwarmer/usernames.txt")
	function := getEnvDefault("WARM_FUNCTION", "sync_games")

	interval, err := time.ParseDuration(getEnvDefault("WARM_INTERVAL", "1h"))
	if err != nil {
		log.Fatalf("invalid WARM_INTERVAL: %v", err)
	}

	client := warmer.NewClient(chesscomAPIURL, authToken)
	ctx := context.Background()

	log.Printf("chesscomwarmer: starting, chesscom_api_url=%s usernames_file=%s function=%s interval=%s",
		chesscomAPIURL, usernamesFile, function, interval)

	warmer.RunPass(ctx, client, usernamesFile, function, usernameArgs)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			warmer.RunPass(ctx, client, usernamesFile, function, usernameArgs)
		case <-stop:
			log.Print("chesscomwarmer: shutting down")
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
