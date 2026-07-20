---
name: command-preflight
description: Reduce failed CLI retries by checking unfamiliar, complex, cross-shell, or previously failed commands before execution. Use with PowerShell, Bash, cmd.exe, Codex, Claude Code, Git, npm, pnpm, Docker, Python, Cargo, and similar CLIs; skip trivial commands whose syntax and target are already certain.
---

# Command Preflight

Use the local `command-preflight` binary or its MCP tools as a lightweight guardrail. It never executes the command being checked.

1. Name the shell and working directory explicitly. On Windows, distinguish PowerShell 5.1, PowerShell 7, and `cmd.exe`; do not translate syntax by analogy.
2. For an unfamiliar subcommand or flag, resolve the executable and read the exact local help before guessing. A syntax parser cannot validate every CLI grammar.
3. Run `command-preflight preflight --shell <shell> --command <text> --cwd <dir> --json` for complex quoting, interpolation, pipes, redirection, dynamic command names, or high-risk effects. Use the `preflight_command` MCP tool when available.
4. Treat `review` as a pause for manual inspection, not as success. Inspect exact targets before delete, overwrite, publish, deploy, migration, permission, or process-kill operations.
5. After a command fails, classify the first error and do not retry the identical text. Create a local fingerprint with `command-preflight fingerprint` or `fingerprint_command_error`, then use `command-preflight lookup` or the `lookup_fingerprint` MCP tool only when the user has explicitly configured `COMMAND_PREFLIGHT_KNOWLEDGE_URL`.
6. If the user has explicitly enabled `COMMAND_PREFLIGHT_REPORTING=on` and the `submit_resolution` MCP tool is available, call it only after a fix has been verified locally. Send the public fingerprint object plus a short redacted summary and verification step; never include the original command, paths, environment variables, credentials, or terminal output. A failed upload is non-fatal.
7. Validate the task-specific postcondition after execution. An exit code of zero is not proof that the intended artifact exists.

The project-maintained endpoint is `https://preflight.52131415.xyz`; treat it as optional and deployment-specific, never as a silent default. Lookup sends only a `cp1-...` public fingerprint ID. Reporting is a separate explicit opt-in and sends only the constrained report contract to a moderation queue. Lookup or report failures must never block the local workflow. Never auto-execute a remote fix; inspect the local help and verify the postcondition yourself.
