package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"dasyra/internal/app"
	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/boot"
	"dasyra/internal/arch/llmapi"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

type benchOptions struct {
	Model, Effort, BaseURL, Kind, KeyEnv string
	Timeout                              time.Duration
	DryRun                               bool
	NoEmbeddings                         bool
	Roles                                map[string]benchRole
}

type benchRole struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

type benchStep struct {
	Request llm.Request `json:"request"`
	Reply   llm.Message `json:"reply"`
	Error   string      `json:"error,omitempty"`
}

type benchCall struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Result string `json:"result"`
}

type benchResult struct {
	Case      benchCase   `json:"case"`
	Repeat    int         `json:"repeat"`
	Seconds   float64     `json:"seconds"`
	Sent      []string    `json:"sent"`
	Reactions []string    `json:"reactions"`
	Calls     []benchCall `json:"tools"`
	Steps     []benchStep `json:"steps"`
	Before    benchState  `json:"before"`
	After     benchState  `json:"after"`
	Usage     []benchRow  `json:"usage"`
	Logs      string      `json:"logs"`
	Error     string      `json:"error,omitempty"`
}

type benchReport struct {
	Started  time.Time     `json:"started"`
	Revision string        `json:"revision"`
	Modified bool          `json:"modified"`
	Options  benchOptions  `json:"options"`
	Results  []benchResult `json:"results"`
}

func bench(log *slog.Logger, args []string) error {
	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	casesPath := flags.String("cases", "var/bench/cases.json", "scenario JSON file")
	only := flags.String("only", "", "comma-separated case ids or glob patterns")
	out := flags.String("out", "", "new output directory (default var/bench/runs/<time>)")
	list := flags.Bool("list", false, "list cases without opening a database or calling a model")
	repeat := flags.Int("repeat", 1, "independent runs of each case")
	rolesPath := flags.String("roles", "", "per-role model/effort JSON file")
	opts := benchOptions{}
	flags.StringVar(&opts.Model, "model", "", "model override for all scenario modes, otherwise role defaults")
	flags.StringVar(&opts.Effort, "effort", "", "reasoning effort override: none, low, medium, high, xhigh")
	flags.StringVar(&opts.BaseURL, "base-url", "", "provider API base URL, otherwise OpenRouter")
	flags.StringVar(&opts.Kind, "kind", dom.ProviderOpenRouter, "provider kind: openrouter or openai")
	flags.StringVar(&opts.KeyEnv, "key-env", "OPENROUTER_API_KEY", "environment variable containing the API key")
	flags.DurationVar(&opts.Timeout, "timeout", 3*time.Minute, "maximum duration of one scenario")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "assemble and save prompts without calling a model")
	flags.BoolVar(&opts.NoEmbeddings, "no-embeddings", false, "use FTS alone; dry runs always skip embeddings")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *repeat < 1 || opts.Timeout <= 0 {
		return errors.New("bench: unexpected arguments, invalid repeat or timeout")
	}
	if opts.Kind != dom.ProviderOpenRouter && opts.Kind != dom.ProviderOpenAI {
		return errors.New("bench: kind must be openrouter or openai")
	}
	if opts.Kind == dom.ProviderOpenAI && opts.BaseURL == "" {
		return errors.New("bench: openai kind requires -base-url")
	}
	if !benchEffort(opts.Effort) {
		return errors.New("bench: unknown reasoning effort")
	}
	if *rolesPath != "" {
		var err error
		opts.Roles, err = readBenchRoles(*rolesPath)
		if err != nil {
			return err
		}
	}
	cases, err := readBenchCases(*casesPath, *only)
	if err != nil {
		return err
	}
	if *list {
		for _, c := range cases {
			fmt.Printf("%-32s %-10s %-12s %s\n", c.ID, c.Mode, c.Group, c.Title)
		}
		return nil
	}
	key := os.Getenv(opts.KeyEnv)
	if !opts.DryRun && key == "" && opts.Kind == dom.ProviderOpenRouter {
		return fmt.Errorf("bench: %s is empty", opts.KeyEnv)
	}
	if *out == "" {
		*out = filepath.Join("var", "bench", "runs", time.Now().Format("20060102-150405.000000000"))
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		return err
	}
	// Refuse reuse: a report and its per-case databases are never overwritten by another run.
	if err := os.Mkdir(*out, 0700); err != nil {
		return err
	}
	report := benchReport{Started: time.Now(), Options: opts}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				report.Revision = s.Value
			case "vcs.modified":
				report.Modified = s.Value == "true"
			}
		}
	}
	if err := saveBenchReport(*out, report); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := 0
	for _, c := range cases {
		for n := 1; n <= *repeat; n++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := fmt.Sprintf("%s-%02d", c.ID, n)
			log.Info("Bench scenario", "id", c.ID, "repeat", n)
			result := runBenchCase(ctx, c, opts, filepath.Join(*out, name+".sqlite"), key, nil)
			result.Repeat = n
			if result.Error != "" {
				failures++
			}
			report.Results = append(report.Results, result)
			fmt.Print(benchCaseText(result))
			if err := saveBenchReport(*out, report); err != nil {
				return err
			}
		}
	}
	fmt.Printf("\nResults: %s\n", filepath.Join(*out, "report.md"))
	if failures > 0 {
		return fmt.Errorf("bench: %d scenarios had execution errors; results saved", failures)
	}
	return nil
}

