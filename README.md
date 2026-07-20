# command-preflight

Privacy-first command checks for coding-agent CLIs.

`command-preflight` reduces wasted retries caused by shell syntax, path, executable, and cross-shell mistakes. It is designed for Codex CLI, Claude Code, and other MCP-capable terminal agents.

The default client is local-only:

- no account or API key
- no telemetry or network calls
- no command execution from the preflight tool
- redaction and error fingerprints are generated on the host

## Install

Release installers will be published for Windows, macOS, and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/cocojojo5213/command-preflight/main/scripts/install.sh | sh
```

PowerShell:

```powershell
irm https://raw.githubusercontent.com/cocojojo5213/command-preflight/main/scripts/install.ps1 | iex
```

Until the first release is published, build from source:

```bash
go install github.com/cocojojo5213/command-preflight/cmd/command-preflight@main
```

## Use directly

```bash
command-preflight preflight --shell bash --command "printf '%s\\n' ok" --json
command-preflight fingerprint --shell powershell --command "git checkout" --stderr "unknown option" --exit-code 129
command-preflight doctor
```

Exit codes are `0` for passed, `1` for failed, and `2` for review.

## Connect a CLI

Codex:

```bash
codex mcp add command-preflight -- command-preflight mcp
```

Claude Code:

```bash
claude mcp add --scope user command-preflight -- command-preflight mcp
```

See [integrations](integrations/) for generic MCP configuration and Skill installation.

## Optional cloud knowledge service

The same repository also builds `command-preflight-server`. It is a small self-hosted HTTP service for curated, privacy-preserving error knowledge. It is not required by the local client, and this project does not operate a hosted endpoint in the MVP.

Start it with Docker Compose:

```bash
git clone https://github.com/cocojojo5213/command-preflight.git
cd command-preflight
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/healthz
```

The Compose setup binds to localhost, stores data in a Docker named volume, and disables writes to the knowledge API. Put a TLS-terminating reverse proxy in front of it before exposing it to a network. To deliberately change the bind address, copy `.env.example` to `.env` and set `COMMAND_PREFLIGHT_BIND`; do not expose an unauthenticated instance directly to the Internet. A host bind mount can be selected with `COMMAND_PREFLIGHT_DATA_VOLUME=./data` (the directory must be writable by container UID `65532`).

Lookups contain only a public fingerprint ID:

```bash
curl -fsS http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

The authenticated `PUT` endpoint is for operator-curated seed data, not anonymous uploads. The current client does not send commands, environment variables, or terminal output to this service. Automatic opt-in lookup/report adapters are intentionally a follow-up feature and must preserve this contract.

## Development

```bash
go test ./...
go run ./cmd/command-preflight doctor
go run ./cmd/command-preflight mcp
```

See [the cloud deployment guide](docs/cloud.md), [the architecture](docs/architecture.md), and [the privacy model](docs/privacy.md) before enabling an opt-in lookup or reporting adapter.

## Status

This is an early open-source MVP. Shell parsing and CLI grammar are inherently platform- and version-specific; a successful preflight is not permission to skip normal approvals or postcondition checks.
