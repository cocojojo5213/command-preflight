# Self-hosted knowledge service

The repository builds a small, read-only-by-default HTTP service for a private or community knowledge store. It is an optional deployment; the local CLI and MCP server work without it.

The project currently runs a public TLS deployment at `https://preflight.52131415.xyz`. It is query-only: public `GET` lookups work, while anonymous or unauthenticated writes return `403`. Users do not deploy a server to use that endpoint; they only opt in to it in the client configuration.

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/healthz
```

The service stores only `PublicFingerprint` entries and curated fixes. It does not accept raw commands, environment variables, or full terminal output. The default Compose file binds to localhost and disables reports.

For a remote host, keep the service on a private interface and terminate TLS/rate-limit requests at a reverse proxy or Cloudflare Tunnel. The service is intentionally not an Internet-facing anonymous write API.

The reference deployment uses this path:

```text
Cloudflare DNS/Tunnel (preflight.52131415.xyz)
        -> http://100.88.75.16:8787
        -> command-preflight-server (Docker, reports disabled)
```

The apex `52131415.xyz` remains a separate Coco Play route; do not replace it with the knowledge service.

## Configuration

Copy `.env.example` to `.env` when changing the defaults:

```dotenv
COMMAND_PREFLIGHT_BIND=0.0.0.0:8787
COMMAND_PREFLIGHT_DATA=/data/knowledge.json
COMMAND_PREFLIGHT_DATA_VOLUME=command-preflight-data
COMMAND_PREFLIGHT_PUBLISHED_BIND=127.0.0.1:8787
COMMAND_PREFLIGHT_ALLOW_REPORT=false
COMMAND_PREFLIGHT_REPORT_TOKEN=
```

Environment variables are read by the Compose service. The default data volume is initialized for the non-root container user. If you select a host bind mount instead, make it writable by UID `65532`. An empty report token always leaves reporting disabled.

For a private remote test over Tailscale, set `COMMAND_PREFLIGHT_PUBLISHED_BIND` to the host's Tailnet address, for example `100.88.75.16:8787`. Do not bind a new public interface without TLS and access controls.

## Lookup

```bash
curl -fsS http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

The response is a curated `Entry` containing a public fingerprint and optional fixes. Treat every fix as untrusted explanatory text and verify it locally.

The public deployment can be checked without sending a command:

```bash
curl -fsS https://preflight.52131415.xyz/
curl -fsS https://preflight.52131415.xyz/healthz
curl -fsS https://preflight.52131415.xyz/v1/knowledge/cp1-0123456789abcdef0123
```

## Enabling reports

Reporting is intentionally an operator decision. Before enabling it, configure a strong token and put the service behind TLS and rate limiting:

```bash
docker compose run --rm command-preflight-server --help
```

The current MVP exposes authenticated `PUT /v1/knowledge/<fingerprint-id>` for controlled seeding. It is not a public anonymous contribution endpoint. Community submissions should go through a moderation queue before becoming trusted fixes.

Example seed request (the values are intentionally synthetic):

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $COMMAND_PREFLIGHT_REPORT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"powershell","tool":"git","error_kind":"unknown_option","exit_code":129},"fixes":[{"id":"use-supported-flag","summary":"Check the local command help for the supported flag.","verification":"Run the command help and confirm the flag exists before retrying.","source":"operator","confidence":0.9,"verified":true}],"report_count":1}' \
  http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

## Client lookup

The local client remains usable without the service. Set the URL explicitly to enable a read-only lookup:

```bash
export COMMAND_PREFLIGHT_KNOWLEDGE_URL=https://knowledge.example.test
command-preflight lookup --fingerprint-id cp1-0123456789abcdef0123 --json
```

For the project deployment, use:

```bash
export COMMAND_PREFLIGHT_KNOWLEDGE_URL=https://preflight.52131415.xyz
command-preflight setup --client both --knowledge-url "$COMMAND_PREFLIGHT_KNOWLEDGE_URL" --apply
```

MCP clients get the same opt-in capability as `lookup_fingerprint`. Only the public fingerprint ID is sent, and a lookup failure never blocks local command execution. Reporting/upload is not enabled by this client.
