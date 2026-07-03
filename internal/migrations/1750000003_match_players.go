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

		c := core.NewBaseCollection("match_players")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true, CascadeDelete: true},
			&core.TextField{Name: "riot_name_snapshot"},
			&core.TextField{Name: "riot_tag_snapshot"},
			&core.TextField{Name: "agent"},
			&core.TextField{Name: "team"},
			&core.BoolField{Name: "won"},
			&core.NumberField{Name: "kills", OnlyInt: true},
			&core.NumberField{Name: "deaths", OnlyInt: true},
			&core.NumberField{Name: "assists", OnlyInt: true},
			&core.NumberField{Name: "headshots", OnlyInt: true},
			&core.NumberField{Name: "bodyshots", OnlyInt: true},
			&core.NumberField{Name: "legshots", OnlyInt: true},
			&core.NumberField{Name: "damage_made", OnlyInt: true},
			&core.NumberField{Name: "damage_received", OnlyInt: true},
			&core.NumberField{Name: "score", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_match_players_match_player", true, "match, player", "")
		c.AddIndex("idx_match_players_player", false, "player", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("match_players")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
