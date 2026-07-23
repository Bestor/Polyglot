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
		rounds, err := app.FindCollectionByNameOrId("rounds")
		if err != nil {
			return err
		}
		players, err := app.FindCollectionByNameOrId("players")
		if err != nil {
			return err
		}
		weapons, err := app.FindCollectionByNameOrId("weapons")
		if err != nil {
			return err
		}

		// Grain: 1 row per kill event. Self-kills exist (bomb detonation,
		// ability expiry) - killer can equal victim, and killer/victim
		// teams can match. Filter these out for KDA-style analysis.
		c := core.NewBaseCollection("kills")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "round", CollectionId: rounds.Id, Required: true, CascadeDelete: true},
			&core.NumberField{Name: "round_number", OnlyInt: true},
			&core.NumberField{Name: "time_in_round_ms", OnlyInt: true},
			&core.NumberField{Name: "time_in_match_ms", OnlyInt: true},
			&core.RelationField{Name: "killer", CollectionId: players.Id, Required: true},
			&core.TextField{Name: "killer_team"},
			&core.RelationField{Name: "victim", CollectionId: players.Id, Required: true},
			&core.TextField{Name: "victim_team"},
			&core.RelationField{Name: "weapon", CollectionId: weapons.Id},
			&core.BoolField{Name: "secondary_fire_mode"},
			&core.NumberField{Name: "kill_x", OnlyInt: true}, // victim death location
			&core.NumberField{Name: "kill_y", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_kills_killer", false, "killer", "")
		c.AddIndex("idx_kills_victim", false, "victim", "")
		c.AddIndex("idx_kills_match_round", false, "match, round_number", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("kills")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
