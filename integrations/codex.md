# Codex

The local MCP server works with Codex CLI, the Codex IDE extension, and the Codex desktop app.

The release installer registers this automatically when Codex is installed. A manual offline setup is:

After installing the binary, register the stdio server:

```bash
codex mcp add command-preflight -- command-preflight mcp
```

Check the connection with:

```bash
codex mcp list
```

Install the bundled Skill into the global skill directory when you want the model-facing workflow as well:

```bash
command-preflight install-skill --target codex
```

This uses `$CODEX_HOME/skills` when `CODEX_HOME` is set, otherwise `~/.codex/skills`. Set `COMMAND_PREFLIGHT_CODEX_SKILL_DIR` when a host uses a custom Skill directory.

To explicitly enable the project-maintained read-only lookup, configure the MCP process environment in one step:

```bash
command-preflight setup --client codex --knowledge-url https://preflight.52131415.xyz --apply
```

The default client is local-only. It does not add a network endpoint or upload command data. Lookup is opt-in and sends only public fingerprint IDs; see [docs/cloud.md](../docs/cloud.md) for self-hosting or the project endpoint.
