package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("match_teams")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.TextField{Name: "team_id", Required: true}, // 'Red' | 'Blue'
			&core.NumberField{Name: "rounds_won", OnlyInt: true},
			&core.NumberField{Name: "rounds_lost", OnlyInt: true},
			&core.BoolField{Name: "won"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_match_teams_match_team", true, "match, team_id", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("match_teams")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
