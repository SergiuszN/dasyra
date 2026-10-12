package llm

import (
	"context"
	"log/slog"
)

// Executor runs one tool call and returns its result for the model.
type Executor func(ctx context.Context, name, arguments string) string

// Loop lets the model call tools until it answers with text.
type Loop struct {
	Client        Client
	Model         Model
	MaxIterations int
	ErrorText     string // the answer when the model fails
	Log           *slog.Logger
}

const finalizeText = "You have reached the maximum number of tool calls. Please provide your final answer based on the information you have gathered. Do not call any more tools."

// Run returns the model's final answer, ErrorText when it could not get one.
func (l Loop) Run(ctx context.Context, messages []Message, tools []Tool, exec Executor) string {
	for iteration := 1; iteration <= l.MaxIterations; iteration++ {
		reply, err := l.Client.Chat(ctx, Request{Model: l.Model, Messages: messages, Tools: tools})
		if err != nil {
			l.Log.Error("AI client error", "error", err)
			return l.ErrorText
		}

		messages = append(messages, reply)
		if len(reply.ToolCalls) == 0 {
			return reply.Text
		}
		for _, call := range reply.ToolCalls {
			messages = append(messages, Message{Role: "tool", ToolCallID: call.ID, Text: exec(ctx, call.Name, call.Arguments)})
		}

		// Three steps before the limit the model has to answer with what it has
		if iteration == l.MaxIterations-3 {
			return l.finalize(ctx, messages, tools)
		}
	}
	return l.ErrorText
}

func (l Loop) finalize(ctx context.Context, messages []Message, tools []Tool) string {
	reply, err := l.Client.Chat(ctx, Request{Model: l.Model, Messages: append(messages, User(finalizeText)), Tools: tools})
	if err != nil || reply.Text == "" {
		l.Log.Error("AI client error on final response", "error", err)
		return l.ErrorText
	}
	return reply.Text
}
