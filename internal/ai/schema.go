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
	"matches":       "One row per cached Valorant match.",
	"match_players": "One row per player per match - the fact table for statistical questions. Join to matches on match_players.match = matches.id and to players on match_players.player = players.id.",
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
	"matches": {
		"match_id":      "Provider match identifier.",
		"map":           "Map name the match was played on.",
		"mode":          "Game mode label as reported by the provider.",
		"queue":         "Queue id, e.g. competitive, unrated.",
		"season":        "Relation to seasons.id; may be empty if the season hasn't been cached yet.",
		"season_id_raw": "The provider's season id for this match, always populated even when the season relation is empty.",
		"game_start":    "Match start timestamp.",
		"rounds_played": "Total rounds played in the match.",
		"raw_json":      "Full raw provider response for this match, kept for fields not otherwise modeled. Prefer the typed columns for aggregate queries.",
	},
	"match_players": {
		"match":              "Relation to matches.id.",
		"player":             "Relation to players.id.",
		"riot_name_snapshot": "Player's display name at the time this match was played (names can change later).",
		"riot_tag_snapshot":  "Player's tag at the time this match was played.",
		"agent":              "Agent (character) played.",
		"team":               "Team identifier, e.g. Red/Blue.",
		"won":                "Whether this player's team won the match.",
		"kills":              "Kills in this match.",
		"deaths":             "Deaths in this match.",
		"assists":            "Assists in this match.",
		"headshots":          "Headshot hit count. Headshot % = headshots / (headshots + bodyshots + legshots).",
		"bodyshots":          "Bodyshot hit count.",
		"legshots":           "Legshot hit count.",
		"damage_made":        "Total damage dealt by this player in this match.",
		"damage_received":    "Total damage received by this player in this match.",
		"score":              "Combat score in this match.",
	},
}

// tableNames is the fixed set of collections exposed to the AI, in a
// deliberate join-friendly order (referenced tables before referencing
// ones).
var tableNames = []string{"players", "seasons", "matches", "match_players"}

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
