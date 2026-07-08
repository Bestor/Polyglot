package store

import (
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"val-analyzer/internal/data_sources"
)

type MatchStore struct {
	app core.App
}

func NewMatchStore(app core.App) *MatchStore {
	return &MatchStore{app: app}
}

func (s *MatchStore) Exists(matchID string) (bool, error) {
	_, err := s.app.FindFirstRecordByFilter("matches", "match_id = {:id}", dbx.Params{"id": matchID})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// PlayerCoverage reports how many matches are already cached locally for
// playerID with started_at <= until (nil = no upper bound, i.e. as of
// now), and the oldest such match's started_at (zero if none), without
// touching the upstream data source. The oldest value tells a caller how
// deep the local cache reaches for this player - e.g. compare it against a
// requested start date to see whether that date is already covered.
//
// This is a best-effort signal, not an airtight gap-free guarantee: since
// SyncPlayerMatches always walks a player's history from most recent
// backward, an unbroken run naturally accumulates gap-free coverage from
// "now" back to the deepest point it ever reached - but a sync that was
// cut short by its match cap before reconnecting with previously-cached
// data could in principle leave a gap. In practice this is rare and the
// cache is always self-healing on the next sync, so it's an acceptable
// tradeoff against calling the upstream API on every question.
func (s *MatchStore) PlayerCoverage(playerID string, until *time.Time) (count int, oldest time.Time, err error) {
	filter := "match_players_via_match.player = {:pid}"
	params := dbx.Params{"pid": playerID}
	if until != nil {
		filter += " && started_at <= {:until}"
		params["until"] = until.UTC().Format(types.DefaultDateLayout)
	}

	records, err := s.app.FindRecordsByFilter("matches", filter, "started_at", 0, 0, params)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(records) == 0 {
		return 0, time.Time{}, nil
	}

	return len(records), records[0].GetDateTime("started_at").Time(), nil
}

// saveMatchCtx carries the per-call caches and stores used while persisting
// a single match, so the same player/agent/tier/weapon isn't re-resolved
// via a DB lookup every time it's referenced (a single match can reference
// the same player or weapon hundreds of times across rounds and kills).
type saveMatchCtx struct {
	app core.App

	// region is the match's region, used when opportunistically upserting
	// a player discovered only as a match participant (PlayerRef carries
	// no region of its own) - a reasonable default since match
	// participants are practically always on the same shard as the match.
	region string

	players *PlayerStore
	agents  *AgentStore
	tiers   *TierStore
	weapons *WeaponStore

	playerIDs map[string]string // puuid -> players.id
	agentIDs  map[string]string // agent_id -> agents.id
	tierIDs   map[int]string    // tier_id -> tiers.id
	weaponIDs map[string]string // weapon_id -> weapons.id

	matchesCol              *core.Collection
	matchTeamsCol           *core.Collection
	matchPlayersCol         *core.Collection
	roundsCol               *core.Collection
	roundPlayerStatsCol     *core.Collection
	damageEventsCol         *core.Collection
	killsCol                *core.Collection
	killAssistsCol          *core.Collection
	eventPlayerLocationsCol *core.Collection
}

func newSaveMatchCtx(app core.App) (*saveMatchCtx, error) {
	ctx := &saveMatchCtx{
		app:       app,
		players:   NewPlayerStore(app),
		agents:    NewAgentStore(app),
		tiers:     NewTierStore(app),
		weapons:   NewWeaponStore(app),
		playerIDs: map[string]string{},
		agentIDs:  map[string]string{},
		tierIDs:   map[int]string{},
		weaponIDs: map[string]string{},
	}

	cols := map[string]**core.Collection{
		"matches":                &ctx.matchesCol,
		"match_teams":            &ctx.matchTeamsCol,
		"match_players":          &ctx.matchPlayersCol,
		"rounds":                 &ctx.roundsCol,
		"round_player_stats":     &ctx.roundPlayerStatsCol,
		"damage_events":          &ctx.damageEventsCol,
		"kills":                  &ctx.killsCol,
		"kill_assists":           &ctx.killAssistsCol,
		"event_player_locations": &ctx.eventPlayerLocationsCol,
	}
	for name, dst := range cols {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return nil, err
		}
		*dst = col
	}

	return ctx, nil
}

func (c *saveMatchCtx) resolvePlayer(ref data_sources.PlayerRef) (string, error) {
	if ref.PUUID == "" {
		return "", nil
	}
	if id, ok := c.playerIDs[ref.PUUID]; ok {
		return id, nil
	}

	player, err := c.players.Upsert(Player{PUUID: ref.PUUID, Name: ref.Name, Tag: ref.Tag, Region: c.region})
	if err != nil {
		return "", err
	}
	c.playerIDs[ref.PUUID] = player.ID
	return player.ID, nil
}

