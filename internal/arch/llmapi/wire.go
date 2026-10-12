package llmapi

import (
	"encoding/json"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// The chat completions format, with OpenRouter's extras.

type request struct {
	Model           string         `json:"model"`
	Messages        []message      `json:"messages"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Tools           []tool         `json:"tools,omitempty"`
	Provider        map[string]any `json:"provider,omitempty"`
	Reasoning       map[string]any `json:"reasoning,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
}

type message struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"` // a string, parts, or null for an assistant message with tool calls only
	ToolCalls  []toolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Reasoning  json.RawMessage `json:"reasoning_details,omitempty"`
}

type part struct {
	Type         string            `json:"type"`
	Text         string            `json:"text,omitempty"`
	ImageURL     map[string]string `json:"image_url,omitempty"`
	CacheControl map[string]string `json:"cache_control,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type tool struct {
	Type     string   `json:"type"`
	Function llm.Tool `json:"function"`
}

type response struct {
	Choices []struct {
		Message struct {
			Content   *string         `json:"content"`
			ToolCalls []toolCall      `json:"tool_calls"`
			Reasoning json.RawMessage `json:"reasoning_details"`
		} `json:"message"`
	} `json:"choices"`
	Usage *usage `json:"usage"`
}

type usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
	Seconds          float64 `json:"seconds"`
	// DeepSeek reports its cache in its own fields
	PromptCacheHit int `json:"prompt_cache_hit_tokens"`
	PromptDetails  struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u usage) record(model string, payer llm.Payer) dom.Usage {
	cached := u.PromptDetails.CachedTokens
	if cached == 0 {
		cached = u.PromptCacheHit
	}
	return dom.Usage{
		UserID: payer.UserID, Purpose: payer.Purpose, Model: model,
		PromptTokens: u.PromptTokens, CachedTokens: cached, CacheWriteTokens: u.PromptDetails.CacheWriteTokens,
		CompletionTokens: u.CompletionTokens, ReasoningTokens: u.CompletionDetails.ReasoningTokens, AudioSeconds: u.Seconds, Cost: u.Cost,
	}
}

// encode builds the request. Plain OpenAI-compatible APIs reject what they don't know, so they get only the
// standard fields: no provider sorting or max_tokens (each model has its own limit), reasoning_effort only when
// reasoning is on, and no reasoning_details passed back.
func encode(r llm.Request, openRouter bool) request {
	tools := make([]tool, len(r.Tools))
	for i, t := range r.Tools {
		tools[i] = tool{Type: "function", Function: t}
	}
	out := request{Model: r.Model.ID, Messages: encodeMessages(r.Model.ID, r.Messages), Tools: tools}
	if openRouter {
		out.MaxTokens = 100000
		out.Provider = map[string]any{"sort": "price"}
		out.Reasoning = map[string]any{"effort": r.Model.Effort, "exclude": true}
		return out
	}
	if r.Model.Effort != "" && r.Model.Effort != "none" {
		out.ReasoningEffort = r.Model.Effort
	}
	for i := range out.Messages {
		out.Messages[i].Reasoning = nil
	}
	return out
}

// encodeMessages adds cache breakpoints: OpenAI models cache the prompt prefix by themselves, Anthropic ones only
// up to cache_control marks (at most 4). Stable parts (system, calendar, history) get one, and so does the last
// message, so each step of a tool loop reads the previous one from the cache.
func encodeMessages(model string, messages []llm.Message) []message {
	anthropic := strings.HasPrefix(model, "anthropic/")
	out := make([]message, len(messages))
	for i, m := range messages {
		out[i] = encodeMessage(m, anthropic && (m.Cache || i == len(messages)-1))
	}
	return out
}

func encodeMessage(m llm.Message, breakpoint bool) message {
	out := message{Role: m.Role, ToolCallID: m.ToolCallID, Reasoning: m.Reasoning}
	for _, c := range m.ToolCalls {
		var call toolCall
		call.ID, call.Type, call.Function.Name, call.Function.Arguments = c.ID, "function", c.Name, c.Arguments
		out.ToolCalls = append(out.ToolCalls, call)
	}

	switch {
	case len(m.Images) > 0:
		parts := []part{{Type: "text", Text: m.Text}}
		for _, url := range m.Images {
			parts = append(parts, part{Type: "image_url", ImageURL: map[string]string{"url": url}})
		}
		if breakpoint {
			parts[len(parts)-1].CacheControl = ephemeral
		}
		out.Content = parts
	case m.Text == "" && m.Role == "assistant":
		out.Content = nil
	case breakpoint && m.Text != "":
		out.Content = []part{{Type: "text", Text: m.Text, CacheControl: ephemeral}}
	default:
		out.Content = m.Text
	}
	return out
}

var ephemeral = map[string]string{"type": "ephemeral"}

func decode(r response) (llm.Message, bool) {
	if len(r.Choices) == 0 {
		return llm.Message{}, false
	}
	m := r.Choices[0].Message
	out := llm.Message{Role: "assistant", Reasoning: m.Reasoning}
	if m.Content != nil {
		out.Text = *m.Content
	}
	for _, c := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	return out, true
}
