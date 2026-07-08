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

		c := core.NewBaseCollection("round_player_stats")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "round", CollectionId: rounds.Id, Required: true, CascadeDelete: true},
			// Denormalized alongside the "round" relation so the AI can
			// filter/sort by round number without an extra join.
			&core.NumberField{Name: "round_number", OnlyInt: true},
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true},
			&core.NumberField{Name: "score", OnlyInt: true},
			&core.NumberField{Name: "kills", OnlyInt: true},
			&core.NumberField{Name: "headshots", OnlyInt: true},
			&core.NumberField{Name: "bodyshots", OnlyInt: true},
			&core.NumberField{Name: "legshots", OnlyInt: true},
			// Economy snapshot at round start.
			&core.NumberField{Name: "loadout_value", OnlyInt: true},
			&core.NumberField{Name: "money_remaining", OnlyInt: true},
			&core.RelationField{Name: "weapon", CollectionId: weapons.Id},
			&core.TextField{Name: "armor_id"},
			&core.TextField{Name: "armor_name"}, // 'Heavy Armor' | 'Light Armor' | 'Regen Shield'
			&core.BoolField{Name: "was_afk"},
			&core.BoolField{Name: "received_penalty"},
			&core.BoolField{Name: "stayed_in_spawn"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_round_player_stats_round_player", true, "round, player", "")
		c.AddIndex("idx_round_player_stats_player", false, "player", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("round_player_stats")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
