package discordbot

import (
	"context"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bwmarrin/discordgo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const askCommandName = "ask"

// answerTimeout bounds how long a single /ask tool-use loop is allowed to
// run, so a stuck upstream call (Claude, or mcpserver/polyglot) can't hang
// a Discord interaction indefinitely. Generous because a cold sync_matches
// call can take several minutes under HenrikDev's rate-limit backoff
// (2s/4s/8s/16s/32s per retried page, sometimes across several pages) -
// Discord's interaction token stays valid for 15 minutes after a deferred
// response, so that's the real ceiling, not anything shorter.
const answerTimeout = 10 * time.Minute

// RegisterAndServe opens the Discord session, registers the /ask slash
// command, and wires up its handler. guildID empty registers the command
// globally (~1hr propagation); a non-empty guild ID registers it instantly
// for that one server, useful during development.
func RegisterAndServe(session *discordgo.Session, guildID string, ai anthropic.Client, model string, mcpSession *mcp.ClientSession, tools []anthropic.ToolUnionParam) error {
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != askCommandName {
			return
		}
		question := i.ApplicationCommandData().Options[0].StringValue()

		// Tool-use loops take longer than Discord's ~3s initial-response
		// window, so acknowledge immediately and edit in the real answer.
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			slog.Error("discordbot: failed to defer interaction", "error", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
		defer cancel()

		slog.Info("discordbot: ask", "question", question)
		answer, err := Answer(ctx, ai, model, mcpSession, tools, question)
		if err != nil {
			slog.Error("discordbot: answer failed", "question", question, "error", err)
			answer = "Sorry, something went wrong answering that."
		}

		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &answer}); err != nil {
			slog.Error("discordbot: failed to edit interaction response", "error", err)
		}
	})

	if err := session.Open(); err != nil {
		return err
	}

	_, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, &discordgo.ApplicationCommand{
		Name:        askCommandName,
		Description: "Ask a statistical question about a Valorant player",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "Your question",
				Required:    true,
			},
		},
	})
	return err
}
