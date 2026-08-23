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

`docker-compose up -d` alone reuses the existing image — only `config.yaml` is volume-mounted, so Go/template/static edits need a rebuild: `docker-compose --env-file .env up -d --build`.

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

**Diagram download:** every successful tool call result gets appended to the assistant message as an `API Result (<tool_name>):` heading followed by a fenced `json` code block (`web-server.go` around the tool-call execution loop). `renderChatMessage` (same file) special-cases that fenced block: it wraps it in `<div class="api-result-block" data-tool="...">` and appends a `.diagram-download-btn` button. `web/static/js/app.js`'s `initializeDiagramDownload()` delegates clicks on that button, reads the adjacent `<pre><code>` JSON, fetches `web/static/diagram/template.html` (a generalized copy of `prototypes/node-link-diagram/index.html` — same node-link visualization, but "virtual service" wording swapped for neutral "top-level item" since this now renders arbitrary Avi API results, not just VS lists), swaps its embedded `DATA` between the `/*__AVI_DIAGRAM_DATA_START__*/`/`/*__AVI_DIAGRAM_DATA_END__*/` sentinel comments, and downloads the result as a standalone HTML file via a `Blob` + `<a download>`. No backend endpoint involved — generation is entirely client-side. If you touch `renderChatMessage`'s fence-parsing state machine, keep `pendingToolName` line up with the `API Result (` heading format, or the button/wrapper stops appearing.

**Testing Blob downloads in-browser:** chat Export and the diagram download button both use `Blob` + `<a download>`; Claude-in-Chrome can't inspect a real OS save dialog. To verify, monkeypatch `URL.createObjectURL` and `HTMLAnchorElement.prototype.click` before triggering the action, to capture the Blob/filename instead.

**Frontend:** Two UI implementations coexist:
- `web/templates/*.html` + `web/static/{css,js}` — Gin-rendered HTML with HTMX (`/htmx/*` routes), this is what's actually served in production/Docker.
- `web/src/` — a separate React/TypeScript app (`web/package.json`, react-scripts) that does not appear to be built into the Docker image or referenced by the Go server. Confirm which UI a task targets before editing.

Template/static paths are resolved relative to cwd at startup (`internal/web/web-server.go`, `setupRouter`): it checks for `web/templates` vs `templates`, so the binary works both from the repo root (local dev) and from `/web` (Docker `WORKDIR`).

**Observability:** `internal/langfuse` sends LLM interaction traces to a Langfuse instance if `langfuse.enabled` is set; optional, wired in `NewServer`.

**Config:** `internal/config/config.go` uses Viper — defaults set in code, overridable by `config.yaml` and then environment variables (prefix-less binds like `AVI_HOST`, `MISTRAL_API_KEY`, `LLM_PROVIDER`, etc. — see `Load()` for the full env var list). `avi.host`/`username`/`password` are always required regardless of provider.

`.env` is gitignored and, when present, typically holds real dev credentials plus a `SERVER_PORT` that overrides `config.yaml`'s placeholders (`docker-compose.yml` itself defaults `SERVER_PORT` to 8088, not `config.yaml`'s 8083) — check `.env` before assuming `config.yaml` reflects what's actually running. If testing a scratch local binary alongside the docker dev instance, override the port: `SERVER_PORT=<free-port> ./build/bin/aviagent -config config.yaml`.

## Git

Solo project (single maintainer, no team review process). Direct commits and pushes to `main` are authorized — this overrides the global "never push directly to main" rule.

## Versioning

`AppVersion` in `main.go` is shown in the UI header badge and in `/api/health`, and is auto-bumped (patch version) on every commit by a `pre-commit` hook installed at `.git/hooks/pre-commit`. The hook stages its own edit to `main.go`, so a fresh commit always ships a new version without manual bumping. **This hook is not tracked by git** (`.git/hooks` never is) — a fresh clone needs it recreated (or copied from another clone) to get the same behavior.

## Repo layout notes

