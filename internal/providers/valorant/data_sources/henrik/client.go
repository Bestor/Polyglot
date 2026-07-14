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
	"strconv"
	"time"

	"val-analyzer/internal/providers/valorant/data_sources"
	"val-analyzer/internal/ratelimit"
)

// Rate-limit retry tuning: the local token-bucket limiter (ratelimit.Limiter)
// already throttles the steady-state request rate, but a burst of matches
// within a single sync can still occasionally exceed HenrikDev's actual
// enforcement. Rather than failing the whole sync on one 429, pause and
// retry the single request - honoring a Retry-After header when the API
// sends one, falling back to exponential backoff otherwise.
const (
	maxRateLimitRetries  = 5
	rateLimitBackoffBase = 2 * time.Second
	rateLimitBackoffCap  = 60 * time.Second
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

func (c *Client) GetMatchList(ctx context.Context, region, platform, puuid string, size, offset int) ([]data_sources.MatchListEntry, error) {
	if size <= 0 {
		size = maxMatchListSize
	}
	if size > maxMatchListSize {
		size = maxMatchListSize
	}

	var resp matchListResponse
	path := fmt.Sprintf("/valorant/v4/by-puuid/matches/%s/%s/%s", url.PathEscape(region), url.PathEscape(platform), url.PathEscape(puuid))
	query := url.Values{"size": {fmt.Sprintf("%d", size)}}
	if offset > 0 {
		// HenrikDev's v4 matches endpoints support paging back through a
		// player's history via ?start=INDEX (an offset in match count, not
		// a page number), in addition to size.
		query.Set("start", fmt.Sprintf("%d", offset))
	}
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

	players := make([]data_sources.PlayerMatchStats, 0, len(resp.Data.Players))
	for _, p := range resp.Data.Players {
		players = append(players, data_sources.PlayerMatchStats{
			PUUID:             p.PUUID,
			Name:              p.Name,
			Tag:               p.Tag,
			Team:              p.TeamID,
			PartyID:           p.PartyID,
			Platform:          p.Platform,
			AgentID:           p.Agent.ID,
			AgentName:         p.Agent.Name,
			TierID:            p.Tier.ID,
			TierName:          p.Tier.Name,
			AccountLevel:      p.AccountLevel,
			SessionPlaytimeMs: p.SessionPlaytimeMs,

			Score:          p.Stats.Score,
			Kills:          p.Stats.Kills,
			Deaths:         p.Stats.Deaths,
			Assists:        p.Stats.Assists,
			Headshots:      p.Stats.Headshots,
			Bodyshots:      p.Stats.Bodyshots,
			Legshots:       p.Stats.Legshots,
			DamageDealt:    p.Stats.Damage.Dealt,
			DamageReceived: p.Stats.Damage.Received,

			CastsGrenade:  p.AbilityCasts.Grenade,
			CastsAbility1: p.AbilityCasts.Ability1,
			CastsAbility2: p.AbilityCasts.Ability2,
			CastsUltimate: p.AbilityCasts.Ultimate,

			EconSpentOverall:        p.Economy.Spent.Overall,
			EconSpentAvg:            p.Economy.Spent.Average,
			EconLoadoutValueOverall: p.Economy.LoadoutValue.Overall,
			EconLoadoutValueAvg:     p.Economy.LoadoutValue.Average,

			AFKRounds:            p.Behavior.AFKRounds,
			FriendlyFireIncoming: p.Behavior.FriendlyFire.Incoming,
			FriendlyFireOutgoing: p.Behavior.FriendlyFire.Outgoing,
			RoundsInSpawn:        p.Behavior.RoundsInSpawn,
		})
	}

	teams := make([]data_sources.TeamResult, 0, len(resp.Data.Teams))
	for _, t := range resp.Data.Teams {
		teams = append(teams, data_sources.TeamResult{
			TeamID:     t.TeamID,
			RoundsWon:  t.Rounds.Won,
			RoundsLost: t.Rounds.Lost,
			Won:        t.Won,
		})
	}

	rounds := make([]data_sources.RoundDetail, 0, len(resp.Data.Rounds))
	for _, r := range resp.Data.Rounds {
		rounds = append(rounds, toRoundDetail(r))
	}

	kills := make([]data_sources.KillEvent, 0, len(resp.Data.Kills))
	for _, k := range resp.Data.Kills {
		kills = append(kills, toKillEvent(k))
	}

	return data_sources.MatchDetail{
		MatchID:       resp.Data.Metadata.MatchID,
		Map:           resp.Data.Metadata.Map.Name,
		MapID:         resp.Data.Metadata.Map.ID,
		GameVersion:   resp.Data.Metadata.GameVersion,
		QueueID:       resp.Data.Metadata.Queue.ID,
		QueueModeType: resp.Data.Metadata.Queue.ModeType,
		SeasonID:      resp.Data.Metadata.Season.ID,
		Platform:      resp.Data.Metadata.Platform,
		Region:        resp.Data.Metadata.Region,
		Cluster:       resp.Data.Metadata.Cluster,
		StartedAt:     startedAt,
		GameLengthMs:  resp.Data.Metadata.GameLengthMs,
		IsCompleted:   resp.Data.Metadata.IsCompleted,

		Teams:   teams,
		Players: players,
		Rounds:  rounds,
		Kills:   kills,

		Raw: raw,
	}, nil
}

func toPlayerRef(w wirePlayerRef) data_sources.PlayerRef {
	return data_sources.PlayerRef{PUUID: w.PUUID, Name: w.Name, Tag: w.Tag, Team: w.Team}
}

func toPlayerRefs(ws []wirePlayerRef) []data_sources.PlayerRef {
	refs := make([]data_sources.PlayerRef, 0, len(ws))
	for _, w := range ws {
		refs = append(refs, toPlayerRef(w))
	}
	return refs
}

func toLocation(w wireLocation) data_sources.Location {
	return data_sources.Location{X: w.X, Y: w.Y}
}

func toWeapon(w wireWeapon) data_sources.Weapon {
	return data_sources.Weapon{ID: w.ID, Name: w.Name, Type: w.Type}
}

func toPlayerLocations(ws []wirePlayerLocation) []data_sources.PlayerLocation {
	locs := make([]data_sources.PlayerLocation, 0, len(ws))
	for _, w := range ws {
		locs = append(locs, data_sources.PlayerLocation{
			Player:      toPlayerRef(w.Player),
			ViewRadians: w.ViewRadians,
			Location:    toLocation(w.Location),
		})
	}
	return locs
}

func toRoundDetail(r matchRound) data_sources.RoundDetail {
	detail := data_sources.RoundDetail{
		Number:      r.ID,
		Result:      r.Result,
		Ceremony:    r.Ceremony,
		WinningTeam: r.WinningTeam,
	}

	if r.Plant != nil {
		detail.Plant = &data_sources.PlantEvent{
			RoundTimeMs:     r.Plant.RoundTimeMs,
			Site:            r.Plant.Site,
			Location:        toLocation(r.Plant.Location),
			Player:          toPlayerRef(r.Plant.Player),
			PlayerLocations: toPlayerLocations(r.Plant.PlayerLocations),
		}
	}
	if r.Defuse != nil {
		detail.Defuse = &data_sources.DefuseEvent{
			RoundTimeMs:     r.Defuse.RoundTimeMs,
			Location:        toLocation(r.Defuse.Location),
			Player:          toPlayerRef(r.Defuse.Player),
			PlayerLocations: toPlayerLocations(r.Defuse.PlayerLocations),
		}
	}

	detail.PlayerStats = make([]data_sources.RoundPlayerStat, 0, len(r.Stats))
	for _, s := range r.Stats {
		var armorID, armorName string
		if s.Economy.Armor != nil {
			armorID, armorName = s.Economy.Armor.ID, s.Economy.Armor.Name
		}

		damageEvents := make([]data_sources.DamageEvent, 0, len(s.DamageEvents))
		for _, d := range s.DamageEvents {
			damageEvents = append(damageEvents, data_sources.DamageEvent{
				Victim:    toPlayerRef(d.Player),
				Damage:    d.Damage,
				Headshots: d.Headshots,
				Bodyshots: d.Bodyshots,
				Legshots:  d.Legshots,
			})
		}

		detail.PlayerStats = append(detail.PlayerStats, data_sources.RoundPlayerStat{
			Player:          toPlayerRef(s.Player),
			Score:           s.Stats.Score,
			Kills:           s.Stats.Kills,
			Headshots:       s.Stats.Headshots,
			Bodyshots:       s.Stats.Bodyshots,
			Legshots:        s.Stats.Legshots,
			LoadoutValue:    s.Economy.LoadoutValue,
			MoneyRemaining:  s.Economy.Remaining,
			Weapon:          toWeapon(s.Economy.Weapon),
			ArmorID:         armorID,
			ArmorName:       armorName,
			WasAFK:          s.WasAFK,
			ReceivedPenalty: s.ReceivedPenalty,
			StayedInSpawn:   s.StayedInSpawn,
			DamageEvents:    damageEvents,
		})
	}

	return detail
}

func toKillEvent(k matchKillEvent) data_sources.KillEvent {
	return data_sources.KillEvent{
		RoundNumber:       k.Round,
		TimeInRoundMs:     k.TimeInRoundMs,
		TimeInMatchMs:     k.TimeInMatchMs,
		Killer:            toPlayerRef(k.Killer),
		Victim:            toPlayerRef(k.Victim),
		Assistants:        toPlayerRefs(k.Assistants),
		Location:          toLocation(k.Location),
		Weapon:            toWeapon(k.Weapon),
		SecondaryFireMode: k.SecondaryFireMode,
		PlayerLocations:   toPlayerLocations(k.PlayerLocations),
	}
}

// zeroUUID is HenrikDev's sentinel for "no parent" (an episode's ParentID)
// rather than an omitted/empty field.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

func (c *Client) GetSeasons(ctx context.Context) ([]data_sources.Season, error) {
	var resp contentResponse
	query := url.Values{"locale": {"en-US"}}
	if err := c.doGet(ctx, "/valorant/v1/content", query, &resp); err != nil {
		return nil, err
	}

	seasons := make([]data_sources.Season, 0, len(resp.Data.Acts))
	for _, a := range resp.Data.Acts {
		parentID := a.ParentID
		if parentID == zeroUUID {
			parentID = ""
		}
		seasons = append(seasons, data_sources.Season{
			SeasonID:       a.ID,
			ShortCode:      a.Name,
			ParentSeasonID: parentID,
			IsActive:       a.IsActive,
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
// do so without a second round trip. A 429 response is retried in place
// (up to maxRateLimitRetries times) rather than immediately failing the
// call - see the rate-limit retry tuning comment above.
func (c *Client) doGetRaw(ctx context.Context, path string, query url.Values, out any) ([]byte, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
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

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			slog.Error("henrik: reading response body failed", "path", path, "error", readErr, "duration_ms", time.Since(start).Milliseconds())
			return nil, readErr
		}

		duration := time.Since(start)

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRateLimitRetries {
			wait := retryAfterDuration(resp.Header, attempt)
			slog.Warn("henrik: rate limited, pausing before retry", "path", path, "wait", wait, "attempt", attempt+1, "duration_ms", duration.Milliseconds())
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

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
}

// retryAfterDuration decides how long to pause before retrying a 429,
// honoring a Retry-After header (seconds or an HTTP date) when present and
// falling back to exponential backoff based on attempt otherwise. Always
// capped at rateLimitBackoffCap.
func retryAfterDuration(header http.Header, attempt int) time.Duration {
	if v := header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return capDuration(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				return capDuration(d)
			}
		}
	}

	return capDuration(rateLimitBackoffBase * time.Duration(1<<attempt))
}

func capDuration(d time.Duration) time.Duration {
	if d > rateLimitBackoffCap {
		return rateLimitBackoffCap
	}
	return d
}
