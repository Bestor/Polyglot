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

// PlayerRef identifies a player as referenced from within a round/kill/damage
// event. Matches may include players never otherwise resolved by the
// caller (teammates, opponents), so events carry enough identity to
// opportunistically cache them too.
type PlayerRef struct {
	PUUID string
	Name  string
	Tag   string
	Team  string
}

// Weapon describes the weapon/ability/bomb involved in a kill or round of
// combat. ID may be empty (the bomb) or a non-UUID sentinel like
// "Ability1"/"Ultimate" for ability kills.
type Weapon struct {
	ID   string
	Name string
	Type string // "Weapon" | "Ability" | "Bomb"
}

// Location is a 2D map-space coordinate.
type Location struct {
	X int
	Y int
}

// PlayerLocation snapshots one alive player's position at the moment of a
// kill, plant, or defuse.
type PlayerLocation struct {
	Player      PlayerRef
	ViewRadians float64
	Location    Location
}

// DamageEvent aggregates the damage one player dealt to another within a
// single round. Attacker == Victim is valid (e.g. self/fall damage).
type DamageEvent struct {
	Victim    PlayerRef
	Damage    int
	Headshots int
	Bodyshots int
	Legshots  int
}

// RoundPlayerStat is one player's stat line for a single round, including
// the damage they dealt that round.
type RoundPlayerStat struct {
	Player          PlayerRef
	Score           int
	Kills           int
	Headshots       int
	Bodyshots       int
	Legshots        int
	LoadoutValue    int
	MoneyRemaining  int
	Weapon          Weapon
	ArmorID         string
	ArmorName       string
	WasAFK          bool
	ReceivedPenalty bool
	StayedInSpawn   bool
	DamageEvents    []DamageEvent
}

// PlantEvent describes the spike plant in a round, if one occurred.
type PlantEvent struct {
	RoundTimeMs     int
	Site            string
	Location        Location
	Player          PlayerRef
	PlayerLocations []PlayerLocation
}

// DefuseEvent describes the spike defuse in a round, if one occurred.
type DefuseEvent struct {
	RoundTimeMs     int
	Location        Location
	Player          PlayerRef
	PlayerLocations []PlayerLocation
}

// RoundDetail is the full detail of a single round within a match.
type RoundDetail struct {
	Number      int
	Result      string
	Ceremony    string
	WinningTeam string
	Plant       *PlantEvent
	Defuse      *DefuseEvent
	PlayerStats []RoundPlayerStat
}

// KillEvent is a single kill within a match.
type KillEvent struct {
	RoundNumber       int
	TimeInRoundMs     int
	TimeInMatchMs     int
	Killer            PlayerRef
	Victim            PlayerRef
	Assistants        []PlayerRef
	Location          Location
	Weapon            Weapon
	SecondaryFireMode bool
	PlayerLocations   []PlayerLocation
}

// TeamResult is one team's outcome in a match.
type TeamResult struct {
	TeamID     string
	RoundsWon  int
	RoundsLost int
	Won        bool
}

// PlayerMatchStats holds one player's match-total stats and identity as of
// that match.
type PlayerMatchStats struct {
	PUUID             string
	Name              string
	Tag               string
	Team              string
	PartyID           string
	Platform          string
	AgentID           string
	AgentName         string
	TierID            int
	TierName          string
	AccountLevel      int
	SessionPlaytimeMs int

	Score          int
	Kills          int
	Deaths         int
	Assists        int
	Headshots      int
	Bodyshots      int
	Legshots       int
	DamageDealt    int
	DamageReceived int

	CastsGrenade  int
	CastsAbility1 int
	CastsAbility2 int
	CastsUltimate int

	EconSpentOverall        int
	EconSpentAvg            float64
	EconLoadoutValueOverall int
	EconLoadoutValueAvg     float64

	AFKRounds            float64
	FriendlyFireIncoming float64
	FriendlyFireOutgoing float64
	RoundsInSpawn        float64
}

// MatchDetail is the full detail of a single match: metadata, per-player
// totals, per-team results, and the round-by-round and kill-by-kill
// breakdown.
type MatchDetail struct {
	MatchID       string
	Map           string
	MapID         string
	GameVersion   string
	QueueID       string
	QueueModeType string
	SeasonID      string
	Platform      string
	Region        string
	Cluster       string
	StartedAt     time.Time
	GameLengthMs  int
	IsCompleted   bool

	Teams   []TeamResult
	Players []PlayerMatchStats
	Rounds  []RoundDetail
	Kills   []KillEvent

	Raw []byte // full raw provider response, for future-proofing
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
	// GetMatchList returns up to size match list entries, most recent
	// first, starting at offset matches back from the most recent (0 =
	// the most recent match). Used to page backward through a player's
	// history when a sync needs matches older than the first page.
	GetMatchList(ctx context.Context, region, platform, puuid string, size, offset int) ([]MatchListEntry, error)
	GetMatch(ctx context.Context, region, matchID string) (MatchDetail, error)
	GetSeasons(ctx context.Context) ([]Season, error)
}
