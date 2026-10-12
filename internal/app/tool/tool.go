// Package tool holds the functions Dasyra can call. Each tool is a small type with its spec and a Run method.
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// Session is the state of one run: who it is for and what the model has seen with its content.
type Session struct {
	User     *dom.User
	Memories dom.Seen // only memories shown in full can be rewritten
	Events   dom.Seen // only events shown with their id can be changed
	Notes    dom.Seen // only notes read in full can get new content
}

func NewSession(u *dom.User) *Session {
	return &Session{User: u, Memories: dom.Seen{}, Events: dom.Seen{}, Notes: dom.Seen{}}
}

type Tool interface {
	Spec() llm.Tool
	// Run returns the result for the model; validation problems are results too, an error means the tool broke
	Run(ctx context.Context, s *Session, a Args) (string, error)
}

// Registry runs tools by name.
type Registry struct {
	tools []Tool
	log   *slog.Logger
}

func NewRegistry(log *slog.Logger, tools ...Tool) *Registry {
	return &Registry{tools: tools, log: log}
}

func (r *Registry) Specs() []llm.Tool {
	specs := make([]llm.Tool, len(r.tools))
	for i, t := range r.tools {
		specs[i] = t.Spec()
	}
	return specs
}

// Execute runs a tool call and returns its result with the arguments that counted.
func (r *Registry) Execute(ctx context.Context, s *Session, name, arguments string) (string, Args) {
	args := ParseArgs(arguments)
	for _, t := range r.tools {
		if t.Spec().Name != name {
			continue
		}
		result, err := t.Run(ctx, s, args)
		if err != nil {
			r.log.Error("Tool execution failed", "tool", name, "error", err)
			return fmt.Sprintf("Error executing %s: %s", name, err), args
		}
		return result, args
	}
	return "Unknown tool: " + name, args
}

// toJSON encodes tool output the way the model reads it best: unescaped unicode, slashes and ampersands.
func toJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return `{"error":"` + err.Error() + `"}`
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
