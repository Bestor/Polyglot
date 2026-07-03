// Package data_sources defines a provider-agnostic interface for fetching
// Valorant match data, plus the shared DTOs used between that interface and
// the ingest/cache layer. Concrete providers (e.g. HenrikDev) live in their
// own subpackages and implement Source, so swapping providers later doesn't
// require reworking the caching logic.
package data_sources

import (
	"context"
	"time"
)

// Account is a resolved Riot ID.
type Account struct {
	PUUID  string
	Name   string
	Tag    string
	Region string
}

// MatchListEntry is a lightweight reference to a match returned by a
// player's match history, before the full match detail has been fetched.
type MatchListEntry struct {
	MatchID   string
	StartedAt time.Time
}

// PlayerStats holds one player's per-match statistics.
type PlayerStats struct {
	PUUID          string
	Name           string
	Tag            string
	Agent          string
	Team           string
	Won            bool
	Kills          int
	Deaths         int
	Assists        int
	Headshots      int
	Bodyshots      int
	Legshots       int
	DamageMade     int
	DamageReceived int
	Score          int
}

// MatchDetail is the full detail of a single match, including every
// player's stats.
type MatchDetail struct {
	MatchID      string
	Map          string
	Mode         string
	Queue        string
	SeasonID     string
	StartedAt    time.Time
	RoundsPlayed int
	Players      []PlayerStats
	Raw          []byte // full raw provider response, for future-proofing
}

// Season describes a competitive season/act as reported by the provider.
type Season struct {
	SeasonID       string
	ShortCode      string
	ParentSeasonID string
	IsActive       bool
}

// Source is implemented by concrete Valorant data providers (e.g. HenrikDev,
// or the official Riot API in the future).
type Source interface {
	GetAccountByRiotID(ctx context.Context, name, tag string) (Account, error)
	// GetMatchList returns up to size match list entries, most recent first.
	GetMatchList(ctx context.Context, region, platform, puuid string, size int) ([]MatchListEntry, error)
	GetMatch(ctx context.Context, region, matchID string) (MatchDetail, error)
	GetSeasons(ctx context.Context) ([]Season, error)
}
