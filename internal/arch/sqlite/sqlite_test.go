package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"dasyra/internal/dom"
)

var (
	ctx       = context.Background()
	warsaw, _ = time.LoadLocation("Europe/Warsaw")
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"), warsaw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrateIsIdempotentAndSeedsTheOwner(t *testing.T) {
	db := openTest(t)
	applied, err := db.Migrate(ctx)
	if err != nil || len(applied) != 0 {
		t.Fatalf("second run applied %v, %v", applied, err)
	}
	owner, err := Users{db}.Get(ctx, dom.OwnerID)
	if err != nil || owner == nil || owner.Name != "" || owner.HasProfile() || owner.Timezone != "UTC" {
		t.Fatalf("owner %+v %v", owner, err)
	}
}

func TestUsersBindOwnerAndRoundTrip(t *testing.T) {
	users := Users{openTest(t)}
	must(t, users.BindOwnerChat(ctx, 555))
	must(t, users.BindOwnerChat(ctx, 777)) // bound only once
	u, err := users.ByChat(ctx, 555)
	if err != nil || u == nil || u.ID != dom.OwnerID {
		t.Fatalf("by chat %+v %v", u, err)
	}

	u.Timezone, u.Profile, u.LastReflectedAt = "Europe/Kyiv", "new profile", time.Date(2026, 10, 9, 3, 0, 0, 0, warsaw)
	must(t, users.Update(ctx, u))
	got, _ := users.Get(ctx, dom.OwnerID)
	if got.Timezone != "Europe/Kyiv" || got.Profile != "new profile" || !got.LastReflectedAt.Equal(u.LastReflectedAt) {
		t.Fatalf("got %+v", got)
	}
	if all, _ := users.AllWithChat(ctx); len(all) != 1 {
		t.Fatalf("users with chat: %d", len(all))
	}
}

func TestInvitesAreSingleUse(t *testing.T) {
	invites := Invites{openTest(t)}
	must(t, invites.Create(ctx, "abc", "Danil"))
	if ok, _ := invites.Available(ctx, "abc"); !ok {
		t.Fatal("new invite must be available")
	}
	must(t, invites.Use(ctx, "abc", 2))
	if ok, _ := invites.Available(ctx, "abc"); ok {
		t.Fatal("used invite must not be available")
	}

	must(t, invites.Create(ctx, "def", ""))
	must(t, invites.Revoke(ctx, "abc")) // used ones stay
	must(t, invites.Revoke(ctx, "def"))
	all, err := invites.All(ctx)
	if err != nil || len(all) != 1 || all[0].Code != "abc" || all[0].Note != "Danil" || all[0].UserID != 2 || !all[0].Used() || all[0].CreatedAt.IsZero() {
		t.Fatalf("invites %+v %v", all, err)
	}
}

func TestDisabledUserKeepsChatButLeavesTheJobs(t *testing.T) {
	users := Users{openTest(t)}
	must(t, users.BindOwnerChat(ctx, 555))
	must(t, users.Insert(ctx, &dom.User{ID: 2, Name: "Danil", ChatID: 777, Timezone: "UTC", CreatedAt: time.Now()}))
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, warsaw)
	must(t, users.SetDisabled(ctx, 2, at))

	u, _ := users.ByChat(ctx, 777)
	if u == nil || !u.Disabled() || !u.DisabledAt.Equal(at) {
		t.Fatalf("disabled user %+v", u)
	}
	must(t, users.Update(ctx, u)) // a reply saving the user it loaded leaves the switch alone
	if active, _ := users.AllWithChat(ctx); len(active) != 1 || active[0].ID != dom.OwnerID {
		t.Fatalf("active %+v", active)
	}
	if all, _ := users.All(ctx); len(all) != 2 {
		t.Fatalf("all %+v", all)
	}
	must(t, users.SetDisabled(ctx, 2, time.Time{}))
	if active, _ := users.AllWithChat(ctx); len(active) != 2 {
		t.Fatalf("back on: %+v", active)
	}
}

func message(id int64, role dom.Role, content string, at time.Time) *dom.Message {
	return &dom.Message{ID: id, Role: role, Content: content, CreatedAt: at, TelegramID: id}
}

func TestMessagesHistoryAndCounters(t *testing.T) {
	messages := Messages{openTest(t)}
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	must(t, messages.Insert(ctx, 1, message(1, dom.RoleUser, "привет", base)))
	reply := message(2, dom.RoleAssistant, "check-in", base.Add(time.Minute))
	reply.TelegramID, reply.CheckIn = 0, true
	must(t, messages.Insert(ctx, 1, reply))
	must(t, messages.Insert(ctx, 2, message(3, dom.RoleUser, "чужое", base)))

	last, err := messages.Last(ctx, 1, 10)
	if err != nil || len(last) != 2 || last[0].ID != 2 || !last[0].CheckIn || !last[1].CreatedAt.Equal(base) {
		t.Fatalf("last %+v %v", last, err)
	}
	if n, _ := messages.Count(ctx, 1); n != 2 {
		t.Fatalf("count %d", n)
	}
	if n, _ := messages.CheckInsSinceUserMessage(ctx, 1); n != 1 {
		t.Fatalf("check-ins %d", n)
	}
	if has, _ := messages.HasUserMessageAfter(ctx, 1, 1); has {
		t.Fatal("no user message after 1")
	}
	if seen, _ := messages.Seen(ctx, 1, 1, base.Add(-time.Minute)); !seen {
		t.Fatal("telegram message 1 was seen")
	}
	after, _ := messages.After(ctx, 1, base.Add(-time.Second), 10)
	if len(after) != 2 || after[0].ID != 1 {
		t.Fatalf("after %+v", after)
	}
}

