// Package boot wires the app together: the composition root of every command.
package boot

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config comes from the environment.
type Config struct {
	DBPath        string
	Listen        string
	OpenRouterKey string
	SearxngURL    string
	BraveKey      string // Brave Search API, the fallback when SearXNG fails
	TelegramToken string
	OwnerChatID   int64 // binds the owner account (user 1) to its chat on first start
	WebhookSecret string
	AdminLogin    string
	AdminPassword string
	ServerZone    *time.Location // message and memory times are stored in it
}

func ConfigFromEnv() (Config, error) {
	zone, err := time.LoadLocation(env("SERVER_TZ", "UTC"))
	if err != nil {
		return Config{}, fmt.Errorf("SERVER_TZ: %w", err)
	}
	owner, _ := strconv.ParseInt(os.Getenv("TELEGRAM_CHAT_ID"), 10, 64)
	return Config{
		DBPath:        env("DB_PATH", "db.sqlite"),
		Listen:        env("LISTEN", ":8080"),
		OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"),
		SearxngURL:    env("SEARXNG_URL", "http://searxng:8080"),
		BraveKey:      os.Getenv("BRAVE_API_KEY"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		OwnerChatID:   owner,
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		AdminLogin:    os.Getenv("ADMIN_LOGIN"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		ServerZone:    zone,
	}, nil
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
