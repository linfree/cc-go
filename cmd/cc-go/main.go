package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/server"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

func main() {
	root := &cobra.Command{
		Use:   "cc-go",
		Short: "WeChat bot for remote Claude Code management",
	}

	root.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the cc-go service",
		RunE:  runStart,
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := store.Open()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Reset zombie active sessions from previous runs
	st.ResetActiveSessions()

	needSetupWechat := false
	needSetupClaude := false

	// Check Claude CLI
	if cfg.ClaudeCLIPath == "" && cfg.AutoFindClaude {
		path, err := claude.FindClaudeCLI()
		if err == nil {
			cfg.ClaudeCLIPath = path
			cfg.Save()
		}
	}
	if cfg.ClaudeCLIPath == "" {
		needSetupClaude = true
	}

	// Create WeChat client always (needed for QR code login even without token)
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
	} else {
		needSetupWechat = true
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

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
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
		os.Exit(0)
	}()

	// Open browser
	if cfg.AutoOpenBrowser {
		url := fmt.Sprintf("http://localhost:%d", cfg.WebPort)
		if needSetupWechat {
			url += "/#/wechat"
		} else if needSetupClaude {
			url += "/#/claude"
		}
		openBrowser(url)
	}

	addr := fmt.Sprintf(":%d", cfg.WebPort)
	log.Printf("cc-go server starting on %s", addr)
	return srv.Router().Run(addr)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}