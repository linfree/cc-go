package bridge

import (
	"testing"

	"github.com/linfree/cc-go/internal/config"
)

func TestSplitLongMessage_Short(t *testing.T) {
	result := splitLongMessage("hello", 3500)
	if len(result) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(result))
	}
	if result[0] != "hello" {
		t.Errorf("expected 'hello', got '%s'", result[0])
	}
}

func TestSplitLongMessage_Long(t *testing.T) {
	longText := ""
	for i := 0; i < 4000; i++ {
		longText += "a"
	}
	result := splitLongMessage(longText, 3500)
	if len(result) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(result))
	}
	// Check that [1/2] and [2/2] markers are present
	if result[0][len(result[0])-5:] != "[1/2]" {
		t.Errorf("expected [1/2] marker in first chunk, got: %s", result[0][len(result[0])-10:])
	}
	if result[1][len(result[1])-5:] != "[2/2]" {
		t.Errorf("expected [2/2] marker in second chunk, got: %s", result[1][len(result[1])-10:])
	}
}

func TestTruncateInput_Short(t *testing.T) {
	result := truncateInput(map[string]interface{}{"command": "ls -la"})
	if result != "ls -la" {
		t.Errorf("expected 'ls -la', got '%s'", result)
	}
}

func TestTruncateInput_Long(t *testing.T) {
	longCmd := ""
	for i := 0; i < 300; i++ {
		longCmd += "a"
	}
	result := truncateInput(map[string]interface{}{"command": longCmd})
	if len(result) > 203 {
		t.Errorf("expected truncated output, got %d chars", len(result))
	}
}

func TestTruncateInput_NoCommand(t *testing.T) {
	result := truncateInput(map[string]interface{}{"key": "value"})
	if result == "" {
		t.Error("expected non-empty output")
	}
}

func TestNewBridge(t *testing.T) {
	cfg := config.DefaultConfig()
	b := New(cfg, nil)
	if b == nil {
		t.Fatal("expected non-nil bridge")
	}
	if b.ActiveSessionID() != "" {
		t.Error("expected no active session")
	}
}

func TestWSEvent(t *testing.T) {
	cfg := config.DefaultConfig()
	b := New(cfg, nil)

	// Emit and receive
	b.emit(WSEvent{Event: "test", SessionID: "sid"})
	select {
	case evt := <-b.EventBus():
		wsEvt, ok := evt.(WSEvent)
		if !ok {
			t.Error("expected WSEvent")
		}
		if wsEvt.Event != "test" {
			t.Errorf("expected 'test', got '%s'", wsEvt.Event)
		}
	default:
		t.Error("expected event in bus")
	}
}