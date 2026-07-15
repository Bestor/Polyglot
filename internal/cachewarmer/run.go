package cachewarmer

import (
	"context"
	"log/slog"
)

// RunPass reads playersFile fresh (so on-disk edits take effect on the
// very next tick without a restart) and fires one POST /warm per player
// tag, sequentially. Sequential is deliberate, not a missed optimization:
// each POST /warm call itself returns in milliseconds (it only enqueues
// a background job on polyglot's side - the actual slow work happens
// there, decoupled from this loop entirely), so there is no wall-clock
// benefit to firing them concurrently, only added complexity.
func RunPass(ctx context.Context, client *Client, playersFile, datasource, function string) {
	tags, err := ReadPlayerTags(playersFile)
	if err != nil {
		slog.Error("cachewarmer: reading players file, skipping this cycle", "path", playersFile, "error", err)
		return
	}
	if len(tags) == 0 {
		slog.Warn("cachewarmer: players file is empty or missing, skipping this cycle", "path", playersFile)
		return
	}

	slog.Info("cachewarmer: starting warm pass", "players", len(tags))
	for _, tag := range tags {
		jobID, err := client.Warm(ctx, datasource, function, tag)
		if err != nil {
			slog.Error("cachewarmer: warm request failed", "player_tag", tag, "error", err)
			continue
		}
		slog.Info("cachewarmer: warm job started", "player_tag", tag, "job_id", jobID)
	}
}
