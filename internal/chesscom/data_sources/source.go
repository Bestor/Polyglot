// Package data_sources defines a provider-agnostic interface for fetching
// chess.com player/game data, plus the shared DTOs used between that
// interface and the ingest/cache layer. The one concrete provider
// (chess.com's own Published Data API) lives in its own subpackage and
// implements Source, mirroring internal/valorant/data_sources' own split
// between interface and concrete HenrikDev client.
package data_sources

import (
	"context"
	"time"
)

// Profile is a chess.com player's public profile.
type Profile struct {
	PlayerID    int64 // chess.com's stable numeric id - survives username changes
	Username    string
	Name        string
	Title       string // e.g. "GM"/"IM"/"FM" - empty for an untitled player
	CountryCode string
	Followers   int
	JoinedAt    time.Time
	Status      string // e.g. "basic"|"premium"|"closed"
	AvatarURL   string
}

// RatingSnapshot is one category's current standing, as reported by
// chess.com's stats endpoint. Which fields are populated depends on the
// category - see internal/chesscom's player_ratings migration for the
// per-category shape this maps onto: chess_bullet/blitz/rapid/daily
// populate every field, tactics only BestRating+BestDate, puzzle_rush only
// BestRating+Attempts, fide only LastRating.
type RatingSnapshot struct {
	Category    string
	LastRating  int
	LastRD      int
	LastDate    time.Time
	BestRating  int
	BestDate    time.Time
	BestGameURL string
	Wins        int
	Losses      int
	Draws       int
	Attempts    int // puzzle_rush only
}

// GameArchive identifies one monthly archive of a player's completed
// games. Every month except the current calendar one is immutable once
// chess.com closes it out.
type GameArchive struct {
	Year, Month int
}

// GamePlayer is one side's identity/result within a single game.
type GamePlayer struct {
	Username string
	Rating   int
	Result   string // chess.com's raw result string: "win"|"checkmated"|"resigned"|"timeout"|"agreed"|...
}

// Game is one completed game's facts, as reported by a monthly archive.
// The move-by-move detail lives only in PGN - see internal/chesscom/pgn.
type Game struct {
	URL         string
	UUID        string
	PGN         string
	TimeControl string
	EndTime     time.Time // chess.com reports only when a game ended, never when it started
	Rated       bool
	TimeClass   string // "bullet"|"blitz"|"rapid"|"daily"
	Rules       string // "chess" or a variant name
	White       GamePlayer
	Black       GamePlayer
	FEN         string
	ECO         string
	// WhiteAccuracy/BlackAccuracy are nil when the game was never analyzed
	// by chess.com (most games) - only some are, and only after the fact.
	WhiteAccuracy *float64
	BlackAccuracy *float64
	Raw           []byte // full raw provider response for this one game, for future-proofing
}

// Source is implemented by a concrete chess.com Published Data API client.
// No account-resolution method: a username is the identifier directly,
// unlike Valorant's Riot ID -> PUUID indirection.
type Source interface {
	GetPlayerProfile(ctx context.Context, username string) (Profile, error)
	GetPlayerStats(ctx context.Context, username string) ([]RatingSnapshot, error)
	// GetArchiveList returns every month this player has games in, oldest
	// first - the complete, finite list chess.com itself exposes, unlike
	// Valorant's offset-paged "most recent N" match list.
	GetArchiveList(ctx context.Context, username string) ([]GameArchive, error)
	GetMonthGames(ctx context.Context, username string, year, month int) ([]Game, error)
}
