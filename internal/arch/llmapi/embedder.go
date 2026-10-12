package llmapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const embeddingTimeout = 30 * time.Second

// embed calls the OpenAI-style /embeddings endpoint, which OpenRouter and most compatible APIs (Ollama too) serve.
func (c *Client) embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": model, "input": texts})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, embeddingTimeout)
	defer cancel()
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage *usage `json:"usage"`
	}
	if err := c.post(ctx, "/embeddings", "application/json", bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	if resp.Usage != nil {
		c.record(ctx, model, *resp.Usage)
	}
	vectors := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index >= 0 && d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}
	for i, v := range vectors {
		if len(v) == 0 {
			return nil, fmt.Errorf("no embedding for input %d", i)
		}
	}
	return vectors, nil
}
