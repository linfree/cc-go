package main

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/server"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/ui"
	"github.com/linfree/cc-go/internal/wechat"
)

//go:embed cc-go.ico
var appIcon []byte

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	st, err := store.Open()
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	st.ResetActiveSessions()

	if cfg.ClaudeCLIPath == "" && cfg.AutoFindClaude {
		path, err := claude.FindClaudeCLI()
		if err == nil {
			cfg.ClaudeCLIPath = path
			cfg.Save()
		}
	}

	wc := wechat.NewClient(cfg.Wechat.BaseURL, cfg.Wechat.BotToken, wechat.ParseLoginTime(cfg.Wechat.LoginTime), cfg.Wechat.GetCDNBaseUrl())
	if cfg.Wechat.LastFromID != "" {
		wc.SetLastContact(wechat.ContactInfo{FromID: cfg.Wechat.LastFromID, ContextToken: cfg.Wechat.LastContextToken})
	}
	wc.SetContactCallback(func(ci wechat.ContactInfo) {
		cfg.Wechat.LastFromID = ci.FromID
		cfg.Wechat.LastContextToken = ci.ContextToken
		cfg.Save()
	})

	if cfg.Wechat.BotToken != "" {
		wc.Start()
	}

	reconnectCfg := wechat.ReconnectConfig{
		SessionDuration:           24 * time.Hour,
		ActivationWarningHours:    cfg.Wechat.GetActivationWarningHours(),
		ActivationReminderMinutes: cfg.Wechat.GetActivationReminderMinutes(),
		ForceBefore:               30 * time.Minute,
	}
	wc.SetTokenUpdateCallback(func(token, baseURL string) {
		cfg.Wechat.BotToken = token
		if baseURL != "" {
			cfg.Wechat.BaseURL = baseURL
		}
		cfg.Wechat.LoginTime = time.Now().Format(time.RFC3339)
		cfg.Save()
		wc.SetStatus(wechat.StatusConnected)
		wc.StartReconnectTimer(reconnectCfg)
	})
	if cfg.Wechat.BotToken != "" {
		wc.StartReconnectTimer(reconnectCfg)
	}

	br := bridge.New(cfg, st)
	if wc != nil {
		br.SetWechatClient(wc)
		go func() {
			for msg := range wc.Messages() {
				br.HandleWechatMessage(msg)
			}
		}()
	}

	if cfg.AutoResumeLatest {
		sessions, err := st.ListSessions()
		if err == nil && len(sessions) > 0 {
			for _, s := range sessions {
				if s.Status != "active" && s.WorkDir != "" {
					log.Printf("auto-resuming latest session: %s (%s)", s.Name, s.ID)
					if err := br.StartSession(claude.StartOptions{
						WorkDir:  s.WorkDir,
						Name:     s.Name,
						ResumeID: s.ID,
					}); err != nil {
						log.Printf("auto-resume failed: %v", err)
					}
					break
				}
			}
		}
	}

	ln, port := listenWithFallback(cfg.WebPort)
	if port != cfg.WebPort {
		log.Printf("port %d was in use, falling back to port %d", cfg.WebPort, port)
	}
	cfg.ActualPort = port
	defer ln.Close()

	srv := server.New(cfg, st, br, wc)

	RegisterStaticRoutes(srv.Router())

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		br.StopSession()
		if wc != nil {
			wc.Stop()
		}
		os.Exit(0)
	}()

	appUI := ui.New(port, appIcon)

	// onReady: start HTTP server after UI is set up
	appUI.Run(func() {
		if port != cfg.WebPort {
			appUI.ShowMessage("cc-go",
				fmt.Sprintf("端口 %d 已被占用，自动切换到端口 %d", cfg.WebPort, port))
		}
		log.Printf("cc-go server starting on :%d", port)
		go func() {
			if err := srv.Router().RunListener(ln); err != nil {
				log.Printf("server error: %v", err)
			}
		}()
	})

	// UI exited -- clean up
	log.Println("shutting down...")
	done := make(chan struct{}, 1)
	go func() {
		br.StopSession()
		if wc != nil {
			wc.Stop()
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
	}
}

func listenWithFallback(start int) (net.Listener, int) {
	// Try configured port and a few fallbacks
	for port := start; port < start+10; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, port
		}
	}
	// Ultimate fallback: let the OS assign a free port (always works).
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	log.Printf("all ports %d-%d were in use, using OS-assigned port %d", start, start+9, port)
	return ln, port
}