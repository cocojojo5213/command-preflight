# Self-hosted knowledge service

The repository builds a small HTTP service for privacy-preserving command
knowledge. The local client works without it. Lookups are opt-in, and reports
are a separate opt-in that enter an operator moderation queue.

The reference deployment uses:

```text
Cloudflare DNS/Tunnel (preflight.52131415.xyz)
        -> http://100.88.75.16:8787
        -> command-preflight-server (Docker)
```

The apex `52131415.xyz` remains a separate Coco Play route.

## Start the service

```bash
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/healthz
```

The service stores published entries and queued reports in one atomic JSON
file by default. It does not require a separate database. The container binds
to localhost on the host unless `COMMAND_PREFLIGHT_PUBLISHED_BIND` is changed.
Put a TLS reverse proxy or Cloudflare Tunnel in front of any network-facing
deployment.

## Configuration

Copy `.env.example` to `.env` when changing defaults:

```dotenv
COMMAND_PREFLIGHT_BIND=0.0.0.0:8787
COMMAND_PREFLIGHT_DATA=/data/knowledge.json
COMMAND_PREFLIGHT_PUBLISHED_BIND=127.0.0.1:8787
COMMAND_PREFLIGHT_DATA_VOLUME=command-preflight-data
COMMAND_PREFLIGHT_ALLOW_REPORT=false
COMMAND_PREFLIGHT_ADMIN_TOKEN=
COMMAND_PREFLIGHT_REPORT_SUBMIT_TOKEN=
COMMAND_PREFLIGHT_ALLOW_PROXIED_ADMIN=false
COMMAND_PREFLIGHT_REPORTS_PER_MINUTE=60
COMMAND_PREFLIGHT_REPORT_RETENTION_DAYS=30
```

`COMMAND_PREFLIGHT_ADMIN_TOKEN` protects the moderation API and operator seed
writes. Forwarded admin requests are rejected by default, so the public
Cloudflare route cannot be used for moderation even with the endpoint path;
use localhost or Tailnet. `COMMAND_PREFLIGHT_REPORT_TOKEN` remains accepted for compatibility,
but a new deployment should use the admin name. The optional submit token is
useful for private installations; a public community queue can leave it empty
and enforce per-client rate limits at the edge. The service also applies a
privacy-preserving global request limit without retaining source addresses.

An empty admin token always keeps report submission disabled at server startup.
The service never stores client IP addresses, user-agent strings, account
identifiers, raw commands, paths, environment variables, or terminal output.
Cloudflare and the network path still process ordinary transport metadata.

## Public lookup

```bash
curl -fsS http://127.0.0.1:8787/v1/knowledge/cp1-0123456789abcdef0123
```

Only published entries are returned. Fix text is explanatory data and must be
verified locally; the client never auto-executes it.

## Moderated report queue

The public report endpoint accepts only a versioned fingerprint and short fix
summary/verification text. It never accepts a command or terminal transcript.
Enable it only after setting a strong admin token and configuring TLS and rate
limiting:

```dotenv
COMMAND_PREFLIGHT_ALLOW_REPORT=true
COMMAND_PREFLIGHT_ADMIN_TOKEN=<long-random-value>
COMMAND_PREFLIGHT_REPORT_RETENTION_DAYS=30
```

A client with explicit reporting enabled submits a constrained payload:

```bash
curl -fsS -X POST https://preflight.52131415.xyz/v1/reports \
  -H 'Content-Type: application/json' \
  --data '{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"powershell","tool":"git","error_kind":"unknown_option","exit_code":129},"fix":{"summary":"Use the flag supported by the local command help.","verification":"Run local help and confirm the replacement flag before retrying.","verified":true}}'
```

This returns a `pending` report ID. It does not make the report public.

The operator reviews a batch through an authenticated management path. Keep
these routes on localhost/Tailnet or behind an access policy; do not expose an
admin token to clients:

```bash
export COMMAND_PREFLIGHT_ADMIN_TOKEN='<long-random-value>'
curl -fsS -H "Authorization: Bearer $COMMAND_PREFLIGHT_ADMIN_TOKEN" \
  'http://127.0.0.1:8787/v1/admin/reports?status=pending&limit=100'

curl -fsS -X POST \
  -H "Authorization: Bearer $COMMAND_PREFLIGHT_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"reviews":[{"id":"rpt-...","decision":"approve","reason":"Safe, specific, and locally verifiable."}]}' \
  http://127.0.0.1:8787/v1/admin/reports/review

curl -fsS -X POST \
  -H "Authorization: Bearer $COMMAND_PREFLIGHT_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{}' \
  http://127.0.0.1:8787/v1/admin/reports/publish
```

Review decisions can be `approve`, `reject`, or `hold`. Publishing promotes
only approved reports and marks their fixes `community-reviewed`. Rejected and
published queue records are pruned on startup and daily after the configured
retention period;
published knowledge remains available. An authenticated operator can delete an
individual queue record with `DELETE /v1/admin/reports/{id}`; this also leaves
any previously published knowledge intact.

The queue is intentionally model-agnostic. An operator can inspect the
redacted batch in Codex, Claude, or another local tool and submit the decisions.
Uploaded text is untrusted input: never execute it during review.
See [the moderation runbook](moderation.md) for the daily review criteria and
two-step confirmation workflow.

## Client opt-in

Lookup remains read-only and separate from reporting:

```bash
command-preflight setup --client both \
  --knowledge-url https://preflight.52131415.xyz --apply
```

To enable verified-resolution reports as well:

```bash
command-preflight setup --client both \
  --knowledge-url https://preflight.52131415.xyz \
  --report-url https://preflight.52131415.xyz \
  --enable-reporting --apply
```

The equivalent MCP environment is:

```text
COMMAND_PREFLIGHT_REPORTING=on
COMMAND_PREFLIGHT_REPORT_URL=https://preflight.52131415.xyz
```

The `submit_resolution` tool appears only with that explicit switch. The model
should call it after a fix is verified, sending only the public fingerprint and
redacted summary/verification. A failed upload never blocks local execution.
