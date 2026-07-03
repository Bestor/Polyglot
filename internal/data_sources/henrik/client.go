// Package henrik implements data_sources.Source against the unofficial
// HenrikDev Valorant API (https://docs.henrikdev.xyz).
package henrik

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"val-analyzer/internal/data_sources"
	"val-analyzer/internal/ratelimit"
)

type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
	limiter *ratelimit.Limiter
}

func NewClient(baseURL, apiKey string, limiter *ratelimit.Limiter) *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: baseURL,
		apiKey:  apiKey,
		limiter: limiter,
	}
}

var _ data_sources.Source = (*Client)(nil)

func (c *Client) GetAccountByRiotID(ctx context.Context, name, tag string) (data_sources.Account, error) {
	var resp accountResponse
	path := fmt.Sprintf("/valorant/v2/account/%s/%s", url.PathEscape(name), url.PathEscape(tag))
	if err := c.doGet(ctx, path, nil, &resp); err != nil {
		return data_sources.Account{}, err
	}

	return data_sources.Account{
		PUUID:  resp.Data.PUUID,
		Name:   resp.Data.Name,
		Tag:    resp.Data.Tag,
		Region: resp.Data.Region,
	}, nil
}

// maxMatchListSize is a conservative cap on the "size" query param.
// HenrikDev's matchlist endpoints are documented to reject sizes above a
// per-endpoint ceiling (commonly around 25-30); requesting more than this
// in one call risks a 400, so larger requests should be handled by the
// caller as multiple syncs over time rather than a single huge page.
const maxMatchListSize = 25

func (c *Client) GetMatchList(ctx context.Context, region, platform, puuid string, size int) ([]data_sources.MatchListEntry, error) {
	if size <= 0 {
		size = maxMatchListSize
	}
	if size > maxMatchListSize {
		size = maxMatchListSize
	}

	var resp matchListResponse
	path := fmt.Sprintf("/valorant/v4/by-puuid/matches/%s/%s/%s", url.PathEscape(region), url.PathEscape(platform), url.PathEscape(puuid))
	query := url.Values{"size": {fmt.Sprintf("%d", size)}}
	if err := c.doGet(ctx, path, query, &resp); err != nil {
		return nil, err
	}

	entries := make([]data_sources.MatchListEntry, 0, len(resp.Data))
	for _, m := range resp.Data {
		startedAt, _ := time.Parse(time.RFC3339, m.Metadata.StartedAt)
		entries = append(entries, data_sources.MatchListEntry{
			MatchID:   m.Metadata.MatchID,
			StartedAt: startedAt,
		})
	}
	return entries, nil
}

func (c *Client) GetMatch(ctx context.Context, region, matchID string) (data_sources.MatchDetail, error) {
	var resp matchResponse
	path := fmt.Sprintf("/valorant/v4/match/%s/%s", url.PathEscape(region), url.PathEscape(matchID))

	raw, err := c.doGetRaw(ctx, path, nil, &resp)
	if err != nil {
		return data_sources.MatchDetail{}, err
	}

	startedAt, _ := time.Parse(time.RFC3339, resp.Data.Metadata.StartedAt)

	players := make([]data_sources.PlayerStats, 0, len(resp.Data.Players))
	for _, p := range resp.Data.Players {
		players = append(players, data_sources.PlayerStats{
			PUUID:          p.PUUID,
			Name:           p.Name,
			Tag:            p.Tag,
			Agent:          p.Character.Name,
			Team:           p.TeamID,
			Won:            p.Won,
			Kills:          p.Stats.Kills,
			Deaths:         p.Stats.Deaths,
			Assists:        p.Stats.Assists,
			Headshots:      p.Stats.Headshots,
			Bodyshots:      p.Stats.Bodyshots,
			Legshots:       p.Stats.Legshots,
			DamageMade:     p.Stats.DamageMade,
			DamageReceived: p.Stats.DamageReceived,
			Score:          p.Stats.Score,
		})
	}

	return data_sources.MatchDetail{
		MatchID:      resp.Data.Metadata.MatchID,
		Map:          resp.Data.Metadata.Map.Name,
		Mode:         resp.Data.Metadata.Mode,
		Queue:        resp.Data.Metadata.Queue.ID,
		SeasonID:     resp.Data.Metadata.SeasonID,
		StartedAt:    startedAt,
		RoundsPlayed: resp.Data.Metadata.RoundsPlayed,
		Players:      players,
		Raw:          raw,
	}, nil
}

func (c *Client) GetSeasons(ctx context.Context) ([]data_sources.Season, error) {
	var resp contentResponse
	query := url.Values{"locale": {"en-US"}}
	if err := c.doGet(ctx, "/valorant/v1/content", query, &resp); err != nil {
		return nil, err
	}

	seasons := make([]data_sources.Season, 0, len(resp.Seasons))
	for _, s := range resp.Seasons {
		seasons = append(seasons, data_sources.Season{
			SeasonID:       s.ID,
			ParentSeasonID: s.ParentID,
			IsActive:       s.IsActive,
		})
	}
	return seasons, nil
}

func (c *Client) doGet(ctx context.Context, path string, query url.Values, out any) error {
	_, err := c.doGetRaw(ctx, path, query, out)
	return err
}

// doGetRaw performs the request and also returns the raw response body, so
// callers that want to persist the full payload (e.g. matches.raw_json) can
// do so without a second round trip.
func (c *Client) doGetRaw(ctx context.Context, path string, query url.Values, out any) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	start := time.Now()
	slog.Info("henrik: request", "path", path, "query", query.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("henrik: request failed", "path", path, "error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("henrik: reading response body failed", "path", path, "error", err, "duration_ms", time.Since(start).Milliseconds())
		return nil, err
	}

	duration := time.Since(start)

	if resp.StatusCode >= 400 {
		slog.Warn("henrik: request returned error status", "path", path, "status", resp.StatusCode, "duration_ms", duration.Milliseconds())
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	slog.Info("henrik: request complete", "path", path, "status", resp.StatusCode, "bytes", len(body), "duration_ms", duration.Milliseconds())

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return nil, fmt.Errorf("decoding henrikdev response: %w", err)
		}
	}

	return body, nil
}
