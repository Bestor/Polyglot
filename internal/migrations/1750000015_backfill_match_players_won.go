package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// This is a data-only backfill, not a schema change: match_players.won was
// added to the schema but SaveMatch (internal/store/matches.go) never
// actually set it, so every match synced before that fix defaulted to
// false regardless of the real outcome. match_teams.won has always been
// populated correctly, so derive the correct value from there.
func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
			UPDATE match_players
			SET won = (
				SELECT mt.won FROM match_teams mt
				WHERE mt.match = match_players.match AND mt.team_id = match_players.team
			)
		`).Execute()
		return err
	}, func(app core.App) error {
		// Not reversible - the prior (incorrect, always-false) values
		// aren't worth restoring.
		return nil
	})
}
