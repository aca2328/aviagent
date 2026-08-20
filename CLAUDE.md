# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go web server that lets users query/manage a VMware Avi (NSX ALB) Load Balancer via natural language, backed by an LLM (Mistral AI or Ollama). Gin serves both a JSON API and an HTMX-based server-rendered chat UI.

## Commands

```bash
# Build & run
go build -o build/bin/aviagent .          # or: make build
go run . -config config.yaml              # or: make run-dev
make run                                  # build then run

# Tests
go test ./...                             # all tests
go test ./internal/avi/...                # single package
go test -run TestName ./internal/avi/...  # single test
make test                                 # verbose, race detector, coverage -> build/coverage/coverage.out
make test-integration                     # tag=integration, ./tests/integration/... (no such dir currently exists)

# Lint / vet / format
go vet ./...
make fmt                                  # gofmt + goimports
make lint                                 # golangci-lint (not installed by default; make setup-dev installs it)

# Docker
docker-compose --env-file .env up -d --scale ollama=0   # Mistral/python provider, no Ollama container
docker-compose --env-file .env up -d                    # Ollama provider, includes Ollama service
make docker-build / make docker-compose-up
```

There is no `cmd/server` directory — `main.go` lives at the repo root, so `go build .` (not `go build ./cmd/server`) is correct. Note `make build-all` in the Makefile still references `./cmd/server` and is currently broken; don't trust it without checking.

## Architecture

**Entry point:** `main.go` loads config, builds a `web.Server` (`internal/web/web-server.go`), and runs it behind `net/http` with graceful shutdown.

**Two LLM providers, selected by `config.Provider` (`"ollama"` or `"python"`)** — set in `config.yaml`/`LLM_PROVIDER` env var:
- `"ollama"` → `internal/llm` talks directly to a local Ollama HTTP endpoint.
- `"python"` → `internal/python` (`PythonBridge`) shells out to a Python subprocess (`python3 -m python_mistral.bridge <command> <json>`) which wraps the official Mistral Python SDK (`python_mistral/client.py`). This is the recommended/default provider (more reliable tool-calling than raw Ollama JSON parsing). Go and Python communicate via JSON over argv/stdout, not a long-lived socket — each call spawns a fresh interpreter.
- **Gotcha:** `.env.example` and the README call the second provider `"mistral"`, but `internal/config/config.go` (`validateConfig`) only accepts the literal strings `"ollama"` or `"python"`. Setting `LLM_PROVIDER=mistral` will fail config validation.

Provider selection is threaded through `internal/web/web-server.go` at nearly every handler (`s.config.Provider == "ollama"` branches for history/model-list format, since Ollama and the Python/Mistral bridge use different chat-history and model-list shapes).

**Avi client** (`internal/avi/avi-client.go`): hand-rolled HTTP client against the Avi Controller REST API — session or basic auth (`avi.auth_method`), response caching with TTL, retry logic for transient errors. `internal/avi/avi-client-official.go` (`OfficialClient`, wraps `github.com/vmware/alb-sdk`) is an alternate implementation that exists but is **not wired into `web-server.go`** — the hand-rolled `Client` is what's actually used in `getAviClient()`.

**Tool calling loop:** `internal/llm/tools.go` defines the Avi operations as LLM tool/function schemas (`GetAviToolDefinitions`). `web-server.go`'s `processChatMessage` → `executeToolCall` dispatches a model's tool call to the matching `avi.Client` method. `avi_tools_definition.json` at the repo root is a reference/export of these same tool definitions, not something the Go code loads at runtime — if you change `internal/llm/tools.go`, update this file too if it needs to stay in sync (check before assuming it's live).

**Frontend:** Two UI implementations coexist:
- `web/templates/*.html` + `web/static/{css,js}` — Gin-rendered HTML with HTMX (`/htmx/*` routes), this is what's actually served in production/Docker.
- `web/src/` — a separate React/TypeScript app (`web/package.json`, react-scripts) that does not appear to be built into the Docker image or referenced by the Go server. Confirm which UI a task targets before editing.

Template/static paths are resolved relative to cwd at startup (`internal/web/web-server.go`, `setupRouter`): it checks for `web/templates` vs `templates`, so the binary works both from the repo root (local dev) and from `/web` (Docker `WORKDIR`).

**Observability:** `internal/langfuse` sends LLM interaction traces to a Langfuse instance if `langfuse.enabled` is set; optional, wired in `NewServer`.

**Config:** `internal/config/config.go` uses Viper — defaults set in code, overridable by `config.yaml` and then environment variables (prefix-less binds like `AVI_HOST`, `MISTRAL_API_KEY`, `LLM_PROVIDER`, etc. — see `Load()` for the full env var list). `avi.host`/`username`/`password` are always required regardless of provider.

## Git

Solo project (single maintainer, no team review process). Direct commits and pushes to `main` are authorized — this overrides the global "never push directly to main" rule.

## Repo layout notes

- `archive/` — old test scripts, logs, and analysis docs deliberately moved out of the way; not part of the active codebase (see `archive/README.md`).
- Root-level `test_*.py`, `test_new_ui.html`, `compare_payloads.sh`, `script.py` — ad hoc manual test/debug scripts, not part of `go test`/CI.
- `internal/tests/virtual_services_e2e_test.go` — E2E-style test using `httptest`, in its own `tests` package.
- `monitoring/` — Prometheus/Grafana config for the optional `docker-compose --profile monitoring` stack.
