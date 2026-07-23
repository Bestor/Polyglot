package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// This is a data-only backfill, not a schema change: players discovered
// opportunistically as match participants (internal/store/matches.go
// resolvePlayer) were never given a region before that code was fixed to
// pass the match's own region, since PlayerRef/PlayerMatchStats don't carry
// one of their own - leaving players.region empty, which the upstream API
// rejects outright on any later sync call for that player ("Invalid
// region"). Derive the correct region from a match they're already known
// to have played in where possible, falling back to "na" for anyone with
// no cached matches to derive it from.
func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
			UPDATE players
			SET region = COALESCE(
				(SELECT m.region FROM match_players mp
				 JOIN matches m ON mp.match = m.id
				 WHERE mp.player = players.id
				 LIMIT 1),
				'na'
			)
			WHERE region = '' OR region IS NULL
		`).Execute()
		return err
	}, func(app core.App) error {
		// Not reversible - the prior (incorrect, empty) values aren't
		// worth restoring.
		return nil
	})
}
