package valorant

import "val-analyzer/internal/dataprovider"

// tables mirrors, table-by-table and field-by-field, the 15 domain
// collections created by internal/migrations (players.go through
// event_player_locations.go, plus the players.history_exhausted addition
// in 1750000017). Since those collections already exist, ensureTable
// (internal/polyglot/registry.go) finds every one of these via
// FindCollectionByNameOrId and never actually creates anything from this
// data - it's used only to (a) merge Description onto live-introspected
// columns for GET /metadata, and (b) claim table-name ownership in the
// onboarding collision registry. Order matches internal/migrations'
// numbering (dependency-safe: referenced tables before referencing ones).
var tables = []dataprovider.TableSpec{
	{
		Name: "players",
		Description: "One row per known Valorant player (identified by Riot PUUID), including anyone who has appeared in a cached match, " +
			"not just players explicitly looked up.",
		Fields: []dataprovider.FieldSpec{
			{Name: "riot_puuid", Type: dataprovider.FieldText, Required: true,
				Description: "Stable Riot player identifier; the true identity key (name/tag can change)."},
			{Name: "riot_name", Type: dataprovider.FieldText, Required: true,
				Description: "Last-known Riot display name (without the #tag)."},
			{Name: "riot_tag", Type: dataprovider.FieldText, Required: true,
				Description: "Last-known Riot tag (the part after #)."},
			{Name: "region", Type: dataprovider.FieldText, Max: 16,
				Description: "Riot shard/region, e.g. na, eu, ap, kr, latam, br."},
			{Name: "last_synced_match_at", Type: dataprovider.FieldDate,
				Description: "Timestamp of the most recent match we've synced for this player; not a gameplay stat."},
			{Name: "history_exhausted", Type: dataprovider.FieldBool,
				Description: "One-way flag: true once a backward sync has, at least once, walked all the way back to this player's true first match."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_players_puuid", Unique: true, Columns: []string{"riot_puuid"}},
		},
	},
	{
		Name: "seasons",
		Description: "Reference table mapping a competitive season/act id to a human-readable label and date range. May be sparsely " +
			"populated - matches.season_id_raw is always present even when a match's season row doesn't exist yet here.",
		Fields: []dataprovider.FieldSpec{
			{Name: "season_id", Type: dataprovider.FieldText, Required: true, Description: "The season/act id as reported by the data provider."},
			{Name: "short_code", Type: dataprovider.FieldText, Description: "Short provider code for the season (e.g. an episode/act abbreviation)."},
			{Name: "label", Type: dataprovider.FieldText, Description: "Hand-curated human-readable name, e.g. 'Episode 7 Act 3'. May be empty if not yet curated."},
			{Name: "parent_season_id", Type: dataprovider.FieldText, Description: "If this row is an act, the season_id of its parent episode."},
			{Name: "is_active", Type: dataprovider.FieldBool, Description: "Whether this is the currently active season per the data provider."},
			{Name: "start_date", Type: dataprovider.FieldDate},
			{Name: "end_date", Type: dataprovider.FieldDate},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_seasons_season_id", Unique: true, Columns: []string{"season_id"}},
		},
	},
	{
		Name:        "maps",
		Description: "Reference table of Valorant maps, opportunistically populated from observed matches.",
		Fields: []dataprovider.FieldSpec{
			{Name: "map_id", Type: dataprovider.FieldText, Required: true, Description: "The provider's map id."},
			{Name: "name", Type: dataprovider.FieldText, Description: "Map name, e.g. 'Ascent', 'Lotus'."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_maps_map_id", Unique: true, Columns: []string{"map_id"}},
		},
	},
	{
		Name:        "agents",
		Description: "Reference table of Valorant agents (characters), opportunistically populated from observed matches.",
		Fields: []dataprovider.FieldSpec{
			{Name: "agent_id", Type: dataprovider.FieldText, Required: true, Description: "The provider's agent id."},
			{Name: "name", Type: dataprovider.FieldText, Description: "Agent name, e.g. 'Jett', 'Chamber'."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_agents_agent_id", Unique: true, Columns: []string{"agent_id"}},
		},
	},
	{
		Name:        "weapons",
		Description: "Reference table of weapons, abilities, and the bomb, opportunistically populated from observed kills/rounds. weapon_id is empty for the bomb.",
		Fields: []dataprovider.FieldSpec{
			{Name: "weapon_id", Type: dataprovider.FieldText, Description: "The provider's weapon/ability id. Empty string for the bomb."},
			{Name: "name", Type: dataprovider.FieldText, Description: "Weapon/ability name, e.g. 'Vandal', 'Paint Shells'. May be empty."},
			{Name: "type", Type: dataprovider.FieldText, Description: "'Weapon' | 'Ability' | 'Bomb'."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_weapons_weapon_id", Unique: true, Columns: []string{"weapon_id"}},
		},
	},
	{
		Name:        "tiers",
		Description: "Reference table of competitive rank tiers (0 = Unrated), opportunistically populated from observed matches.",
		Fields: []dataprovider.FieldSpec{
			{Name: "tier_id", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Competitive rank tier number. 0 = Unrated."},
			{Name: "name", Type: dataprovider.FieldText, Description: "Rank name, e.g. 'Gold 3', 'Unrated'."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_tiers_tier_id", Unique: true, Columns: []string{"tier_id"}},
		},
	},
	{
		Name:        "matches",
		Description: "One row per cached Valorant match.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match_id", Type: dataprovider.FieldText, Required: true, Description: "Provider match identifier."},
			{Name: "map", Type: dataprovider.FieldRelation, RelationTable: "maps", Description: "Relation to maps.id."},
			{Name: "game_version", Type: dataprovider.FieldText, Description: "Game client version the match was played on."},
			{Name: "queue_id", Type: dataprovider.FieldText, Description: "Queue id, e.g. 'competitive', 'unrated'."},
			{Name: "queue_mode_type", Type: dataprovider.FieldText, Description: "Queue mode type, e.g. 'Standard'."},
			{Name: "season", Type: dataprovider.FieldRelation, RelationTable: "seasons", Description: "Relation to seasons.id; may be empty if the season hasn't been cached yet."},
			{Name: "season_id_raw", Type: dataprovider.FieldText, Description: "The provider's season id for this match, always populated even when the season relation is empty."},
			{Name: "platform", Type: dataprovider.FieldText, Description: "Platform the match was played on, e.g. 'pc'."},
			{Name: "region", Type: dataprovider.FieldText, Description: "Riot shard/region the match was played in."},
			{Name: "cluster", Type: dataprovider.FieldText, Description: "Game server cluster/location, e.g. 'US Central (Illinois)'."},
			{Name: "started_at", Type: dataprovider.FieldDate, Description: "Match start timestamp."},
			{Name: "game_length_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total match duration in milliseconds."},
			{Name: "is_completed", Type: dataprovider.FieldBool, Description: "Whether the match ran to completion."},
			{Name: "raw_json", Type: dataprovider.FieldJSON, MaxSize: 4 << 20,
				Description: "Full raw provider response for this match, kept for fields not otherwise modeled. Prefer the typed tables/columns for aggregate queries."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_matches_match_id", Unique: true, Columns: []string{"match_id"}},
			{Name: "idx_matches_started_at", Columns: []string{"started_at"}},
			{Name: "idx_matches_season", Columns: []string{"season"}},
			{Name: "idx_matches_map", Columns: []string{"map"}},
		},
	},
	{
		Name:        "match_teams",
		Description: "One row per team per match (always 2 rows per match: Red and Blue).",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "team_id", Type: dataprovider.FieldText, Required: true, Description: "'Red' | 'Blue'."},
			{Name: "rounds_won", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Rounds this team won."},
			{Name: "rounds_lost", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Rounds this team lost."},
			{Name: "won", Type: dataprovider.FieldBool, Description: "Whether this team won the match."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_match_teams_match_team", Unique: true, Columns: []string{"match", "team_id"}},
		},
	},
	{
		Name: "match_players",
		Description: "One row per player per match - the main box-score fact table with match-total stats. Join to matches on " +
			"match_players.match = matches.id and to players on match_players.player = players.id.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "player", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, CascadeDelete: true, Description: "Relation to players.id."},
			{Name: "riot_name_snapshot", Type: dataprovider.FieldText, Description: "Player's display name at the time this match was played (names can change later)."},
			{Name: "riot_tag_snapshot", Type: dataprovider.FieldText, Description: "Player's tag at the time this match was played."},
			{Name: "team", Type: dataprovider.FieldText, Description: "Team identifier for this match, 'Red' | 'Blue'. Join to match_teams on (match, team) = (match, team_id) to get win/loss."},
			{Name: "party_id", Type: dataprovider.FieldText, Description: "Groups players who queued together in the same party for this match."},
			{Name: "platform", Type: dataprovider.FieldText, Description: "Platform this player played on, e.g. 'pc'."},
			{Name: "agent", Type: dataprovider.FieldRelation, RelationTable: "agents", Description: "Relation to agents.id - the agent this player played."},
			{Name: "tier", Type: dataprovider.FieldRelation, RelationTable: "tiers", Description: "Relation to tiers.id - this player's competitive rank in this match."},
			{Name: "account_level", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Player's account level at the time of the match."},
			{Name: "session_playtime_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "How long the player's client session had been running, in milliseconds."},
			{Name: "won", Type: dataprovider.FieldBool, Description: "Whether this player's team won the match."},
			{Name: "score", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Combat score in this match."},
			{Name: "kills", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Kills in this match."},
			{Name: "deaths", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Deaths in this match."},
			{Name: "assists", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Assists in this match."},
			{Name: "headshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Headshot hit count. Headshot % = headshots / (headshots + bodyshots + legshots)."},
			{Name: "bodyshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Bodyshot hit count."},
			{Name: "legshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Legshot hit count."},
			{Name: "damage_dealt", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total damage dealt by this player in this match."},
			{Name: "damage_received", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total damage received by this player in this match."},
			{Name: "casts_grenade", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Number of times the grenade (C) ability was cast in this match."},
			{Name: "casts_ability1", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Number of times ability 1 (Q) was cast in this match."},
			{Name: "casts_ability2", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Number of times ability 2 (E) was cast in this match."},
			{Name: "casts_ultimate", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Number of times the ultimate (X) was cast in this match."},
			{Name: "econ_spent_overall", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total creds spent across the match."},
			{Name: "econ_spent_avg", Type: dataprovider.FieldNumber, Description: "Average creds spent per round."},
			{Name: "econ_loadout_value_overall", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total loadout value across the match."},
			{Name: "econ_loadout_value_avg", Type: dataprovider.FieldNumber, Description: "Average loadout value per round."},
			{Name: "afk_rounds", Type: dataprovider.FieldNumber, Description: "Number of rounds this player was AFK for."},
			{Name: "friendly_fire_incoming", Type: dataprovider.FieldNumber, Description: "Friendly-fire damage received from teammates."},
			{Name: "friendly_fire_outgoing", Type: dataprovider.FieldNumber, Description: "Friendly-fire damage dealt to teammates."},
			{Name: "rounds_in_spawn", Type: dataprovider.FieldNumber, Description: "Number of rounds this player stayed in spawn."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_match_players_match_player", Unique: true, Columns: []string{"match", "player"}},
			{Name: "idx_match_players_player", Columns: []string{"player"}},
		},
	},
	{
		Name:        "rounds",
		Description: "One row per round per match - round outcome plus plant/defuse details.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "round_number", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "0-indexed round number within the match."},
			{Name: "result", Type: dataprovider.FieldText, Description: "'Elimination' | 'Defuse' | 'Detonate'."},
			{Name: "ceremony", Type: dataprovider.FieldText, Description: "Round-end ceremony, e.g. 'CeremonyClutch', 'CeremonyThrifty', 'CeremonyAce'."},
			{Name: "winning_team", Type: dataprovider.FieldText, Description: "'Red' | 'Blue' - the team that won this round."},
			{Name: "plant_time_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Time into the round the spike was planted, in milliseconds. Null if no plant occurred."},
			{Name: "plant_site", Type: dataprovider.FieldText, Description: "Bomb site planted, 'A' | 'B'. Null if no plant occurred."},
			{Name: "plant_x", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space X coordinate of the plant."},
			{Name: "plant_y", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space Y coordinate of the plant."},
			{Name: "planter", Type: dataprovider.FieldRelation, RelationTable: "players", Description: "Relation to players.id - who planted the spike. Empty if no plant occurred."},
			{Name: "defuse_time_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Time into the round the spike was defused, in milliseconds. Null if no defuse occurred."},
			{Name: "defuse_x", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space X coordinate of the defuse."},
			{Name: "defuse_y", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space Y coordinate of the defuse."},
			{Name: "defuser", Type: dataprovider.FieldRelation, RelationTable: "players", Description: "Relation to players.id - who defused the spike. Empty if no defuse occurred."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_rounds_match_round", Unique: true, Columns: []string{"match", "round_number"}},
		},
	},
	{
		Name: "round_player_stats",
		Description: "One row per player per round - the round-by-round box score (score, kills, shot placement, economy at round start). " +
			"Join to rounds on round_player_stats.round = rounds.id.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "round", Type: dataprovider.FieldRelation, RelationTable: "rounds", Required: true, CascadeDelete: true, Description: "Relation to rounds.id."},
			{Name: "round_number", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "0-indexed round number, denormalized from rounds for convenient filtering without a join."},
			{Name: "player", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id."},
			{Name: "score", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Combat score earned this round."},
			{Name: "kills", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Kills this round."},
			{Name: "headshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Headshot hits this round."},
			{Name: "bodyshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Bodyshot hits this round."},
			{Name: "legshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Legshot hits this round."},
			{Name: "loadout_value", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "This player's loadout value at the start of the round."},
			{Name: "money_remaining", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Creds remaining after buying, at the start of the round."},
			{Name: "weapon", Type: dataprovider.FieldRelation, RelationTable: "weapons", Description: "Relation to weapons.id - primary weapon held at the start of the round."},
			{Name: "armor_id", Type: dataprovider.FieldText, Description: "Provider id of the armor equipped, if any."},
			{Name: "armor_name", Type: dataprovider.FieldText, Description: "'Heavy Armor' | 'Light Armor' | 'Regen Shield', if any armor was equipped."},
			{Name: "was_afk", Type: dataprovider.FieldBool, Description: "Whether this player was AFK during this round."},
			{Name: "received_penalty", Type: dataprovider.FieldBool, Description: "Whether this player received a penalty this round."},
			{Name: "stayed_in_spawn", Type: dataprovider.FieldBool, Description: "Whether this player stayed in spawn this round."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_round_player_stats_round_player", Unique: true, Columns: []string{"round", "player"}},
			{Name: "idx_round_player_stats_player", Columns: []string{"player"}},
		},
	},
	{
		Name: "damage_events",
		Description: "One row per (round, attacker, victim) damage aggregate - total damage one player dealt another within a single round. " +
			"Self-damage rows exist (attacker = victim, e.g. bomb/fall damage).",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "round", Type: dataprovider.FieldRelation, RelationTable: "rounds", Required: true, CascadeDelete: true, Description: "Relation to rounds.id."},
			{Name: "round_number", Type: dataprovider.FieldNumber, OnlyInt: true},
			{Name: "attacker", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id - who dealt the damage."},
			{Name: "victim", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id - who received the damage. Can equal attacker (self/fall damage)."},
			{Name: "damage", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Total damage dealt in this (round, attacker, victim) aggregate."},
			{Name: "headshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Headshot hits contributing to this damage total."},
			{Name: "bodyshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Bodyshot hits contributing to this damage total."},
			{Name: "legshots", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Legshot hits contributing to this damage total."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_damage_events_round_attacker_victim", Unique: true, Columns: []string{"round", "attacker", "victim"}},
			{Name: "idx_damage_events_attacker", Columns: []string{"attacker"}},
		},
	},
	{
		Name: "kills",
		Description: "One row per kill event, with round timing, location, and weapon. Self-kills exist (killer = victim, e.g. bomb detonation or ability expiry) " +
			"- filter killer != victim for KDA-style analysis.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "round", Type: dataprovider.FieldRelation, RelationTable: "rounds", Required: true, CascadeDelete: true, Description: "Relation to rounds.id."},
			{Name: "round_number", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "0-indexed round number, denormalized from rounds."},
			{Name: "time_in_round_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Time into the round the kill occurred, in milliseconds."},
			{Name: "time_in_match_ms", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Time into the match the kill occurred, in milliseconds."},
			{Name: "killer", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id."},
			{Name: "killer_team", Type: dataprovider.FieldText, Description: "Killer's team at the time, 'Red' | 'Blue'."},
			{Name: "victim", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id."},
			{Name: "victim_team", Type: dataprovider.FieldText, Description: "Victim's team at the time, 'Red' | 'Blue'."},
			{Name: "weapon", Type: dataprovider.FieldRelation, RelationTable: "weapons", Description: "Relation to weapons.id - weapon/ability/bomb used."},
			{Name: "secondary_fire_mode", Type: dataprovider.FieldBool, Description: "Whether the kill used the weapon's secondary fire mode."},
			{Name: "kill_x", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space X coordinate of the victim's death."},
			{Name: "kill_y", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space Y coordinate of the victim's death."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_kills_killer", Columns: []string{"killer"}},
			{Name: "idx_kills_victim", Columns: []string{"victim"}},
			{Name: "idx_kills_match_round", Columns: []string{"match", "round_number"}},
		},
	},
	{
		Name:        "kill_assists",
		Description: "One row per assist per kill (0..n rows per kill in kills).",
		Fields: []dataprovider.FieldSpec{
			{Name: "kill", Type: dataprovider.FieldRelation, RelationTable: "kills", Required: true, CascadeDelete: true, Description: "Relation to kills.id."},
			{Name: "assister", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_kill_assists_kill_assister", Unique: true, Columns: []string{"kill", "assister"}},
		},
	},
	{
		Name: "event_player_locations",
		Description: "One row per alive player's position snapshot at a kill, plant, or defuse event. High volume; only useful for positional/heatmap-style " +
			"analysis, not general stat questions. event_kill is set only for event_type='kill'.",
		Fields: []dataprovider.FieldSpec{
			{Name: "match", Type: dataprovider.FieldRelation, RelationTable: "matches", Required: true, CascadeDelete: true, Description: "Relation to matches.id."},
			{Name: "round", Type: dataprovider.FieldRelation, RelationTable: "rounds", Required: true, CascadeDelete: true, Description: "Relation to rounds.id."},
			{Name: "round_number", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "0-indexed round number, denormalized from rounds."},
			{Name: "event_type", Type: dataprovider.FieldText, Required: true, Description: "'kill' | 'plant' | 'defuse'."},
			{Name: "event_kill", Type: dataprovider.FieldRelation, RelationTable: "kills", CascadeDelete: true, Description: "Relation to kills.id, set only when event_type='kill'."},
			{Name: "player", Type: dataprovider.FieldRelation, RelationTable: "players", Required: true, Description: "Relation to players.id - the alive player whose position this snapshot records."},
			{Name: "loc_x", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space X coordinate."},
			{Name: "loc_y", Type: dataprovider.FieldNumber, OnlyInt: true, Description: "Map-space Y coordinate."},
			{Name: "view_radians", Type: dataprovider.FieldNumber, Description: "Player's view direction in radians."},
			{Name: "created", Type: dataprovider.FieldAutodate, OnCreate: true},
			{Name: "updated", Type: dataprovider.FieldAutodate, OnCreate: true, OnUpdate: true},
		},
		Indexes: []dataprovider.IndexSpec{
			{Name: "idx_event_player_locations_unique", Unique: true, Columns: []string{"round", "event_type", "event_kill", "player"}},
		},
	},
}