func (c *saveMatchCtx) resolveAgent(id, name string) (string, error) {
	if id == "" {
		return "", nil
	}
	if recID, ok := c.agentIDs[id]; ok {
		return recID, nil
	}

	agent, err := c.agents.Upsert(id, name)
	if err != nil {
		return "", err
	}
	c.agentIDs[id] = agent.ID
	return agent.ID, nil
}

func (c *saveMatchCtx) resolveTier(id int, name string) (string, error) {
	if recID, ok := c.tierIDs[id]; ok {
		return recID, nil
	}

	tier, err := c.tiers.Upsert(id, name)
	if err != nil {
		return "", err
	}
	c.tierIDs[id] = tier.ID
	return tier.ID, nil
}

func (c *saveMatchCtx) resolveWeapon(w data_sources.Weapon) (string, error) {
	if w.ID == "" && w.Name == "" {
		return "", nil
	}
	if recID, ok := c.weaponIDs[w.ID]; ok {
		return recID, nil
	}

	weapon, saved, err := c.weapons.Upsert(w.ID, w.Name, w.Type)
	if err != nil {
		return "", err
	}
	if !saved {
		return "", nil
	}
	c.weaponIDs[w.ID] = weapon.ID
	return weapon.ID, nil
}

func (c *saveMatchCtx) savePlayerLocations(matchID, roundID string, roundNumber int, eventType, eventKillID string, locs []data_sources.PlayerLocation) error {
	for _, loc := range locs {
		playerID, err := c.resolvePlayer(loc.Player)
		if err != nil {
			return err
		}

		rec := core.NewRecord(c.eventPlayerLocationsCol)
		rec.Set("match", matchID)
		rec.Set("round", roundID)
		rec.Set("round_number", roundNumber)
		rec.Set("event_type", eventType)
		rec.Set("event_kill", eventKillID)
		rec.Set("player", playerID)
		rec.Set("loc_x", loc.Location.X)
		rec.Set("loc_y", loc.Location.Y)
		rec.Set("view_radians", loc.ViewRadians)
		if err := c.app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// SaveMatch persists a match and its full normalized breakdown (teams,
// players, rounds, round-by-round player stats, damage events, kills,
// assists, and player-location snapshots) in a single transaction. Every
// player referenced anywhere in the match - not just the ones explicitly
// looked up - is opportunistically upserted into the players collection,
// so a later question about a teammate/opponent who already showed up in
// a cached match doesn't require re-resolving their identity.
//
// seasonRecordID may be empty if the match's season hasn't been cached in
// the seasons collection yet; season_id_raw is always stored regardless.
func (s *MatchStore) SaveMatch(detail data_sources.MatchDetail, seasonRecordID string) error {
	return s.app.RunInTransaction(func(txApp core.App) error {
		c, err := newSaveMatchCtx(txApp)
		if err != nil {
			return err
		}
		c.region = detail.Region

		mapStore := NewMapStore(txApp)
		mapRow, err := mapStore.Upsert(detail.MapID, detail.Map)
		if err != nil {
			return err
		}

		matchRec := core.NewRecord(c.matchesCol)
		matchRec.Set("match_id", detail.MatchID)
		matchRec.Set("map", mapRow.ID)
		matchRec.Set("game_version", detail.GameVersion)
		matchRec.Set("queue_id", detail.QueueID)
		matchRec.Set("queue_mode_type", detail.QueueModeType)
		matchRec.Set("season_id_raw", detail.SeasonID)
		matchRec.Set("platform", detail.Platform)
		matchRec.Set("region", detail.Region)
		matchRec.Set("cluster", detail.Cluster)
		matchRec.Set("started_at", detail.StartedAt)
		matchRec.Set("game_length_ms", detail.GameLengthMs)
		matchRec.Set("is_completed", detail.IsCompleted)
		if len(detail.Raw) > 0 {
			matchRec.Set("raw_json", string(detail.Raw))
		}
		if seasonRecordID != "" {
			matchRec.Set("season", seasonRecordID)
		}
		if err := txApp.Save(matchRec); err != nil {
			return err
		}
		matchID := matchRec.Id

		teamWon := make(map[string]bool, len(detail.Teams))
		for _, team := range detail.Teams {
			teamWon[team.TeamID] = team.Won

			rec := core.NewRecord(c.matchTeamsCol)
			rec.Set("match", matchID)
			rec.Set("team_id", team.TeamID)
			rec.Set("rounds_won", team.RoundsWon)
			rec.Set("rounds_lost", team.RoundsLost)
			rec.Set("won", team.Won)
			if err := txApp.Save(rec); err != nil {
				return err
			}
		}

		for _, p := range detail.Players {
			playerID, err := c.resolvePlayer(data_sources.PlayerRef{PUUID: p.PUUID, Name: p.Name, Tag: p.Tag, Team: p.Team})
			if err != nil {
				return err
			}
			agentID, err := c.resolveAgent(p.AgentID, p.AgentName)
			if err != nil {
				return err
			}
			tierID, err := c.resolveTier(p.TierID, p.TierName)
			if err != nil {
				return err
			}

			rec := core.NewRecord(c.matchPlayersCol)
			rec.Set("match", matchID)
			rec.Set("player", playerID)
			rec.Set("riot_name_snapshot", p.Name)
			rec.Set("riot_tag_snapshot", p.Tag)
			rec.Set("team", p.Team)
			rec.Set("won", teamWon[p.Team])
			rec.Set("party_id", p.PartyID)
			rec.Set("platform", p.Platform)
			rec.Set("agent", agentID)
			rec.Set("tier", tierID)
			rec.Set("account_level", p.AccountLevel)
			rec.Set("session_playtime_ms", p.SessionPlaytimeMs)
			rec.Set("score", p.Score)
			rec.Set("kills", p.Kills)
			rec.Set("deaths", p.Deaths)
			rec.Set("assists", p.Assists)
			rec.Set("headshots", p.Headshots)
			rec.Set("bodyshots", p.Bodyshots)
			rec.Set("legshots", p.Legshots)
			rec.Set("damage_dealt", p.DamageDealt)
			rec.Set("damage_received", p.DamageReceived)
			rec.Set("casts_grenade", p.CastsGrenade)
			rec.Set("casts_ability1", p.CastsAbility1)
			rec.Set("casts_ability2", p.CastsAbility2)
			rec.Set("casts_ultimate", p.CastsUltimate)
			rec.Set("econ_spent_overall", p.EconSpentOverall)
			rec.Set("econ_spent_avg", p.EconSpentAvg)
			rec.Set("econ_loadout_value_overall", p.EconLoadoutValueOverall)
			rec.Set("econ_loadout_value_avg", p.EconLoadoutValueAvg)
			rec.Set("afk_rounds", p.AFKRounds)
			rec.Set("friendly_fire_incoming", p.FriendlyFireIncoming)
			rec.Set("friendly_fire_outgoing", p.FriendlyFireOutgoing)
			rec.Set("rounds_in_spawn", p.RoundsInSpawn)
			if err := txApp.Save(rec); err != nil {
				return err
			}
		}

		roundIDs := make(map[int]string, len(detail.Rounds))
		for _, round := range detail.Rounds {
			rec := core.NewRecord(c.roundsCol)
			rec.Set("match", matchID)
			rec.Set("round_number", round.Number)
			rec.Set("result", round.Result)
			rec.Set("ceremony", round.Ceremony)
			rec.Set("winning_team", round.WinningTeam)

			if round.Plant != nil {
				planterID, err := c.resolvePlayer(round.Plant.Player)
				if err != nil {
					return err
				}
				rec.Set("plant_time_ms", round.Plant.RoundTimeMs)
				rec.Set("plant_site", round.Plant.Site)
				rec.Set("plant_x", round.Plant.Location.X)
				rec.Set("plant_y", round.Plant.Location.Y)
				rec.Set("planter", planterID)
			}
			if round.Defuse != nil {
				defuserID, err := c.resolvePlayer(round.Defuse.Player)
				if err != nil {
					return err
				}
				rec.Set("defuse_time_ms", round.Defuse.RoundTimeMs)
				rec.Set("defuse_x", round.Defuse.Location.X)
				rec.Set("defuse_y", round.Defuse.Location.Y)
				rec.Set("defuser", defuserID)
			}

			if err := txApp.Save(rec); err != nil {
				return err
			}
			roundID := rec.Id
			roundIDs[round.Number] = roundID

			if round.Plant != nil {
				if err := c.savePlayerLocations(matchID, roundID, round.Number, "plant", "", round.Plant.PlayerLocations); err != nil {
					return err
				}
			}
			if round.Defuse != nil {
				if err := c.savePlayerLocations(matchID, roundID, round.Number, "defuse", "", round.Defuse.PlayerLocations); err != nil {
					return err
				}
			}

			for _, ps := range round.PlayerStats {
				playerID, err := c.resolvePlayer(ps.Player)
				if err != nil {
					return err
				}
				weaponID, err := c.resolveWeapon(ps.Weapon)
				if err != nil {
					return err
				}

				rpsRec := core.NewRecord(c.roundPlayerStatsCol)
				rpsRec.Set("match", matchID)
				rpsRec.Set("round", roundID)
				rpsRec.Set("round_number", round.Number)
				rpsRec.Set("player", playerID)
				rpsRec.Set("score", ps.Score)
				rpsRec.Set("kills", ps.Kills)
				rpsRec.Set("headshots", ps.Headshots)
				rpsRec.Set("bodyshots", ps.Bodyshots)
				rpsRec.Set("legshots", ps.Legshots)
				rpsRec.Set("loadout_value", ps.LoadoutValue)
				rpsRec.Set("money_remaining", ps.MoneyRemaining)
				rpsRec.Set("weapon", weaponID)
				rpsRec.Set("armor_id", ps.ArmorID)
				rpsRec.Set("armor_name", ps.ArmorName)
				rpsRec.Set("was_afk", ps.WasAFK)
				rpsRec.Set("received_penalty", ps.ReceivedPenalty)
				rpsRec.Set("stayed_in_spawn", ps.StayedInSpawn)
				if err := txApp.Save(rpsRec); err != nil {
					return err
				}

				// HenrikDev can report more than one damage_events entry for
				// the same (attacker, victim) pair within a round (e.g.
				// separate weapon-switch encounters), which would otherwise
				// collide with the (round, attacker, victim) unique index -
				// aggregate them into a single per-round total instead.
				type damageAgg struct {
					victim                                 data_sources.PlayerRef
					damage, headshots, bodyshots, legshots int
				}
				aggByVictim := make(map[string]*damageAgg, len(ps.DamageEvents))
				victimOrder := make([]string, 0, len(ps.DamageEvents))
				for _, de := range ps.DamageEvents {
					agg, ok := aggByVictim[de.Victim.PUUID]
					if !ok {
						agg = &damageAgg{victim: de.Victim}
						aggByVictim[de.Victim.PUUID] = agg
						victimOrder = append(victimOrder, de.Victim.PUUID)
					}
					agg.damage += de.Damage
					agg.headshots += de.Headshots
					agg.bodyshots += de.Bodyshots
					agg.legshots += de.Legshots
				}

				for _, puuid := range victimOrder {
					agg := aggByVictim[puuid]
					victimID, err := c.resolvePlayer(agg.victim)
					if err != nil {
						return err
					}

					deRec := core.NewRecord(c.damageEventsCol)
					deRec.Set("match", matchID)
					deRec.Set("round", roundID)
					deRec.Set("round_number", round.Number)
					deRec.Set("attacker", playerID)
					deRec.Set("victim", victimID)
					deRec.Set("damage", agg.damage)
					deRec.Set("headshots", agg.headshots)
					deRec.Set("bodyshots", agg.bodyshots)
					deRec.Set("legshots", agg.legshots)
					if err := txApp.Save(deRec); err != nil {
						return err
					}
				}
			}
		}

		for _, kill := range detail.Kills {
			roundID, ok := roundIDs[kill.RoundNumber]
			if !ok {
				slog.Warn("store: kill references unknown round, skipping", "match_id", detail.MatchID, "round_number", kill.RoundNumber)
				continue
			}

			killerID, err := c.resolvePlayer(kill.Killer)
			if err != nil {
				return err
			}
			victimID, err := c.resolvePlayer(kill.Victim)
			if err != nil {
				return err
			}
			weaponID, err := c.resolveWeapon(kill.Weapon)
			if err != nil {
				return err
			}

			killRec := core.NewRecord(c.killsCol)
			killRec.Set("match", matchID)
			killRec.Set("round", roundID)
			killRec.Set("round_number", kill.RoundNumber)
			killRec.Set("time_in_round_ms", kill.TimeInRoundMs)
			killRec.Set("time_in_match_ms", kill.TimeInMatchMs)
			killRec.Set("killer", killerID)
			killRec.Set("killer_team", kill.Killer.Team)
			killRec.Set("victim", victimID)
			killRec.Set("victim_team", kill.Victim.Team)
			killRec.Set("weapon", weaponID)
			killRec.Set("secondary_fire_mode", kill.SecondaryFireMode)
			killRec.Set("kill_x", kill.Location.X)
			killRec.Set("kill_y", kill.Location.Y)
			if err := txApp.Save(killRec); err != nil {
				return err
			}
			killID := killRec.Id

			for _, assistant := range kill.Assistants {
				assisterID, err := c.resolvePlayer(assistant)
				if err != nil {
					return err
				}

				assistRec := core.NewRecord(c.killAssistsCol)
				assistRec.Set("kill", killID)
				assistRec.Set("assister", assisterID)
				if err := txApp.Save(assistRec); err != nil {
					return err
				}
			}

			if err := c.savePlayerLocations(matchID, roundID, kill.RoundNumber, "kill", killID, kill.PlayerLocations); err != nil {
				return err
			}
		}

		return nil
	})
}
