package tool_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
)

func TestSaveTodosAddsChangesAndDrops(t *testing.T) {
	e := newEnv(t)
	todos := sqlite.Todos{DB: e.db}
	e.registry = tool.NewRegistry(discard, tool.SaveTodos{Todos: todos, IDs: &sqlite.IDs{}})

	got := e.run("save_todos", `{"items":[{"id":0,"text":"Invoice","day":"2026-10-12","status":""},{"text":"Wash brushes","day":""},{"text":"Stories","day":"2026-10-12 Mon 09:00"}]}`)
	if strings.Count(got, "Added #") != 3 {
		t.Fatalf("got %q", got)
	}
	list, _ := todos.Current(ctx, 1, time.Now().Add(-time.Hour), 10)
	if len(list) != 3 || list[0].Day != "2026-10-12" || list[1].Day != "2026-10-12" || list[2].Day != "" {
		t.Fatalf("open items by day, no day last: %+v", list)
	}

	invoice, brushes, stories := list[0].ID, list[2].ID, list[1].ID
	got = e.run("save_todos", fmt.Sprintf(`{"items":[{"id":%d,"status":"done"},{"id":%d,"day":"2026-10-13","text":"Wash the brushes"},{"id":%d,"status":"dropped"},{"id":42,"status":"done"}]}`, invoice, brushes, stories))
	if !strings.Contains(got, "(done)") || !strings.Contains(got, "Dropped") || !strings.Contains(got, "#42 not found") {
		t.Fatalf("got %q", got)
	}
	done, _ := todos.Get(ctx, 1, invoice)
	moved, _ := todos.Get(ctx, 1, brushes)
	dropped, _ := todos.Get(ctx, 1, stories)
	if !done.Done() || moved.Day != "2026-10-13" || moved.Text != "Wash the brushes" || dropped != nil {
		t.Fatalf("done %+v, moved %+v, dropped %+v", done, moved, dropped)
	}

	list, _ = todos.Current(ctx, 1, time.Now().Add(time.Hour), 10)
	if len(list) != 1 || list[0].ID != brushes {
		t.Fatalf("done before doneSince must be gone: %+v", list)
	}
	if got := e.run("save_todos", `{"items":[{"text":""}]}`); !strings.Contains(got, "needs text") {
		t.Fatalf("got %q", got)
	}
}
