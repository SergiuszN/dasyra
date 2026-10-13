package boot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"dasyra/internal/app"
	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/httpd"
	"dasyra/internal/arch/llmapi"
	"dasyra/internal/arch/schedule"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/arch/telegram"
	"dasyra/internal/arch/web"
)

// App is the wired application.
type App struct {
	Config     Config
	DB         *sqlite.DB
	Users      sqlite.Users
	Invites    sqlite.Invites
	Telegram   *telegram.Client
	Dasyra     *app.Dasyra
	Reflection *app.Reflection
	Meaning    tool.Meaning
	CheckIns   *app.CheckIns
	Reminders  *app.Reminders
	Inbox      *app.Inbox
	Work       *app.Background
	AI         *llmapi.Router
	Log        *slog.Logger
}

// Build wires everything on an open database. work is the context replies and jobs run on.
func Build(work context.Context, cfg Config, db *sqlite.DB, log *slog.Logger) *App {
	return build(work, cfg, db, log, nil)
}

func build(work context.Context, cfg Config, db *sqlite.DB, log *slog.Logger, bench *benchPorts) *App {
	ids := &sqlite.IDs{}
	users, messages, memories, events := sqlite.Users{DB: db}, sqlite.Messages{DB: db}, sqlite.Memories{DB: db}, sqlite.Events{DB: db}
	toolCalls, notes, todos := sqlite.ToolCalls{DB: db}, sqlite.Notes{DB: db}, sqlite.Todos{DB: db}
	ai := &llmapi.Router{Settings: sqlite.AISettings{DB: db}, Log: log,
		Default: &llmapi.Client{Key: cfg.OpenRouterKey, Usage: sqlite.UsageLog{DB: db}, Log: log}}
	chat := &telegram.Client{Token: cfg.TelegramToken, Log: log}
	var messenger app.Messenger = chat
	var model llm.Client = ai

	meaning := tool.Meaning{Vectors: sqlite.Vectors{DB: db}, Embed: ai, Log: log}
	if bench != nil {
		messenger, model = bench.chat, bench.ai
		meaning = tool.Meaning{Log: log}
		if bench.embed != nil {
			meaning.Vectors, meaning.Embed = sqlite.Vectors{DB: db}, bench.embed
		}
	}
	recall := tool.Recall{Memories: memories, Meaning: meaning}
	reader := tool.TextReader{AI: model, Log: log}
	pdf := &web.PDF{Log: log}
	fetch := tool.WebFetch{Pages: web.NewPages(pdf, false), Reader: reader, Log: log}
	chatTools := []tool.Tool{
		tool.CreateMemory{Memories: memories, IDs: ids},
		tool.UpdateMemory{Memories: memories},
		tool.DeleteMemory{Memories: memories},
		tool.ListMemories{Memories: memories, Recall: recall},
		tool.SaveEvent{Events: events, IDs: ids},
		tool.DeleteEvent{Events: events},
		tool.ListEvents{Events: events},
		tool.SaveTodos{Todos: todos, IDs: ids},
		tool.SaveNote{Notes: notes, IDs: ids},
		tool.FindNotes{Notes: notes, Meaning: meaning},
		tool.SendNote{Notes: notes, Messages: messages, Chat: messenger, IDs: ids},
		tool.DeleteNote{Notes: notes},
		fetch,
		tool.WebSearch{Search: web.NewSearch(cfg.SearxngURL, log), Fetch: fetch, AI: model, Log: log},
		tool.FetchJSON{API: web.NewJSON(false)},
		tool.ReadDocument{Messages: messages, Files: messenger, PDF: pdf, Reader: reader},
		tool.SearchOldMessages{Messages: messages, Meaning: meaning},
		tool.UpdateProfile{Users: users},
		tool.RecentResults{ToolCalls: toolCalls},
	}
	reflectionTools := []tool.Tool{
		tool.CreateMemory{Memories: memories, IDs: ids},
		tool.UpdateMemory{Memories: memories},
		tool.MergeMemories{Memories: memories},
		tool.DeleteMemory{Memories: memories},
		tool.UpdateProfile{Users: users},
	}
	if bench != nil {
		for i, t := range chatTools {
			chatTools[i] = bench.decorate(t)
		}
		for i, t := range reflectionTools {
			reflectionTools[i] = bench.decorate(t)
		}
	}

	dasyra := &app.Dasyra{Users: users, Messages: messages, Memories: memories, Events: events, Notes: notes, Todos: todos, Recall: recall, ToolCalls: toolCalls,
		Tools: tool.NewRegistry(log, chatTools...), AI: model, Chat: messenger, IDs: ids, Log: log, PartPause: 2 * time.Second}
	bg := app.NewBackground(work)
	return &App{
		Config: cfg, DB: db, Meaning: meaning, Users: users, Invites: sqlite.Invites{DB: db}, Telegram: chat, Dasyra: dasyra, Work: bg, AI: ai, Log: log,
		Reflection: &app.Reflection{Users: users, Messages: messages, Memories: memories, Tools: tool.NewRegistry(log, reflectionTools...), AI: model, Log: log},
		CheckIns:   &app.CheckIns{Users: users, Messages: messages, Plans: sqlite.Settings{DB: db}, Dasyra: dasyra, Gate: ai, Zone: cfg.ServerZone, Log: log},
		Reminders:  &app.Reminders{Users: users, Messages: messages, Events: events, Dasyra: dasyra, Log: log},
		Inbox: &app.Inbox{Users: users, Invites: sqlite.Invites{DB: db}, Messages: messages, Dasyra: dasyra, Chat: chat,
			Voice: ai, IDs: ids, Log: log, Debounce: 2 * time.Second, Work: bg, Timezone: cfg.ServerZone.String()},
	}
}

