package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// SaveTodos keeps the to-do list the context shows: a whole list the user sends goes in with one call.
type SaveTodos struct {
	Todos dom.Todos
	IDs   IDs
}

const SaveTodosName = "save_todos"

func (SaveTodos) Spec() llm.Tool {
	return Spec(SaveTodosName, "Keep the user's to-do list (shown in your context with #ids): add the tasks they plan, mark them done, move them to another day, reword or drop them. Several items in one call",
		Objects("items", "Items to add (text and day, no id) or to change (id with the fields that change)",
			Int("id", "Id of an item to change, leave empty to add one"),
			Str("text", "The task in the user's words, short. Required to add"),
			Str("day", "Day it is planned for, Y-m-d. Leave empty for no particular day"),
			Enum("status", "done when finished, dropped to remove it, open to reopen", "open", "done", "dropped"),
		).Required(),
	)
}

func (t SaveTodos) Run(ctx context.Context, s *Session, a Args) (string, error) {
	items := a.Objects("items")
	if len(items) == 0 {
		return "Error: pass the items to add or change.", nil
	}
	results := make([]string, len(items))
	for i, item := range items {
		result, err := t.save(ctx, s, item)
		if err != nil {
			return "", err
		}
		results[i] = result
	}
	return strings.Join(results, "\n"), nil
}

func (t SaveTodos) save(ctx context.Context, s *Session, a Args) (string, error) {
	day, err := s.User.ParseDay(a.String("day"))
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	status := a.String("status")
	if id := a.Int("id"); id > 0 {
		return t.update(ctx, s, id, day, status, a)
	}
	text := a.String("text")
	if text == "" {
		return "Error: a new item needs text.", nil
	}
	todo := &dom.Todo{ID: t.IDs.Next(), Text: text, Day: day, CreatedAt: time.Now()}
	if status == "done" {
		todo.DoneAt = &todo.CreatedAt
	}
	if err := t.Todos.Insert(ctx, s.User.ID, todo); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added #%d %s.", todo.ID, todo.Text), nil
}

func (t SaveTodos) update(ctx context.Context, s *Session, id int64, day, status string, a Args) (string, error) {
	todo, err := t.Todos.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if todo == nil {
		return fmt.Sprintf("Error: item #%d not found.", id), nil
	}
	if status == "dropped" {
		return fmt.Sprintf("Dropped #%d %s.", id, todo.Text), t.Todos.Delete(ctx, s.User.ID, id)
	}
	if text := a.String("text"); text != "" {
		todo.Text = text
	}
	if a.Has("day") {
		todo.Day = day
	}
	switch {
	case status == "done" && !todo.Done():
		done := time.Now()
		todo.DoneAt = &done
	case status == "open":
		todo.DoneAt = nil
	}
	if err := t.Todos.Update(ctx, s.User.ID, todo); err != nil {
		return "", err
	}
	state := "open"
	if todo.Done() {
		state = "done"
	}
	return fmt.Sprintf("Updated #%d %s (%s).", id, todo.Text, state), nil
}
