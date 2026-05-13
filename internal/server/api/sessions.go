package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	r.POST("/sync", func(c *gin.Context) {
		br.SyncSessions()
		c.JSON(http.StatusOK, gin.H{"status": "synced"})
	})

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
		storeSessions, err := st.ListSessions()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		activeID := br.ActiveSessionID()
		var result []gin.H
		for _, s := range storeSessions {
			status := s.Status
			if s.ID == activeID {
				status = "active"
			}
			name := s.Name
			if name == "" {
				name = s.WorkDir
			}
				created := s.CreatedAt.Format(time.RFC3339)
				modified := s.LastActiveAt.Format(time.RFC3339)
				result = append(result, gin.H{
					"seq":           s.Seq,
					"id":            s.ID,
					"name":          name,
					"work_dir":      s.WorkDir,
					"status":        status,
					"message_count": s.MessageCount,
					"created":       created,
					"modified":      modified,
					"history_path":  s.HistoryPath,
					"git_branch":    s.GitBranch,
					"model":         s.Model,
				})
		}
		if result == nil {
			result = []gin.H{}
		}
		c.JSON(http.StatusOK, result)
	})

	r.GET("/sessions/active", func(c *gin.Context) {
		activeID := br.ActiveSessionID()
		if activeID == "" {
			c.JSON(http.StatusOK, gin.H{"active": nil})
			return
		}
		// Try store first, fall back to JSONL discovery
		if storeSess, _ := st.GetSession(activeID); storeSess != nil {
			c.JSON(http.StatusOK, gin.H{"active": gin.H{
				"id":         storeSess.ID,
				"name":       storeSess.Name,
				"work_dir":   storeSess.WorkDir,
				"status":     storeSess.Status,
				"created":    storeSess.CreatedAt.Format(time.RFC3339),
				"modified":   storeSess.LastActiveAt.Format(time.RFC3339),
				"git_branch": storeSess.GitBranch,
			}})
			return
		}
		hs := claude.FindSession(activeID)
		if hs == nil {
			c.JSON(http.StatusOK, gin.H{"active": nil})
			return
		}
		name := hs.FirstPrompt
		c.JSON(http.StatusOK, gin.H{"active": gin.H{
			"id":         hs.ID,
			"name":       name,
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
		status := "stopped"
		if br.ActiveSessionID() == id {
			status = "active"
		}
		// Try store first (covers new sessions not yet on disk)
		if storeSess, _ := st.GetSession(id); storeSess != nil {
			c.JSON(http.StatusOK, gin.H{
				"id":            storeSess.ID,
				"name":          storeSess.Name,
				"work_dir":      storeSess.WorkDir,
				"status":        status,
				"created":       storeSess.CreatedAt.Format(time.RFC3339),
				"modified":      storeSess.LastActiveAt.Format(time.RFC3339),
				"git_branch":    storeSess.GitBranch,
				"message_count": storeSess.MessageCount,
				"model":         storeSess.Model,
			})
			return
		}
		// Fall back to JSONL discovery
		hs := claude.FindSession(id)
		if hs == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
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
			// Sync latest data from JSONL
			if ss, _ := st.GetSession(id); ss != nil {
				if hs.Model != "" {
					st.UpdateSessionField(id, "model", hs.Model)
				}
				if hs.GitBranch != "" {
					st.UpdateSessionField(id, "git_branch", hs.GitBranch)
				}
				if hs.MessageCount > 0 {
					st.UpdateSessionField(id, "message_count", hs.MessageCount)
				}
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
		id := c.Param("id")
		st.DeleteSession(id)
		// Remove the actual JSONL file from disk
		if hs := claude.FindSession(id); hs != nil {
			os.Remove(hs.FilePath)
			dir := filepath.Dir(hs.FilePath)
			if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
				os.Remove(dir)
			}
		}
		claude.InvalidateSessionCache()
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	})

	r.PATCH("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}
		if err := st.UpdateSessionName(id, req.Name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "renamed"})
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