// IndexAll embeds every new or changed record of every user.
func (a *App) IndexAll(ctx context.Context) error {
	users, err := a.Users.AllWithChat(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := a.Meaning.IndexAll(ctx, u.ID); err != nil {
			return fmt.Errorf("user %d: %w", u.ID, err)
		}
		a.Log.Info("Records embedded", "user_id", u.ID)
	}
	return nil
}

// ReflectAll runs the nightly memory upkeep for every user.
func (a *App) ReflectAll(ctx context.Context) error {
	a.Log.Info("Nightly reflection triggered")
	users, err := a.Users.AllWithChat(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		// The day's messages and notes get their vectors, searches then embed only what came after
		if err := a.Meaning.IndexAll(ctx, u.ID); err != nil {
			a.Log.Error("Embedding the records failed", "user_id", u.ID, "error", err)
		}
		if err := a.Reflection.Reflect(ctx, u); err != nil {
			a.Log.Error("Reflection failed.", "user_id", u.ID, "error", err)
		}
	}
	return nil
}

// Scheduler holds the periodic jobs, at server-time hours.
func (a *App) Scheduler() *schedule.Scheduler {
	return &schedule.Scheduler{Location: a.Config.ServerZone, Log: a.Log, Jobs: []*schedule.Job{
		// Every hour: the check-in hours are set in the admin panel
		{Name: "proactive", When: schedule.EveryMinutes(60), Run: a.CheckIns.Scheduled},
		{Name: "reminders", When: schedule.EveryMinutes(5), Run: a.Reminders.Run},
		{Name: "reflect", When: schedule.AtHours(3), Run: a.ReflectAll},
	}}
}

// Handler serves the Telegram webhook and the admin panel.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /telegram/webhook", &telegram.Webhook{Secret: a.Config.WebhookSecret, Inbox: a.Inbox, Log: a.Log})
	admin := &httpd.Admin{Report: sqlite.Report{DB: a.DB}, Users: a.Users, AI: sqlite.AISettings{DB: a.DB}, Plans: sqlite.Settings{DB: a.DB}, CheckAI: a.AI.Check, SuggestPrices: a.AI.SuggestPrices,
		Login: a.Config.AdminLogin, Password: a.Config.AdminPassword, Log: a.Log}
	admin.Register(mux)
	return mux
}
