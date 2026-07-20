# Architecture

## Local path

The local executable is the source of truth for parsing, normalization, redaction, and fingerprints. Client integrations should call it instead of reimplementing shell rules.

```text
Codex / Claude / another CLI
        |  skill, hook, or MCP
        v
command-preflight (local binary)
        |-- shell parser adapter
        |-- executable and cwd checks
        |-- risk classification
        `-- privacy-preserving fingerprint
```

The binary is intentionally inspection-only. A separate agent or shell tool remains responsible for execution and approval.

## Cloud path (self-hosted, opt-in)

The same repository can build a hosted knowledge service. The service should accept only the normalized fingerprint contract, never raw command text or environment data. The local client remains functional when the service is unavailable.

```text
local agent -> redact + fingerprint -> optional HTTPS lookup
      |
      `-> verified fix + explicit reporting opt-in
                         |
                         v
                  pending report queue
                         |
                  operator batch review
                         |
                         v
                published knowledge store
```

Queued reports and published entries are separate states even when they share
one persistence file. Public report submission can create only `pending`
records; it cannot write to the knowledge store. The authenticated operator
API supports batch approve/reject/hold decisions and a separate publish step.

Cloud responses must include the supported shell/tool version, confidence, provenance, and a local verification step. They must not be treated as executable instructions.

## Extension points

- `internal/core`: platform-neutral policy and data contracts.
- `cmd/command-preflight`: CLI and installer-facing entry point.
- `internal/mcp`: stdio MCP adapter.
- `cmd/command-preflight-server`: read-only-by-default lookup service and opt-in moderated report queue using the same contracts.
- `skills/`: model-facing workflow, kept concise to protect context.
