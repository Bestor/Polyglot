package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ingest"
)

// maxWarmMatches bounds how many matches a single /api/warm call will
// fetch. Unlike the AI's bounded per-question sync, cache warming is an
// explicit bulk-load operation, so it's allowed a much larger cap - the
// data source's rate limit (and doGetRaw's pause-and-retry-on-429
// behavior) is what actually paces the request volume, not this number.
const maxWarmMatches = 500

// handleWarm pre-loads a player's recent match history into the cache
// without going through the AI at all - useful for warming up a player
// before they're asked about, so a later /api/ask question doesn't pay
// the sync latency inline.
func handleWarm(ing *ingest.Service) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req WarmRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Tag) == "" {
			return e.BadRequestError("name and tag are required", nil)
		}

		count := req.Count
		if count <= 0 || count > maxWarmMatches {
			count = maxWarmMatches
		}

		ctx := e.Request.Context()

		player, err := ing.ResolvePlayer(ctx, req.Name, req.Tag)
		if err != nil {
			slog.Error("api: warm failed to resolve player", "name", req.Name, "tag", req.Tag, "error", err)
			return e.InternalServerError("failed to resolve player", err)
		}

		result, err := ing.SyncPlayerMatches(ctx, player, ingest.SyncOptions{MaxMatches: count})
		if err != nil {
			slog.Error("api: warm failed to sync matches", "name", req.Name, "tag", req.Tag, "puuid", player.PUUID, "error", err)
			return e.InternalServerError("failed to sync matches", err)
		}

		slog.Info("api: warm complete", "name", player.Name, "tag", player.Tag, "puuid", player.PUUID, "fetched", result.Fetched, "skipped", result.Skipped)

		return e.JSON(http.StatusOK, WarmResponse{
			PUUID:   player.PUUID,
			Name:    player.Name,
			Tag:     player.Tag,
			Region:  player.Region,
			Fetched: result.Fetched,
			Skipped: result.Skipped,
		})
	}
}
