package store

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

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

// SaveMatch persists a match and its per-player stat rows in a single
// transaction. Every participant is opportunistically upserted into the
// players collection (keyed by PUUID) even if nobody has explicitly asked
// about them yet, so a later question about a teammate/opponent who already
// showed up in a cached match doesn't require re-resolving their identity.
//
// seasonRecordID may be empty if the match's season hasn't been cached in
// the seasons collection yet; season_id_raw is always stored regardless.
func (s *MatchStore) SaveMatch(detail data_sources.MatchDetail, seasonRecordID string) error {
	return s.app.RunInTransaction(func(txApp core.App) error {
		matchesCol, err := txApp.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matchPlayersCol, err := txApp.FindCollectionByNameOrId("match_players")
		if err != nil {
			return err
		}

		matchRec := core.NewRecord(matchesCol)
		matchRec.Set("match_id", detail.MatchID)
		matchRec.Set("map", detail.Map)
		matchRec.Set("mode", detail.Mode)
		matchRec.Set("queue", detail.Queue)
		matchRec.Set("season_id_raw", detail.SeasonID)
		matchRec.Set("game_start", detail.StartedAt)
		matchRec.Set("rounds_played", detail.RoundsPlayed)
		if len(detail.Raw) > 0 {
			matchRec.Set("raw_json", string(detail.Raw))
		}
		if seasonRecordID != "" {
			matchRec.Set("season", seasonRecordID)
		}
		if err := txApp.Save(matchRec); err != nil {
			return err
		}

		playerStore := NewPlayerStore(txApp)
		for _, p := range detail.Players {
			playerRec, err := playerStore.Upsert(Player{PUUID: p.PUUID, Name: p.Name, Tag: p.Tag})
			if err != nil {
				return err
			}

			mpRec := core.NewRecord(matchPlayersCol)
			mpRec.Set("match", matchRec.Id)
			mpRec.Set("player", playerRec.ID)
			mpRec.Set("riot_name_snapshot", p.Name)
			mpRec.Set("riot_tag_snapshot", p.Tag)
			mpRec.Set("agent", p.Agent)
			mpRec.Set("team", p.Team)
			mpRec.Set("won", p.Won)
			mpRec.Set("kills", p.Kills)
			mpRec.Set("deaths", p.Deaths)
			mpRec.Set("assists", p.Assists)
			mpRec.Set("headshots", p.Headshots)
			mpRec.Set("bodyshots", p.Bodyshots)
			mpRec.Set("legshots", p.Legshots)
			mpRec.Set("damage_made", p.DamageMade)
			mpRec.Set("damage_received", p.DamageReceived)
			mpRec.Set("score", p.Score)
			if err := txApp.Save(mpRec); err != nil {
				return err
			}
		}

		return nil
	})
}
