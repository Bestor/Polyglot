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
		kills, err := app.FindCollectionByNameOrId("kills")
		if err != nil {
			return err
		}

		// Grain: 1 row per alive player per snapshot event (kill, plant, or
		// defuse). High volume - only useful for heatmap/positioning
		// analysis. event_kill is only set for event_type='kill'; a round
		// has at most one plant and one defuse, so (round, event_type,
		// player) alone already disambiguates those.
		c := core.NewBaseCollection("event_player_locations")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "round", CollectionId: rounds.Id, Required: true, CascadeDelete: true},
			&core.NumberField{Name: "round_number", OnlyInt: true},
			&core.TextField{Name: "event_type", Required: true}, // 'kill' | 'plant' | 'defuse'
			&core.RelationField{Name: "event_kill", CollectionId: kills.Id, CascadeDelete: true},
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true},
			&core.NumberField{Name: "loc_x", OnlyInt: true},
			&core.NumberField{Name: "loc_y", OnlyInt: true},
			&core.NumberField{Name: "view_radians"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_event_player_locations_unique", true, "round, event_type, event_kill, player", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("event_player_locations")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
