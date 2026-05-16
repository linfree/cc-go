package claude

import (
	"testing"
)

func TestDiscoverSessions_RealData(t *testing.T) {
	sessions, err := DiscoverSessions()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found %d sessions", len(sessions))
	for _, s := range sessions {
		t.Logf("  id=%s project=%s messages=%d", s.ID[:8], s.ProjectPath, s.MessageCount)
		if s.ID == "" {
			t.Error("session ID should not be empty")
		}
	}
}

func TestReadHistory_NonExistentFile(t *testing.T) {
	_, err := ReadHistory("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadHistory_RealFile(t *testing.T) {
	sessions, err := DiscoverSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Skip("no sessions to read")
	}
	// Try multiple sessions in case the first one's file no longer exists on disk
	var msgs []HistoryMessage
	var sessionID string
	for _, s := range sessions {
		msgs, err = ReadHistory(s.FilePath)
		if err == nil {
			sessionID = s.ID[:8]
			break
		}
	}
	if err != nil {
		t.Skipf("no readable session files found: %v", err)
	}
	t.Logf("read %d messages from session %s", len(msgs), sessionID)
}

func TestConvertHistoryLine_User(t *testing.T) {
	raw := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": "hello",
		},
		"timestamp": "2026-05-10T12:00:00.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content hello, got %s", msg.Content)
	}
	if msg.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestConvertHistoryLine_Assistant(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello, how can I help?",
				},
			},
		},
		"timestamp": "2026-05-10T12:00:01.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", msg.Role)
	}
	if msg.Content != "Hello, how can I help?" {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestConvertHistoryLine_WithToolUse(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"name": "Bash",
					"input": map[string]interface{}{
						"command": "ls -la",
					},
				},
			},
		},
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.ToolUse == nil {
		t.Fatal("expected ToolUse block")
	}
	if msg.ToolUse.Name != "Bash" {
		t.Errorf("expected Bash, got %s", msg.ToolUse.Name)
	}
	if msg.ToolUse.Input["command"] != "ls -la" {
		t.Errorf("unexpected command: %v", msg.ToolUse.Input)
	}
}

func TestDecodeProjectName_Windows(t *testing.T) {
	result := DecodeProjectName("G--dev-AI-cc-go")
	t.Logf("decoded: %s", result)
	if result == "" {
		t.Error("expected non-empty decode result")
	}
}

func TestClaudeProjectsDir(t *testing.T) {
	dir, err := ClaudeProjectsDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Error("expected non-empty directory")
	}
	t.Logf("projects dir: %s", dir)
}