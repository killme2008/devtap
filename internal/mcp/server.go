package mcp

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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

// DrainSource represents a single source from which the MCP server drains messages.
type DrainSource struct {
	Store     store.Store
	SessionID string
	Label     string // e.g. "local" or the explicit session name
}

// Server implements an MCP stdio server that exposes devtap tools.
type Server struct {
	sources  []DrainSource
	maxLines int
	input    io.Reader
	output   io.Writer
}

// NewServer creates a new MCP server with a single drain source (backward compatible).
func NewServer(s store.Store, sessionID string, maxLines int) *Server {
	return &Server{
		sources: []DrainSource{
			{Store: s, SessionID: sessionID, Label: sessionID},
		},
		maxLines: maxLines,
		input:    os.Stdin,
		output:   os.Stdout,
	}
}

// NewMultiSourceServer creates a new MCP server with multiple drain sources.
func NewMultiSourceServer(sources []DrainSource, maxLines int) *Server {
	return &Server{
		sources:  sources,
		maxLines: maxLines,
		input:    os.Stdin,
		output:   os.Stdout,
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
			Description: "Get pending build errors and output captured by devtap. Call this at the start of each task and before writing or editing code to check for build failures that need fixing. A separate terminal may have captured new build errors or user messages at any time. Always present the full captured output to the user verbatim — do not summarize, omit, or reinterpret any content, even if the build succeeded.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "get_build_status",
			Description: "Get a summary of pending build output counts for the current session.",
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
	multiSource := len(s.sources) > 1

	var allMessages []store.LogMessage
	var warnings []string
	successCount := 0

	// Track remaining message budget so the total across all sources stays
	// bounded. Drain treats its limit as a message count (not a line count),
	// so the budget must use the same unit.  Line-level truncation is handled
	// afterward by TruncateMessages.
	remaining := s.maxLines

	for _, src := range s.sources {
		if remaining <= 0 {
			break
		}

		messages, err := src.Store.Drain(src.SessionID, remaining)
		if err != nil {
			if multiSource {
				warnings = append(warnings, fmt.Sprintf("[devtap] Source %q unreachable: %v", src.Label, err))
				continue
			}
			s.sendResult(id, CallToolResult{
				Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error draining messages: %v", err)}},
				IsError: true,
			})
			return
		}
		successCount++

		if multiSource {
			for i := range messages {
				host := messages[i].Host
				if host == "" {
					host = "unknown"
				}
				messages[i].Tag = fmt.Sprintf("%s/%s | %s", host, src.Label, messages[i].Tag)
			}
		}

		allMessages = append(allMessages, messages...)
		remaining -= len(messages)
	}

	// Dedup if multi-source
	if multiSource && len(allMessages) > 0 {
		allMessages = DedupMessages(allMessages)
	}

	// All sources failed — report error rather than silently returning "no errors".
	if multiSource && successCount == 0 && len(warnings) > 0 {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error draining messages: %s", strings.Join(warnings, "; "))}},
			IsError: true,
		})
		return
	}

	if len(allMessages) == 0 && len(warnings) == 0 {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: "No pending build errors."}},
		})
		return
	}

	allMessages = TruncateMessages(allMessages, s.maxLines)
	text := FormatMessages(allMessages)

	// Prepend summary and warnings for multi-source
	if multiSource {
		var header strings.Builder
		header.WriteString(fmt.Sprintf("[devtap] Draining from %d sources (%d reachable)\n",
			len(s.sources), successCount))
		for _, w := range warnings {
			header.WriteString(w + "\n")
		}
		if len(allMessages) > 0 {
			header.WriteString("\n")
		}
		text = header.String() + text
	}

	s.sendResult(id, CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	})
}

func (s *Server) handleGetBuildStatus(id any) {
	multiSource := len(s.sources) > 1
	totalCount := 0
	var errors []string
	successCount := 0

	for _, src := range s.sources {
		counts, err := src.Store.Status()
		if err != nil {
			if multiSource {
				errors = append(errors, fmt.Sprintf("source %q: %v", src.Label, err))
				continue
			}
			s.sendResult(id, CallToolResult{
				Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error getting status: %v", err)}},
				IsError: true,
			})
			return
		}
		successCount++
		totalCount += counts[src.SessionID]
	}

	// All sources failed — report error, don't mask as "no pending output".
	if successCount == 0 && len(errors) > 0 {
		s.sendResult(id, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error getting status: %s", strings.Join(errors, "; "))}},
			IsError: true,
		})
		return
	}

	var text string
	if totalCount == 0 {
		text = "No pending build output."
	} else {
		text = fmt.Sprintf("Pending build output: %d message(s).", totalCount)
	}
	if len(errors) > 0 {
		text += fmt.Sprintf(" (warning: %s)", strings.Join(errors, "; "))
	}
	s.sendResult(id, CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	})
}

// DedupMessages removes duplicate messages across sources.
// Dedup key: sha256(timestamp_micros + tag + stream + content)[:16].
// Earlier messages (local) take priority over later ones (configured).
func DedupMessages(messages []store.LogMessage) []store.LogMessage {
	seen := make(map[string]struct{})
	var result []store.LogMessage

	for _, msg := range messages {
		key := dedupKey(msg)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, msg)
	}
	return result
}

// dedupKey computes a dedup key for a message.
// Uses the raw tag (which may include source prefix in multi-source mode),
// but strips the "host/label " prefix so messages from different sources
// with the same content match.
func dedupKey(msg store.LogMessage) string {
	tag := msg.Tag
	// Strip "host/label | " prefix if present (e.g. "myhost/local | make" → "make")
	if sep := strings.Index(tag, " | "); sep >= 0 {
		tag = tag[sep+3:]
	}

	content := strings.Join(msg.Lines, "\n")
	raw := strconv.FormatInt(msg.Timestamp.UnixMicro(), 10) + "\x00" + tag + "\x00" + msg.Stream + "\x00" + content
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:8])
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

// FormatMessagesRaw outputs raw lines without [devtap: tag] headers.
// Messages are separated by blank lines.
func FormatMessagesRaw(messages []store.LogMessage) string {
	var sb strings.Builder
	wroteAny := false
	for _, msg := range messages {
		if len(msg.Lines) == 0 {
			continue
		}
		if wroteAny {
			sb.WriteString("\n")
		}
		for _, line := range msg.Lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		wroteAny = true
	}
	return strings.TrimRight(sb.String(), "\n")
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
