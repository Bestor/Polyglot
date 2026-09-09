package warmer

import (
	"bufio"
	"os"
	"strings"
)

// ReadLines reads a newline-delimited identifier list: blank lines and
// lines whose first non-whitespace character is "#" are ignored, every
// other line is returned verbatim (e.g. a Riot ID "OrBest#NA1" or a
// chess.com username) - no inline "# comment" stripping, since "#" can
// also be meaningful within an identifier itself (as it is for a Riot ID),
// so stripping it would be ambiguous.
//
// A missing file is treated the same as an empty one (nil, nil) rather
// than an error, so a fresh checkout with no watchlist file yet doesn't
// crash a warmer binary - see RunPass, which logs and skips a cycle for
// either case.
func ReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
