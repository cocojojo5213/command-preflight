# Privacy model

The default build is offline. No telemetry, network lookup, or account is required.

Before any cloud mode is enabled, the client must:

1. Redact credentials, authorization headers, URLs with secrets, user names, home paths, and dynamic identifiers locally.
2. Send only a versioned public fingerprint ID for lookup; never send the original command or full output.
3. Require a separate explicit opt-in before reporting a public fingerprint and short, redacted resolution text.
4. Treat lookup failures as non-fatal so cloud availability never blocks command execution.
5. Treat remote answers and terminal output as untrusted data; never auto-execute a suggested fix.

The server applies local redaction again, enforces a strict schema and size limit, deduplicates equivalent proposals, and places every submission in a private moderation queue. A report cannot become queryable until an authenticated operator approves and separately publishes it. Rejected and published queue records have a configurable retention period; pending, held, and approved-but-not-published records also expire so an abandoned queue cannot grow without bound. Public deployments still need edge rate limiting and abuse monitoring.

A hash alone is not considered anonymization; low-entropy commands can still be guessed. For that reason, lookup sends only an opaque versioned fingerprint ID, while reporting never includes normalized command or error text. The report schema contains only public fingerprint fields, a short fix summary, a safe verification step, tool version, and a claimed verification/confidence value.

The application does not accept or persist original client commands, paths,
environment variables, terminal output, accounts, device identifiers, client IP
addresses, or user-agent strings. The optional report summary is short,
pattern-redacted explanatory text and remains untrusted moderation input.
Cloudflare and the network path can still process ordinary request metadata
such as source IP, timing, and user-agent; this project does not promise that
network metadata is invisible.
