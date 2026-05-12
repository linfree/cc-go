package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web-dist/*
var webAssets embed.FS

var assetFS http.FileSystem

func RegisterStaticRoutes(r *gin.Engine) {
	sub, err := fs.Sub(webAssets, "web-dist")
	if err != nil {
		return
	}
	assetFS = http.FS(sub)
	fileServer := http.FileServer(assetFS)

	r.GET("/cc-go.png", func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/favicon.svg", func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/assets/*filepath", func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/fonts/*filepath", func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/ws") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		data, err := webAssets.ReadFile("web-dist/index.html")
		if err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}