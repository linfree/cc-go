package wechat

import (
	"fmt"
	"log"
	"time"
)

type ReconnectConfig struct {
	SessionDuration           time.Duration
	ActivationWarningHours    int
	ActivationReminderMinutes int
	ForceBefore               time.Duration
}

var DefaultReconnectConfig = ReconnectConfig{
	SessionDuration:           24 * time.Hour,
	ActivationWarningHours:    20,
	ActivationReminderMinutes: 60,
	ForceBefore:               30 * time.Minute,
}

func (c *Client) StartReconnectTimer(cfg ReconnectConfig) {
	// Stop any existing reconnect goroutine
	c.mu.Lock()
	if c.reconnectStopCh != nil {
		close(c.reconnectStopCh)
	}
	c.reconnectStopCh = make(chan struct{})
	stopCh := c.reconnectStopCh
	c.mu.Unlock()

	go func() {
		var qrcode string
		var lastReminder time.Time
		var qrPollStop chan struct{}
		activationStarted := false

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopCh:
				stopQRPoll(qrPollStop)
				return
			case <-stopCh:
				stopQRPoll(qrPollStop)
				return
			case <-ticker.C:
			}

			if c.Status() != StatusConnected {
				continue
			}

			loginTime := c.LoginTime()
			elapsed := time.Since(loginTime)
			remaining := loginTime.Add(cfg.SessionDuration).Sub(time.Now())

			if remaining <= cfg.ForceBefore {
				log.Println("[reconnect] forcing reconnect, session expired")
				c.SetStatus(StatusExpired)
				stopQRPoll(qrPollStop)
				qrPollStop = nil
				activationStarted = false
				continue
			}

			if elapsed >= time.Duration(cfg.ActivationWarningHours)*time.Hour {
				if !activationStarted {
					activationStarted = true
					var err error
					qrcode, _, err = c.GetQRCode()
					if err != nil {
						log.Printf("[reconnect] failed to get qrcode: %v", err)
						activationStarted = false
						continue
					}
					lastReminder = time.Now()
					c.sendActivationReminder(qrcode)
					qrPollStop = make(chan struct{})
					go c.pollQRCodeConfirmation(&qrcode, qrPollStop, stopCh)
				} else if time.Since(lastReminder) >= time.Duration(cfg.ActivationReminderMinutes)*time.Minute {
					lastReminder = time.Now()
					c.sendActivationReminder(qrcode)
				}
			}
		}
	}()
}

func (c *Client) sendActivationReminder(qrcode string) {
	ct := c.LastContact()
	if ct.FromID == "" {
		log.Println("[reconnect] no last contact, cannot send reminder")
		return
	}
	text := fmt.Sprintf(
		"### 登录提醒\n\n[重新点击激活机器人](https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=%s&bot_type=3)",
		qrcode,
	)
	if err := c.SendMessage(ct.FromID, ct.ContextToken, text); err != nil {
		log.Printf("[reconnect] failed to send activation reminder: %v", err)
	}
}

func (c *Client) pollQRCodeConfirmation(qrcode *string, pollStop, reconnectStop chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollStop:
			return
		case <-reconnectStop:
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
		}

		confirmed, token, baseURL, err := c.CheckQRCodeStatus(*qrcode)
		if err != nil {
			continue
		}
		if confirmed {
			c.SetToken(token, baseURL)
			log.Println("[reconnect] qrcode confirmed, token updated")
			c.mu.RLock()
			cb := c.onTokenUpdate
			c.mu.RUnlock()
			if cb != nil {
				cb(token, baseURL)
			}
			return
		}

		// If QR code expired, re-fetch
		// The API may return status="expired" — CheckQRCodeStatus currently doesn't expose
		// the status string, so we re-fetch proactively every 5 min
	}
}

func (c *Client) TriggerRelogin() error {
	qrcode, _, err := c.GetQRCode()
	if err != nil {
		return err
	}
	c.sendActivationReminder(qrcode)
	go c.pollQRCodeConfirmation(&qrcode, make(chan struct{}), nil)
	return nil
}

func stopQRPoll(ch chan struct{}) {
	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}