# Self-hosted knowledge service

The repository builds a small, read-only-by-default HTTP service for a private or community knowledge store. It is an optional deployment; the local CLI and MCP server work without it.

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/healthz
```

The service stores only `PublicFingerprint` entries and curated fixes. It does not accept raw commands, environment variables, or full terminal output. The default Compose file binds to localhost and disables reports.

For a remote host, keep the service on a private interface and terminate TLS/rate-limit requests at a reverse proxy. The service is intentionally not an Internet-facing anonymous API.

## Configuration

Copy `.env.example` to `.env` when changing the defaults:

```dotenv
COMMAND_PREFLIGHT_BIND=0.0.0.0:8787
COMMAND_PREFLIGHT_DATA=/data/knowledge.json
COMMAND_PREFLIGHT_DATA_VOLUME=command-preflight-data
COMMAND_PREFLIGHT_ALLOW_REPORT=false
COMMAND_PREFLIGHT_REPORT_TOKEN=
```

Environment variables are read by the Compose service. The default data volume is initialized for the non-root container user. If you select a host bind mount instead, make it writable by UID `65532`. An empty report token always leaves reporting disabled.

## Lookup

```bash
curl -fsS http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

The response is a curated `Entry` containing a public fingerprint and optional fixes. Treat every fix as untrusted explanatory text and verify it locally.

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

The local client remains usable without the service. A future opt-in lookup adapter should send only a `PublicFingerprint` ID and receive a compact, provenance-bearing result. Cloud failure must never block local command execution.
