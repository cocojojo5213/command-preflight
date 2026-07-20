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
5. After a command fails, classify the first error and do not retry the identical text. Create a local fingerprint with `command-preflight fingerprint` or `fingerprint_command_error`, then use `command-preflight lookup` or the `lookup_fingerprint` MCP tool when an opt-in knowledge URL is configured.
6. Validate the task-specific postcondition after execution. An exit code of zero is not proof that the intended artifact exists.

Keep telemetry disabled unless the user explicitly enables a compatible, privacy-reviewed endpoint. Never send raw commands, environment variables, paths, or unredacted output to a remote service.
