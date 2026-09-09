package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// player_months is the sync-state/coverage bookkeeping for a player's game
// archives - the airtight, no-HistoryExhausted-flag-needed replacement for
// Valorant's backward-paging state (see internal/chesscom/ingest). A row
// marked synced for a non-current month is provably complete forever,
// since chess.com's monthly archives are immutable once the month ends.
func init() {
	m.Register(func(app core.App) error {
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("player_months")
		c.Fields.Add(
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true, CascadeDelete: true},
			&core.NumberField{Name: "year", Required: true, OnlyInt: true},
			&core.NumberField{Name: "month", Required: true, OnlyInt: true},
			&core.DateField{Name: "synced_at"},
			&core.NumberField{Name: "game_count", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_player_months_player_month", true, "player, year, month", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("player_months")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
