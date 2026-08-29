# VMware Avi Load Balancer LLM Agent

A Go web agent that lets you manage a VMware Avi (NSX ALB) Load Balancer in plain English. Gin serves both a JSON API and an HTMX-based chat UI; an LLM (Mistral AI or Ollama) turns your questions into calls against the Avi Controller REST API, covering 170+ object types through a generic MCP tool set. Every write is blocked by default until you explicitly unlock it.

## Features

- **Natural language interface** — ask about virtual services, pools, health monitors, service engines and analytics in plain English
- **Comprehensive API coverage** — 170+ Avi object types via a generic MCP tool set (list/get/create/update/patch/delete/action), with a static fallback tool set for when MCP is unavailable
- **Mistral AI or Ollama** — Mistral is the recommended default (more reliable tool-calling); Ollama works for local/offline use
- **Three-column UI** — session history, conversation, and a live trace inspector showing every LLM and Avi API call as it happens, correlated per conversation turn
- **Read-only by default** — every new conversation starts read-only; any create/update/delete/scale attempt is blocked and shown as a confirmation card (what it would have sent) instead of being applied. Unlock a whole session from the composer, or approve one blocked action at a time — the server re-checks the mode itself rather than trusting the browser
- **Persistent chat history** — conversations save to disk and are listed in the session rail, kept for 30 days
- **Structured result tables** — virtual service, pool, health monitor and service engine results render as scannable tables, with the raw JSON always one click away

## Quick Start

Requires Docker and Docker Compose, and access to an Avi Controller.

```bash
git clone https://github.com/aca2328/aviagent.git
cd aviagent

# Interactive setup — prompts for your Mistral API key and Avi credentials
./start-mistral.sh

# Or for local/offline use with Ollama instead
./start-ollama.sh
```

Both scripts write a `.env` file and start the app with `docker-compose`. Open `http://localhost:8088` once it's up.

**Manual setup**, if you'd rather not use the scripts:

```bash
cp .env.example .env
# edit .env: set AVI_HOST/AVI_USERNAME/AVI_PASSWORD, and either
# MISTRAL_API_KEY (LLM_PROVIDER=python) or leave it for Ollama

docker-compose --env-file .env up -d --scale ollama=0   # Mistral, no Ollama container
docker-compose --env-file .env up -d                    # Ollama, includes the Ollama service

curl http://localhost:8088/api/health
```

> **Docker is the only supported way to run this app** — there's no supported `go run`/binary path. See `Development` below for building and testing without running it.

## Usage

1. **Open** `http://localhost:8088`.
2. **Pick a model** from the top bar.
3. **Ask a question**, or use a starter prompt on the empty-state screen.
4. **Read-only / Read-write**: the composer's toggle controls whether the assistant may write to the controller (default: read-only).
5. **Trace inspector** (right panel): every LLM and Avi API call streams in live. Click a message's `N tools · Dms · M objects` chip to isolate that turn's steps; click a step to jump back to its message.
6. **Session history** (left rail): past conversations are listed newest-first, titled from their first message, and kept for 30 days. Click one to reload it, or start a **New session**.

### Example queries

```
"List all virtual services"
"Which pools have no health monitor?"
"Show SE groups and their capacity"
"Get analytics for the last hour"
"Scale out the backend pool for app1 to 5 servers"    # blocked unless the session is read-write
```

## Configuration

Settings load from `config.yaml`, then environment variables (which win on conflict). See `.env.example` for the full list; the essentials:

```yaml
avi:
  host: "avi-controller.example.com"
  username: "admin"
  password: "your-secure-password"
  tenant: "admin"
  auth_method: "session"  # "session" (recommended) or "basic"

provider: "python"  # "python" (Mistral, recommended) or "ollama"

mistral:
  api_key: "your-mistral-api-key"
  default_model: "mistral-medium"

llm:  # used when provider: "ollama"
  ollama_host: "http://localhost:11434"
  default_model: "llama3.2"

sessions:
  dir: "data/sessions"  # one JSONL file per session; kept for 30 days
```

