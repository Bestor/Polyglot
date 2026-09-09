package pgn

import "testing"

const clockedGamePGN = `[Event "Live Chess"]
[Site "Chess.com"]
[Date "2026.08.15"]
[White "player1"]
[Black "player2"]
[Result "1-0"]
[TimeControl "600"]

1. e4 {[%clk 0:09:58]} 1... e5 {[%clk 0:09:57.3]} 2. Nf3 {[%clk 0:09:55]} 2... Nc6 {[%clk 0:09:56]} 1-0
`

func TestParse_ClockedGame(t *testing.T) {
	moves, err := Parse(clockedGamePGN)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(moves) != 4 {
		t.Fatalf("expected 4 moves, got %d: %+v", len(moves), moves)
	}

	want := []struct {
		san   string
		color string
		clk   float64
	}{
		{"e4", "white", 9*60 + 58},
		{"e5", "black", 9*60 + 57.3},
		{"Nf3", "white", 9*60 + 55},
		{"Nc6", "black", 9*60 + 56},
	}
	for i, w := range want {
		m := moves[i]
		if m.SAN != w.san {
			t.Errorf("move %d: SAN = %q, want %q", i, m.SAN, w.san)
		}
		if m.Color != w.color {
			t.Errorf("move %d: Color = %q, want %q", i, m.Color, w.color)
		}
		if m.Ply != i+1 {
			t.Errorf("move %d: Ply = %d, want %d", i, m.Ply, i+1)
		}
		if m.ClockSeconds == nil {
			t.Fatalf("move %d: ClockSeconds is nil, want %v", i, w.clk)
		}
		if *m.ClockSeconds != w.clk {
			t.Errorf("move %d: ClockSeconds = %v, want %v", i, *m.ClockSeconds, w.clk)
		}
	}

	// Move numbers: ply 1,2 -> move 1; ply 3,4 -> move 2.
	if moves[0].MoveNumber != 1 || moves[1].MoveNumber != 1 {
		t.Errorf("expected moves 0,1 to have MoveNumber 1, got %d,%d", moves[0].MoveNumber, moves[1].MoveNumber)
	}
	if moves[2].MoveNumber != 2 || moves[3].MoveNumber != 2 {
		t.Errorf("expected moves 2,3 to have MoveNumber 2, got %d,%d", moves[2].MoveNumber, moves[3].MoveNumber)
	}
}

const dailyGamePGN = `[Event "Let's Play!"]
[Site "Chess.com"]
[Date "2026.07.01"]
[White "player1"]
[Black "player2"]
[Result "0-1"]
[TimeControl "1/259200"]

1. d4 d5 2. c4 e6 3. Nc3 Nf6 0-1
`

func TestParse_DailyGame_NoClockAnnotations(t *testing.T) {
	moves, err := Parse(dailyGamePGN)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(moves) != 6 {
		t.Fatalf("expected 6 moves, got %d: %+v", len(moves), moves)
	}
	for i, m := range moves {
		if m.ClockSeconds != nil {
			t.Errorf("move %d (%s): expected nil ClockSeconds for a daily game, got %v", i, m.SAN, *m.ClockSeconds)
		}
	}

	wantSAN := []string{"d4", "d5", "c4", "e6", "Nc3", "Nf6"}
	for i, want := range wantSAN {
		if moves[i].SAN != want {
			t.Errorf("move %d: SAN = %q, want %q", i, moves[i].SAN, want)
		}
	}
}

const specialMovesPGN = `[Event "Live Chess"]
[Result "1-0"]

1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 4. Ba4 Nf6 5. O-O Be7 6. Re1 b5 7. Bb3 O-O
8. exd5 Nxd5 9. Nxe5 Nxe5 10. e8=Q+ Kxe8 11. Qh5 g6 12. Qxf7# 1-0
`

func TestParse_CastlingCapturesChecksPromotion(t *testing.T) {
	moves, err := Parse(specialMovesPGN)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	sanByPly := make(map[int]string, len(moves))
	for _, m := range moves {
		sanByPly[m.Ply] = m.SAN
	}

	cases := []struct {
		ply  int
		want string
	}{
		{9, "O-O"},    // white kingside castle
		{14, "O-O"},   // black kingside castle
		{15, "exd5"},  // pawn capture
		{16, "Nxd5"},  // piece capture
		{19, "e8=Q+"}, // promotion with check
		{23, "Qxf7#"}, // capture with checkmate
	}
	for _, c := range cases {
		got, ok := sanByPly[c.ply]
		if !ok {
			t.Errorf("no move found at ply %d", c.ply)
			continue
		}
		if got != c.want {
			t.Errorf("ply %d: SAN = %q, want %q", c.ply, got, c.want)
		}
	}
}

const nagAndVariationPGN = `[Event "Live Chess"]
[Result "1-0"]

1. e4 $1 e5 (1... c5 2. Nf3 d6) 2. Nf3 {[%clk 0:09:55]} Nc6 1-0
`

func TestParse_SkipsNAGsAndVariations(t *testing.T) {
	moves, err := Parse(nagAndVariationPGN)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Only the mainline moves should appear - the variation's "c5"/"d6"
	// must not leak in, and the $1 NAG must not be mistaken for a move.
	wantSAN := []string{"e4", "e5", "Nf3", "Nc6"}
	if len(moves) != len(wantSAN) {
		t.Fatalf("expected %d moves, got %d: %+v", len(wantSAN), len(moves), moves)
	}
	for i, want := range wantSAN {
		if moves[i].SAN != want {
			t.Errorf("move %d: SAN = %q, want %q", i, moves[i].SAN, want)
		}
	}
}
