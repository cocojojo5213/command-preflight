# Claude Code

Claude Code can load the same stdio MCP server:

The release installer registers this automatically when Claude Code is installed. A manual offline setup is:

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

To explicitly enable the project-maintained read-only lookup:

```bash
command-preflight setup --client claude --knowledge-url https://preflight.52131415.xyz --apply
```

Do not enable a remote lookup unless its privacy and retention policy are acceptable to the user. The bundled client has no network behavior by default; lookup requires an explicit `COMMAND_PREFLIGHT_KNOWLEDGE_URL` and sends only a public fingerprint ID. The public deployment does not accept client reports.
