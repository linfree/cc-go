package wechat

import (
	"testing"
	"time"
)

func TestReconnectConfig_Defaults(t *testing.T) {
	cfg := DefaultReconnectConfig
	if cfg.SessionDuration != 24*time.Hour {
		t.Error("expected 24h session duration")
	}
	if cfg.WarningBefore != 2*time.Hour {
		t.Error("expected 2h warning")
	}
	if cfg.ForceBefore != 30*time.Minute {
		t.Error("expected 30min force")
	}
}

func TestReconnectTimer_DoesNotCrash(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now())
	c.Start()
	c.SetStatus(StatusConnected)
	// Short timer for quick test
	cfg := DefaultReconnectConfig
	cfg.SessionDuration = 1 * time.Second
	cfg.WarningBefore = 500 * time.Millisecond
	cfg.ForceBefore = 200 * time.Millisecond
	c.StartReconnectTimer(cfg)
	time.Sleep(2 * time.Second)
	c.Stop()
	// Test passes if no panic — doesn't verify functional behavior (needs real WeChat connection)
}

func TestReconnectTimer_RespectsDisconnected(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now())
	// Don't call c.Start() — stay disconnected, timer should not act
	cfg := DefaultReconnectConfig
	cfg.SessionDuration = 10 * time.Millisecond
	cfg.WarningBefore = 5 * time.Millisecond
	cfg.ForceBefore = 2 * time.Millisecond
	c.StartReconnectTimer(cfg)
	time.Sleep(100 * time.Millisecond)
	// Should still be disconnected
	if c.Status() != StatusDisconnected {
		t.Errorf("expected disconnected, got %s", c.Status())
	}
	c.stopCh <- struct{}{}
}