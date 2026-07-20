package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cocojojo5213/command-preflight/internal/cloud"
	"github.com/cocojojo5213/command-preflight/internal/core"
)

type Config struct {
	KnowledgeURL string
}

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
	return ServeWithConfig(input, output, Config{KnowledgeURL: os.Getenv("COMMAND_PREFLIGHT_KNOWLEDGE_URL")})
}

func ServeWithConfig(input io.Reader, output io.Writer, config Config) error {
	var knowledgeClient *cloud.Client
	if strings.TrimSpace(config.KnowledgeURL) != "" {
		client, err := cloud.NewClient(config.KnowledgeURL)
		if err != nil {
			return fmt.Errorf("configure knowledge lookup: %w", err)
		}
		knowledgeClient = client
	}
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
		if err := handleRequest(encoder, req, knowledgeClient); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleRequest(encoder *json.Encoder, req request, knowledgeClient *cloud.Client) error {
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
		instructions := "Inspection only. Never execute commands or upload telemetry. Validate locally before acting."
		if knowledgeClient != nil {
			instructions += " An opt-in knowledge lookup is available and sends only public fingerprint IDs."
		}
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "command-preflight", "version": core.Version},
			"instructions":    instructions,
		}})
	case "ping":
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{}})
	case "tools/list":
		tools := []map[string]interface{}{
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
		}
		if knowledgeClient != nil {
			tools = append(tools, map[string]interface{}{
				"name":        "lookup_fingerprint",
				"description": "Look up a public error fingerprint in the explicitly configured knowledge service. Sends only the cp1 fingerprint ID; never sends command text or terminal output.",
				"inputSchema": map[string]interface{}{
					"type": "object", "properties": map[string]interface{}{
						"fingerprint_id": map[string]string{"type": "string", "description": "A cp1-v1 public fingerprint ID"},
					}, "required": []string{"fingerprint_id"},
				},
			})
		}
		return encoder.Encode(response{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{"tools": tools}})
	case "tools/call":
		return handleToolCall(encoder, id, req.Params, knowledgeClient)
	default:
		return writeError(encoder, id, -32601, "method not found", req.Method)
	}
}

func handleToolCall(encoder *json.Encoder, id interface{}, raw json.RawMessage, knowledgeClient *cloud.Client) error {
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
	case "lookup_fingerprint":
		if knowledgeClient == nil {
			return writeError(encoder, id, -32602, "knowledge lookup is not configured", "set COMMAND_PREFLIGHT_KNOWLEDGE_URL to enable it")
		}
		fingerprintID, err := stringArg(params.Arguments, "fingerprint_id", true)
		if err != nil {
			return writeError(encoder, id, -32602, "invalid fingerprint_id", err.Error())
		}
		entry, found, err := knowledgeClient.Lookup(context.Background(), fingerprintID)
		if err != nil {
			return writeError(encoder, id, -32002, "knowledge lookup failed", err.Error())
		}
		result = map[string]interface{}{"fingerprint_id": fingerprintID, "found": found}
		if found {
			result.(map[string]interface{})["entry"] = entry
		}
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
