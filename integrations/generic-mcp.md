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

The first two tools never execute a command or send network requests. The optional lookup tool performs a read-only request to the configured service.

To enable that tool, add `COMMAND_PREFLIGHT_KNOWLEDGE_URL` to the MCP server's environment in the client configuration. Leave it unset for offline operation.
