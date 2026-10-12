package tool_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

var (
	ctx     = context.Background()
	discard = slog.New(slog.NewTextHandler(io.Discard, nil))
)

type env struct {
	db       *sqlite.DB
	memories sqlite.Memories
	events   sqlite.Events
	session  *tool.Session
	registry *tool.Registry
}

func newEnv(t *testing.T) *env {
	t.Helper()
	zone, _ := time.LoadLocation("Europe/Warsaw")
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.sqlite"), zone)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ids := &sqlite.IDs{}
	e := &env{db: db, memories: sqlite.Memories{DB: db}, events: sqlite.Events{DB: db}}
	e.session = tool.NewSession(&dom.User{ID: 1, Name: "Alex", Timezone: "Europe/Kyiv"})
	e.registry = tool.NewRegistry(discard,
		tool.CreateMemory{Memories: e.memories, IDs: ids}, tool.UpdateMemory{Memories: e.memories},
		tool.MergeMemories{Memories: e.memories}, tool.ListMemories{Memories: e.memories, Recall: tool.Recall{Memories: e.memories}},
		tool.SaveEvent{Events: e.events, IDs: ids}, tool.DeleteEvent{Events: e.events}, tool.ListEvents{Events: e.events},
		tool.UpdateProfile{Users: sqlite.Users{DB: db}},
	)
	return e
}

func (e *env) run(name, args string) string {
	result, _ := e.registry.Execute(ctx, e.session, name, args)
	return result
}

func TestArgsDropWhatModelsFillOptionalFieldsWith(t *testing.T) {
	a := tool.ParseArgs(`{"id": 0, "query": "  ", "category": null, "ids": [], "limit": "5", "recurring": false, "name": " x "}`)
	if a.Has("query") || a.Has("category") || a.Has("ids") || !a.Has("id") || !a.Has("recurring") {
		t.Fatalf("args %v", a)
	}
	if a.Int("limit") != 5 || a.String("name") != "x" || a.IntOr("offset", 20) != 20 {
		t.Fatalf("args %v", a)
	}
	if tool.ParseArgs("not json").JSON() != "[]" {
		t.Fatal("broken arguments are empty")
	}
}

func TestSpecKeepsPropertyOrder(t *testing.T) {
	spec := tool.Spec("t", "d", tool.Str("b", "B").Required(), tool.Int("a", "A"), tool.Enum("mode", "M", "x", "y"))
	want := `{"properties":{"b":{"description":"B","type":"string"},"a":{"description":"A","type":"integer"},"mode":{"description":"M","enum":["x","y"],"type":"string"}},"required":["b"],"type":"object"}`
	if string(spec.Parameters) != want {
		t.Fatalf("got  %s\nwant %s", spec.Parameters, want)
	}
	if !json.Valid(tool.Spec("e", "d").Parameters) {
		t.Fatal("no parameters")
	}
}

func TestUnknownToolAndBrokenTool(t *testing.T) {
	e := newEnv(t)
	if got := e.run("nope", "{}"); got != "Unknown tool: nope" {
		t.Fatalf("got %q", got)
	}
}

func TestMemoryIsUpdatedOnlyAfterItWasSeen(t *testing.T) {
	e := newEnv(t)
	created := e.run("create_memory", `{"name":"Музыка","category":"preferences","content":"Любит Iron Maiden и SOAD"}`)
	if !strings.HasPrefix(created, "Memory created with id ") {
		t.Fatalf("created %q", created)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(created, "Memory created with id "), ".")

	fresh := tool.NewSession(e.session.User)
	if got, _ := e.registry.Execute(ctx, fresh, "update_memory", `{"id":`+id+`,"content":"x"}`); !strings.Contains(got, "was not read in this session") {
		t.Fatalf("blind update allowed: %q", got)
	}
	if got := e.run("list_memories", `{"query":"музыка"}`); !strings.Contains(got, "Iron Maiden") {
		t.Fatalf("list %q", got)
	}
	if got := e.run("update_memory", `{"id":`+id+`,"content":"Любит SOAD"}`); got != "Memory #"+id+" updated." {
		t.Fatalf("update %q", got)
	}
}

func TestListMemoriesWithoutQueryShowsTitlesOnly(t *testing.T) {
	e := newEnv(t)
	e.run("create_memory", `{"name":"Dota","category":"skill","content":"secret recipe"}`)
	got := e.run("list_memories", `{"limit":-1}`)
	if strings.Contains(got, "secret recipe") || !strings.Contains(got, "| Name: Dota | Category: skill") {
		t.Fatalf("got %q", got)
	}
}

