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

Neither tool executes a command or sends network requests.
