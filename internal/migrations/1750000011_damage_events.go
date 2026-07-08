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

		// Grain: 1 row per (round, attacker, victim) damage aggregate. The
		// source nests these under each attacker's round stats; damage
		// dealt to self (e.g. own bomb/fall damage) is a valid row where
		// attacker == victim.
		c := core.NewBaseCollection("damage_events")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "round", CollectionId: rounds.Id, Required: true, CascadeDelete: true},
			&core.NumberField{Name: "round_number", OnlyInt: true},
			&core.RelationField{Name: "attacker", CollectionId: players.Id, Required: true},
			&core.RelationField{Name: "victim", CollectionId: players.Id, Required: true},
			&core.NumberField{Name: "damage", OnlyInt: true},
			&core.NumberField{Name: "headshots", OnlyInt: true},
			&core.NumberField{Name: "bodyshots", OnlyInt: true},
			&core.NumberField{Name: "legshots", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_damage_events_round_attacker_victim", true, "round, attacker, victim", "")
		c.AddIndex("idx_damage_events_attacker", false, "attacker", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("damage_events")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
