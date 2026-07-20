# Codex

The local MCP server works with Codex CLI, the Codex IDE extension, and the Codex desktop app.

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

The default client is local-only. It does not add a network endpoint or upload command data. An optional self-hosted knowledge lookup is documented in [docs/cloud.md](../docs/cloud.md); set `COMMAND_PREFLIGHT_KNOWLEDGE_URL` explicitly to enable it, and it sends only fingerprint IDs.
