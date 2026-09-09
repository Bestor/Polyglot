// Package pgn extracts the move list (SAN + per-move clock time) from a
// chess.com game's PGN text. Deliberately does not replay moves into board
// state - no legality checking, no per-move FEN - since that would need a
// real chess move-generation engine, a materially bigger dependency than
// text parsing for a scope that only needs SAN + clock (see
// internal/chesscom's schema notes on why a per-move fen_after column is
// explicitly out of scope). Pure string in, struct out - no I/O, so this
// package is trivially unit-testable and has no dependency on anything
// else in internal/chesscom.
package pgn

import (
	"regexp"
	"strconv"
	"strings"
)

// Move is one half-move (ply) extracted from a game's movetext.
type Move struct {
	Ply        int    // 1-based half-move index
	MoveNumber int    // (Ply+1)/2 - the conventional move number both colors share
	Color      string // "white" | "black"
	SAN        string
	// ClockSeconds is nil when the move carries no %clk annotation -
	// expected for daily/correspondence games, not an error.
	ClockSeconds *float64
}

var (
	// leadingMoveNumberRe strips a move-number prefix ("1.", "12...", ...)
	// whether or not it's followed by a space before the SAN move -
	// chess.com's own export always includes the space, but this is
	// robust to either style. A token left empty after stripping was
	// purely a move-number marker, not a move.
	leadingMoveNumberRe = regexp.MustCompile(`^\d+\.+`)

	// clkRe extracts a %clk annotation's value from within a comment's
	// full text, regardless of what else that comment contains (e.g. an
	// %eval annotation alongside it).
	clkRe = regexp.MustCompile(`\[%clk\s+([0-9:.]+)\]`)
)

var resultTokens = map[string]bool{"1-0": true, "0-1": true, "1/2-1/2": true, "*": true}

// Parse extracts the move list from a full PGN game (the header block plus
// movetext, exactly as chess.com's API returns it in a game's pgn field).
// Variations (parenthesized sub-lines) are skipped entirely - chess.com's
// own generated PGN never includes them for a played game, only NAGs and
// comments, which are handled directly.
func Parse(pgnText string) ([]Move, error) {
	tokens := tokenize(stripHeaders(pgnText))

	var moves []Move
	ply := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" || strings.HasPrefix(tok, "{") {
			continue
		}
		if resultTokens[tok] {
			continue
		}
		if strings.HasPrefix(tok, "$") {
			continue // NAG, e.g. $1
		}

		san := leadingMoveNumberRe.ReplaceAllString(tok, "")
		if san == "" {
			continue // was purely a move-number marker like "1." or "12..."
		}

		ply++
		color := "white"
		if ply%2 == 0 {
			color = "black"
		}
		move := Move{Ply: ply, MoveNumber: (ply + 1) / 2, Color: color, SAN: san}

		// A clock comment, when present, immediately follows its move
		// token in chess.com's own export format.
		if i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "{") {
			if secs, ok := parseClockComment(tokens[i+1]); ok {
				move.ClockSeconds = &secs
			}
			i++
		}

		moves = append(moves, move)
	}

	return moves, nil
}

// stripHeaders drops the PGN header block (lines starting with "["), plus
// the blank line separating it from the movetext, returning everything
// from the first non-header, non-blank line onward.
func stripHeaders(pgnText string) string {
	lines := strings.Split(pgnText, "\n")
	start := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[") {
			continue
		}
		start = i
		break
	}
	return strings.Join(lines[start:], "\n")
}

// tokenize splits movetext on whitespace, except that a {...} comment is
// kept as one token (starting with "{", used as a marker by Parse) and a
// (...) variation is dropped entirely, nesting-aware.
func tokenize(s string) []string {
	var tokens []string
	var buf strings.Builder
	var braceBuf strings.Builder
	inBrace := false
	depth := 0

	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	for _, r := range s {
		switch {
		case inBrace:
			if r == '}' {
				tokens = append(tokens, "{"+braceBuf.String()+"}")
				braceBuf.Reset()
				inBrace = false
			} else {
				braceBuf.WriteRune(r)
			}
		case depth > 0:
			switch r {
			case '(':
				depth++
			case ')':
				depth--
			}
		case r == '{':
			flush()
			inBrace = true
		case r == '(':
			flush()
			depth++
		case r == ' ' || r == '\n' || r == '\t' || r == '\r':
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// parseClockComment extracts and parses a %clk annotation from a full
// comment token (including its surrounding braces), returning the clock
// value in seconds. ok is false when the comment has no %clk annotation at
// all (e.g. a pure %eval comment, or none).
func parseClockComment(commentToken string) (seconds float64, ok bool) {
	m := clkRe.FindStringSubmatch(commentToken)
	if m == nil {
		return 0, false
	}
	return parseClockDuration(m[1])
}

// parseClockDuration parses a chess.com %clk value, "H:MM:SS" or
// "H:MM:SS.f" (fractional seconds appear at very fast time controls).
func parseClockDuration(s string) (float64, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, err1 := strconv.Atoi(parts[0])
	minutes, err2 := strconv.Atoi(parts[1])
	seconds, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return float64(hours)*3600 + float64(minutes)*60 + seconds, true
}
