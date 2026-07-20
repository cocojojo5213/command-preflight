# command-preflight

Privacy-first command checks for coding-agent CLIs.

`command-preflight` reduces wasted retries caused by shell syntax, path, executable, and cross-shell mistakes. It is designed for Codex CLI, Claude Code, and other MCP-capable terminal agents.

The default client is local-only:

- no account or API key
- no telemetry or network calls
- no command execution from the preflight tool
- redaction and error fingerprints are generated on the host

## Install

The latest binaries are published on the [GitHub Releases page](https://github.com/cocojojo5213/command-preflight/releases).

Windows users should download the matching ZIP, extract every file, and double-click `INSTALL.cmd`. It installs the binary under `%LOCALAPPDATA%\CommandPreflight`, installs the bundled Skills, and registers MCP for any installed Codex or Claude Code CLI. It asks before enabling the optional read-only community lookup. Do not double-click `command-preflight.exe` as an installer; it is a command-line program.

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/cocojojo5213/command-preflight/main/scripts/install.sh | sh
```

PowerShell:

```powershell
irm https://raw.githubusercontent.com/cocojojo5213/command-preflight/main/scripts/install.ps1 | iex
```

To explicitly opt in to the project-maintained read-only knowledge lookup during setup:

```powershell
$env:COMMAND_PREFLIGHT_KNOWLEDGE_URL='https://preflight.52131415.xyz'; irm https://raw.githubusercontent.com/cocojojo5213/command-preflight/main/scripts/install.ps1 | iex
```

The installer keeps lookup offline when that variable is absent. To build from source:

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

The installer runs the equivalent setup automatically. To configure it later, use:

Codex:

```bash
codex mcp add command-preflight -- command-preflight mcp
```

Claude Code:

```bash
claude mcp add --scope user command-preflight -- command-preflight mcp
```

See [integrations](integrations/) for generic MCP configuration and Skill installation.

To configure the optional public lookup for both clients in one step:

```bash
command-preflight setup --client both --knowledge-url https://preflight.52131415.xyz --apply
```

If you are asking an AI agent to install this project, give it the repository URL and ask it to detect the host OS, use the release installer, run `doctor`, and verify the MCP registration. Keep cloud lookup disabled unless you explicitly want the read-only endpoint.

The Codex Skill is installed under `$CODEX_HOME/skills` or `~/.codex/skills` by default. Set `COMMAND_PREFLIGHT_CODEX_SKILL_DIR` for a non-standard layout.

## Optional cloud knowledge service

The same repository also builds `command-preflight-server`. It is a small read-only-by-default HTTP service for curated, privacy-preserving error knowledge. It is not required by the local client.

The project currently maintains a public TLS endpoint for read-only lookups:

```text
https://preflight.52131415.xyz
```

It accepts GET lookups by public fingerprint ID and returns `403` for writes. Cloudflare handles normal request metadata such as an IP address; the application stores only curated public fingerprints and fixes. Forks and private deployments can use their own endpoint.

Start it with Docker Compose:

```bash
git clone https://github.com/cocojojo5213/command-preflight.git
cd command-preflight
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/healthz
```

The Compose setup binds to localhost, stores data in a Docker named volume, and disables writes to the knowledge API. Put a TLS-terminating reverse proxy or Cloudflare Tunnel in front of it before exposing it to a network. To deliberately change the container listen address, copy `.env.example` to `.env` and set `COMMAND_PREFLIGHT_BIND`; to publish only on a private Tailnet interface, set `COMMAND_PREFLIGHT_PUBLISHED_BIND` to that IP and port. Do not expose an unauthenticated instance directly to the Internet. A host bind mount can be selected with `COMMAND_PREFLIGHT_DATA_VOLUME=./data` (the directory must be writable by container UID `65532`).

Lookups contain only a public fingerprint ID:

```bash
curl -fsS http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

Enable a local lookup explicitly by setting the URL (the default remains offline):

```bash
export COMMAND_PREFLIGHT_KNOWLEDGE_URL=http://127.0.0.1:8787
command-preflight lookup --fingerprint-id cp1-0123456789abcdef0123 --json
```

For the public endpoint:

```bash
export COMMAND_PREFLIGHT_KNOWLEDGE_URL=https://preflight.52131415.xyz
command-preflight lookup --fingerprint-id cp1-0123456789abcdef0123 --json
```

The lookup sends only the `cp1-...` ID. It never sends commands, environment variables, paths, or terminal output. The authenticated `PUT` endpoint is for operator-curated seed data, not anonymous uploads; automatic reporting remains disabled on the public deployment.

## Development

```bash
go test ./...
go run ./cmd/command-preflight doctor
go run ./cmd/command-preflight mcp
```

See [the cloud deployment guide](docs/cloud.md), [the architecture](docs/architecture.md), and [the privacy model](docs/privacy.md) before enabling an opt-in lookup or reporting adapter.

## Status

This is an early open-source MVP. Shell parsing and CLI grammar are inherently platform- and version-specific; a successful preflight is not permission to skip normal approvals or postcondition checks.
