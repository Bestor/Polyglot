// Command discordbot is an MCP client: it connects to a running mcpserver
// instance, lists its tools, and lets Claude's tool-use loop decide which
// ones to call to answer a Discord user's /ask question.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/bwmarrin/discordgo"

	"val-analyzer/internal/discordbot"
	"val-analyzer/internal/logging"
)

func main() {
	debug := getEnvBool("DEBUG", false)
	logging.Init(debug)

	discordToken := mustEnv("DISCORD_BOT_TOKEN")
	mcpURL := mustEnv("MCP_URL")
	anthropicKey := mustEnv("ANTHROPIC_API_KEY")
	model := getEnvDefault("ANTHROPIC_MODEL", string(anthropic.ModelClaudeOpus4_8))
	guildID := os.Getenv("DISCORD_GUILD_ID") // optional; empty = global command

	ctx := context.Background()

	mcpSession, err := discordbot.NewMCPSession(ctx, mcpURL)
	if err != nil {
		log.Fatalf("connecting to mcpserver at %s: %v", mcpURL, err)
	}
	toolsResult, err := mcpSession.ListTools(ctx, nil)
	if err != nil {
		log.Fatalf("listing mcp tools: %v", err)
	}
	tools := discordbot.ToolsToAnthropic(toolsResult.Tools)

	ai := anthropic.NewClient(option.WithAPIKey(anthropicKey))

	discordSession, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("creating discord session: %v", err)
	}

	if err := discordbot.RegisterAndServe(discordSession, guildID, ai, model, mcpSession, tools); err != nil {
		log.Fatalf("starting discord bot: %v", err)
	}
	defer discordSession.Close()

	log.Printf("discordbot: running, %d tools available, model=%s", len(tools), model)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || v == "true"
}
