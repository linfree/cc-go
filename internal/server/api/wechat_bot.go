package api

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/wechat"
)

func registerWechatBotRoutes(r *gin.RouterGroup, wc *wechat.Client) {
	bot := r.Group("/wechat-bot")

	bot.GET("/status", func(c *gin.Context) {
		connected := wc != nil && wc.Status() == wechat.StatusConnected
		ct := wc.LastContact()
		c.JSON(http.StatusOK, gin.H{
			"connected":            connected,
			"default_to_user_id":   ct.FromID,
			"default_context_token": ct.ContextToken,
		})
	})

	bot.POST("/send/text", func(c *gin.Context) {
		if wc == nil || wc.Status() != wechat.StatusConnected {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat bot not connected"})
			return
		}
		var req sendTextRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Text == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
			return
		}
		toID, contextToken, err := resolveTarget(wc, req.ToUserID, req.ContextToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := wc.SendMessage(toID, contextToken, req.Text); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "sent", "to_user_id": toID})
	})

	bot.POST("/send/image", sendMediaHandler(wc, "image", func(wc *wechat.Client, toID, ctxToken, filePath string) error {
		return wc.SendImage(toID, ctxToken, filePath)
	}))

	bot.POST("/send/file", sendMediaHandler(wc, "file", func(wc *wechat.Client, toID, ctxToken, filePath string) error {
		return wc.SendFile(toID, ctxToken, filePath)
	}))

	bot.POST("/send/video", sendMediaHandler(wc, "video", func(wc *wechat.Client, toID, ctxToken, filePath string) error {
		return wc.SendVideo(toID, ctxToken, filePath)
	}))
}

type sendTextRequest struct {
	ToUserID     string `json:"to_user_id,omitempty"`
	ContextToken string `json:"context_token,omitempty"`
	Text         string `json:"text"`
}

type sendMediaRequest struct {
	ToUserID     string `json:"to_user_id,omitempty"`
	ContextToken string `json:"context_token,omitempty"`
	FilePath     string `json:"file_path"`
}

func resolveTarget(wc *wechat.Client, reqToID, reqContextToken string) (string, string, error) {
	if reqToID != "" && reqContextToken != "" {
		return reqToID, reqContextToken, nil
	}
	ct := wc.LastContact()
	if ct.FromID == "" {
		return "", "", fmt.Errorf("no target specified and no last contact available")
	}
	toID := reqToID
	if toID == "" {
		toID = ct.FromID
	}
	contextToken := reqContextToken
	if contextToken == "" {
		contextToken = ct.ContextToken
	}
	return toID, contextToken, nil
}

func sendMediaHandler(wc *wechat.Client, mediaType string, sendFn func(*wechat.Client, string, string, string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if wc == nil || wc.Status() != wechat.StatusConnected {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat bot not connected"})
			return
		}
		var req sendMediaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.FilePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file_path is required"})
			return
		}
		info, err := os.Stat(req.FilePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file not found or inaccessible: " + req.FilePath})
			return
		}
		toID, ctxToken, err := resolveTarget(wc, req.ToUserID, req.ContextToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := sendFn(wc, toID, ctxToken, req.FilePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":      "sent",
			"to_user_id":  toID,
			"media_type":  mediaType,
			"file_size":   info.Size(),
		})
	}
}