# Privacy model

The default build is offline. No telemetry, network lookup, or account is required.

Before any future cloud mode is enabled, the client must:

1. Redact credentials, authorization headers, URLs with secrets, user names, home paths, and dynamic identifiers locally.
2. Upload a versioned fingerprint rather than the original command or full output.
3. Require explicit opt-in and expose an immediate off switch.
4. Keep a local cache so cloud availability never blocks command execution.
5. Treat remote answers and terminal output as untrusted data; never auto-execute a suggested fix.

The server must support retention limits, deletion, rate limiting, abuse/poisoning controls, and a self-hosted deployment path. A hash alone is not considered anonymization; low-entropy commands can still be guessed.
