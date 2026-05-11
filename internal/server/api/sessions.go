package api

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/store"
)

func registerClaudeRoutes(r *gin.RouterGroup, st *store.Store, br *bridge.Bridge) {
	r.GET("/claude/path", func(c *gin.Context) {
		path, err := claude.FindClaudeCLI()
		c.JSON(http.StatusOK, gin.H{"path": path, "error": errToStr(err)})
	})

	r.POST("/claude/path", func(c *gin.Context) {
		var req struct{ Path string `json:"path"` }
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		version, err := claude.ValidateClaudeCLI(req.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"path": req.Path, "version": version})
	})

	r.POST("/claude/auto-detect", func(c *gin.Context) {
		path, err := claude.FindClaudeCLI()
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "claude CLI not found"})
			return
		}
		version, err := claude.ValidateClaudeCLI(path)
		c.JSON(http.StatusOK, gin.H{"path": path, "version": version, "valid": err == nil})
	})
}

var startTime = time.Now()

func registerSessionRoutes(r *gin.RouterGroup, st *store.Store, br *bridge.Bridge) {
	r.GET("/stats", func(c *gin.Context) {
		historySessions, _ := claude.DiscoverSessions()
		activeID := br.ActiveSessionID()

		activeCount := 0
		totalMessages := 0
		for _, hs := range historySessions {
			totalMessages += hs.MessageCount
			if hs.ID == activeID {
				activeCount++
			}
		}

		uptime := time.Since(startTime)
		days := int(uptime.Hours()) / 24
		hours := int(uptime.Hours()) % 24
		minutes := int(uptime.Minutes()) % 60
		uptimeStr := ""
		if days > 0 {
			uptimeStr += fmt.Sprintf("%dd ", days)
		}
		uptimeStr += fmt.Sprintf("%dh %dm", hours, minutes)

		c.JSON(http.StatusOK, gin.H{
			"active_sessions": activeCount,
			"total_sessions":  len(historySessions),
			"total_messages":  totalMessages,
			"uptime":          uptimeStr,
			"version":         "v1.0.0",
		})
	})

	r.GET("/sessions", func(c *gin.Context) {
		historySessions, _ := claude.DiscoverSessions()
		activeID := br.ActiveSessionID()
		var result []gin.H
		for _, hs := range historySessions {
			status := "stopped"
			if hs.ID == activeID {
				status = "active"
			}
			// Try to get model from store
			storeSess, _ := st.GetSession(hs.ID)
			model := ""
			if storeSess != nil {
				model = storeSess.Model
			}
			result = append(result, gin.H{
				"id":            hs.ID,
				"name":          hs.FirstPrompt,
				"work_dir":      hs.ProjectPath,
				"status":        status,
				"message_count": hs.MessageCount,
				"created":       hs.Created,
				"modified":      hs.Modified,
				"history_path":  hs.FilePath,
				"git_branch":    hs.GitBranch,
				"model":         model,
			})
		}
		if result == nil {
			result = []gin.H{}
		}
		sort.Slice(result, func(i, j int) bool {
			return result[i]["modified"].(string) > result[j]["modified"].(string)
		})
		c.JSON(http.StatusOK, result)
	})

	r.GET("/sessions/active", func(c *gin.Context) {
		activeID := br.ActiveSessionID()
		if activeID == "" {
			c.JSON(http.StatusOK, gin.H{"active": nil})
			return
		}
		hs := claude.FindSession(activeID)
		if hs == nil {
			c.JSON(http.StatusOK, gin.H{"active": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"active": gin.H{
			"id":         hs.ID,
			"name":       hs.FirstPrompt,
			"work_dir":   hs.ProjectPath,
			"created":    hs.Created,
			"modified":   hs.Modified,
			"git_branch": hs.GitBranch,
		}})
	})

	r.GET("/sessions/active/log", func(c *gin.Context) {
		activeID := br.ActiveSessionID()
		if activeID == "" {
			c.JSON(http.StatusOK, []claude.LogEntry{})
			return
		}
		entries, err := claude.ReadLog(activeID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, entries)
	})

	r.GET("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")
		hs := claude.FindSession(id)
		if hs == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		status := "stopped"
		if br.ActiveSessionID() == id {
			status = "active"
		}
		c.JSON(http.StatusOK, gin.H{
			"id":         hs.ID,
			"name":       hs.FirstPrompt,
			"work_dir":   hs.ProjectPath,
			"status":     status,
			"created":    hs.Created,
			"modified":   hs.Modified,
			"git_branch": hs.GitBranch,
		})
	})

	r.GET("/sessions/:id/history", func(c *gin.Context) {
		id := c.Param("id")
		hs := claude.FindSession(id)
		if hs == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "history file not found"})
			return
		}
		msgs, err := claude.ReadHistory(hs.FilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, msgs)
	})

	r.POST("/sessions/start", func(c *gin.Context) {
		var req struct {
			WorkDir string `json:"work_dir"`
			Model   string `json:"model"`
			Name    string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := checkWorkDir(req.WorkDir); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := br.StartSession(claude.StartOptions{WorkDir: req.WorkDir, Model: req.Model, Name: req.Name}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "started"})
	})

	r.POST("/sessions/:id/resume", func(c *gin.Context) {
		id := c.Param("id")
		hs := claude.FindSession(id)
		if hs == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		if err := checkWorkDir(hs.ProjectPath); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "工作目录不存在或无法访问: " + hs.ProjectPath})
			return
		}
		if err := br.StartSession(claude.StartOptions{WorkDir: hs.ProjectPath, Name: hs.FirstPrompt, ResumeID: id}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
	})

	r.POST("/sessions/:id/stop", func(c *gin.Context) {
		br.StopSession()
		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	})

	r.POST("/sessions/:id/message", func(c *gin.Context) {
		id := c.Param("id")
		if br.ActiveSessionID() != id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session is not active"})
			return
		}
		var req struct{ Content string `json:"content"` }
		if err := c.ShouldBindJSON(&req); err != nil || req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
			return
		}
		if err := br.SendUserMessage(req.Content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "sent"})
	})

	r.DELETE("/sessions/:id", func(c *gin.Context) {
		st.DeleteSession(c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	})
}

func checkWorkDir(dir string) error {
	if dir == "" {
		return os.ErrNotExist
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrNotExist
	}
	return nil
}

func errToStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}