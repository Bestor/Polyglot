package ai

import (
	"github.com/pocketbase/pocketbase/core"
)

type ColumnDescription struct {
	Name        string
	Type        string
	Description string
}

type TableDescription struct {
	Name        string
	Description string
	Columns     []ColumnDescription
}

// tableNotes and columnNotes are hand-authored semantic descriptions merged
// onto the live, introspected schema. Anything not listed here still shows
// up with just its name/type - these are annotations, not the source of
// truth for structure.
var tableNotes = map[string]string{
	"players":       "One row per known Valorant player (identified by Riot PUUID), including anyone who has appeared in a cached match, not just players explicitly looked up.",
	"seasons":       "Reference table mapping a competitive season/act id to a human-readable label and date range. May be sparsely populated - matches.season_id_raw is always present even when a match's season row doesn't exist yet here.",
	"maps":          "Reference table of Valorant maps, opportunistically populated from observed matches.",
	"agents":        "Reference table of Valorant agents (characters), opportunistically populated from observed matches.",
	"weapons":       "Reference table of weapons, abilities, and the bomb, opportunistically populated from observed kills/rounds. weapon_id is empty for the bomb.",
	"tiers":         "Reference table of competitive rank tiers (0 = Unrated), opportunistically populated from observed matches.",
	"matches":       "One row per cached Valorant match.",
	"match_teams":   "One row per team per match (always 2 rows per match: Red and Blue).",
	"match_players": "One row per player per match - the main box-score fact table with match-total stats. Join to matches on match_players.match = matches.id and to players on match_players.player = players.id.",
	"rounds":        "One row per round per match - round outcome plus plant/defuse details.",
	"round_player_stats": "One row per player per round - the round-by-round box score (score, kills, shot placement, economy at round start). " +
		"Join to rounds on round_player_stats.round = rounds.id.",
	"damage_events": "One row per (round, attacker, victim) damage aggregate - total damage one player dealt another within a single round. " +
		"Self-damage rows exist (attacker = victim, e.g. bomb/fall damage).",
	"kills": "One row per kill event, with round timing, location, and weapon. Self-kills exist (killer = victim, e.g. bomb detonation or ability expiry) " +
		"- filter killer != victim for KDA-style analysis.",
	"kill_assists": "One row per assist per kill (0..n rows per kill in kills).",
	"event_player_locations": "One row per alive player's position snapshot at a kill, plant, or defuse event. High volume; only useful for positional/heatmap-style " +
		"analysis, not general stat questions. event_kill is set only for event_type='kill'.",
}

