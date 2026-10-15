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
	Memories dom.Seen        // only memories shown in full can be rewritten
	Events   dom.Seen        // only events shown with their id can be changed
	Notes    dom.Seen        // only notes read in full can get new content
	Cookbook *dom.Cookbook   // nil when the user has none connected
	Recipes  map[string]bool // slugs of the cookbook recipes read in full, only they can be rewritten
	Expert   *ExpertAnswer   // set by ask_expert: it ends the run and goes to the user instead of her reply
}

func NewSession(u *dom.User) *Session {
	return &Session{User: u, Memories: dom.Seen{}, Events: dom.Seen{}, Notes: dom.Seen{}, Recipes: map[string]bool{}}
}

type Tool interface {
	Spec() llm.Tool
	// Run returns the result for the model; validation problems are results too, an error means the tool broke
	Run(ctx context.Context, s *Session, a Args) (string, error)
}

// Conditional is a tool only some users have, such as the cookbook ones: the model sees it only when enabled.
type Conditional interface {
	Enabled(s *Session) bool
}

// Enabled tells whether the model may see and call the tool in this run.
func Enabled(t Tool, s *Session) bool {
	c, ok := t.(Conditional)
	return !ok || c.Enabled(s)
}

// Registry runs tools by name.
type Registry struct {
	tools []Tool
	log   *slog.Logger
}

func NewRegistry(log *slog.Logger, tools ...Tool) *Registry {
	return &Registry{tools: tools, log: log}
}

// Specs are the tools enabled for the session, always in the same order: they are part of the cached prompt.
func (r *Registry) Specs(s *Session) []llm.Tool {
	var specs []llm.Tool
	for _, t := range r.tools {
		if Enabled(t, s) {
			specs = append(specs, t.Spec())
		}
	}
	return specs
}

// Execute runs a tool call and returns its result with the arguments that counted.
func (r *Registry) Execute(ctx context.Context, s *Session, name, arguments string) (string, Args) {
	args := ParseArgs(arguments)
	for _, t := range r.tools {
		if t.Spec().Name != name || !Enabled(t, s) {
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
