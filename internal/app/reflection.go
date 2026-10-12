package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/dom"
)

const (
	reflectionIterations = 20
	reflectionMessages   = 400
	reflectionMemories   = 500
)

// Reflection is the nightly memory upkeep: picks up facts missed during the day, merges duplicates,
// drops stale one-off memories and writes a short journal entry. It never messages the user.
type Reflection struct {
	Users    dom.Users
	Messages dom.Messages
	Memories dom.Memories
	Tools    *tool.Registry // create, update, merge and delete memories, update the profile
	AI       llm.Client
	Log      *slog.Logger
}

func (r *Reflection) Reflect(ctx context.Context, u *dom.User) error {
	ctx = llm.For(ctx, u.ID, llm.PurposeReflection)
	conversation, err := r.Messages.After(ctx, u.ID, u.LastReflectedAt, reflectionMessages)
	if err != nil {
		return err
	}
	if !hasUserMessage(conversation) {
		r.Log.Info("Reflection skipped, nothing new.", "user_id", u.ID)
		return nil
	}

	session := tool.NewSession(u)
	memories, err := r.reviewedMemories(ctx, session)
	if err != nil {
		return err
	}

	from, to := u.Local(conversation[0].CreatedAt), u.Local(conversation[len(conversation)-1].CreatedAt)
	profile := "None yet."
	if u.HasProfile() {
		profile = strings.TrimSpace(u.Profile)
	}
	messages := []llm.Message{
		llm.System(reflectionPrompt(u, u.Local(now()).Format("2006-01-02"), journalName(from.Format("2006-01-02"), to.Format("2006-01-02")))),
		llm.User("Profile:\n" + profile),
		llm.User("Current memories:\n" + formatMemories(memories)),
		llm.User(fmt.Sprintf("Conversation from %s to %s:\n%s", from.Format("2006-01-02"), to.Format("2006-01-02"), chatLines(u, conversation))),
	}

	answer := llm.Loop{Client: r.AI, Model: llm.Reflection, MaxIterations: reflectionIterations, ErrorText: "I had trouble processing your request. Please try again.", Log: r.Log}.
		Run(ctx, messages, r.Tools.Specs(), func(ctx context.Context, name, arguments string) string {
			result, _ := r.Tools.Execute(ctx, session, name, arguments)
			return result
		})
	r.Log.Info("Reflection finished.", "user_id", u.ID, "messages", len(conversation), "answer", answer)

	// Continue from the last reviewed message, so nothing written during the run is skipped next time
	u.LastReflectedAt = conversation[len(conversation)-1].CreatedAt
	return r.Users.Update(ctx, u)
}

// reviewedMemories are all memories but the journal, which only piles up; they are shown in full, so they count as seen.
func (r *Reflection) reviewedMemories(ctx context.Context, s *tool.Session) ([]dom.Memory, error) {
	all, err := r.Memories.List(ctx, s.User.ID, "", 0, reflectionMemories)
	if err != nil {
		return nil, err
	}
	var reviewed []dom.Memory
	for _, m := range all {
		if m.Category != dom.CategoryJournal {
			s.Memories.Mark(m.ID)
			reviewed = append(reviewed, m)
		}
	}
	return reviewed, nil
}

func hasUserMessage(messages []dom.Message) bool {
	for _, m := range messages {
		if m.Role == dom.RoleUser {
			return true
		}
	}
	return false
}

func journalName(fromDay, toDay string) string {
	if fromDay == toDay {
		return "Journal " + fromDay
	}
	return "Journal " + fromDay + " – " + toDay
}

func formatMemories(memories []dom.Memory) string {
	if len(memories) == 0 {
		return "None yet."
	}
	lines := make([]string, len(memories))
	for i, m := range memories {
		lines[i] = fmt.Sprintf("#%d [%s] %s: %s (updated %s)", m.ID, m.Category, m.Name, m.Content, m.UpdatedAt.Format("2006-01-02"))
	}
	return strings.Join(lines, "\n")
}
