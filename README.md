# logstruct

> Русская версия — [README.ru.md](README.ru.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/lux/logstruct.svg)](https://pkg.go.dev/github.com/lux/logstruct)
[![Go Version](https://img.shields.io/github/go-mod/go-version/lux/logstruct)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**AI-powered log analysis for Go services. Embed as a library, view in a local UI.**

logstruct is a Go library that groups your service's logs into templates, scores them by anomaly signals, and uses an LLM to produce human-readable summaries and suggested actions.

## Why

Most Go services log to files or stdout. Reading 50 000 lines at 3 AM is not a debugging strategy. logstruct does three things:

- **Groups** similar log lines into templates (Drain algorithm, ~2.4M events/sec).
- **Prioritizes** clusters by anomaly signals: frequency spikes, novel patterns, burst rate, cross-service spread.
- **Explains** the highest-priority clusters via your chosen LLM (LM Studio / Ollama locally, or OpenRouter in the cloud).

It is a library, not a service. You add it to your existing Go program; no separate process to deploy.

## Install

```bash
go get github.com/lux/logstruct
```

For the optional read-only UI:

```bash
go install github.com/lux/logstruct/cmd/logstruct@latest
```

## Quickstart — file mode

Tail one or more log files and browse clusters in the local dashboard:

```go
ll, err := logstruct.New(logstruct.Config{
    Sources: []logstruct.SourceConfig{
        {Kind: "file", Path: "/var/log/myapp.log"},
    },
    AI: logstruct.AIConfig{
        Provider: "logstruct-ai", // local LM Studio / Ollama
    },
})
if err != nil {
    log.Fatal(err)
}
defer ll.Close()

if err := ll.Start(context.Background()); err != nil {
    log.Fatal(err)
}

// Open the dashboard:
//   logstruct ui --db ./logstruct.db
select {} // run until interrupted
```

Or via YAML config file:

```yaml
# logstruct.yaml
sources:
  - kind: file
    path: /var/log/myapp.log
    format: auto        # auto | json | text
    start_from: end     # end | beginning

ai:
  provider: logstruct-ai

storage:
  kind: sqlite
  sqlite_path: ./logstruct.db
```

```go
ll, err := logstruct.NewFromYAML("logstruct.yaml")
```

## Quickstart — inline mode

Report errors directly from your error-handling paths:

```go
ll, err := logstruct.New(logstruct.Config{
    AI: logstruct.AIConfig{
        Provider: "openrouter",
        APIKey:   os.Getenv("OPENROUTER_API_KEY"),
        Model:    "anthropic/claude-3.5-haiku",
    },
    Inline: logstruct.InlineConfig{
        Enabled:     true,
        MinPriority: 40, // trigger AI analysis when cluster priority >= 40
    },
})
if err != nil {
    log.Fatal(err)
}
defer ll.Close()
ll.Start(ctx)

// In your error paths:
if err := db.Query(q); err != nil {
    ll.Report(ctx, err, logstruct.Fields{"query": q, "user_id": userID})
    return err
}
```

`Report` is non-blocking. If the pipeline channel is full, the event is dropped and counted in `Stats().Dropped`.

## Synchronous one-shot analysis

For a single error where you want an immediate answer without waiting for clustering:

```go
rec, err := ll.AnalyzeNow(ctx, err.Error(), logstruct.Fields{"order_id": id})
if err == nil {
    log.Printf("severity=%s summary=%s", rec.Severity, rec.Summary)
    for _, action := range rec.SuggestedActions {
        log.Printf("  - %s", action)
    }
}
```

## View clusters in the UI

```bash
logstruct ui --db ./logstruct.db       # SQLite
logstruct ui --postgres DSN          # Postgres
```

Opens at `http://localhost:8765`. The UI is read-only — it never ingests events.

## Storage

By default logstruct writes to a local SQLite file (`./logstruct.db`). No setup required.

For multi-instance deployments or shared dashboards, use Postgres:

```yaml
storage:
  kind: postgres
  postgres_dsn: postgres://user:pass@host:5432/logstruct?sslmode=disable
```

The database must exist; logstruct creates its tables on first run via embedded migrations:

```bash
logstruct migrate --postgres DSN
```

## AI providers

| Provider | Default base URL | Notes |
|----------|-----------------|-------|
| `logstruct-ai` | `http://localhost:1234/v1` | Local LLM via LM Studio, Ollama, or any OpenAI-compatible server. Free, private. Temperature forced to 0 for stable structured output. |
| `openrouter` | `https://openrouter.ai/api/v1` | Hosted models. Requires `api_key` and `model`. |
| `fake` | — | In-memory stub for tests. |

## Configuration reference

See [docs/CONFIG.md](docs/CONFIG.md) for the full field reference with defaults and examples.

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for pipeline internals, storage schema, and LLM integration.

## Status

logstruct is pre-1.0. The public API (`logstruct.New`, `Config`, `Report`, `AnalyzeNow`, `RecentRecommendations`) is stable enough for experimentation. Minor changes are expected before 1.0.

## Documentation

- [Configuration reference](docs/CONFIG.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
- [Russian translation](README.ru.md)

## License

Apache 2.0.
