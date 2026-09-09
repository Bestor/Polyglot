package chesscom

import (
	"encoding/json"
	"testing"
)

// realStatsFixture is a trimmed copy of a live GET /pub/player/{username}/stats
// response (captured against the real API while building this client) -
// exercises every shape chess.com's stats endpoint actually returns: a
// time-control category (chess_bullet), a bare-number category (fide), a
// highest/lowest category (tactics), and a puzzle_rush category with only
// "best" present (no "daily").
const realStatsFixture = `{
	"chess_daily": {"last":{"rating":2239,"date":1770563021,"rd":103},"best":{"rating":2464,"date":1397136740,"game":"https://www.chess.com/game/daily/84604826"},"record":{"win":73,"loss":11,"draw":4}},
	"chess960_daily": {"last":{"rating":1231,"date":1444458214,"rd":230},"best":{"rating":1489,"date":1397073007,"game":"https://www.chess.com/game/daily/87191830"},"record":{"win":1,"loss":2,"draw":0}},
	"chess_bullet": {"last":{"rating":3358,"date":1788026237,"rd":29},"best":{"rating":3570,"date":1605136047,"game":"https://www.chess.com/game/live/5710095242"},"record":{"win":16820,"loss":2453,"draw":1141}},
	"fide": 2814,
	"tactics": {"highest":{"rating":2730,"date":1389043258},"lowest":{"rating":2730,"date":1389043258}},
	"puzzle_rush": {"best":{"total_attempts":126,"score":123}}
}`

func TestParseStats_RealFixture(t *testing.T) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(realStatsFixture), &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	snapshots := parseStats(raw)
	byCategory := make(map[string]int)
	for _, s := range snapshots {
		byCategory[s.Category]++
	}

	wantCategories := []string{"chess_daily", "chess960_daily", "chess_bullet", "fide", "tactics", "puzzle_rush_best"}
	for _, c := range wantCategories {
		if byCategory[c] != 1 {
			t.Errorf("expected exactly 1 snapshot for category %q, got %d", c, byCategory[c])
		}
	}
	if _, ok := byCategory["puzzle_rush_daily"]; ok {
		t.Error("expected no puzzle_rush_daily snapshot - the fixture has no \"daily\" key")
	}
	if len(snapshots) != len(wantCategories) {
		t.Errorf("expected %d total snapshots, got %d: %+v", len(wantCategories), len(snapshots), snapshots)
	}

	byName := make(map[string]int)
	for i, s := range snapshots {
		byName[s.Category] = i
	}

	bullet := snapshots[byName["chess_bullet"]]
	if bullet.LastRating != 3358 || bullet.LastRD != 29 {
		t.Errorf("chess_bullet last: got rating=%d rd=%d", bullet.LastRating, bullet.LastRD)
	}
	if bullet.BestRating != 3570 || bullet.BestGameURL != "https://www.chess.com/game/live/5710095242" {
		t.Errorf("chess_bullet best: got rating=%d url=%q", bullet.BestRating, bullet.BestGameURL)
	}
	if bullet.Wins != 16820 || bullet.Losses != 2453 || bullet.Draws != 1141 {
		t.Errorf("chess_bullet record: got win=%d loss=%d draw=%d", bullet.Wins, bullet.Losses, bullet.Draws)
	}

	fide := snapshots[byName["fide"]]
	if fide.LastRating != 2814 {
		t.Errorf("fide: got LastRating=%d, want 2814", fide.LastRating)
	}

	tactics := snapshots[byName["tactics"]]
	if tactics.BestRating != 2730 {
		t.Errorf("tactics: got BestRating=%d, want 2730 (from \"highest\")", tactics.BestRating)
	}

	puzzleRush := snapshots[byName["puzzle_rush_best"]]
	if puzzleRush.BestRating != 123 || puzzleRush.Attempts != 126 {
		t.Errorf("puzzle_rush_best: got score(BestRating)=%d attempts=%d, want 123/126", puzzleRush.BestRating, puzzleRush.Attempts)
	}
}

func TestParseStats_PuzzleRushBothModes(t *testing.T) {
	raw := map[string]json.RawMessage{
		"puzzle_rush": json.RawMessage(`{"best":{"total_attempts":126,"score":123},"daily":{"total_attempts":20,"score":45}}`),
	}
	snapshots := parseStats(raw)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots (best + daily), got %d: %+v", len(snapshots), snapshots)
	}
	byName := make(map[string]int)
	for _, s := range snapshots {
		byName[s.Category]++
	}
	if byName["puzzle_rush_best"] != 1 || byName["puzzle_rush_daily"] != 1 {
		t.Errorf("expected one of each puzzle_rush_best/puzzle_rush_daily, got %+v", byName)
	}
}
