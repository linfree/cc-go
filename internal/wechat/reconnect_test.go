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
	if cfg.ActivationWarningHours != 20 {
		t.Error("expected 20h activation warning")
	}
	if cfg.ActivationReminderMinutes != 60 {
		t.Error("expected 60min reminder")
	}
	if cfg.ForceBefore != 30*time.Minute {
		t.Error("expected 30min force")
	}
}

func TestReconnectTimer_DoesNotCrash(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now())
	c.Start()
	c.SetStatus(StatusConnected)
	cfg := DefaultReconnectConfig
	cfg.SessionDuration = 1 * time.Second
	cfg.ActivationWarningHours = 0
	cfg.ForceBefore = 200 * time.Millisecond
	c.StartReconnectTimer(cfg)
	time.Sleep(2 * time.Second)
	c.Stop()
}

func TestReconnectTimer_RespectsDisconnected(t *testing.T) {
	c := NewClient(DefaultBaseURL, "", time.Now())
	cfg := DefaultReconnectConfig
	cfg.SessionDuration = 10 * time.Millisecond
	cfg.ActivationWarningHours = 0
	cfg.ForceBefore = 2 * time.Millisecond
	c.StartReconnectTimer(cfg)
	time.Sleep(100 * time.Millisecond)
	if c.Status() != StatusDisconnected {
		t.Errorf("expected disconnected, got %s", c.Status())
	}
	c.stopCh <- struct{}{}
}