package llmapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dasyra/internal/app/llm"
)

// A decision is a few hundred milliseconds, a gate that hangs only delays what it guards
const decisionTimeout = 30 * time.Second

// Probability asks a decision model through OpenRouter's Decisions API: it reads the state and answers a yes/no
// question with the probability of yes.
func (r *Router) Probability(ctx context.Context, m llm.Model, state []string, question string) (float64, error) {
	c, m := r.route(ctx, m)
	return c.probability(ctx, m.ID, state, question)
}

func (c *Client) probability(ctx context.Context, model string, state []string, question string) (float64, error) {
	if !c.openRouter() {
		return 0, fmt.Errorf("%s: decision models are served only by OpenRouter", c.name())
	}
	body, err := json.Marshal(map[string]any{"model": model, "state": state,
		"questions": map[string]any{"yes": map[string]string{"type": "noul", "instructions": question}}})
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, decisionTimeout)
	defer cancel()

	var resp struct {
		Answers map[string]struct {
			Noul *float64 `json:"noul"`
		} `json:"answers"`
		Usage *struct {
			InputTokens  int     `json:"input_tokens"`
			OutputTokens int     `json:"output_tokens"`
			Cost         float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := c.post(ctx, c.decisionsURL(), "application/json", bytes.NewReader(body), &resp); err != nil {
		return 0, err
	}
	if u := resp.Usage; u != nil {
		c.record(ctx, model, usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, Cost: u.Cost})
	}
	yes := resp.Answers["yes"].Noul
	if yes == nil {
		return 0, errors.New("invalid response from the decision model: no answer")
	}
	return *yes, nil
}

// decisionsURL: the Decisions API is an alpha next to v1, https://openrouter.ai/api/alpha/decisions
func (c *Client) decisionsURL() string {
	return strings.TrimSuffix(c.url(""), "/v1") + "/alpha/decisions"
}
