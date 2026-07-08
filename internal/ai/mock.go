package ai

import (
	"context"
	"fmt"
	"strings"
)

// sanityCheckSQL groups cached match_players rows by player so the mock
// provider can prove the full fetch -> cache -> schema -> query -> answer
// pipeline works end-to-end, without needing any real language
// understanding of the question.
const sanityCheckSQL = `
SELECT p.riot_name, p.riot_tag, COUNT(*) AS matches_cached
FROM match_players mp
JOIN players p ON p.id = mp.player
GROUP BY mp.player
ORDER BY matches_cached DESC
LIMIT 5
`

// MockProvider is a placeholder Provider used until a real AI backend is
// chosen. It ignores the question's meaning, but exercises Request.Query
// for real so the surrounding plumbing (schema introspection, read-only
// executor) can be verified before any model is wired in.
type MockProvider struct{}

func (MockProvider) Answer(ctx context.Context, req Request) (Response, error) {
	result, err := req.Query(ctx, sanityCheckSQL)
	if err != nil {
		return Response{}, fmt.Errorf("mock provider sanity query: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[mock provider] question: %q\n", req.Question)
	fmt.Fprintf(&b, "top cached players by match_players rows:\n")
	for _, row := range result.Rows {
		fmt.Fprintf(&b, "  %v\n", row)
	}
	if len(result.Rows) == 0 {
		fmt.Fprintf(&b, "  (no cached match_players rows yet)\n")
	}

	return Response{Answer: b.String()}, nil
}

var _ Provider = MockProvider{}
