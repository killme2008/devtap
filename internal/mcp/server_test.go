package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
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
			"test-session":  3,
			"other-session": 1,
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
	if !strings.Contains(text, "3 message(s)") {
		t.Errorf("should show count for current session, got %q", text)
	}
	// Should NOT include other sessions
	if strings.Contains(text, "other-session") {
		t.Error("should not include other sessions")
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

// --- Multi-source tests ---

// errStore implements store.Store and returns configurable errors on Drain/Status.
type errStore struct {
	mockStore
	drainErr  error
	statusErr error
}

func (e *errStore) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	if e.drainErr != nil {
		return nil, e.drainErr
	}
	return e.mockStore.Drain(sessionID, maxLines)
}

func (e *errStore) Status() (map[string]int, error) {
	if e.statusErr != nil {
		return nil, e.statusErr
	}
	return e.mockStore.Status()
}

// budgetTrackingStore records the maxLines argument passed to each Drain call.
type budgetTrackingStore struct {
	mockStore
	drainBudgets []int
}

func (b *budgetTrackingStore) Drain(sessionID string, maxLines int) ([]store.LogMessage, error) {
	b.drainBudgets = append(b.drainBudgets, maxLines)
	return b.mockStore.Drain(sessionID, maxLines)
}

func TestMultiSourceDrainMerge(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	exitCode := 1

	localStore := &mockStore{
		messages: []store.LogMessage{
			{Timestamp: ts, Tag: "make", Stream: "stderr", Lines: []string{"local error"}, Host: "myhost"},
			{Timestamp: ts, Tag: "make", Stream: "exit", ExitCode: &exitCode, Host: "myhost"},
		},
	}
	remoteStore := &mockStore{
		messages: []store.LogMessage{
			{Timestamp: ts.Add(time.Second), Tag: "ci", Stream: "stderr", Lines: []string{"remote error"}, Host: "ci-runner"},
		},
	}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: localStore, SessionID: "local-session", Label: "local"},
		{Store: remoteStore, SessionID: "remote-session", Label: "myproject"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text

	// Should contain multi-source header
	if !strings.Contains(text, "Draining from 2 sources") {
		t.Errorf("missing multi-source header in: %s", text)
	}
	// Should contain both local and remote messages
	if !strings.Contains(text, "local error") {
		t.Errorf("missing local error in: %s", text)
	}
	if !strings.Contains(text, "remote error") {
		t.Errorf("missing remote error in: %s", text)
	}
	// Should contain host labels
	if !strings.Contains(text, "myhost") {
		t.Errorf("missing host label 'myhost' in: %s", text)
	}
	if !strings.Contains(text, "ci-runner") {
		t.Errorf("missing host label 'ci-runner' in: %s", text)
	}
}