func TestMessagesSearchAndDocuments(t *testing.T) {
	messages := Messages{openTest(t)}
	now := time.Now().Truncate(time.Second)
	must(t, messages.Insert(ctx, 1, message(1, dom.RoleUser, "Хочу купить мотоцикл Honda", now.AddDate(0, -1, 0))))
	doc := message(2, dom.RoleUser, `[file "menu.pdf", read_document id 2]`, now)
	doc.DocumentFileID = "file-xyz"
	must(t, messages.Insert(ctx, 1, doc))

	found, err := messages.Search(ctx, 1, dom.SearchTerms("мотоциклы"), 10, nil, nil)
	if err != nil || len(found) != 1 || found[0].ID != 1 {
		t.Fatalf("search %+v %v", found, err)
	}
	from := now.AddDate(0, 0, -1)
	if found, _ := messages.Search(ctx, 1, dom.SearchTerms("мотоцикл"), 10, &from, nil); len(found) != 0 {
		t.Fatal("from must filter old messages out")
	}
	if id, _ := messages.DocumentFileID(ctx, 1, 2); id != "file-xyz" {
		t.Fatalf("document %q", id)
	}
}

func TestMemoriesSearchAndArchive(t *testing.T) {
	db := openTest(t)
	memories := Memories{db}
	now := time.Now().Truncate(time.Second)
	m := &dom.Memory{ID: 10, Name: "Музыка", Category: "preferences", Content: "Любит SOAD и Арию", CreatedAt: now, UpdatedAt: now}
	must(t, memories.Insert(ctx, 1, m))
	must(t, memories.Insert(ctx, 1, &dom.Memory{ID: 11, Name: "Dota", Category: "skill", Content: "opendota API", CreatedAt: now, UpdatedAt: now}))

	found, err := memories.Search(ctx, 1, dom.SearchTerms("музыку"), "", 0, 10)
	if err != nil || len(found) != 1 || found[0].ID != 10 {
		t.Fatalf("search %+v %v", found, err)
	}
	if found, _ := memories.Search(ctx, 1, dom.SearchTerms("музыку"), "skill", 0, 10); len(found) != 0 {
		t.Fatal("category must filter")
	}
	if list, _ := memories.List(ctx, 1, "skill", 0, 10); len(list) != 1 || list[0].ID != 11 {
		t.Fatalf("list %+v", list)
	}

	m.Content = "Любит SOAD"
	must(t, memories.Update(ctx, 1, m))
	must(t, memories.Delete(ctx, 1, 11))
	var archived int
	must(t, db.QueryRow("SELECT COUNT(*) FROM memories_archive WHERE user_id = 1").Scan(&archived))
	if got, _ := memories.Get(ctx, 1, 10); archived != 2 || got.Content != "Любит SOAD" {
		t.Fatalf("archived %d, memory %+v", archived, got)
	}
	if got, _ := memories.Get(ctx, 2, 10); got != nil {
		t.Fatal("another user's memory must stay hidden")
	}
}

func TestToolCallsLatestRun(t *testing.T) {
	calls := ToolCalls{openTest(t)}
	must(t, calls.SaveRun(ctx, 1, 100, []dom.ToolCall{{Tool: "web_search", Args: `{"query":"a"}`, Result: "r1"}}))
	must(t, calls.SaveRun(ctx, 1, 200, []dom.ToolCall{{Tool: "web_fetch", Args: `{}`, Result: "r2"}, {Tool: "list_memories", Args: `[]`, Result: "r3"}}))
	latest, err := calls.LatestRun(ctx, 1)
	if err != nil || len(latest) != 2 || latest[0].ReplyID != 200 || latest[1].Result != "r3" {
		t.Fatalf("latest %+v %v", latest, err)
	}
}

func TestBackupIsAWorkingCopy(t *testing.T) {
	db := openTest(t)
	path := filepath.Join(t.TempDir(), "backup.sqlite")
	must(t, db.Backup(ctx, path))
	copied, err := Open(path, warsaw)
	must(t, err)
	defer copied.Close()
	if applied, err := copied.Migrate(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("backup is not migrated: %v %v", applied, err)
	}
}

func TestSearchFindsWordFormsAndDropsCommonTerms(t *testing.T) {
	messages := Messages{openTest(t)}
	now := time.Now().Truncate(time.Second)
	for i := range commonFloor {
		must(t, messages.Insert(ctx, 1, message(int64(100+i), dom.RoleUser, fmt.Sprintf("что-то обычное номер %d", i), now)))
	}
	must(t, messages.Insert(ctx, 1, message(1, dom.RoleUser, "Wystawiłam fakturę dla firmy, что дальше", now)))
	must(t, messages.Insert(ctx, 2, message(2, dom.RoleUser, "faktura", now)))

	found, err := messages.Search(ctx, 1, dom.SearchTerms("Faktura"), 10, nil, nil)
	if err != nil || len(found) != 1 || found[0].ID != 1 {
		t.Fatalf("another form, another user's message left out: %+v %v", found, err)
	}
	if found, _ := messages.Search(ctx, 1, dom.SearchTerms("что обычное"), 10, nil, nil); len(found) != 0 {
		t.Fatalf("terms in most of the user's messages tell nothing: %+v", found)
	}
	if found, _ := messages.Search(ctx, 1, dom.SearchTerms("что faktura"), 10, nil, nil); len(found) != 1 {
		t.Fatalf("the rare term still finds: %+v", found)
	}
}
