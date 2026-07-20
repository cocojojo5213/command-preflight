# Claude Code

Claude Code can load the same stdio MCP server:

```bash
claude mcp add --scope user command-preflight -- command-preflight mcp
```

Verify it with:

```bash
claude mcp get command-preflight
```

The bundled Skill can be copied to Claude Code's user skill directory with:

```bash
command-preflight install-skill --target claude
```

Do not enable a remote lookup/report adapter unless its privacy and retention policy are acceptable to the user. The bundled client has no network behavior by default; lookup requires an explicit `COMMAND_PREFLIGHT_KNOWLEDGE_URL`.
