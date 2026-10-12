// Command dasyra runs the bot: the webhook server with the in-process scheduler, or one job at a time.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dasyra/internal/app/tool"
	"dasyra/internal/arch/boot"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/arch/telegram"
	"dasyra/internal/dom"
)

const usage = `usage: dasyra <command>
  serve      webhook server, admin panel and scheduled jobs (default)
  poll       long polling instead of the webhook, for local runs
  migrate    apply database migrations
  backup <file>
             consistent copy of the database, also while the bot runs
  invite     create an invite link
  proactive  run check-ins once, whatever the hour
  reminders  run due reminders once
  reflect    run the nightly reflection once
  index      embed every new or changed memory, message and note now (the nightly run does it before reflection)
  chat [-as <user id>] [-at "2006-01-02 15:04"] <message>
             send a message as the owner (or another user, at another time) and print the reply,
             Telegram faked: live checks and replays on a DB copy
  search [results|answer|research] <query>
             run one web search as the owner and print the result: checks that search works`

const shutdownGrace = 2 * time.Minute

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel()}))
	if err := run(log, command()); err != nil {
		log.Error("dasyra failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, cmd string) error {
	cfg, err := boot.ConfigFromEnv()
	if err != nil {
		return err
	}
	// Message, memory and tool call times are the server's wall clock
	time.Local = cfg.ServerZone
	db, err := sqlite.Open(cfg.DBPath, cfg.ServerZone)
	if err != nil {
		return err
	}
	defer db.Close()
	if applied, err := db.Migrate(context.Background()); err != nil {
		return err
	} else if len(applied) > 0 {
		log.Info("Migrations applied", "names", applied)
	}

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	work, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	a := boot.Build(work, cfg, db, log)
	if err := a.Users.BindOwnerChat(stop, cfg.OwnerChatID); err != nil {
		return err
	}

	switch cmd {
	case "serve":
		return serve(stop, work, a)
	case "poll":
		telegram.Poll(stop, a.Telegram, a.Inbox, log)
		a.Work.Wait()
		return nil
	case "migrate":
		return nil
	case "backup":
		if len(os.Args) < 3 {
			return errors.New(usage)
		}
		return db.Backup(stop, os.Args[2])
	case "invite":
		return invite(stop, a)
	case "proactive":
		return a.CheckIns.Run(work)
	case "reminders":
		return a.Reminders.Run(work)
	case "reflect":
		return a.ReflectAll(work)
	case "index":
		return a.IndexAll(work)
	case "search":
		return search(work, a)
	case "chat":
		return chat(work, a)
	}
	return errors.New(usage)
}

// serve runs until a signal, then stops taking work and lets replies and jobs in progress finish.
func serve(stop, work context.Context, a *boot.App) error {
	server := &http.Server{Addr: a.Config.Listen, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	scheduler := a.Scheduler()
	go scheduler.Run(stop, work)
	go func() {
		<-stop.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	a.Log.Info("Dasyra is listening", "addr", a.Config.Listen)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return waitFor(shutdownGrace, a.Work.Wait, scheduler.Wait)
}

func waitFor(grace time.Duration, waits ...func()) error {
	done := make(chan struct{})
	go func() {
		for _, wait := range waits {
			wait()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(grace):
		return errors.New("shutdown grace period passed with work in progress, dropped replies are picked up by reminders")
	}
}

func invite(ctx context.Context, a *boot.App) error {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	code := hex.EncodeToString(buf)
	if err := a.Invites.Create(ctx, code); err != nil {
		return err
	}
	if name, err := a.Telegram.Username(ctx); err == nil && name != "" {
		fmt.Printf("Invite link: https://t.me/%s?start=%s\n", name, code)
		return nil
	}
	fmt.Printf("Invite code: %s (send /start %s to the bot)\n", code, code)
	return nil
}

func search(ctx context.Context, a *boot.App) error {
	mode, words := "results", os.Args[2:]
	if len(words) > 0 && (words[0] == "results" || words[0] == "answer" || words[0] == "research") {
		mode, words = words[0], words[1:]
	}
	query := strings.Join(words, " ")
	if strings.TrimSpace(query) == "" {
		return errors.New("usage: dasyra search [results|answer|research] <query>")
	}
	owner, err := a.Users.Get(ctx, dom.OwnerID)
	if err != nil {
		return err
	}
	args, _ := json.Marshal(map[string]string{"query": query, "mode": mode})
	result, _ := a.Dasyra.Tools.Execute(ctx, tool.NewSession(owner), tool.WebSearchName, string(args))
	fmt.Println(result)
	return nil
}

func command() string {
	if len(os.Args) < 2 {
		return "serve"
	}
	return os.Args[1]
}

func logLevel() slog.Level {
	if os.Getenv("LOG_LEVEL") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
