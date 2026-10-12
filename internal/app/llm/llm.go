// Package llm is the application's view of a chat model: messages, tools and the tool loop.
// The wire format (OpenRouter, cache breakpoints) lives in arch/llmapi.
package llm

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role       string // system, user, assistant, tool
	Text       string
	Images     []string // data URLs, sent as image parts after Text
	ToolCalls  []ToolCall
	ToolCallID string
	Reasoning  json.RawMessage // reasoning_details, passed back so the model keeps its thinking across tool calls
	Cache      bool            // a stable prefix part: Anthropic models cache only up to such breakpoints
}

func System(text string) Message { return Message{Role: "system", Text: text} }
func User(text string) Message   { return Message{Role: "user", Text: text} }

// Cached marks a stable part of the prompt, see Message.Cache.
func (m Message) Cached() Message {
	m.Cache = true
	return m
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON
}

// Tool is a function the model may call, Parameters is its JSON schema.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Request struct {
	Model    Model
	Messages []Message
	Tools    []Tool
}

type Client interface {
	// Chat returns the assistant message, an error when there is none
	Chat(ctx context.Context, r Request) (Message, error)
}
