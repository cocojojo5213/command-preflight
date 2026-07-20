# Generic MCP clients

The server speaks MCP over stdio and exposes inspection-only tools. A generic client can use this configuration:

```json
{
  "mcpServers": {
    "command-preflight": {
      "command": "command-preflight",
      "args": ["mcp"]
    }
  }
}
```

Tools:

- `preflight_command`: shell syntax, working directory, executable resolution, and risk checks.
- `fingerprint_command_error`: local redaction and deterministic error fingerprinting.
- `lookup_fingerprint`: available only when `COMMAND_PREFLIGHT_KNOWLEDGE_URL` is explicitly configured; sends only a public fingerprint ID.
- `submit_resolution`: available only when `COMMAND_PREFLIGHT_REPORTING=on` and a report URL are explicitly configured; sends a redacted fingerprint and short fix text to the server's moderation queue.

The inspection tools never execute a command or send network requests. The optional lookup tool performs a read-only request; the report tool performs a constrained POST only after the model has verified a fix.

To enable that tool, add `COMMAND_PREFLIGHT_KNOWLEDGE_URL` to the MCP server's environment in the client configuration. Leave it unset for offline operation.

To enable community reporting separately, add `COMMAND_PREFLIGHT_REPORTING=on` and `COMMAND_PREFLIGHT_REPORT_URL` (or reuse the knowledge URL). Reporting is disabled unless both the explicit switch and a URL are present. A private server may also use `COMMAND_PREFLIGHT_REPORT_SUBMIT_TOKEN`.

For example, the stdio process environment can contain:

```json
{
  "mcpServers": {
    "command-preflight": {
      "command": "command-preflight",
      "args": ["mcp"],
      "env": {
        "COMMAND_PREFLIGHT_KNOWLEDGE_URL": "https://preflight.52131415.xyz",
        "COMMAND_PREFLIGHT_REPORTING": "on",
        "COMMAND_PREFLIGHT_REPORT_URL": "https://preflight.52131415.xyz"
      }
    }
  }
}
```

Lookup is an explicit read-only opt-in; reporting is a separate explicit opt-in. The service never receives the original command or terminal output.
