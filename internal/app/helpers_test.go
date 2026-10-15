package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
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

// script plays the model: each call takes the next reply, the last one repeats.
type script struct {
	mu       sync.Mutex
	replies  []llm.Message
	requests []llm.Request
}

func answers(texts ...string) *script {
	s := &script{}
	for _, t := range texts {
		s.replies = append(s.replies, llm.Message{Role: "assistant", Text: t})
	}
	return s
}

func (s *script) thenCall(name, args string) *script {
	s.replies = append([]llm.Message{{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: name, Name: name, Arguments: args}}}}, s.replies...)
	return s
}

func (s *script) Chat(_ context.Context, r llm.Request) (llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r)
	reply := s.replies[0]
	if len(s.replies) > 1 {
		s.replies = s.replies[1:]
	}
	return reply, nil
}

// prompt is everything the model saw in its first call.
func (s *script) prompt() string {
	var b strings.Builder
	for _, m := range s.requests[0].Messages {
		b.WriteString(m.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

type chat struct {
	mu        sync.Mutex
	sent      []string
	reactions []string
}

func (c *chat) Send(_ context.Context, _ int64, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, text)
	return nil
}

func (c *chat) SendMarkdown(ctx context.Context, chatID int64, text string) error {
	return c.Send(ctx, chatID, "md:"+text)
}

func (c *chat) React(_ context.Context, _, _ int64, emoji string) error {
	c.reactions = append(c.reactions, emoji)
	return nil
}

func (c *chat) Typing(context.Context, int64) {}

func (c *chat) Download(context.Context, string) ([]byte, error) {
	return []byte("\x89PNG\r\n\x1a\n"), nil
}

type world struct {
	db       *sqlite.DB
	ids      *sqlite.IDs
	user     *dom.User
	messages sqlite.Messages
	events   sqlite.Events
	ai       *script
	chat     *chat
	dasyra   *Dasyra
}

func newWorld(t *testing.T, ai *script) *world {
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
	w := &world{db: db, ids: &sqlite.IDs{}, messages: sqlite.Messages{DB: db}, events: sqlite.Events{DB: db}, ai: ai, chat: &chat{}}
	users := sqlite.Users{DB: db}
	if err := users.BindOwnerChat(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	w.user, _ = users.Get(ctx, dom.OwnerID)
	w.user.Name, w.user.Timezone = "Alex", "Europe/Warsaw"
	w.user.Profile = "Alex is 30 and lives in Gdańsk, Poland.\nNorth stars for this year: start an own business, run a marathon.\nRespond in Russian."
	if err := users.Update(ctx, w.user); err != nil {
		t.Fatal(err)
	}
	memories := sqlite.Memories{DB: db}
	w.dasyra = &Dasyra{Users: users, Messages: w.messages, Memories: memories, Events: w.events, Notes: sqlite.Notes{DB: db}, Todos: sqlite.Todos{DB: db}, Recall: tool.Recall{Memories: memories}, ToolCalls: sqlite.ToolCalls{DB: db},
		AI: ai, Chat: w.chat, IDs: w.ids, Log: discard,
		Tools: tool.NewRegistry(discard, tool.CreateMemory{Memories: memories, IDs: w.ids}, tool.ListMemories{Memories: memories, Recall: tool.Recall{Memories: memories}},
			tool.SaveEvent{Events: w.events, IDs: w.ids}, tool.UpdateProfile{Users: users},
			tool.WebSearch{Search: offline{}, Log: discard}, tool.RecentResults{ToolCalls: sqlite.ToolCalls{DB: db}},
			tool.AskExpert{AI: ai, Tools: tool.NewRegistry(discard), Log: discard}),
	}
	return w
}

// say stores a message as if it came at the given time.
func (w *world) say(t *testing.T, role dom.Role, text string, at time.Time) *dom.Message {
	t.Helper()
	m := &dom.Message{ID: w.ids.Next(), Role: role, Content: text, CreatedAt: at}
	if role == dom.RoleUser {
		m.TelegramID = m.ID % 100000
	}
	if err := w.messages.Insert(ctx, w.user.ID, m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (w *world) history(t *testing.T) dom.History {
	t.Helper()
	h, err := w.messages.Last(ctx, w.user.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type offline struct{}

func (offline) Search(context.Context, string) ([]tool.SearchResult, error) {
	return []tool.SearchResult{{Title: "Weather", URL: "https://pogoda.pl", Snippet: "sunny"}}, nil
}

// at fixes the app clock for a test.
func at(t *testing.T, clock time.Time) {
	old := now
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = old })
}
