package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestLimiter_AllowsBurstThenBlocks(t *testing.T) {
	l := NewLimiter(60, 2) // 60/min = 1/sec, burst 2

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 2; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("unexpected error on burst token %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected burst tokens to be immediate, took %v", elapsed)
	}

	waitStart := time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if waited := time.Since(waitStart); waited < 500*time.Millisecond {
		t.Fatalf("expected to wait for the next token, only waited %v", waited)
	}
}

func TestLimiter_RespectsContextCancellation(t *testing.T) {
	l := NewLimiter(1, 1) // burst of 1, exhausted by the first call

	ctx := context.Background()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("unexpected error consuming burst token: %v", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := l.Wait(cctx); err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
}
