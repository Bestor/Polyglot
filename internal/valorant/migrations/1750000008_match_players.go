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
		agents, err := app.FindCollectionByNameOrId("agents")
		if err != nil {
			return err
		}
		tiers, err := app.FindCollectionByNameOrId("tiers")
		if err != nil {
			return err
		}

		c := core.NewBaseCollection("match_players")
		c.Fields.Add(
			&core.RelationField{Name: "match", CollectionId: matches.Id, Required: true, CascadeDelete: true},
			&core.RelationField{Name: "player", CollectionId: players.Id, Required: true, CascadeDelete: true},
			&core.TextField{Name: "riot_name_snapshot"},
			&core.TextField{Name: "riot_tag_snapshot"},
			&core.TextField{Name: "team"}, // 'Red' | 'Blue'
			&core.TextField{Name: "party_id"},
			&core.TextField{Name: "platform"},
			&core.RelationField{Name: "agent", CollectionId: agents.Id},
			&core.RelationField{Name: "tier", CollectionId: tiers.Id},
			&core.NumberField{Name: "account_level", OnlyInt: true},
			&core.NumberField{Name: "session_playtime_ms", OnlyInt: true},
			&core.BoolField{Name: "won"},
			&core.NumberField{Name: "score", OnlyInt: true},
			&core.NumberField{Name: "kills", OnlyInt: true},
			&core.NumberField{Name: "deaths", OnlyInt: true},
			&core.NumberField{Name: "assists", OnlyInt: true},
			&core.NumberField{Name: "headshots", OnlyInt: true},
			&core.NumberField{Name: "bodyshots", OnlyInt: true},
			&core.NumberField{Name: "legshots", OnlyInt: true},
			&core.NumberField{Name: "damage_dealt", OnlyInt: true},
			&core.NumberField{Name: "damage_received", OnlyInt: true},
			&core.NumberField{Name: "casts_grenade", OnlyInt: true},
			&core.NumberField{Name: "casts_ability1", OnlyInt: true},
			&core.NumberField{Name: "casts_ability2", OnlyInt: true},
			&core.NumberField{Name: "casts_ultimate", OnlyInt: true},
			&core.NumberField{Name: "econ_spent_overall", OnlyInt: true},
			&core.NumberField{Name: "econ_spent_avg"},
			&core.NumberField{Name: "econ_loadout_value_overall", OnlyInt: true},
			&core.NumberField{Name: "econ_loadout_value_avg"},
			// Source reports these as floats even though they're round
			// counts, so no OnlyInt here.
			&core.NumberField{Name: "afk_rounds"},
			&core.NumberField{Name: "friendly_fire_incoming"},
			&core.NumberField{Name: "friendly_fire_outgoing"},
			&core.NumberField{Name: "rounds_in_spawn"},
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
