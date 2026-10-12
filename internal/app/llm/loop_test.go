package llm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type endless struct{ calls int }

// Chat calls a tool forever, unless asked to finalize.
func (e *endless) Chat(_ context.Context, r Request) (Message, error) {
	e.calls++
	if last := r.Messages[len(r.Messages)-1]; last.Text == finalizeText {
		return Message{Role: "assistant", Text: "final"}, nil
	}
	return Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "1", Name: "t", Arguments: "{}"}}}, nil
}

type broken struct{}

func (broken) Chat(context.Context, Request) (Message, error) { return Message{}, errors.New("down") }

var log = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestLoopFinalizesThreeStepsBeforeTheLimit(t *testing.T) {
	ai := &endless{}
	executed := 0
	got := Loop{Client: ai, MaxIterations: 15, ErrorText: "err", Log: log}.Run(context.Background(), []Message{User("hi")}, nil,
		func(context.Context, string, string) string { executed++; return "ok" })
	if got != "final" || executed != 12 || ai.calls != 13 {
		t.Fatalf("got %q, executed %d, calls %d", got, executed, ai.calls)
	}
}

func TestLoopReturnsErrorTextWhenTheModelFails(t *testing.T) {
	got := Loop{Client: broken{}, MaxIterations: 15, ErrorText: "err", Log: log}.Run(context.Background(), nil, nil, nil)
	if got != "err" {
		t.Fatalf("got %q", got)
	}
}

func TestPayerKeepsTheUserForNestedCalls(t *testing.T) {
	ctx := For(context.Background(), 5, PurposeChat)
	nested := For(ctx, 0, PurposeTextReader)
	if p := PayerOf(nested); p.UserID != 5 || p.Purpose != PurposeTextReader || PayerOf(context.Background()).Purpose != PurposeOther {
		t.Fatalf("payer %+v", p)
	}
}
