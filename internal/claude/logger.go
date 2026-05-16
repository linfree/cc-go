package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type LogEntry struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content"`
	Tool    string `json:"tool,omitempty"`
	Input   string `json:"input,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
}

type SessionLogger struct {
	sessionID string
	logPath   string
	file      *os.File
	enc       *json.Encoder
	logCh     chan LogEntry
	closed    bool
	mu        sync.Mutex
	closeOnce sync.Once
}

func NewSessionLogger(sessionID string) (*SessionLogger, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logDir := filepath.Join(home, ".cc-go", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, sessionID+".jsonl")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	sl := &SessionLogger{
		sessionID: sessionID,
		logPath:   logPath,
		file:      f,
		enc:       json.NewEncoder(f),
		logCh:     make(chan LogEntry, 200),
	}
	go sl.writeLoop()
	return sl, nil
}

func (sl *SessionLogger) writeLoop() {
	for entry := range sl.logCh {
		sl.enc.Encode(entry)
	}
}

func (sl *SessionLogger) Log(entry LogEntry) {
	sl.mu.Lock()
	closed := sl.closed
	sl.mu.Unlock()
	if closed {
		return
	}
	select {
	case sl.logCh <- entry:
	default:
	}
}

func (sl *SessionLogger) User(text string) {
	sl.Log(LogEntry{Type: "user", Role: "user", Content: text})
}

func (sl *SessionLogger) Assistant(text string) {
	sl.Log(LogEntry{Type: "assistant", Role: "assistant", Content: text})
}

func (sl *SessionLogger) ToolUse(name, input string) {
	sl.Log(LogEntry{Type: "tool_call", Tool: name, Input: input})
}

func (sl *SessionLogger) Thinking(thinking string) {
	sl.Log(LogEntry{Type: "thinking", Content: thinking})
}

func (sl *SessionLogger) PermissionRequest(tool, input string) {
	sl.Log(LogEntry{Type: "permission", Detail: "request", Tool: tool, Input: input})
}

func (sl *SessionLogger) PermissionResponse(allowed bool) {
	detail := "denied"
	if allowed {
		detail = "approved"
	}
	sl.Log(LogEntry{Type: "permission", Detail: detail})
}

func (sl *SessionLogger) Result(reason string, durationMs int64, numTurns int) {
	sl.Log(LogEntry{Type: "result", Detail: reason})
}

func (sl *SessionLogger) Error(err string) {
	sl.Log(LogEntry{Type: "error", Error: err})
}

func (sl *SessionLogger) Close() {
	sl.mu.Lock()
	sl.closed = true
	sl.mu.Unlock()
	sl.closeOnce.Do(func() {
		close(sl.logCh)
		sl.file.Close()
	})
}

// ReadLog reads the full log file content
func ReadLog(sessionID string) ([]LogEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(home, ".cc-go", "logs", sessionID+".jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []LogEntry
	dec := json.NewDecoder(f)
	for dec.More() {
		var e LogEntry
		if err := dec.Decode(&e); err == nil {
			entries = append(entries, e)
		}
	}
	if entries == nil {
		entries = []LogEntry{}
	}
	return entries, nil
}