# Prompt bench

`dasyra bench` runs conversation scenarios through the current compiled prompts, tool schemas, model loop and
SQLite tools. It prints results for human review; it does not grade model behaviour. A scenario supplies the
profile, prior conversation, memories, events, tasks, notes, latest message and review notes. Review notes and
source annotations are kept out of the model context.

Build after editing a prompt: prompts are embedded in the binary. `make bench` rebuilds and loads `.env` when
present. The default case file is `var/bench/cases.json`; conversation-derived suites and reports belong in ignored
`var/bench` or outside the public repository. `example.json` is an independent demonstration fixture.

```sh
make bench BENCH_ARGS='-cases bench/example.json -dry-run'
make bench BENCH_ARGS='-cases var/bench/cases.json -roles var/bench/roles.json'
make bench BENCH_ARGS='-only "*reminder*,*agreement*" -roles var/bench/roles.json -repeat 3'
./bin/dasyra bench -list
```

Each case and repetition starts in a fresh database. The command never opens `DB_PATH`, reads production provider
settings or sends Telegram messages. Models use the key named by `-key-env` (`OPENROUTER_API_KEY` by default).
Semantic search uses the configured embedding model; `-no-embeddings` selects FTS alone. Web search, page reading,
JSON fetching and document reading return fixed tool fixtures instead of accessing outside content. An unmatched
external call returns an error to the model and appears in the report. This isolates the conversation prompts:
web synthesis, TextReader and transcription are not exercised by these fixtures. Check-in cases invoke the
check-in prompt directly, without the scheduler's eligibility checks or decision-model gate. Reminder cases use
the actual due-event job and its trigger. Reflection cases use the actual nightly reflection flow.

`-dry-run` saves assembled requests with a stub `NO_MESSAGE`; it makes no AI or embedding calls and is only for
inspecting fixtures and prompts. It is not a behavioural baseline. Case times are fixed, in the user's zone.
Generated storage ids and tool-write timestamps still come from the real storage implementation.

## Settings and output

A role settings file uses the same role keys as the admin panel. For example:

```json
{
  "dasyra": {"model": "your/chat-model", "effort": "xhigh"},
  "dasyra_check_in": {"model": "your/chat-model", "effort": "high"},
  "reflection": {"model": "your/reflection-model", "effort": "high"}
}
```

Omitted settings use role defaults from `internal/app/llm/models.go`. `-model` and `-effort` override model calls
across scenario modes, after role settings; they do not change the embedding model. All configured roles use the
same provider (`-kind openrouter` by default). For an OpenAI-compatible provider use `-kind openai -base-url <API
base URL> -key-env <key variable>`. Local providers may leave that key variable empty.

Results are saved after every case, by default in a new `var/bench/runs/<timestamp>` directory:

- `report.md`: context, review notes, delivered messages/reactions, raw final answer (including `NO_MESSAGE`),
  tool calls with results, changes to stored state and usage.
- `results.json`: full scenario fixtures, all model requests with tool schemas, responses, initial/final state,
  role settings, logs, usage, build revision and whether the build was modified. Reasoning content is omitted.
- `<case>-<repeat>.sqlite`: the isolated database for further inspection; API keys are never stored in it.

`-out` must name a new directory. `-only` accepts comma-separated ids or glob patterns. `-repeat` repeats each
case independently. `-timeout` bounds one scenario (default `3m`). Execution errors are reported and the suite
continues; they produce a nonzero exit code. Behavioural mistakes are left for the reviewer.

Keep the same case file, role settings and embedding mode when comparing prompt revisions. Review both the text
and state changes: a fluent answer may still create a duplicate event or save an unsupported fact.

## Case format

The case file is a JSON array. See `example.json` for the smallest case. Supported modes are `chat` (default),
`checkin`, `reminder` and `reflection`. `at` and explicit timestamps use local `YYYY-MM-DD HH:MM:SS` or RFC3339.
History is chronological; omitted history times are one minute apart immediately before `at`. Omitted ids are
assigned in array order, from 1, separately for each kind of record. Explicit ids must be positive.

```json
[
  {
    "id": "example-done",
    "title": "Complete an existing task",
    "group": "working",
    "mode": "chat",
    "at": "2026-11-04 10:00:00",
    "user": {"name": "Alex", "profile": "Respond in English.", "timezone": "Europe/London"},
    "history": [{"role": "assistant", "text": "The parcel is on your list."}],
    "message": "I collected the parcel.",
    "todos": [{"id": 301, "text": "Collect parcel", "day": "2026-11-04"}],
    "review": ["Mark task 301 done, without adding a duplicate."]
  }
]
```

Optional seed fields:

- `memories`: `id`, `name`, `category`, `content`.
- `events`: `id`, `title`, `start`, optional `end`, `location`, `repeat` (weekday array), `reminded` (timestamp).
  A reminder scenario needs an unreminded event due in the preceding 30 minutes, including `at` itself.
- `todos`: `id`, `text`, optional `day`, `done` (boolean).
- `notes`: `id`, `collection`, `title`, `content`, optional `tags`, `source`.
- `recent_tools`: `reply_id` referencing an assistant history entry, `calls` with `tool`, `args` (JSON string),
  `result`. These exercise the production recent-tool hint and `recent_tool_results`.
- `mocks`: `tool`, optional `match` (exact argument values as strings), `result`. The first matching fixture is
  reused on every matching call. Omit `match` for a fallback result for that tool; put specific matches first.
- `source`: a private annotation for the reviewer. It is not sent to the model.

Photos/audio transport, the embedding model itself, live web reliability and scheduler behaviour require their
own tests. The bench compares the responses and actions of the assistant on controlled conversation states.
