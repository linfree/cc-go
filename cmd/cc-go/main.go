package main

import (
	_ "embed"
	"fmt"
	"log"
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

	wc := wechat.NewClient(cfg.Wechat.BaseURL, cfg.Wechat.BotToken, wechat.ParseLoginTime(cfg.Wechat.LoginTime))
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
		wc.StartReconnectTimer(wechat.DefaultReconnectConfig)
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

	srv := server.New(cfg, st, br, wc)

	RegisterStaticRoutes(srv.Router())
	RegisterStatusRoute(srv.Router(), cfg.WebPort)

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

	appUI := ui.New(cfg.WebPort, appIcon)

	// onReady: start HTTP server after UI is set up
	appUI.Run(func() {
		addr := fmt.Sprintf(":%d", cfg.WebPort)
		log.Printf("cc-go server starting on %s", addr)
		go func() {
			if err := srv.Router().Run(addr); err != nil {
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