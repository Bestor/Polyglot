// Package chesscom implements data_sources.Source against chess.com's
// Published Data API (https://www.chess.com/news/view/published-data-api):
// public, unauthenticated, no per-minute rate limit documented for serial
// (one-at-a-time) requests - only bursts of parallel requests risk a 429 -
// but chess.com's own guidance asks for a descriptive User-Agent (with
// contact info) so they can warn before blocking, which is why UserAgent
// is a required constructor argument, not optional.
package chesscom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"val-analyzer/internal/chesscom/data_sources"
	"val-analyzer/internal/ratelimit"
	"val-analyzer/internal/upstreamhttp"
)

type Client struct {
	fetch     *upstreamhttp.Client
	baseURL   string
	userAgent string
}

func NewClient(baseURL, userAgent string, limiter *ratelimit.Limiter) *Client {
	return &Client{
		fetch:     upstreamhttp.NewClient("chesscom", limiter, 15*time.Second),
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		userAgent: userAgent,
	}
}

var _ data_sources.Source = (*Client)(nil)

func (c *Client) headers() map[string]string {
	return map[string]string{
		"User-Agent": c.userAgent,
		"Accept":     "application/json",
	}
}

func (c *Client) GetPlayerProfile(ctx context.Context, username string) (data_sources.Profile, error) {
	var resp wireProfile
	path := fmt.Sprintf("/pub/player/%s", url.PathEscape(strings.ToLower(username)))
	if _, err := c.fetch.Get(ctx, c.baseURL+path, c.headers(), &resp); err != nil {
		return data_sources.Profile{}, err
	}

	return data_sources.Profile{
		PlayerID:    resp.PlayerID,
		Username:    resp.Username,
		Name:        resp.Name,
		Title:       resp.Title,
		CountryCode: lastPathSegment(resp.Country),
		Followers:   resp.Followers,
		JoinedAt:    unixOrZero(resp.Joined),
		Status:      resp.Status,
		AvatarURL:   resp.Avatar,
	}, nil
}

func (c *Client) GetPlayerStats(ctx context.Context, username string) ([]data_sources.RatingSnapshot, error) {
	var resp map[string]json.RawMessage
	path := fmt.Sprintf("/pub/player/%s/stats", url.PathEscape(strings.ToLower(username)))
	if _, err := c.fetch.Get(ctx, c.baseURL+path, c.headers(), &resp); err != nil {
		return nil, err
	}
	return parseStats(resp), nil
}

func (c *Client) GetArchiveList(ctx context.Context, username string) ([]data_sources.GameArchive, error) {
	var resp wireArchivesResponse
	path := fmt.Sprintf("/pub/player/%s/games/archives", url.PathEscape(strings.ToLower(username)))
	if _, err := c.fetch.Get(ctx, c.baseURL+path, c.headers(), &resp); err != nil {
		return nil, err
	}

	archives := make([]data_sources.GameArchive, 0, len(resp.Archives))
	for _, u := range resp.Archives {
		a, err := parseArchiveURL(u)
		if err != nil {
			return nil, err
		}
		archives = append(archives, a)
	}
	return archives, nil
}

func (c *Client) GetMonthGames(ctx context.Context, username string, year, month int) ([]data_sources.Game, error) {
	var resp wireMonthGamesResponse
	path := fmt.Sprintf("/pub/player/%s/games/%04d/%02d", url.PathEscape(strings.ToLower(username)), year, month)
	if _, err := c.fetch.Get(ctx, c.baseURL+path, c.headers(), &resp); err != nil {
		return nil, err
	}

	games := make([]data_sources.Game, 0, len(resp.Games))
	for _, g := range resp.Games {
		raw, err := json.Marshal(g)
		if err != nil {
			return nil, err
		}

		game := data_sources.Game{
			URL:         g.URL,
			UUID:        g.UUID,
			PGN:         g.PGN,
			TimeControl: g.TimeControl,
			EndTime:     unixOrZero(g.EndTime),
			Rated:       g.Rated,
			TimeClass:   g.TimeClass,
			Rules:       g.Rules,
			White:       data_sources.GamePlayer{Username: g.White.Username, Rating: g.White.Rating, Result: g.White.Result},
			Black:       data_sources.GamePlayer{Username: g.Black.Username, Rating: g.Black.Rating, Result: g.Black.Result},
			FEN:         g.FEN,
			ECO:         g.ECO,
			Raw:         raw,
		}
		if g.Accuracies != nil {
			white, black := g.Accuracies.White, g.Accuracies.Black
			game.WhiteAccuracy, game.BlackAccuracy = &white, &black
		}
		games = append(games, game)
	}
	return games, nil
}

// parseArchiveURL extracts (year, month) from an archive URL's trailing
// two path segments, e.g. ".../games/2024/01".
func parseArchiveURL(u string) (data_sources.GameArchive, error) {
	parts := strings.Split(strings.TrimRight(u, "/"), "/")
	if len(parts) < 2 {
		return data_sources.GameArchive{}, fmt.Errorf("unexpected archive url shape: %q", u)
	}
	yearStr, monthStr := parts[len(parts)-2], parts[len(parts)-1]
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return data_sources.GameArchive{}, fmt.Errorf("parsing archive year from %q: %w", u, err)
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil {
		return data_sources.GameArchive{}, fmt.Errorf("parsing archive month from %q: %w", u, err)
	}
	return data_sources.GameArchive{Year: year, Month: month}, nil
}

// lastPathSegment extracts the trailing path segment of a URL, e.g.
// "https://api.chess.com/pub/country/US" -> "US". Returns "" for an empty
// input rather than erroring - a profile with no country set is valid.
func lastPathSegment(u string) string {
	if u == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u, "/"), "/")
	return parts[len(parts)-1]
}

// unixOrZero converts a unix-seconds timestamp to time.Time, leaving the
// zero value alone for an absent (0) timestamp rather than mapping it to
// the unix epoch.
func unixOrZero(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
