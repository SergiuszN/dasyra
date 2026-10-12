package tool_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

type sent struct{ texts []string }

func (s *sent) Send(_ context.Context, _ int64, text string) error {
	s.texts = append(s.texts, text)
	return nil
}

type noteEnv struct {
	*env
	notes    sqlite.Notes
	messages sqlite.Messages
	chat     *sent
}

func newNoteEnv(t *testing.T) *noteEnv {
	e := &noteEnv{env: newEnv(t), chat: &sent{}}
	e.notes, e.messages = sqlite.Notes{DB: e.db}, sqlite.Messages{DB: e.db}
	ids := &sqlite.IDs{}
	e.registry = tool.NewRegistry(discard,
		tool.SaveNote{Notes: e.notes, IDs: ids}, tool.FindNotes{Notes: e.notes},
		tool.SendNote{Notes: e.notes, Messages: e.messages, Chat: e.chat, IDs: ids}, tool.DeleteNote{Notes: e.notes},
	)
	return e
}

func (e *noteEnv) noteID(t *testing.T, result string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(result, "Note #%d", &id); err != nil {
		t.Fatalf("no note id in %q", result)
	}
	return id
}

const recipe = "Шарлотка\n\n4 яйца, 200 г сахара, 160 г муки, 3 яблока.\n\nВзбить яйца с сахаром 7 минут."

func TestSaveNoteReusesCollectionAndRefusesTheSameTitle(t *testing.T) {
	e := newNoteEnv(t)
	e.run("save_note", `{"collection": "Кулинарная книга", "title": "Шарлотка", "content": "`+strings.ReplaceAll(recipe, "\n", `\n`)+`"}`)

	id := e.noteID(t, e.run("save_note", `{"collection": "кулинарная книга", "title": "Борщ", "content": "свёкла"}`))
	if n, _ := e.notes.Get(ctx, 1, id); n.Collection != "Кулинарная книга" {
		t.Fatalf("collection %q", n.Collection)
	}
	if got := e.run("save_note", `{"collection": "Кулинарная книга", "title": "шарлотка", "content": "другая"}`); !strings.Contains(got, "already has note") {
		t.Fatalf("duplicate: %s", got)
	}
	if got := e.run("save_note", `{"collection": "Кулинарная книга", "title": "Плов", "content": ""}`); !strings.Contains(got, "needs collection, title and content") {
		t.Fatalf("no content: %s", got)
	}
}

func TestNoteTextIsReplacedOnlyAfterItWasRead(t *testing.T) {
	e := newNoteEnv(t)
	id := e.noteID(t, e.run("save_note", `{"collection": "Фильмы", "title": "Дюна", "content": "посмотреть"}`))
	e.session = tool.NewSession(e.session.User) // a later run

	if got := e.run("save_note", fmt.Sprintf(`{"id": %d, "content": "посмотрели, 9/10"}`, id)); !strings.Contains(got, "was not read") {
		t.Fatalf("blind rewrite: %s", got)
	}
	e.run("save_note", fmt.Sprintf(`{"id": %d, "tags": "фантастика"}`, id))
	e.run("find_notes", fmt.Sprintf(`{"id": %d}`, id))
	e.run("save_note", fmt.Sprintf(`{"id": %d, "content": "посмотрели, 9/10"}`, id))

	n, _ := e.notes.Get(ctx, 1, id)
	if n.Content != "посмотрели, 9/10" || n.Tags != "фантастика" || n.Title != "Дюна" {
		t.Fatalf("note %+v", n)
	}
}

func TestFindNotesShowsTitlesAndCollections(t *testing.T) {
	e := newNoteEnv(t)
	e.run("save_note", `{"collection": "Кулинарная книга", "title": "Вишнёвый пирог", "tags": "пирог, выпечка", "content": "вишня и тесто"}`)
	e.run("save_note", `{"collection": "Кулинарная книга", "title": "Борщ", "tags": "суп", "content": "свёкла"}`)
	e.run("save_note", `{"collection": "Подарки", "title": "Лампа для Поли", "content": "IKEA"}`)

	if got := e.run("find_notes", `{"collection": "", "query": "", "id": 0}`); !strings.Contains(got, `"name":"Кулинарная книга","notes":2`) || !strings.Contains(got, "Подарки") {
		t.Fatalf("collections: %s", got)
	}
	got := e.run("find_notes", `{"collection": "кулинарная книга", "query": "пироги"}`)
	if !strings.Contains(got, "Вишнёвый пирог") || strings.Contains(got, "Борщ") || strings.Contains(got, "вишня и тесто") {
		t.Fatalf("search shows titles only: %s", got)
	}
}

func TestSendNoteDeliversItAsSavedAndLeavesAShortLine(t *testing.T) {
	e := newNoteEnv(t)
	id := e.noteID(t, e.run("save_note", `{"collection": "Кулинарная книга", "title": "Шарлотка", "content": "4 яйца", "source": "https://example.com/r"}`))

	if got := e.run("send_note", fmt.Sprintf(`{"id": %d}`, id)); !strings.Contains(got, "was sent") {
		t.Fatalf("send: %s", got)
	}
	if len(e.chat.texts) != 1 || e.chat.texts[0] != "Шарлотка\n\n4 яйца\n\nhttps://example.com/r" {
		t.Fatalf("sent %q", e.chat.texts)
	}
	h, _ := e.messages.Last(ctx, 1, 5)
	if len(h) != 1 || h[0].Role != dom.RoleAssistant || !strings.Contains(h[0].Content, `"Шарлотка"`) || strings.Contains(h[0].Content, "4 яйца") {
		t.Fatalf("history %+v", h)
	}

	e.run("delete_note", fmt.Sprintf(`{"id": %d}`, id))
	if n, _ := e.notes.Get(ctx, 1, id); n != nil {
		t.Fatal("not deleted")
	}
	var archived int
	e.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notes_archive WHERE id = ?", id).Scan(&archived)
	if archived != 1 {
		t.Fatalf("archived %d", archived)
	}
}
