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
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("rounds")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			// Not Required: PocketBase's NumberField.Required means "non-zero",
			// but round_number is 0-indexed so the first round is 0.
			&core.NumberField{Name: "round_number", OnlyInt: true},
			&core.TextField{Name: "result"},       // 'Elimination' | 'Defuse' | 'Detonate'
			&core.TextField{Name: "ceremony"},     // 'CeremonyClutch', 'CeremonyThrifty', ...
			&core.TextField{Name: "winning_team"}, // 'Red' | 'Blue'
			// Plant (nullable: not every round has one).
			&core.NumberField{Name: "plant_time_ms", OnlyInt: true},
			&core.TextField{Name: "plant_site"},
			&core.NumberField{Name: "plant_x", OnlyInt: true},
			&core.NumberField{Name: "plant_y", OnlyInt: true},
			&core.RelationField{Name: "planter", CollectionId: players.Id},
			// Defuse (nullable).
			&core.NumberField{Name: "defuse_time_ms", OnlyInt: true},
			&core.NumberField{Name: "defuse_x", OnlyInt: true},
			&core.NumberField{Name: "defuse_y", OnlyInt: true},
			&core.RelationField{Name: "defuser", CollectionId: players.Id},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_rounds_match_round", true, "match, round_number", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("rounds")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
