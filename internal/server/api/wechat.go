package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/wechat"
)

func registerWechatRoutes(r *gin.RouterGroup, cfg *config.Config, wc *wechat.Client) {
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
		c.JSON(http.StatusOK, gin.H{
			"connected":  connected,
			"status":     status,
			"login_time": cfg.Wechat.LoginTime,
			"bot_name":   "cc-go",
			"wxid":       wxid,
		})
	})

	r.POST("/wechat/disconnect", func(c *gin.Context) {
		if wc != nil {
			wc.Stop()
		}
		c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
	})
}