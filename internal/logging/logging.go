// Package logging configures the process-wide slog default logger shared
// by all of val-analyzer's binaries (cmd/polyglot, cmd/mcpserver).
package logging

import (
	"log/slog"
	"os"
)

// Init sets the default slog logger's minimum level: Debug when debug is
// true (full SQL text, request/response payloads, and tool-call arguments
// become visible), Info otherwise (concise summaries only - what happened
// and how long it took, not the full payload).
func Init(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
