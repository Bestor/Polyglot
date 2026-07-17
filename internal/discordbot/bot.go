package discordbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/bwmarrin/discordgo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const askCommandName = "ask"

// discordMaxMessageLength is Discord's hard cap on a message's content
// field. InteractionResponseEdit rejects anything longer outright, the
// same way it rejects empty content - see finalizeAnswer.
const discordMaxMessageLength = 2000

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
		content, attachment := finalizeAnswer(question, answer, err)

		edit := &discordgo.WebhookEdit{Content: &content}
		if attachment != nil {
			edit.Files = []*discordgo.File{attachment}
		}

		if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
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

// finalizeAnswer turns Answer's result into content (and, sometimes, a
// file attachment) guaranteed safe to hand to InteractionResponseEdit:
// never empty (Discord's API rejects empty content outright, error 50006 -
// which is exactly what happened when Answer legitimately returned "" with
// no error, e.g. a tool-use round whose final turn had no text content)
// and never longer than Discord's discordMaxMessageLength (which Answer's
// own MaxTokens budget doesn't guarantee - a large table can still exceed
// it even at a generous token budget, and Discord rejects that outright
// too, error 50035/BASE_TYPE_MAX_LENGTH).
//
// An answer that's too long is never truncated - it's attached as a
// complete .md file instead, so nothing the model wrote is lost. This is
// purely a Discord-transport concern: the AI is never told about it and
// never sees a difference in how it's asked to write an answer - it always
// writes as if going straight into the message body, and this function
// decides after the fact how to actually deliver it.
func finalizeAnswer(question, answer string, err error) (content string, attachment *discordgo.File) {
	if err != nil {
		slog.Error("discordbot: answer failed", "question", question, "error", err)
		return "Sorry, something went wrong answering that.", nil
	}

	if strings.TrimSpace(answer) == "" {
		slog.Error("discordbot: answer succeeded but produced no text", "question", question)
		return "Sorry, I wasn't able to put together a complete answer to that - try narrowing the question (e.g. fewer players or matches at once) and asking again.", nil
	}

	if len(answer) > discordMaxMessageLength {
		slog.Warn("discordbot: answer exceeded Discord's message length limit, attaching as a file instead", "question", question, "length", len(answer))
		return "Your answer was too long for a Discord message, so here it is as a file instead:", &discordgo.File{
			Name:        "answer.md",
			ContentType: "text/markdown",
			Reader:      strings.NewReader(answer),
		}
	}

	return answer, nil
}