func TestMultiSourceDedup(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	localStore := &mockStore{
		messages: []store.LogMessage{
			{Timestamp: ts, Tag: "make", Stream: "stderr", Lines: []string{"same error"}, Host: "myhost"},
		},
	}
	remoteStore := &mockStore{
		messages: []store.LogMessage{
			// Same timestamp, tag, stream, content — should be deduped
			{Timestamp: ts, Tag: "make", Stream: "stderr", Lines: []string{"same error"}, Host: "ci-runner"},
		},
	}

	var in bytes.Buffer
	var out bytes.Buffer

	srv := NewMultiSourceServer([]DrainSource{
		{Store: localStore, SessionID: "s1", Label: "local"},
		{Store: remoteStore, SessionID: "s2", Label: "remote"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text

	// "same error" should appear only once in the output
	count := strings.Count(text, "same error")
	if count != 1 {
		t.Errorf("expected 'same error' to appear once (dedup), got %d times in: %s", count, text)
	}
}

func TestSingleSourceNoPrefix(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	ms := &mockStore{
		messages: []store.LogMessage{
			{Timestamp: time.Now(), Tag: "build", Stream: "stderr", Lines: []string{"error line"}},
		},
	}

	// Single source — backward compatible
	srv := NewServer(ms, "test-session", 100)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text

	// Should NOT contain multi-source header
	if strings.Contains(text, "Draining from") {
		t.Errorf("single source should not have multi-source header: %s", text)
	}
	// Should NOT have host/label | prefix on tags
	if strings.Contains(text, "unknown/") || strings.Contains(text, " | ") {
		t.Errorf("single source should not label tags: %s", text)
	}
	// Should contain the error
	if !strings.Contains(text, "error line") {
		t.Errorf("missing error in: %s", text)
	}
}

func TestMultiSourceConfiguredSourceFailure(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	localStore := &mockStore{
		messages: []store.LogMessage{
			{Timestamp: time.Now(), Tag: "make", Stream: "stderr", Lines: []string{"local only"}, Host: "myhost"},
		},
	}
	failingStore := &errStore{drainErr: fmt.Errorf("connection refused")}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: localStore, SessionID: "local-session", Label: "local"},
		{Store: failingStore, SessionID: "remote-session", Label: "remote"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text

	// Should contain local messages
	if !strings.Contains(text, "local only") {
		t.Errorf("should contain local messages: %s", text)
	}
	// Should contain warning about unreachable source
	if !strings.Contains(text, "unreachable") {
		t.Errorf("should warn about unreachable source: %s", text)
	}
	// Should NOT be an error response
	if result.IsError {
		t.Error("multi-source partial failure should not be an error")
	}
}

func TestMultiSourceStatusMerge(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	localStore := &mockStore{counts: map[string]int{"local-session": 3}}
	remoteStore := &mockStore{counts: map[string]int{"remote-session": 2}}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: localStore, SessionID: "local-session", Label: "local"},
		{Store: remoteStore, SessionID: "remote-session", Label: "remote"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_status"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text
	if !strings.Contains(text, "5 message(s)") {
		t.Errorf("should merge counts (3+2=5), got: %s", text)
	}
}

func TestDedupKey(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	msg1 := store.LogMessage{Timestamp: ts, Tag: "build", Stream: "stderr", Lines: []string{"error"}}
	msg2 := store.LogMessage{Timestamp: ts, Tag: "build", Stream: "stderr", Lines: []string{"error"}}

	if dedupKey(msg1) != dedupKey(msg2) {
		t.Error("identical messages should have same dedup key")
	}

	// Different content
	msg3 := store.LogMessage{Timestamp: ts, Tag: "build", Stream: "stderr", Lines: []string{"different"}}
	if dedupKey(msg1) == dedupKey(msg3) {
		t.Error("different content should produce different dedup key")
	}

	// With source prefix — should still match the original
	msg4 := store.LogMessage{Timestamp: ts, Tag: "myhost/local | build", Stream: "stderr", Lines: []string{"error"}}
	if dedupKey(msg1) != dedupKey(msg4) {
		t.Error("prefix-stripped key should match original")
	}
}

func TestHostFieldInLogMessage(t *testing.T) {
	msg := store.LogMessage{
		Timestamp: time.Now(),
		Tag:       "build",
		Stream:    "stderr",
		Lines:     []string{"error"},
		Host:      "myhost",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded store.LogMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Host != "myhost" {
		t.Errorf("Host: got %q, want %q", decoded.Host, "myhost")
	}

	// Empty host should be omitted in JSON
	msg2 := store.LogMessage{Tag: "build", Stream: "stderr"}
	data2, _ := json.Marshal(msg2)
	if strings.Contains(string(data2), "host") {
		t.Error("empty host should be omitted from JSON")
	}
}

func TestBudgetTracking(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	// Source 1: 3 messages (budget tracks message count, not line count)
	src1 := &budgetTrackingStore{mockStore: mockStore{
		messages: []store.LogMessage{
			{Timestamp: time.Now(), Tag: "make", Stream: "stderr", Lines: []string{"err1", "err2"}, Host: "h1"},
			{Timestamp: time.Now(), Tag: "make", Stream: "stderr", Lines: []string{"err3"}, Host: "h1"},
			{Timestamp: time.Now(), Tag: "make", Stream: "exit", Host: "h1"},
		},
	}}
	// Source 2: 2 messages
	src2 := &budgetTrackingStore{mockStore: mockStore{
		messages: []store.LogMessage{
			{Timestamp: time.Now(), Tag: "ci", Stream: "stderr", Lines: []string{"a", "b", "c"}, Host: "h2"},
			{Timestamp: time.Now(), Tag: "ci", Stream: "exit", Host: "h2"},
		},
	}}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: src1, SessionID: "s1", Label: "local"},
		{Store: src2, SessionID: "s2", Label: "remote"},
	}, 10) // global budget = 10 messages
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Source 1 should receive full budget (10 messages)
	if len(src1.drainBudgets) != 1 || src1.drainBudgets[0] != 10 {
		t.Errorf("source 1 budget: got %v, want [10]", src1.drainBudgets)
	}

	// Source 2 should receive remaining budget (10 - 3 = 7 messages)
	if len(src2.drainBudgets) != 1 || src2.drainBudgets[0] != 7 {
		t.Errorf("source 2 budget: got %v, want [7]", src2.drainBudgets)
	}
}

func TestMultiSourceAllSourcesFailed(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	fail1 := &errStore{drainErr: fmt.Errorf("timeout")}
	fail2 := &errStore{drainErr: fmt.Errorf("connection refused")}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: fail1, SessionID: "s1", Label: "local"},
		{Store: fail2, SessionID: "s2", Label: "remote"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_errors"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	if !result.IsError {
		t.Error("all sources failed should be an error response")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "timeout") {
		t.Errorf("should contain first error, got: %s", text)
	}
	if !strings.Contains(text, "connection refused") {
		t.Errorf("should contain second error, got: %s", text)
	}
}

func TestMultiSourceStatusPartialFailureWarning(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	okStore := &mockStore{counts: map[string]int{"s1": 0}}
	failStore := &errStore{statusErr: fmt.Errorf("connection refused")}

	srv := NewMultiSourceServer([]DrainSource{
		{Store: okStore, SessionID: "s1", Label: "local"},
		{Store: failStore, SessionID: "s2", Label: "remote"},
	}, 10000)
	srv.SetIO(&in, &out)

	sendRequest(t, &in, 1, "tools/call", map[string]any{"name": "get_build_status"})
	if err := srv.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	responses := parseResponses(t, &out)
	raw, _ := json.Marshal(responses[0].Result)
	var result CallToolResult
	_ = json.Unmarshal(raw, &result)

	text := result.Content[0].Text

	// Should report no pending output AND include the warning
	if !strings.Contains(text, "No pending") {
		t.Errorf("should report no pending output, got: %s", text)
	}
	if !strings.Contains(text, "connection refused") {
		t.Errorf("should include partial failure warning, got: %s", text)
	}
	// Should NOT be an error response (partial failure, not total failure)
	if result.IsError {
		t.Error("partial failure should not be an error response")
	}
}
