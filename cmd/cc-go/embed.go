package main

import (
	"embed"
	"fmt"
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

func RegisterStatusRoute(r *gin.Engine, port int) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>cc-go</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #0f172a; color: #e2e8f0;
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; margin: 0;
  }
  .card {
    text-align: center; padding: 48px 32px;
    background: #1e293b; border-radius: 16px;
    box-shadow: 0 4px 24px rgba(0,0,0,0.3);
    max-width: 380px; width: 90%%;
  }
  .status-dot {
    display: inline-block; width: 12px; height: 12px;
    background: #22c55e; border-radius: 50%%;
    animation: pulse 2s infinite;
    margin-right: 8px; vertical-align: middle;
  }
  @keyframes pulse {
    0%%, 100%% { opacity: 1; transform: scale(1); }
    50%% { opacity: 0.5; transform: scale(1.3); }
  }
  h1 { font-size: 20px; font-weight: 600; margin-bottom: 8px; }
  .subtitle { color: #94a3b8; font-size: 13px; margin-bottom: 32px; }
  .btn {
    display: inline-block; padding: 12px 32px;
    background: #6366f1; color: #fff;
    border: none; border-radius: 8px;
    font-size: 15px; font-weight: 500; cursor: pointer;
    text-decoration: none; transition: background 0.2s;
  }
  .btn:hover { background: #4f46e5; }
  .footer { margin-top: 24px; color: #64748b; font-size: 12px; }
</style>
</head>
<body>
  <div class="card">
    <h1><span class="status-dot"></span>cc-go 运行中</h1>
    <p class="subtitle">Web 管理面板端口: %d</p>
    <a class="btn" href="#" onclick="window.open('http://localhost:%d','_blank');return false;">打开 Web 管理面板</a>
    <p class="footer">关闭此窗口不会停止服务</p>
  </div>
</body>
</html>`, port, port)
	r.GET("/status", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(html))
	})
}