// A supplied client is used by infrastructure tests; normal runs construct the real provider here.
func runBenchCase(ctx context.Context, c benchCase, opts benchOptions, dbPath, key string, client llm.Client) (result benchResult) {
	result.Case = c
	start := time.Now()
	defer func() { result.Seconds = time.Since(start).Seconds() }()
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	zone, _ := time.LoadLocation(c.User.Timezone)
	at, _ := c.time(c.At)
	restore := app.FreezeClock(at)
	defer restore()
	db, err := sqlite.Open(dbPath, zone)
	if err != nil {
		result.Error = err.Error()
		return
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		result.Error = err.Error()
		return
	}
	var logs strings.Builder
	log := slog.New(slog.NewTextHandler(&logs, nil))
	defer func() { result.Logs = logs.String() }()
	if client == nil {
		client = &llmapi.Client{Key: key, Kind: opts.Kind, BaseURL: opts.BaseURL, Usage: sqlite.UsageLog{DB: db}, Log: log}
	}
	settings := sqlite.AISettings{DB: db}
	for role, cfg := range opts.Roles {
		if err := settings.SaveRole(ctx, dom.AIRole{Role: role, Model: cfg.Model, Effort: cfg.Effort}); err != nil {
			result.Error = err.Error()
			return
		}
	}
	var embed tool.Embedder
	if !opts.DryRun && !opts.NoEmbeddings {
		embed = &llmapi.Router{Settings: settings, Log: log, Default: &llmapi.Client{Key: key, Kind: opts.Kind, BaseURL: opts.BaseURL, Usage: sqlite.UsageLog{DB: db}, Log: log}}
	}
	ai := &benchAI{client: client, opts: opts, steps: &result.Steps}
	chat := &benchChat{sent: &result.Sent, reactions: &result.Reactions}
	a := boot.BuildBench(ctx, boot.Config{ServerZone: zone}, db, log, ai, chat, embed, func(t tool.Tool) tool.Tool {
		return benchTool{Tool: t, mocks: c.Mocks, calls: &result.Calls}
	})
	a.Dasyra.PartPause = 0
	u, err := seedBench(ctx, db, c)
	if err != nil {
		result.Error = err.Error()
		return
	}
	if result.Before, err = readBenchState(ctx, db); err != nil {
		result.Error = err.Error()
		return
	}
	switch c.Mode {
	case "chat":
		a.Dasyra.Speak(ctx, u, app.Run{})
	case "checkin":
		a.Dasyra.Speak(ctx, u, app.Run{Proactive: true})
	case "reminder":
		err = a.Reminders.Run(ctx)
	case "reflection":
		err = a.Reflection.Reflect(ctx, u)
	}
	result.Calls = benchTranscriptCalls(result.Steps, result.Calls)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		result.Error = err.Error()
	}
	for _, step := range result.Steps {
		if step.Error != "" {
			result.Error = step.Error
		}
	}
	// Inspection remains possible after a timeout; it uses a fresh, bounded context.
	inspect, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if result.After, err = readBenchState(inspect, db); err != nil {
		result.Error = err.Error()
	}
	if result.Usage, err = benchRows(inspect, db, "SELECT model, purpose, prompt_tokens, cached_tokens, completion_tokens, cost FROM ai_usage ORDER BY id"); err != nil {
		result.Error = err.Error()
	}
	return
}

