package chesscom

import (
	"encoding/json"
	"time"

	"val-analyzer/internal/chesscom/data_sources"
)

// chess.com's stats endpoint (GET /pub/player/{username}/stats) is a flat
// object whose keys are category names, but the *shape* of each value
// varies by category - verified live, not just from documentation:
//   - a time-control category (chess_bullet/blitz/rapid/daily,
//     chess960_daily, ...) is {"last":{...},"best":{...},"record":{...}}
//   - "tactics" is {"highest":{...},"lowest":{...}} - only "highest" is
//     mapped to a Rating snapshot (see wireTacticsStat's own comment on why
//     "lowest" has no home in RatingSnapshot).
//   - "puzzle_rush" is {"best":{...}} and/or {"daily":{...}}, each an
//     independent {"total_attempts":..,"score":..} - each present sub-key
//     becomes its own category ("puzzle_rush_best"/"puzzle_rush_daily").
//   - "fide" (and possibly other single-number categories) is a bare int.
// Rather than hardcode the category name set (which chess.com has already
// been observed to exceed simple expectation - e.g. chess960_daily exists
// alongside the more obvious chess_* categories), every value's *shape* is
// discriminated by which keys are present, so an unrecognized-but-similar
// category (or accounts a bare/well-established category is added to) is
// handled the same way without a code change - and never breaks the whole
// call, since only json.Unmarshal ever fails, and simply causes that one
// category to be skipped, not sync_stats to fail outright.

type wireRatingPoint struct {
	Rating int    `json:"rating"`
	Date   int64  `json:"date"` // unix seconds
	RD     int    `json:"rd"`
	Game   string `json:"game"`
}

type wireRecord struct {
	Win  int `json:"win"`
	Loss int `json:"loss"`
	Draw int `json:"draw"`
}

type wireTimeControlStat struct {
	Last   *wireRatingPoint `json:"last"`
	Best   *wireRatingPoint `json:"best"`
	Record *wireRecord      `json:"record"`
}

type wireHighLow struct {
	Rating int   `json:"rating"`
	Date   int64 `json:"date"`
}

// wireTacticsStat's Lowest has no corresponding RatingSnapshot field -
// it's a historical minimum, not a "current" value in the sense
// LastRating/BestRating otherwise mean, and modeling a third number just
// for tactics would be more schema complexity than this one category's
// analytical value justifies for v1.
type wireTacticsStat struct {
	Highest *wireHighLow `json:"highest"`
	Lowest  *wireHighLow `json:"lowest"`
}

type wirePuzzleRushMode struct {
	TotalAttempts int `json:"total_attempts"`
	Score         int `json:"score"`
}

type wirePuzzleRushStat struct {
	Best  *wirePuzzleRushMode `json:"best"`
	Daily *wirePuzzleRushMode `json:"daily"`
}

// parseStats maps a raw stats response into RatingSnapshots, one per
// (category, sub-mode) pair actually present. Order of shape checks
// matters: a time-control category always carries "last"/"record", which
// puzzle_rush and tactics never do, so checking for those first before
// falling through to puzzle_rush's own "best" key (which time-control
// categories coincidentally also have, with an entirely different inner
// shape) never misclassifies one as the other.
func parseStats(raw map[string]json.RawMessage) []data_sources.RatingSnapshot {
	var snapshots []data_sources.RatingSnapshot

	for category, rawVal := range raw {
		var bareNumber int
		if err := json.Unmarshal(rawVal, &bareNumber); err == nil {
			snapshots = append(snapshots, data_sources.RatingSnapshot{Category: category, LastRating: bareNumber})
			continue
		}

		var probe map[string]json.RawMessage
		if err := json.Unmarshal(rawVal, &probe); err != nil {
			continue // an unrecognized shape - skip this one category rather than fail the whole call
		}

		switch {
		case hasAnyKey(probe, "last", "record"):
			var tc wireTimeControlStat
			if err := json.Unmarshal(rawVal, &tc); err != nil {
				continue
			}
			snap := data_sources.RatingSnapshot{Category: category}
			if tc.Last != nil {
				snap.LastRating = tc.Last.Rating
				snap.LastRD = tc.Last.RD
				snap.LastDate = time.Unix(tc.Last.Date, 0)
			}
			if tc.Best != nil {
				snap.BestRating = tc.Best.Rating
				snap.BestGameURL = tc.Best.Game
				snap.BestDate = time.Unix(tc.Best.Date, 0)
			}
			if tc.Record != nil {
				snap.Wins, snap.Losses, snap.Draws = tc.Record.Win, tc.Record.Loss, tc.Record.Draw
			}
			snapshots = append(snapshots, snap)

		case hasAnyKey(probe, "highest"):
			var t wireTacticsStat
			if err := json.Unmarshal(rawVal, &t); err != nil {
				continue
			}
			if t.Highest != nil {
				snapshots = append(snapshots, data_sources.RatingSnapshot{
					Category:   category,
					BestRating: t.Highest.Rating,
					BestDate:   time.Unix(t.Highest.Date, 0),
				})
			}

		case hasAnyKey(probe, "best", "daily"):
			var pr wirePuzzleRushStat
			if err := json.Unmarshal(rawVal, &pr); err != nil {
				continue
			}
			if pr.Best != nil {
				snapshots = append(snapshots, data_sources.RatingSnapshot{
					Category: category + "_best", BestRating: pr.Best.Score, Attempts: pr.Best.TotalAttempts,
				})
			}
			if pr.Daily != nil {
				snapshots = append(snapshots, data_sources.RatingSnapshot{
					Category: category + "_daily", BestRating: pr.Daily.Score, Attempts: pr.Daily.TotalAttempts,
				})
			}
		}
	}

	return snapshots
}

func hasAnyKey(m map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}