var columnNotes = map[string]map[string]string{
	"players": {
		"riot_puuid":           "Stable Riot player identifier; the true identity key (name/tag can change).",
		"riot_name":            "Last-known Riot display name (without the #tag).",
		"riot_tag":             "Last-known Riot tag (the part after #).",
		"region":               "Riot shard/region, e.g. na, eu, ap, kr, latam, br.",
		"last_synced_match_at": "Timestamp of the most recent match we've synced for this player; not a gameplay stat.",
	},
	"seasons": {
		"season_id":        "The season/act id as reported by the data provider.",
		"short_code":       "Short provider code for the season (e.g. an episode/act abbreviation).",
		"label":            "Hand-curated human-readable name, e.g. 'Episode 7 Act 3'. May be empty if not yet curated.",
		"parent_season_id": "If this row is an act, the season_id of its parent episode.",
		"is_active":        "Whether this is the currently active season per the data provider.",
	},
	"maps": {
		"map_id": "The provider's map id.",
		"name":   "Map name, e.g. 'Ascent', 'Lotus'.",
	},
	"agents": {
		"agent_id": "The provider's agent id.",
		"name":     "Agent name, e.g. 'Jett', 'Chamber'.",
	},
	"weapons": {
		"weapon_id": "The provider's weapon/ability id. Empty string for the bomb.",
		"name":      "Weapon/ability name, e.g. 'Vandal', 'Paint Shells'. May be empty.",
		"type":      "'Weapon' | 'Ability' | 'Bomb'.",
	},
	"tiers": {
		"tier_id": "Competitive rank tier number. 0 = Unrated.",
		"name":    "Rank name, e.g. 'Gold 3', 'Unrated'.",
	},
	"matches": {
		"match_id":        "Provider match identifier.",
		"map":             "Relation to maps.id.",
		"game_version":    "Game client version the match was played on.",
		"queue_id":        "Queue id, e.g. 'competitive', 'unrated'.",
		"queue_mode_type": "Queue mode type, e.g. 'Standard'.",
		"season":          "Relation to seasons.id; may be empty if the season hasn't been cached yet.",
		"season_id_raw":   "The provider's season id for this match, always populated even when the season relation is empty.",
		"platform":        "Platform the match was played on, e.g. 'pc'.",
		"region":          "Riot shard/region the match was played in.",
		"cluster":         "Game server cluster/location, e.g. 'US Central (Illinois)'.",
		"started_at":      "Match start timestamp.",
		"game_length_ms":  "Total match duration in milliseconds.",
		"is_completed":    "Whether the match ran to completion.",
		"raw_json":        "Full raw provider response for this match, kept for fields not otherwise modeled. Prefer the typed tables/columns for aggregate queries.",
	},
	"match_teams": {
		"match":       "Relation to matches.id.",
		"team_id":     "'Red' | 'Blue'.",
		"rounds_won":  "Rounds this team won.",
		"rounds_lost": "Rounds this team lost.",
		"won":         "Whether this team won the match.",
	},
	"match_players": {
		"match":                      "Relation to matches.id.",
		"player":                     "Relation to players.id.",
		"riot_name_snapshot":         "Player's display name at the time this match was played (names can change later).",
		"riot_tag_snapshot":          "Player's tag at the time this match was played.",
		"team":                       "Team identifier for this match, 'Red' | 'Blue'. Join to match_teams on (match, team) = (match, team_id) to get win/loss.",
		"party_id":                   "Groups players who queued together in the same party for this match.",
		"platform":                   "Platform this player played on, e.g. 'pc'.",
		"agent":                      "Relation to agents.id - the agent this player played.",
		"tier":                       "Relation to tiers.id - this player's competitive rank in this match.",
		"account_level":              "Player's account level at the time of the match.",
		"session_playtime_ms":        "How long the player's client session had been running, in milliseconds.",
		"won":                        "Whether this player's team won the match.",
		"score":                      "Combat score in this match.",
		"kills":                      "Kills in this match.",
		"deaths":                     "Deaths in this match.",
		"assists":                    "Assists in this match.",
		"headshots":                  "Headshot hit count. Headshot % = headshots / (headshots + bodyshots + legshots).",
		"bodyshots":                  "Bodyshot hit count.",
		"legshots":                   "Legshot hit count.",
		"damage_dealt":               "Total damage dealt by this player in this match.",
		"damage_received":            "Total damage received by this player in this match.",
		"casts_grenade":              "Number of times the grenade (C) ability was cast in this match.",
		"casts_ability1":             "Number of times ability 1 (Q) was cast in this match.",
		"casts_ability2":             "Number of times ability 2 (E) was cast in this match.",
		"casts_ultimate":             "Number of times the ultimate (X) was cast in this match.",
		"econ_spent_overall":         "Total creds spent across the match.",
		"econ_spent_avg":             "Average creds spent per round.",
		"econ_loadout_value_overall": "Total loadout value across the match.",
		"econ_loadout_value_avg":     "Average loadout value per round.",
		"afk_rounds":                 "Number of rounds this player was AFK for.",
		"friendly_fire_incoming":     "Friendly-fire damage received from teammates.",
		"friendly_fire_outgoing":     "Friendly-fire damage dealt to teammates.",
		"rounds_in_spawn":            "Number of rounds this player stayed in spawn.",
	},
	"rounds": {
		"match":          "Relation to matches.id.",
		"round_number":   "0-indexed round number within the match.",
		"result":         "'Elimination' | 'Defuse' | 'Detonate'.",
		"ceremony":       "Round-end ceremony, e.g. 'CeremonyClutch', 'CeremonyThrifty', 'CeremonyAce'.",
		"winning_team":   "'Red' | 'Blue' - the team that won this round.",
		"plant_time_ms":  "Time into the round the spike was planted, in milliseconds. Null if no plant occurred.",
		"plant_site":     "Bomb site planted, 'A' | 'B'. Null if no plant occurred.",
		"plant_x":        "Map-space X coordinate of the plant.",
		"plant_y":        "Map-space Y coordinate of the plant.",
		"planter":        "Relation to players.id - who planted the spike. Empty if no plant occurred.",
		"defuse_time_ms": "Time into the round the spike was defused, in milliseconds. Null if no defuse occurred.",
		"defuse_x":       "Map-space X coordinate of the defuse.",
		"defuse_y":       "Map-space Y coordinate of the defuse.",
		"defuser":        "Relation to players.id - who defused the spike. Empty if no defuse occurred.",
	},
	"round_player_stats": {
		"match":            "Relation to matches.id.",
		"round":            "Relation to rounds.id.",
		"round_number":     "0-indexed round number, denormalized from rounds for convenient filtering without a join.",
		"player":           "Relation to players.id.",
		"score":            "Combat score earned this round.",
		"kills":            "Kills this round.",
		"headshots":        "Headshot hits this round.",
		"bodyshots":        "Bodyshot hits this round.",
		"legshots":         "Legshot hits this round.",
		"loadout_value":    "This player's loadout value at the start of the round.",
		"money_remaining":  "Creds remaining after buying, at the start of the round.",
		"weapon":           "Relation to weapons.id - primary weapon held at the start of the round.",
		"armor_id":         "Provider id of the armor equipped, if any.",
		"armor_name":       "'Heavy Armor' | 'Light Armor' | 'Regen Shield', if any armor was equipped.",
		"was_afk":          "Whether this player was AFK during this round.",
		"received_penalty": "Whether this player received a penalty this round.",
		"stayed_in_spawn":  "Whether this player stayed in spawn this round.",
	},
	"damage_events": {
		"match":        "Relation to matches.id.",
		"round":        "Relation to rounds.id.",
		"round_number": "0-indexed round number, denormalized from rounds.",
		"attacker":     "Relation to players.id - who dealt the damage.",
		"victim":       "Relation to players.id - who received the damage. Can equal attacker (self/fall damage).",
		"damage":       "Total damage dealt in this (round, attacker, victim) aggregate.",
		"headshots":    "Headshot hits contributing to this damage total.",
		"bodyshots":    "Bodyshot hits contributing to this damage total.",
		"legshots":     "Legshot hits contributing to this damage total.",
	},
	"kills": {
		"match":               "Relation to matches.id.",
		"round":               "Relation to rounds.id.",
		"round_number":        "0-indexed round number, denormalized from rounds.",
		"time_in_round_ms":    "Time into the round the kill occurred, in milliseconds.",
		"time_in_match_ms":    "Time into the match the kill occurred, in milliseconds.",
		"killer":              "Relation to players.id.",
		"killer_team":         "Killer's team at the time, 'Red' | 'Blue'.",
		"victim":              "Relation to players.id.",
		"victim_team":         "Victim's team at the time, 'Red' | 'Blue'.",
		"weapon":              "Relation to weapons.id - weapon/ability/bomb used.",
		"secondary_fire_mode": "Whether the kill used the weapon's secondary fire mode.",
		"kill_x":              "Map-space X coordinate of the victim's death.",
		"kill_y":              "Map-space Y coordinate of the victim's death.",
	},
	"kill_assists": {
		"kill":     "Relation to kills.id.",
		"assister": "Relation to players.id.",
	},
	"event_player_locations": {
		"match":        "Relation to matches.id.",
		"round":        "Relation to rounds.id.",
		"round_number": "0-indexed round number, denormalized from rounds.",
		"event_type":   "'kill' | 'plant' | 'defuse'.",
		"event_kill":   "Relation to kills.id, set only when event_type='kill'.",
		"player":       "Relation to players.id - the alive player whose position this snapshot records.",
		"loc_x":        "Map-space X coordinate.",
		"loc_y":        "Map-space Y coordinate.",
		"view_radians": "Player's view direction in radians.",
	},
}

// tableNames is the fixed set of collections exposed to the AI, in a
// deliberate join-friendly order (referenced tables before referencing
// ones).
var tableNames = []string{
	"players", "seasons", "maps", "agents", "weapons", "tiers",
	"matches", "match_teams", "match_players",
	"rounds", "round_player_stats", "damage_events",
	"kills", "kill_assists", "event_player_locations",
}

// BuildSchema introspects the live PocketBase collections so the returned
// structure/types can never drift from the actual schema, and merges in the
// hand-authored notes above for semantic context.
func BuildSchema(app core.App) ([]TableDescription, error) {
	tables := make([]TableDescription, 0, len(tableNames))

	for _, name := range tableNames {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return nil, err
		}

		notes := columnNotes[name]
		columns := make([]ColumnDescription, 0, len(col.Fields))
		for _, f := range col.Fields {
			columns = append(columns, ColumnDescription{
				Name:        f.GetName(),
				Type:        f.Type(),
				Description: notes[f.GetName()],
			})
		}

		tables = append(tables, TableDescription{
			Name:        col.Name,
			Description: tableNotes[name],
			Columns:     columns,
		})
	}

	return tables, nil
}
