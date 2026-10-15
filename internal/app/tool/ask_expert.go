package tool

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

const (
	AskExpertName = "ask_expert"
	// Each round may read several pages, the expert answers with what it has three rounds before the end
	expertIterations = 12
)

// No rules, persona or style on purpose: the expert is a plain assistant, its answer is what the user asked for.
const expertPrompt = "You are a helpful assistant. Current date and time: %s (%s). You can search the web and read pages when the answer depends on current, specific or checkable facts."

// ExpertAnswer goes to the user as the reply: Dasyra's intro line, then the expert's own text.
type ExpertAnswer struct {
	Intro string
	Text  string
}

// Reply is how the answer is stored in the chat: the intro tells her later that these were the expert's words.
func (a ExpertAnswer) Reply() string {
	return a.Intro + "\n\n" + a.Text
}

// AskExpert hands a question to a big model with the web. Its answer replaces her reply: a small model under
// a long prompt and a strict chat style answers hard questions (an email reply, debugging code) much worse.
type AskExpert struct {
	AI    llm.Client
	Tools *Registry // the web tools
	Log   *slog.Logger
}

func (AskExpert) Spec() llm.Tool {
	return Spec(AskExpertName, "Ask the expert, a stronger model with the web. Only when the user explicitly asks for the expert, never on your own. The expert sees nothing but the question, and its answer is sent to the user as your reply, the run ends.",
		Str("question", "The request in the user's language, written as the user's own words in the first person (the expert answers them directly, not you): everything it needs verbatim (the email, code, error, relevant facts about them) and what kind of answer they want").Required(),
		Str("intro", "One short line in the user's language that opens the answer, e.g. \"Спросила эксперта, вот его ответ:\"").Required(),
	)
}

func (t AskExpert) Run(ctx context.Context, s *Session, a Args) (string, error) {
	question := a.String("question")
	if question == "" {
		return "Error: question is required for ask_expert.", nil
	}
	now := s.User.Local(time.Now())
	messages := []llm.Message{
		llm.System(fmt.Sprintf(expertPrompt, now.Format(dom.ShortTime), now.Location())),
		llm.User(question),
	}
	exec := func(ctx context.Context, name, arguments string) string {
		result, _ := t.Tools.Execute(ctx, s, name, arguments)
		return result
	}
	answer := strings.TrimSpace(llm.Loop{Client: t.AI, Model: llm.Expert, MaxIterations: expertIterations, Log: t.Log}.
		Run(llm.For(ctx, 0, llm.PurposeExpert), messages, t.Tools.Specs(s), exec))
	if answer == "" {
		return "Error: the expert did not answer. Tell the user and answer yourself if you can.", nil
	}

	intro := a.String("intro")
	if intro == "" {
		intro = "Expert:"
	}
	s.Expert = &ExpertAnswer{Intro: intro, Text: answer}
	return "The expert's answer was sent to the user as your reply.", nil
}