type benchAI struct {
	client llm.Client
	opts   benchOptions
	steps  *[]benchStep
}

func (a *benchAI) Chat(ctx context.Context, req llm.Request) (llm.Message, error) {
	if role, ok := a.opts.Roles[req.Model.Role]; ok {
		if role.Model != "" {
			req.Model.ID = role.Model
		}
		if role.Effort != "" {
			req.Model.Effort = role.Effort
		}
	}
	if a.opts.Model != "" {
		req.Model.ID = a.opts.Model
	}
	if a.opts.Effort != "" {
		req.Model.Effort = a.opts.Effort
	}
	var reply llm.Message
	var err error
	if a.opts.DryRun {
		reply = llm.Message{Role: "assistant", Text: "NO_MESSAGE"}
	} else {
		reply, err = a.client.Chat(ctx, req)
	}
	step := benchStep{Request: req, Reply: reply}
	// Keep the live loop's reasoning intact, but reports contain only messages and tool calls.
	step.Reply.Reasoning = nil
	step.Request.Messages = append([]llm.Message(nil), req.Messages...)
	for i := range step.Request.Messages {
		step.Request.Messages[i].Reasoning = nil
	}
	if err != nil {
		step.Error = err.Error()
	}
	*a.steps = append(*a.steps, step)
	return reply, err
}

type benchChat struct {
	sent, reactions *[]string
}

func (c *benchChat) Send(_ context.Context, _ int64, text string) error {
	*c.sent = append(*c.sent, text)
	return nil
}
func (c *benchChat) React(_ context.Context, _, _ int64, emoji string) error {
	*c.reactions = append(*c.reactions, emoji)
	return nil
}
func (*benchChat) Typing(context.Context, int64) {}
func (*benchChat) Download(context.Context, string) ([]byte, error) {
	return nil, errors.New("bench has no Telegram file downloads")
}

type benchTool struct {
	tool.Tool
	mocks []benchMock
	calls *[]benchCall
}

// Enabled keeps a tool only some users have hidden the same way when wrapped.
func (t benchTool) Enabled(s *tool.Session) bool { return tool.Enabled(t.Tool, s) }

func (t benchTool) Run(ctx context.Context, s *tool.Session, args tool.Args) (string, error) {
	var result string
	var err error
	if externalBenchTool(t.Spec().Name) {
		result = "Error: no bench fixture matches this external tool call."
		for _, m := range t.mocks {
			if m.Tool == t.Spec().Name && m.matches(args) {
				result = m.Result
				break
			}
		}
	} else {
		result, err = t.Tool.Run(ctx, s, args)
	}
	logged := result
	if err != nil {
		logged = "Error: " + err.Error()
	}
	*t.calls = append(*t.calls, benchCall{Tool: t.Spec().Name, Args: args.JSON(), Result: logged})
	return result, err
}

func externalBenchTool(name string) bool {
	return name == tool.WebSearchName || name == tool.WebFetchName || name == tool.FetchJSONName || name == tool.ReadDocumentName
}

// Include rejected/unknown calls too: the registry decorator only sees tools that actually ran.
func benchTranscriptCalls(steps []benchStep, executed []benchCall) []benchCall {
	var calls []benchCall
	used := make([]bool, len(executed))
	for i, step := range steps {
		for _, call := range step.Reply.ToolCalls {
			entry := benchCall{Tool: call.Name, Args: tool.ParseArgs(call.Arguments).JSON(), Result: "No tool result recorded; this call may not have been executed."}
			for j, actual := range executed {
				if !used[j] && actual.Tool == entry.Tool && actual.Args == entry.Args {
					entry.Result, used[j] = actual.Result, true
					break
				}
			}
			found := false
			for _, next := range steps[i+1:] {
				for _, msg := range next.Request.Messages {
					if msg.Role == "tool" && msg.ToolCallID == call.ID {
						entry.Result, found = msg.Text, true
						break
					}
				}
				if found {
					break
				}
			}
			calls = append(calls, entry)
		}
	}
	return calls
}

func writeBenchJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeBenchFile(path, append(data, '\n'))
}

func writeBenchFile(path string, data []byte) error {
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
