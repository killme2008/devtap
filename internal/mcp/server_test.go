package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/killme2008/devtap/internal/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	messages []store.LogMessage
	counts   map[string]int
}

func (m *mockStore) Write(sessionID string, msg store.LogMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockStore) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	msgs := m.messages
	m.messages = nil
	return msgs, nil
}

func (m *mockStore) Status() (map[string]int, error) {
	if m.counts != nil {
		return m.counts, nil
	}
	if len(m.messages) > 0 {
		return map[string]int{"test": len(m.messages)}, nil
	}
	return map[string]int{}, nil
}

func (m *mockStore) Close() error { return nil }

func sendRequest(t *testing.T, in *bytes.Buffer, id any, method string, params any) {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if id != nil {
		req["id"] = id
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		req["params"] = json.RawMessage(raw)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	in.Write(data)
	in.WriteByte('\n')
}

func parseResponses(t *testing.T, out *bytes.Buffer) []Response {
	t.Helper()
	var responses []Response
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestInitialize(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "initialize", map[string]any{})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	raw, err := json.Marshal(responses[0].Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if result.ProtocolVersion != protocolVersion {
		t.Errorf("protocol version: got %q, want %q", result.ProtocolVersion, protocolVersion)
	}
	if result.ServerInfo.Name != serverName {
		t.Errorf("server name: got %q, want %q", result.ServerInfo.Name, serverName)
	}
}

func TestToolsList(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/list", nil)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	raw, _ := json.Marshal(responses[0].Result)
	var result ToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}

	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	if !names["get_build_errors"] {
		t.Error("missing get_build_errors tool")
	}
	if !names["get_build_status"] {
		t.Error("missing get_build_status tool")
	}
}

func TestGetBuildErrorsEmpty(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{
		"name": "get_build_errors",
	})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if !strings.Contains(result.Content[0].Text, "No pending") {
		t.Errorf("expected 'No pending' message, got %q", result.Content[0].Text)
	}
	if result.IsError {
		t.Error("should not be an error")
	}
}

func TestGetBuildErrorsWithMessages(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	exitCode := 1
	ms := &mockStore{
		messages: []store.LogMessage{
			{
				Timestamp: time.Now(),
				Tag:       "cargo-check",
				Stream:    "stderr",
				Lines:     []string{"error[E0308]: mismatched types", "  --> src/main.rs:42:5"},
			},
			{
				Timestamp: time.Now(),
				Tag:       "cargo-check",
				Stream:    "exit",
				ExitCode:  &exitCode,
			},
		},
	}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{
		"name": "get_build_errors",
	})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "error[E0308]") {
		t.Error("should contain error message")
	}
	if !strings.Contains(text, "cargo-check") {
		t.Error("should contain tag")
	}
	if !strings.Contains(text, "Build failed") {
		t.Error("should indicate build failure")
	}
}

func TestGetBuildStatus(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{
		counts: map[string]int{
			"session-a": 3,
			"session-b": 1,
		},
	}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{
		"name": "get_build_status",
	})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text
	if !strings.Contains(text, "session-a") {
		t.Error("should contain session-a")
	}
	if !strings.Contains(text, "3 message(s)") {
		t.Error("should contain count for session-a")
	}
}

func TestGetBuildStatusEmpty(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{
		"name": "get_build_status",
	})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	if !strings.Contains(result.Content[0].Text, "No pending") {
		t.Errorf("expected 'No pending', got %q", result.Content[0].Text)
	}
}

func TestPing(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "ping", nil)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("ping should not return error: %v", responses[0].Error)
	}
}

func TestUnknownMethod(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "unknown/method", nil)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if responses[0].Error == nil {
		t.Fatal("expected error response")
	}
	if responses[0].Error.Code != -32601 {
		t.Errorf("error code: got %d, want -32601", responses[0].Error.Code)
	}
}

func TestUnknownTool(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{
		"name": "nonexistent_tool",
	})

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	if !result.IsError {
		t.Error("unknown tool should return error")
	}
	if !strings.Contains(result.Content[0].Text, "Unknown tool") {
		t.Error("should mention unknown tool")
	}
}

func TestNotificationNoResponse(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	// notifications/initialized has no ID and should produce no response
	sendRequest(t, &in, nil, "notifications/initialized", nil)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("notification should produce no response, got %q", out.String())
	}
}

func TestMultipleRequests(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	ms := &mockStore{}

	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "initialize", map[string]any{})
	sendRequest(t, &in, nil, "notifications/initialized", nil)
	sendRequest(t, &in, 2, "tools/list", nil)
	sendRequest(t, &in, 3, "ping", nil)

	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	// initialize + tools/list + ping = 3 responses (notification produces none)
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}
}

func TestFormatMessages(t *testing.T) {
	exitCode := 1
	messages := []store.LogMessage{
		{Tag: "mypy", Stream: "stderr", Lines: []string{"main.py:5: error: Incompatible types"}},
		{Tag: "mypy", Stream: "exit", ExitCode: &exitCode},
	}

	text := FormatMessages(messages)
	if !strings.Contains(text, "Build failed") {
		t.Error("should indicate build failure")
	}
	if !strings.Contains(text, "Incompatible types") {
		t.Error("should contain error message")
	}
	if !strings.Contains(text, "mypy") {
		t.Error("should contain tag")
	}
}

func TestFormatMessagesSuccess(t *testing.T) {
	exitCode := 0
	messages := []store.LogMessage{
		{Tag: "go-build", Stream: "stdout", Lines: []string{"ok"}},
		{Tag: "go-build", Stream: "exit", ExitCode: &exitCode},
	}

	text := FormatMessages(messages)
	if !strings.Contains(text, "Build succeeded") {
		t.Error("should indicate success")
	}
}

func TestFormatMessagesNoExitCode(t *testing.T) {
	messages := []store.LogMessage{
		{Tag: "", Stream: "stderr", Lines: []string{"some output"}},
	}

	text := FormatMessages(messages)
	if !strings.Contains(text, "Output") {
		t.Error("should use 'Output' for messages without exit code")
	}
	if !strings.Contains(text, "build") {
		t.Error("should use default tag 'build'")
	}
}
