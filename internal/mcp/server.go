package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/killme2008/devtap/internal/store"
)

// ServerVersion can be set by the caller to match the binary version.
var ServerVersion = "dev"

const (
	protocolVersion = "2024-11-05"
	serverName      = "devtap"
	scannerInitBuf  = 256 * 1024  // 256 KB initial scanner buffer (larger for JSON-RPC)
	scannerMaxBuf   = 1024 * 1024 // 1 MB max scanner buffer
)

// Server implements an MCP stdio server that exposes devtap tools.
type Server struct {
	store     store.Store
	sessionID string
	maxLines  int
	input     io.Reader
	output    io.Writer
}

// NewServer creates a new MCP server.
func NewServer(s store.Store, sessionID string, maxLines int) *Server {
	return &Server{
		store:     s,
		sessionID: sessionID,
		maxLines:  maxLines,
		input:     os.Stdin,
		output:    os.Stdout,
	}
}

// SetIO overrides input/output streams (for testing).
func (s *Server) SetIO(in io.Reader, out io.Writer) {
	s.input = in
	s.output = out
}

// Run starts the server loop, reading JSON-RPC messages from stdin and writing responses to stdout.
func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.input)
	scanner.Buffer(make([]byte, 0, scannerInitBuf), scannerMaxBuf)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		s.sendResult(req.ID, InitializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities: Capabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: ServerInfo{
				Name:    serverName,
				Version: ServerVersion,
			},
		})

	case "notifications/initialized":
		// No response needed for notifications

	case "tools/list":
		s.sendResult(req.ID, ToolsListResult{
			Tools: s.toolDefinitions(),
		})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
			return
		}
		s.handleToolCall(req.ID, params)

	case "ping":
		s.sendResult(req.ID, map[string]any{})

	default:
		// Unknown method: respond with method not found if it has an ID
		if req.ID != nil {
			s.sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}
}

func (s *Server) toolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "get_build_errors",
			Description: "Get pending build errors and output captured by devtap. Call this before writing or editing code to check for build failures that need fixing.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "get_build_status",
			Description: "Get a summary of pending build output counts across all sessions.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
	}
}

func (s *Server) handleToolCall(id any, params CallToolParams) {
	switch params.Name {
	case "get_build_errors":
		s.handleGetBuildErrors(id)
	case "get_build_status":
		s.handleGetBuildStatus(id)
	default:
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", params.Name)}},
			IsError: true,
		})
	}
}

func (s *Server) handleGetBuildErrors(id any) {
	messages, err := s.store.Drain(s.sessionID, s.maxLines)
	if err != nil {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error draining messages: %v", err)}},
			IsError: true,
		})
		return
	}

	if len(messages) == 0 {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: "No pending build errors."}},
		})
		return
	}

	text := FormatMessages(messages)
	s.sendResult(id, CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	})
}

func (s *Server) handleGetBuildStatus(id any) {
	counts, err := s.store.Status()
	if err != nil {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error getting status: %v", err)}},
			IsError: true,
		})
		return
	}

	if len(counts) == 0 {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: "No pending messages."}},
		})
		return
	}

	var sb strings.Builder
	sb.WriteString("Pending build output:\n")
	for session, count := range counts {
		sb.WriteString(fmt.Sprintf("  %s: %d message(s)\n", session, count))
	}

	s.sendResult(id, CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: sb.String()}},
	})
}

func (s *Server) sendResult(id any, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(resp)
}

func (s *Server) sendError(id any, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	}
	s.writeJSON(resp)
}

func (s *Server) writeJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.output, "%s\n", data)
}

// FormatMessages converts log messages into a human-readable string.
func FormatMessages(messages []store.LogMessage) string {
	var sb strings.Builder

	grouped := make(map[string][]store.LogMessage)
	var tagOrder []string
	for _, msg := range messages {
		tag := msg.Tag
		if tag == "" {
			tag = "build"
		}
		if _, exists := grouped[tag]; !exists {
			tagOrder = append(tagOrder, tag)
		}
		grouped[tag] = append(grouped[tag], msg)
	}

	for _, tag := range tagOrder {
		msgs := grouped[tag]

		var exitCode *int
		var lines []string
		for _, msg := range msgs {
			if msg.ExitCode != nil {
				exitCode = msg.ExitCode
			}
			lines = append(lines, msg.Lines...)
		}

		if exitCode != nil && *exitCode != 0 {
			sb.WriteString(fmt.Sprintf("[devtap: %s] Build failed (exit code %d):\n\n", tag, *exitCode))
		} else if exitCode != nil {
			sb.WriteString(fmt.Sprintf("[devtap: %s] Build succeeded:\n\n", tag))
		} else {
			sb.WriteString(fmt.Sprintf("[devtap: %s] Output:\n\n", tag))
		}

		for _, line := range lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
