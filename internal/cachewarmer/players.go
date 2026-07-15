// Package cachewarmer implements cmd/cachewarmer's logic: periodically
// calling polyglot's POST /warm for every player listed in a file, so
// caches stay fresh without waiting on a live question to trigger a sync.
package cachewarmer

import (
	"bufio"
	"os"
	"strings"
)

// ReadPlayerTags reads a newline-delimited player list: blank lines and
// lines whose first non-whitespace character is "#" are ignored, every
// other line is used verbatim as a player_tag value (e.g. "OrBest#NA1") -
// no inline "# comment" stripping, since "#" is also the Riot ID
// name/tag separator and stripping it would be ambiguous.
//
// A missing file is treated the same as an empty one (nil, nil) rather
// than an error, so a fresh checkout with no players.txt yet doesn't
// crash cachewarmer - see RunPass, which logs and skips a cycle for
// either case.
func ReadPlayerTags(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var tags []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tags = append(tags, line)
	}
	return tags, scanner.Err()
}
