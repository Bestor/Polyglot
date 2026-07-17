package discordbot

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFinalizeAnswer_PassesThroughNormalAnswer(t *testing.T) {
	content, attachment := finalizeAnswer("how many kills?", "OrBest had 20 kills.", nil)
	if content != "OrBest had 20 kills." {
		t.Errorf("expected the answer to pass through unchanged, got %q", content)
	}
	if attachment != nil {
		t.Error("expected no attachment for a normal-length answer")
	}
}

func TestFinalizeAnswer_ErrorProducesFallback(t *testing.T) {
	content, attachment := finalizeAnswer("q", "", errors.New("boom"))
	if content == "" {
		t.Fatal("expected a non-empty fallback message on error")
	}
	if strings.Contains(content, "boom") {
		t.Error("expected the raw error not to be leaked into the user-facing message")
	}
	if attachment != nil {
		t.Error("expected no attachment on error")
	}
}

// TestFinalizeAnswer_EmptySuccessProducesFallback reproduces the actual
// production incident: Answer returned "" with a nil error (a tool-use
// round whose final turn had no text content), and the old code sent that
// straight to Discord, which rejects empty content outright (50006).
func TestFinalizeAnswer_EmptySuccessProducesFallback(t *testing.T) {
	for _, answer := range []string{"", "   ", "\n\t"} {
		content, attachment := finalizeAnswer("q", answer, nil)
		if content == "" {
			t.Errorf("finalizeAnswer(%q) returned empty content - Discord would reject this", answer)
		}
		if attachment != nil {
			t.Errorf("finalizeAnswer(%q) unexpectedly returned an attachment", answer)
		}
	}
}

// TestFinalizeAnswer_AttachesOverlongAnswer reproduces the second
// production incident: a genuinely long answer (e.g. a big multi-player
// table) triggered Discord's own BASE_TYPE_MAX_LENGTH rejection (error
// 50035) on send. The fix attaches the complete, untruncated answer as a
// file instead of losing any of it - the AI is never involved in or aware
// of this decision, it's a pure post-processing step here.
func TestFinalizeAnswer_AttachesOverlongAnswer(t *testing.T) {
	huge := strings.Repeat("x", discordMaxMessageLength*2)
	content, attachment := finalizeAnswer("q", huge, nil)

	if len(content) > discordMaxMessageLength {
		t.Errorf("expected the message content itself to be under %d characters, got %d", discordMaxMessageLength, len(content))
	}
	if attachment == nil {
		t.Fatal("expected an attachment for an overlong answer")
	}
	if attachment.Name == "" {
		t.Error("expected the attachment to have a filename")
	}

	got, err := io.ReadAll(attachment.Reader)
	if err != nil {
		t.Fatalf("reading attachment: %v", err)
	}
	if string(got) != huge {
		t.Error("expected the attachment to contain the complete, untruncated answer")
	}
}
