package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/ingest"
)

// maxMatchesPerRequest bounds how many uncached matches a single /api/ask
// call will fetch (per player) from the data source, so one request can
// never block unboundedly under the provider's rate limit.
const maxMatchesPerRequest = 50

func handleAsk(ing *ingest.Service, schema []ai.TableDescription, query ai.QueryFunc, provider ai.Provider) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var req AskRequest
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("invalid request body", err)
		}
		if strings.TrimSpace(req.Question) == "" {
			return e.BadRequestError("question is required", nil)
		}
		if len(req.Players) == 0 {
			return e.BadRequestError("at least one player is required", nil)
		}

		ctx := e.Request.Context()

		var hints []string
		matchesSynced := 0
		playerRegions := make(map[string]string, len(req.Players))
		for _, pr := range req.Players {
			if pr.Name == "" || pr.Tag == "" || pr.Region == "" {
				return e.BadRequestError("each player requires name, tag, and region", nil)
			}

			player, err := ing.ResolvePlayer(ctx, pr.Name, pr.Tag, pr.Region)
			if err != nil {
				slog.Error("api: failed to resolve player", "name", pr.Name, "tag", pr.Tag, "region", pr.Region, "error", err)
				return e.InternalServerError(fmt.Sprintf("failed to resolve player %s#%s", pr.Name, pr.Tag), err)
			}

			result, err := ing.SyncPlayerMatches(ctx, player, pr.Region, ingest.SyncOptions{MaxMatches: maxMatchesPerRequest})
			if err != nil {
				slog.Error("api: failed to sync matches", "name", pr.Name, "tag", pr.Tag, "region", pr.Region, "error", err)
				return e.InternalServerError(fmt.Sprintf("failed to sync matches for %s#%s", pr.Name, pr.Tag), err)
			}
			matchesSynced += result.Fetched
			playerRegions[player.PUUID] = pr.Region

			hints = append(hints, fmt.Sprintf("players.riot_puuid = %q identifies %s#%s", player.PUUID, pr.Name, pr.Tag))
		}

		syncMore := func(ctx context.Context, puuid string, count int) (ai.SyncOutcome, error) {
			region, ok := playerRegions[puuid]
			if !ok {
				err := fmt.Errorf("puuid %q was not one of the players resolved for this request", puuid)
				slog.Error("api: sync_more_matches rejected", "puuid", puuid, "error", err)
				return ai.SyncOutcome{}, err
			}

			result, err := ing.SyncMoreByPUUID(ctx, puuid, region, ingest.SyncOptions{MaxMatches: count})
			if err != nil {
				slog.Error("api: sync_more_matches failed", "puuid", puuid, "region", region, "count", count, "error", err)
				return ai.SyncOutcome{}, err
			}
			matchesSynced += result.Fetched

			return ai.SyncOutcome{Fetched: result.Fetched, Skipped: result.Skipped}, nil
		}

		resp, err := provider.Answer(ctx, ai.Request{
			Question: req.Question,
			Schema:   schema,
			Hints:    hints,
			Query:    query,
			SyncMore: syncMore,
		})
		if err != nil {
			slog.Error("api: provider failed to answer question", "question", req.Question, "error", err)
			return e.InternalServerError("failed to answer question", err)
		}

		return e.JSON(http.StatusOK, AskResponse{
			Answer:        resp.Answer,
			MatchesSynced: matchesSynced,
		})
	}
}
