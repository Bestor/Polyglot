package warmer

import (
	"context"
	"log/slog"
)

// RunPass reads linesFile fresh (so on-disk edits take effect on the very
// next tick without a restart) and fires one POST /warm per identifier,
// sequentially. Sequential is deliberate, not a missed optimization: each
// POST /warm call itself returns in milliseconds (it only enqueues a
// background job on the Data API's side - the actual slow work happens
// there, decoupled from this loop entirely), so there is no wall-clock
// benefit to firing them concurrently, only added complexity.
//
// argsFor maps one identifier line to the args map that function expects
// (e.g. {"player_tag": line} for valorantapi, {"username": line} for
// chesscomapi) - the one piece of domain knowledge a warmer binary needs
// to supply, kept out of this package entirely.
func RunPass(ctx context.Context, client *Client, linesFile, function string, argsFor func(line string) map[string]any) {
	lines, err := ReadLines(linesFile)
	if err != nil {
		slog.Error("warmer: reading watchlist file, skipping this cycle", "path", linesFile, "error", err)
		return
	}
	if len(lines) == 0 {
		slog.Warn("warmer: watchlist file is empty or missing, skipping this cycle", "path", linesFile)
		return
	}

	slog.Info("warmer: starting warm pass", "identifiers", len(lines))
	for _, line := range lines {
		jobID, err := client.Warm(ctx, function, argsFor(line))
		if err != nil {
			slog.Error("warmer: warm request failed", "identifier", line, "error", err)
			continue
		}
		slog.Info("warmer: warm job started", "identifier", line, "job_id", jobID)
	}
}
