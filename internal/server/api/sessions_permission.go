package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
)

func registerPermissionRoutes(r *gin.RouterGroup, br bridgeResponder) {
	r.GET("/sessions/:id/permissions/pending", func(c *gin.Context) {
		id := c.Param("id")
		if br.ActiveSessionID() != id {
			c.JSON(http.StatusOK, []interface{}{})
			return
		}
		c.JSON(http.StatusOK, br.GetPendingPermissions())
	})

	r.POST("/sessions/:id/permission", func(c *gin.Context) {
		id := c.Param("id")
		if br.ActiveSessionID() != id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session is not active"})
			return
		}
		var req struct {
			RequestID string `json:"request_id"`
			Allow     bool   `json:"allow"`
			Answer    string `json:"answer,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.RequestID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request_id is required"})
			return
		}
		var err error
		if req.Answer != "" {
			err = br.RespondWithAnswer(req.RequestID, req.Answer)
		} else {
			err = br.RespondPermission(req.RequestID, req.Allow)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "responded"})
	})
}

type bridgeResponder interface {
	ActiveSessionID() string
	RespondPermission(requestID string, allow bool) error
	RespondWithAnswer(requestID string, answer string) error
	GetPendingPermissions() []bridge.PendingPermissionInfo
}