func TestMergeKeepsTheFirstMemory(t *testing.T) {
	e := newEnv(t)
	first := e.run("create_memory", `{"name":"Bike","category":"hobby","content":"Honda"}`)
	second := e.run("create_memory", `{"name":"Moto","category":"hobby","content":"CB500"}`)
	ids := idOf(first) + "," + idOf(second)
	if got := e.run("merge_memories", `{"ids":[`+ids+`],"name":"Мотоцикл","category":"hobby","content":"Honda CB500"}`); got != "Merged 2 memories into #"+idOf(first)+"." {
		t.Fatalf("merge %q", got)
	}
	if got := e.run("merge_memories", `{"ids":[1,1],"name":"a","category":"b","content":"c"}`); got != "Error: merge needs at least two memory ids." {
		t.Fatalf("got %q", got)
	}
}

func idOf(created string) string {
	return strings.TrimSuffix(strings.TrimPrefix(created, "Memory created with id "), ".")
}

func TestSaveEventCreatesInTheUsersZoneAndUpdatesOnlySeenEvents(t *testing.T) {
	e := newEnv(t)
	created := e.run("save_event", `{"id":-1,"event":"Позвонить маме","start_time":"2030-10-10 09:00:00","location":"","repeat_days":[]}`)
	if !strings.HasPrefix(created, "Event created successfully with ID: ") {
		t.Fatalf("created %q", created)
	}
	id := strings.TrimPrefix(created, "Event created successfully with ID: ")
	if got := e.run("save_event", `{"id":`+id+`,"event":"Позвонить маме","start_time":"2030-10-10 10:00:00"}`); !strings.Contains(got, "read in this session") {
		t.Fatalf("blind update allowed: %q", got)
	}
	if got := e.run("list_events", `{"query":"маме"}`); !strings.Contains(got, `"start_time":"2030-10-10 09:00:00"`) {
		t.Fatalf("list %q", got)
	}
	if got := e.run("save_event", `{"id":`+id+`,"event":"Позвонить маме","start_time":"2030-10-10T07:00:00Z"}`); got != "Event updated successfully: Позвонить маме" {
		t.Fatalf("update %q", got)
	}
	events, _ := e.events.Find(ctx, e.session.User, dom.EventFilter{Query: "маме", Limit: 10})
	if len(events) != 1 || events[0].Start.Format(dom.DateTimeLayout) != "2030-10-10 10:00:00" {
		t.Fatalf("events %+v", events)
	}
	if got := e.run("delete_event", `{"id":`+id+`}`); got != "Event deleted successfully." {
		t.Fatalf("delete %q", got)
	}
}

func TestSaveEventRepeatDaysAndBadDates(t *testing.T) {
	e := newEnv(t)
	e.run("save_event", `{"event":"Дайджест","start_time":"2030-10-10 08:00","repeat_days":["Sunday","mon","tue","wed","thu","fri","sat"]}`)
	events, _ := e.events.Find(ctx, e.session.User, dom.EventFilter{Query: "Дайджест", Limit: 10})
	if len(events) != 1 || events[0].RepeatDays != "mon,tue,wed,thu,fri,sat,sun" {
		t.Fatalf("events %+v", events)
	}
	if got := e.run("save_event", `{"event":"x","start_time":"завтра"}`); !strings.HasPrefix(got, "Error saving event: ") {
		t.Fatalf("got %q", got)
	}
}

func TestUpdateProfileValidatesTheZone(t *testing.T) {
	e := newEnv(t)
	if got := e.run("update_profile", `{"timezone":"Kyiv"}`); !strings.HasPrefix(got, "Error: unknown time zone Kyiv") {
		t.Fatalf("got %q", got)
	}
	if got := e.run("update_profile", `{"timezone":"Europe/Kyiv","name":""}`); got != "Profile updated successfully." || e.session.User.Name != "Alex" {
		t.Fatalf("got %q", got)
	}
	if got := e.run("update_profile", `{}`); got != "Error: profile is required." {
		t.Fatalf("got %q", got)
	}
}

type scriptedAI struct{ answer string }

func (s scriptedAI) Chat(_ context.Context, r llm.Request) (llm.Message, error) {
	return llm.Message{Role: "assistant", Text: s.answer + " (" + r.Model.ID + ")"}, nil
}

func TestTextReaderMarksTruncation(t *testing.T) {
	r := tool.TextReader{AI: scriptedAI{"ok"}, Log: discard}
	got := r.Answer(ctx, strings.Repeat("я", 20), "", 10, "page")
	if got != "ok (openai/gpt-6-luna)\n\n(Only the first 10 characters of the page were read.)" {
		t.Fatalf("got %q", got)
	}
}
