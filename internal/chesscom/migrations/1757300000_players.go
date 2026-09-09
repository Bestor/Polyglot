package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c := core.NewBaseCollection("players")
		c.Fields.Add(
			// The natural key: always known immediately (it's the
			// identifier a caller supplies), and the only thing chess.com's
			// per-game participant data reports about a side (see
			// game_players) - so it's what an opportunistically-cached
			// opponent gets a stub row keyed on, not chesscom_player_id.
			&core.TextField{Name: "chesscom_username", Required: true},
			// Not required, no uniqueness constraint: only populated once a
			// GetPlayerProfile call has actually happened for this
			// username (resolve_player/sync_stats/sync_games's primary
			// player) - an opportunistically-cached opponent stub (from a
			// game's participant data alone, which never includes this)
			// leaves it unset until/unless that username is later resolved
			// directly. A unique index here would break the moment a
			// second such stub existed, since PocketBase stores an unset
			// NumberField as 0, not NULL.
			&core.NumberField{Name: "chesscom_player_id", OnlyInt: true},
			&core.TextField{Name: "name"},
			&core.TextField{Name: "title", Max: 8},
			&core.TextField{Name: "country_code", Max: 8},
			&core.TextField{Name: "status", Max: 16},
			&core.TextField{Name: "avatar_url"},
			&core.NumberField{Name: "followers", OnlyInt: true},
			&core.DateField{Name: "joined_at"},
			&core.DateField{Name: "last_synced_games_at"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_players_username", true, "chesscom_username", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
