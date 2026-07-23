package valorant

import (
	"testing"
	"time"

	"val-analyzer/internal/valorant/ingest"
)

func TestCoverageSufficient(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		coverage ingest.CoverageResult
		opts     ingest.SyncOptions
		want     bool
	}{
		{
			name:     "history exhausted satisfies a full-history request",
			coverage: ingest.CoverageResult{HistoryExhausted: true},
			opts:     ingest.SyncOptions{All: true},
			want:     true,
		},
		{
			name:     "full history request is never sufficient short of exhausted, even with plenty cached",
			coverage: ingest.CoverageResult{Count: 500, HistoryExhausted: false},
			opts:     ingest.SyncOptions{All: true, MaxMatches: 50},
			want:     false,
		},
		{
			name:     "plain count request never short-circuits on count alone - only checking upstream proves freshness",
			coverage: ingest.CoverageResult{Count: 50, HistoryExhausted: false},
			opts:     ingest.SyncOptions{MaxMatches: 50},
			want:     false,
		},
		{
			name:     "plain count request not yet satisfied",
			coverage: ingest.CoverageResult{Count: 10, HistoryExhausted: false},
			opts:     ingest.SyncOptions{MaxMatches: 50},
			want:     false,
		},
		{
			// Regression test: a player whose history was fully walked
			// backward once (HistoryExhausted=true) but who has played new
			// matches since must still be re-synced by a plain /warm
			// sync_matches call - HistoryExhausted proves nothing about
			// staleness at the newest end. This was the actual production
			// bug (OrBest#NA1's cache silently stuck days stale despite
			// repeated /warm calls).
			name:     "history exhausted does not satisfy a plain recency request",
			coverage: ingest.CoverageResult{Count: 500, HistoryExhausted: true},
			opts:     ingest.SyncOptions{MaxMatches: 50},
			want:     false,
		},
		{
			name:     "history exhausted satisfies a date-range request, however early",
			coverage: ingest.CoverageResult{Count: 500, HistoryExhausted: true},
			opts:     ingest.SyncOptions{Since: timePtr(now.Add(-24 * time.Hour))},
			want:     true,
		},
		{
			name:     "date-range request satisfied when cached oldest predates Since",
			coverage: ingest.CoverageResult{Count: 5, Oldest: now.Add(-72 * time.Hour), HistoryExhausted: false},
			opts:     ingest.SyncOptions{Since: timePtr(now.Add(-24 * time.Hour))},
			want:     true,
		},
		{
			name:     "date-range request not satisfied when cached oldest is after Since",
			coverage: ingest.CoverageResult{Count: 5, Oldest: now.Add(-1 * time.Hour), HistoryExhausted: false},
			opts:     ingest.SyncOptions{Since: timePtr(now.Add(-24 * time.Hour))},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coverageSufficient(tt.coverage, tt.opts); got != tt.want {
				t.Errorf("coverageSufficient(%+v, %+v) = %v, want %v", tt.coverage, tt.opts, got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }
