package wechat

import (
	"log"
	"time"
)

type ReconnectConfig struct {
	SessionDuration time.Duration
	WarningBefore   time.Duration
	ForceBefore     time.Duration
}

var DefaultReconnectConfig = ReconnectConfig{
	SessionDuration: 24 * time.Hour,
	WarningBefore:   2 * time.Hour,
	ForceBefore:     30 * time.Minute,
}

func (c *Client) StartReconnectTimer(cfg ReconnectConfig) {
	go func() {
		for {
			select {
			case <-c.stopCh:
				return
			case <-time.After(1 * time.Minute):
			}

			if c.Status() != StatusConnected {
				continue
			}

			remaining := c.LoginTime().Add(cfg.SessionDuration).Sub(time.Now())

			if remaining <= cfg.ForceBefore {
				log.Println("[reconnect] forcing reconnect...")
				// Signal expiry
				c.SetStatus(StatusExpired)
				continue
			}

			if remaining <= cfg.WarningBefore {
				log.Printf("[reconnect] warning: session expires in %v", remaining)
			}
		}
	}()
}