package boot

import (
	"context"
	"log/slog"

	"dasyra/internal/app"
	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
)

type benchPorts struct {
	ai       llm.Client
	chat     app.Messenger
	embed    tool.Embedder
	decorate func(tool.Tool) tool.Tool
}

// BuildBench uses the production wiring with a captured chat, a recorded model and fixed external tool
// results. Storage tools and prompt assembly stay real; a nil embedder selects FTS alone.
func BuildBench(ctx context.Context, cfg Config, db *sqlite.DB, log *slog.Logger, ai llm.Client, chat app.Messenger, embed tool.Embedder, decorate func(tool.Tool) tool.Tool) *App {
	return build(ctx, cfg, db, log, &benchPorts{ai: ai, chat: chat, embed: embed, decorate: decorate})
}
