package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

// topics is a fake embedder: a text about money points one way, anything else the other, in any language.
type topics struct {
	calls  int
	inputs int
	fail   bool
}

func (e *topics) Embed(_ context.Context, texts []string) ([][]float32, string, error) {
	if e.fail {
		return nil, "", errors.New("down")
	}
	e.calls++
	e.inputs += len(texts)
	vectors := make([][]float32, len(texts))
	for i, t := range texts {
		vectors[i] = []float32{0, 1}
		for _, w := range []string{"invoice", "фактур", "faktur", "счёт"} {
			if strings.Contains(strings.ToLower(t), w) {
				vectors[i] = []float32{1, 0}
			}
		}
	}
	return vectors, "fake-model", nil
}

func TestRecallFindsByMeaningAndKeepsVectorsFresh(t *testing.T) {
	e := newEnv(t)
	embed := &topics{}
	recall := tool.Recall{Memories: e.memories, Meaning: tool.Meaning{Vectors: sqlite.Vectors{DB: e.db}, Embed: embed, Log: discard}}
	invoice := &dom.Memory{ID: 1, Name: "Работа", Category: "work", Content: "Каждую пятницу фактура для фирмы", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	dog := &dom.Memory{ID: 2, Name: "Собака", Category: "home", Content: "Собака лает на гостей", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	must(t, e.memories.Insert(ctx, 1, invoice))
	must(t, e.memories.Insert(ctx, 1, dog))
	for id, name := range map[int64]string{3: "Гитара", 4: "Машина"} {
		must(t, e.memories.Insert(ctx, 1, &dom.Memory{ID: id, Name: name, Category: "hobby", Content: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	}

	found, err := recall.Find(ctx, 1, []string{"did I send the invoice?"}, 5)
	if err != nil || len(found) != 1 || found[0].ID != 1 {
		t.Fatalf("no shared word, found by meaning only: %+v %v", found, err)
	}
	if embed.inputs != 5 {
		t.Fatalf("the query and all memories embedded, got %d inputs", embed.inputs)
	}

	recall.Find(ctx, 1, []string{"invoice"}, 5)
	if embed.inputs != 6 {
		t.Fatalf("fresh vectors are not embedded again, got %d inputs", embed.inputs)
	}
	dog.Content, dog.UpdatedAt = "Собака теперь ждёт счёт от ветеринара", time.Now().Add(time.Second)
	must(t, e.memories.Update(ctx, 1, dog))
	found, _ = recall.Find(ctx, 1, []string{"invoice"}, 5)
	if embed.inputs != 8 || len(found) != 2 {
		t.Fatalf("a changed memory is embedded again and found: %d inputs, %+v", embed.inputs, found)
	}

	embed.fail = true
	found, err = recall.Find(ctx, 1, []string{"собака"}, 5)
	if err != nil || len(found) != 1 || found[0].ID != 2 {
		t.Fatalf("words only when embedding fails: %+v %v", found, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMessagesAndNotesAreFoundByMeaning(t *testing.T) {
	e := newEnv(t)
	embed := &topics{}
	vectors := sqlite.Vectors{DB: e.db}
	meaning := tool.Meaning{Vectors: vectors, Embed: embed, Log: discard}
	messages, notes := sqlite.Messages{DB: e.db}, sqlite.Notes{DB: e.db}
	e.registry = tool.NewRegistry(discard, tool.SearchOldMessages{Messages: messages, Meaning: meaning}, tool.FindNotes{Notes: notes, Meaning: meaning})

	old := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	for i, text := range []string{"Wystawiłam fakturę dla firmy", "Собака лает", "Гитара", "Машина", "Погода"} {
		must(t, messages.Insert(ctx, 1, &dom.Message{ID: int64(i + 1), Role: dom.RoleUser, Content: text, CreatedAt: old.AddDate(0, 0, i)}))
	}
	for i, title := range []string{"Счёт за газ", "Шарлотка", "Борщ", "Пицца", "Блины"} {
		must(t, notes.Insert(ctx, 1, &dom.Note{ID: int64(i + 1), Collection: "Разное", Title: title, Content: title, CreatedAt: old, UpdatedAt: old}))
	}

	if got := e.run("search_old_message", `{"query":"invoice"}`); !strings.Contains(got, "fakturę") || strings.Contains(got, "Собака") {
		t.Fatalf("got %s", got)
	}
	if got := e.run("search_old_message", `{"query":"invoice","from":"2026-09-02 00:00:00"}`); strings.Contains(got, "fakturę") {
		t.Fatalf("a meaning hit outside the dates is left out: %s", got)
	}
	if got := e.run("find_notes", `{"query":"invoice"}`); !strings.Contains(got, "Счёт за газ") || strings.Contains(got, "Борщ") {
		t.Fatalf("got %s", got)
	}

	must(t, meaning.IndexAll(ctx, 1))
	for _, kind := range []string{dom.KindMemory, dom.KindMessage, dom.KindNote} {
		if stale, _ := vectors.Stale(ctx, 1, kind, "fake-model", 10); len(stale) != 0 {
			t.Fatalf("%s left stale after the nightly run: %+v", kind, stale)
		}
	}
}
