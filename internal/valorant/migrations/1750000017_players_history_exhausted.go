package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// history_exhausted marks that a backward sync for this player has, at
// least once, walked all the way back to the true start of their match
// history (the data source returned an empty page), rather than merely
// stopping at some cap. Once true, a coverage check can treat any
// requested start date as already satisfied - there's nothing older to
// fetch even if the cached range doesn't reach that far back itself.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.BoolField{Name: "history_exhausted"})
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}
		c.Fields.RemoveByName("history_exhausted")
		return app.Save(c)
	})
}
