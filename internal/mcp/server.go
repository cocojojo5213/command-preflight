package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cocojojo5213/command-preflight/internal/core"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// Serve runs a small stdio MCP server. It intentionally exposes inspection tools only.
func Serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := writeError(encoder, nil, -32700, "invalid JSON", err.Error()); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		if err := handleRequest(encoder, req); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleRequest(encoder *json.Encoder, req request) error {
	id := rawID(req.ID)
	switch req.Method {
	case "initialize":
		protocolVersion := "2024-11-05"
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
			if params.ProtocolVersion != "" {
				protocolVersion = params.ProtocolVersion
			}
		}
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "command-preflight", "version": core.Version},
			"instructions":    "Inspection only. Never execute commands or upload telemetry. Validate locally before acting.",
		}})
	case "ping":
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{}})
	case "tools/list":
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "preflight_command",
					"description": "Check shell syntax, working directory, executable resolution, and risk without executing the command.",
					"inputSchema": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"shell":   map[string]string{"type": "string", "description": "powershell, bash, sh, or cmd"},
							"command": map[string]string{"type": "string"},
							"cwd":     map[string]string{"type": "string"},
						}, "required": []string{"shell", "command"},
					},
				},
				{
					"name":        "fingerprint_command_error",
					"description": "Create a redacted, local error fingerprint for a failed command. No network upload is performed.",
					"inputSchema": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"shell":     map[string]string{"type": "string"},
							"command":   map[string]string{"type": "string"},
							"exit_code": map[string]string{"type": "integer"},
							"stderr":    map[string]string{"type": "string"},
							"stdout":    map[string]string{"type": "string"},
						}, "required": []string{"shell", "command", "exit_code"},
					},
				},
			},
		}})
	case "tools/call":
		return handleToolCall(encoder, id, req.Params)
	default:
		return writeError(encoder, id, -32601, "method not found", req.Method)
	}
}

func handleToolCall(encoder *json.Encoder, id interface{}, raw json.RawMessage) error {
	var params struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return writeError(encoder, id, -32602, "invalid tool parameters", err.Error())
	}
	var result interface{}
	switch params.Name {
	case "preflight_command":
		shell, err := stringArg(params.Arguments, "shell", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid shell", err.Error())
		}
		command, err := stringArg(params.Arguments, "command", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid command", err.Error())
		}
		cwd, err := stringArg(params.Arguments, "cwd", false)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid cwd", err.Error())
		}
		result = core.RunPreflight(core.PreflightOptions{Shell: core.Shell(shell), Command: command, CWD: cwd})
	case "fingerprint_command_error":
		shell, err := stringArg(params.Arguments, "shell", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid shell", err.Error())
		}
		command, err := stringArg(params.Arguments, "command", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid command", err.Error())
		}
		exitCode, err := intArg(params.Arguments, "exit_code", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid exit_code", err.Error())
		}
		stderr, err := stringArg(params.Arguments, "stderr", false)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid stderr", err.Error())
		}
		stdout, err := stringArg(params.Arguments, "stdout", false)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid stdout", err.Error())
		}
		result = core.BuildFingerprint(core.ErrorInput{Shell: core.Shell(shell), Command: command, ExitCode: exitCode, Stderr: stderr, Stdout: stdout})
	default:
		return writeError(encoder, id, -32602, "unknown tool", params.Name)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{
		"content":           []map[string]string{{"type": "text", "text": string(data)}},
		"structuredContent": result,
	}})
}

func stringArg(args map[string]json.RawMessage, name string, required bool) (string, error) {
	raw, ok := args[name]
	if !ok {
		if required {
			return "", fmt.Errorf("missing %s", name)
		}
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func intArg(args map[string]json.RawMessage, name string, required bool) (int, error) {
	raw, ok := args[name]
	if !ok {
		if required {
			return 0, fmt.Errorf("missing %s", name)
		}
		return 0, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func rawID(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func writeError(encoder *json.Encoder, id interface{}, code int, message, data string) error {
	return encoder.Encode(response{JSONRPC: "2.0", ID: id, Error: map[string]interface{}{"code": code, "message": message, "data": data}})
}
