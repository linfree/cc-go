package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/wechat"
)

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}

func registerWechatRoutes(r *gin.RouterGroup, cfg *config.Config, wc *wechat.Client, br *bridge.Bridge) {
	r.GET("/wechat/qrcode", func(c *gin.Context) {
		if wc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat client not initialized"})
			return
		}
		qrcode, qrcodeImg, err := wc.GetQRCode()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		go func() {
			deadline := time.Now().Add(10 * time.Minute)
			for time.Now().Before(deadline) {
				confirmed, token, baseURL, err := wc.CheckQRCodeStatus(qrcode)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				if confirmed {
					wc.SetToken(token, baseURL)
					cfg.Wechat.BotToken = token
					cfg.Wechat.BaseURL = baseURL
					cfg.Wechat.LoginTime = time.Now().Format(time.RFC3339)
					cfg.Save()
					wc.Start()
					return
				}
				time.Sleep(1 * time.Second)
			}
		}()
		c.JSON(http.StatusOK, gin.H{"qrcode": qrcode, "qrcode_img": qrcodeImg})
	})

	r.GET("/wechat/status", func(c *gin.Context) {
		connected := false
		if wc != nil && wc.Status() == wechat.StatusConnected {
			connected = true
		}
		status := "disconnected"
		if wc != nil {
			status = string(wc.Status())
		}
		wxid := cfg.Wechat.BotToken
		if len(wxid) > 16 {
			wxid = wxid[:16]
		}

		resp := gin.H{
			"connected":    connected,
			"status":       status,
			"login_time":   cfg.Wechat.LoginTime,
			"bot_name":     "cc-go",
			"wxid":         wxid,
			"masked_token": maskToken(cfg.Wechat.BotToken),
		}

		if br != nil {
			info := br.GetWechatInfo()
			resp["send_budget"] = info.SendBudget
			resp["buffer_mode"] = info.BufferMode
			resp["buffered_count"] = info.BufferedCount
			resp["last_msg_time"] = info.LastMsgTime
			resp["budget_limit"] = cfg.Wechat.GetSendBudgetLimit()
		}

		if cfg.Wechat.LoginTime != "" {
			loginTime, err := time.Parse(time.RFC3339, cfg.Wechat.LoginTime)
			if err == nil {
				nextReminder := loginTime.Add(time.Duration(cfg.Wechat.GetActivationWarningHours()) * time.Hour)
				resp["next_reminder_time"] = nextReminder.Format(time.RFC3339)
			}
		}

		c.JSON(http.StatusOK, resp)
	})

	r.POST("/wechat/disconnect", func(c *gin.Context) {
		if wc != nil {
			wc.Stop()
		}
		c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
	})

	r.PUT("/wechat/settings", func(c *gin.Context) {
		var req struct {
			SendBudgetLimit           *int    `json:"send_budget_limit"`
			MaxBufferedMessages       *int    `json:"max_buffered_messages"`
			ActivationWarningHours    *int `json:"activation_warning_hours"`
			ActivationReminderMinutes *int `json:"activation_reminder_minutes"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.SendBudgetLimit != nil {
			cfg.Wechat.SendBudgetLimit = *req.SendBudgetLimit
		}
		if req.MaxBufferedMessages != nil {
			cfg.Wechat.MaxBufferedMessages = *req.MaxBufferedMessages
		}
		if req.ActivationWarningHours != nil {
			cfg.Wechat.ActivationWarningHours = *req.ActivationWarningHours
		}
		if req.ActivationReminderMinutes != nil {
			cfg.Wechat.ActivationReminderMinutes = *req.ActivationReminderMinutes
		}
		cfg.Save()
		c.JSON(http.StatusOK, gin.H{"status": "saved"})
	})
}