| Env var | Overrides |
|---|---|
| `AVI_HOST`, `AVI_USERNAME`, `AVI_PASSWORD`, `AVI_TENANT`, `AVI_AUTH_METHOD` | `avi.*` |
| `LLM_PROVIDER` | `provider` (`ollama` or `python`) |
| `MISTRAL_API_KEY`, `MISTRAL_DEFAULT_MODEL` | `mistral.*` |
| `OLLAMA_HOST`, `OLLAMA_DEFAULT_MODEL` | `llm.*` |
| `SESSIONS_DIR` | `sessions.dir` |
| `SERVER_PORT`, `LOG_LEVEL` | `server.port`, `log.level` |

Note the code only accepts the literal provider values `ollama` or `python` (Mistral runs through the `python` provider, a bridge to the Mistral SDK) — if you have an older `.env` with `LLM_PROVIDER=mistral`, config validation will fail at startup until you change it to `python`.

## API Reference

```bash
# Chat (no session — always read-only, see note below)
curl -X POST http://localhost:8088/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "List all virtual services", "model": "mistral-medium"}'

# Sessions
curl http://localhost:8088/api/chat/history                    # list all sessions
curl -X DELETE http://localhost:8088/api/chat/history           # delete all sessions
curl -X DELETE http://localhost:8088/api/sessions/<id>          # delete one session
curl -X POST http://localhost:8088/api/sessions/<id>/mode \
  -H "Content-Type: application/json" -d '{"mode":"read-write"}' # unlock a session

# Models & health
curl http://localhost:8088/api/models
curl http://localhost:8088/api/health

# Direct Avi API proxy (uses this app's configured Avi credentials)
curl "http://localhost:8088/api/avi/virtualservice?limit_by=10"
```

> `/api/chat` doesn't create or use a session, so it's permanently read-only with no unlock path — a write request just comes back as a blocked-write result. Only the web UI tracks a session's read-only/read-write mode.

## Docker Administration

```bash
docker-compose logs -f avi-llm-agent          # follow logs
docker-compose restart                        # restart
docker-compose --env-file .env up -d --build  # rebuild after a code change (config.yaml alone is volume-mounted; nothing else is)
docker-compose down                           # stop (add -v to also drop the sessions volume)
```

Chat sessions live in a named Docker volume (`aviagent-sessions`) so they survive `--build`. To switch providers, set `LLM_PROVIDER` in `.env` (`python` for Mistral, `ollama` for Ollama) and restart:

```bash
docker-compose --env-file .env up -d --build
docker-compose exec ollama ollama pull llama3.2   # only needed after switching to Ollama
```

## Troubleshooting

**Avi controller connection fails**
```bash
curl -k https://<avi-host>/login
curl -u "$AVI_USERNAME:$AVI_PASSWORD" -k https://$AVI_HOST/login
```

**Check which provider is active / debug logs**
```bash
curl -s http://localhost:8088/api/health | jq .
docker-compose exec avi-llm-agent env | grep -E 'LLM_PROVIDER|MISTRAL_API_KEY'
```
Set `level: "debug"` under `log:` in `config.yaml`, then `docker-compose --env-file .env up -d --build` to pick it up.

**Ollama model missing**
```bash
docker-compose exec ollama ollama list
docker-compose exec ollama ollama pull llama3.2
```

**MCP tool calling not working** (falls back to a smaller static tool set) — check `mcp-avi-server/build/index.js` exists; it's a separate npm build, not wired into `make build`:
```bash
cd mcp-avi-server && npm install && npm run build
```

## Development

See `CLAUDE.md` for architecture details. Quick reference:

```bash
go build -o build/bin/aviagent .   # compile check only — the binary isn't meant to be run directly, see Quick Start
go test ./...
go vet ./...
make fmt   # gofmt + goimports
```

Project layout:
```
internal/
  avi/       # Avi Controller REST client
  config/    # Viper-based config loading
  llm/       # LLM client interface + static fallback tool definitions
  mcpavi/    # MCP client that spawns and talks to mcp-avi-server
  python/    # Python/Mistral SDK bridge (subprocess)
  web/       # Gin server, handlers, session store, chat rendering
mcp-avi-server/  # separate TypeScript MCP server (generic Avi CRUD tools)
web/
  templates/ # Gin HTML templates (HTMX), what's actually served
  static/    # CSS/JS for the templates above
  src/       # a separate, unused React app — not built into Docker
```

## License

No license file is currently included in this repository; all rights are reserved by default until one is added.