- `archive/` — old test scripts, logs, and analysis docs deliberately moved out of the way; not part of the active codebase (see `archive/README.md`).
- Root-level `test_*.py`, `test_new_ui.html`, `compare_payloads.sh`, `script.py` — ad hoc manual test/debug scripts, not part of `go test`/CI.
- `internal/tests/virtual_services_e2e_test.go` — E2E-style test using `httptest`, in its own `tests` package.
- `monitoring/` — Prometheus/Grafana config for the optional `docker-compose --profile monitoring` stack.
- `api_answer_model.json` (repo root) — a sample Avi `virtualservice` API response, used as fixture data by the node-link-diagram prototype below. Not loaded by the Go server.
- `prototypes/node-link-diagram/index.html` — standalone, self-contained (no build step, no dependencies, `api_answer_model.json` embedded inline) canvas visualization prototype exploring how to present Avi API responses. Renders the JSON as an interactive node-link graph on a radial layout that eases into place. The response envelope is deliberately **not** drawn: `API Response` (which only holds `count` and `results`) and the `results` array itself carry no information worth a ball, so the eight virtual services *are* the top level. Both nodes still exist in the tree — paths, refs and `raw` subtrees resolve through them — but `HIDDEN`/`TOP_NODES` route layout, links, the search index, breadcrumbs and arrow-key navigation around them; the `count` leaf is dropped from search too, since it has no ball to reveal and restates the header's VS count. Leaf/scalar values are never drawn as balls — they live in the right-hand detail panel, which is the load-bearing consequence for anything that navigates: a search hit on a *value* has no ball to fly to, so it resolves to the leaf's **parent** ball and flashes the matching field row. Key interactions: single click selects + expands (it never collapses — that is double-click, or the `−` button in the panel header, so re-reading an open ball can't destroy the subtree you just opened); `/` or ⌘K opens a search over every key, value and path, with live yellow rings on matching balls; arrow keys walk parent/child/siblings; the toolbar has **Expand VSs** (one level under each virtual service, ~123 balls) and **Expand all** (every depth, 269 balls — only containers become balls, so "everything" is a few hundred, not the 792 values); `F` fits, `X` toggles cross-refs, `D` toggles focus mode (dims everything outside the selection's ancestors, children and refs). The panel's breadcrumb, nested-ball chips, and `*_ref` values are all clickable jump targets. A toggleable cross-reference overlay links balls whose fields share a UUID — resolved refs (the UUID matches another listed object's own `uuid` field, e.g. `vh_parent_vs_ref`) draw straight to that ball, unresolved refs (`pool_ref`, `se_group_ref`, etc., pointing at objects not present in this JSON) get a dedicated synthetic "external object" ball instead, ringed in a collar around its owner and de-overlapped by a relaxation pass. **Gotcha for any new navigation code:** `computeRadialLayout()` only assigns `targetX`/`targetY` to descendants of *expanded* nodes, and `focusOn()` reads those — so always expand ancestors → `rebuildVisible()` → *then* move the camera, or you pan to stale/NaN coords with no error thrown (`revealNode()` already does this in the right order; reuse it). A second one: the link render runs a two-pass dim/bright loop keyed on `focusSet`, and pass 0 is the *dim* pass — with focus mode off nothing is dimmed, so that pass must be skipped entirely (`focusSet ? [0, 1] : [1]`) or no structure links draw at all. **Palette:** built in OKLCH and validated with the `dataviz` skill's six checks (`scripts/validate_palette.js`), not hand-picked — neutrals are one hue family (264°) stepped evenly in lightness, and the three ball hues (blue `--n-vs` / rose `--n-object` / amber `--n-array`) sit at near-equal lightness. That triad is a measured result: green and teal collapse against the rose under protanopia/deuteranopia (ΔE 3.0–5.8), so warm-amber is the only viable third hue. Balls are validated **all-pairs** (they're scattered marks, any two can touch): worst CVD ΔE 11.5, normal-vision 17.2. The panel's value-type dots are a *secondary* channel validated adjacent-only — each sits beside its key and literal value, so identity is never colour-alone. Re-run both after any colour change. Not wired into the Go server or `web/` build — open `index.html` directly, or serve it with any static file server (`python3 -m http.server`) since some tooling blocks `file://` URLs.
