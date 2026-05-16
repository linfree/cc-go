package claude

import (
	"testing"
)

func TestFilterOutEnv(t *testing.T) {
	env := []string{"PATH=/usr/bin", "CLAUDECODE=something", "HOME=/home"}
	result := filterOut(env, "CLAUDECODE")
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(result), result)
	}
	for _, e := range result {
		if len(e) >= 10 && e[:10] == "CLAUDECODE" {
			t.Errorf("CLAUDECODE should have been filtered: %s", e)
		}
	}
}

func TestFilterOutEnv_Nonexistent(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/home"}
	result := filterOut(env, "CLAUDECODE")
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestStartSession_InvalidCLI(t *testing.T) {
	_, err := Start(StartOptions{
		CLIPath: "/nonexistent/claude",
		WorkDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error for invalid CLI path")
	}
}

func TestStartSession_ValidCLI(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-session",
		PermMode: "default",
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if sess.Status != StatusActive {
		t.Errorf("expected active, got %s", sess.Status)
	}
	if sess.PID() == 0 {
		t.Error("expected non-zero PID")
	}
	// Clean up
	sess.Stop()
}

func TestSendMessage(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-msg",
		PermMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	err = sess.SendMessage("list files please")
	if err != nil {
		t.Errorf("send failed: %v", err)
	}

	// Wait a bit for response
	select {
	case evt := <-sess.Events():
		t.Logf("Got event type=%s", evt.Type)
		if evt.Type == EventError {
			t.Logf("Error event: %s", evt.Error)
		}
	default:
		t.Log("no immediate event (expected)")
	}
}

func TestProtocolParser_SystemEvent(t *testing.T) {
	raw := map[string]interface{}{
		"type":       "system",
		"subtype":    "init",
		"session_id": "abc-123",
	}
	evt := parseEvent(raw, "")
	if evt.Type != EventSystem {
		t.Errorf("expected system, got %s", evt.Type)
	}
	if evt.SessionID != "abc-123" {
		t.Errorf("expected session abc-123, got %s", evt.SessionID)
	}
}

func TestProtocolParser_AssistantTextEvent(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello world",
				},
			},
		},
	}
	evt := parseEvent(raw, "sid")
	if evt.Text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", evt.Text)
	}
	if len(evt.Content) != 1 || evt.Content[0].Text != "Hello world" {
		t.Errorf("content block mismatch")
	}
}

func TestProtocolParser_ToolUseEvent(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
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
	evt := parseEvent(raw, "sid")
	if len(evt.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(evt.Content))
	}
	if evt.Content[0].Name != "Bash" {
		t.Errorf("expected Bash, got %s", evt.Content[0].Name)
	}
	if evt.Content[0].Input["command"] != "ls -la" {
		t.Errorf("unexpected command: %v", evt.Content[0].Input)
	}
}

func TestProtocolParser_ControlRequestEvent(t *testing.T) {
	raw := map[string]interface{}{
		"type":       "control_request",
		"request_id": "req-001",
		"request": map[string]interface{}{
			"subtype":   "can_use_tool",
			"tool_name": "Bash",
			"input": map[string]interface{}{
				"command": "rm -rf /tmp/test",
			},
		},
	}
	evt := parseEvent(raw, "sid")
	if evt.Type != EventControlRequest {
		t.Errorf("expected control_request, got %s", evt.Type)
	}
	if evt.RequestID != "req-001" {
		t.Errorf("expected req-001, got %s", evt.RequestID)
	}
	if evt.ToolName != "Bash" {
		t.Errorf("expected Bash, got %s", evt.ToolName)
	}
}

func TestProtocolParser_ResultEvent(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "result",
		"subtype":     "success",
		"result":      "The command succeeded",
		"is_error":    false,
		"stop_reason": "end_turn",
		"duration_ms": float64(1234),
		"num_turns":   float64(2),
	}
	evt := parseEvent(raw, "sid")
	if evt.Type != EventResult {
		t.Errorf("expected result, got %s", evt.Type)
	}
	if evt.DurationMs != 1234 {
		t.Errorf("expected 1234ms, got %d", evt.DurationMs)
	}
	if evt.NumTurns != 2 {
		t.Errorf("expected 2 turns, got %d", evt.NumTurns)
	}
	if evt.StopReason != "end_turn" {
		t.Errorf("expected end_turn, got %s", evt.StopReason)
	}
}

func TestRespondPermission_Allow(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-perm",
		PermMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	err = sess.RespondPermission("test-req-id", true, "")
	if err != nil {
		t.Errorf("RespondPermission failed: %v", err)
	}
}

func TestRespondPermission_Deny(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-perm-deny",
		PermMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	err = sess.RespondPermission("test-req-id", false, "Rejected by user")
	if err != nil {
		t.Errorf("RespondPermission failed: %v", err)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusStopped != "stopped" {
		t.Error("StatusStopped should be 'stopped'")
	}
	if StatusActive != "active" {
		t.Error("StatusActive should be 'active'")
	}
}