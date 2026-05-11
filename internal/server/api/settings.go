package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/config"
)

func registerPushRoutes(r *gin.RouterGroup, cfg *config.Config) {
	r.GET("/push/types", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"types": []gin.H{
			{"key": "permission", "label": "权限请求", "required": true},
			{"key": "claude_response", "label": "Claude 响应内容", "required": false},
			{"key": "tool_use", "label": "工具调用通知", "required": false},
			{"key": "session_status", "label": "会话状态变更", "required": false},
			{"key": "resource_usage", "label": "资源使用统计", "required": false},
		}})
	})

	r.GET("/push/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"push_types": cfg.PushTypes})
	})

	r.PUT("/push/settings", func(c *gin.Context) {
		var req struct{ PushTypes []string `json:"push_types"` }
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.PushTypes = req.PushTypes
		cfg.Save()
		c.JSON(http.StatusOK, gin.H{"push_types": cfg.PushTypes})
	})
}

func registerSettingsRoutes(r *gin.RouterGroup, cfg *config.Config) {
	r.GET("/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, cfg)
	})

	r.PUT("/settings", func(c *gin.Context) {
		var req struct {
			ClaudeCLIPath   string `json:"claude_cli_path"`
			AutoFindClaude  *bool  `json:"auto_find_claude"`
			PermissionMode  string `json:"permission_mode"`
			Language        string `json:"language"`
			WebPort         int    `json:"web_port"`
			AutoOpenBrowser *bool  `json:"auto_open_browser"`
			ClaudeEnvVars   string `json:"claude_env_vars"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.ClaudeCLIPath != "" {
			cfg.ClaudeCLIPath = req.ClaudeCLIPath
		}
		if req.AutoFindClaude != nil {
			cfg.AutoFindClaude = *req.AutoFindClaude
		}
		if req.PermissionMode != "" {
			cfg.PermissionMode = req.PermissionMode
		}
		if req.Language != "" {
			cfg.Language = req.Language
		}
		if req.WebPort > 0 {
			cfg.WebPort = req.WebPort
		}
		if req.AutoOpenBrowser != nil {
			cfg.AutoOpenBrowser = *req.AutoOpenBrowser
		}
		cfg.ClaudeEnvVars = req.ClaudeEnvVars
		cfg.Save()
		c.JSON(http.StatusOK, cfg)
	})

	r.GET("/bot-commands", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"commands": cfg.BotCommands})
	})

	r.PUT("/bot-commands", func(c *gin.Context) {
		var req struct{ Commands []config.BotCommand `json:"commands"` }
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.BotCommands = req.Commands
		cfg.Save()
		c.JSON(http.StatusOK, gin.H{"commands": cfg.BotCommands})
	})
}