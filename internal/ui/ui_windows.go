//go:build windows

package ui

import (
	"fmt"
	"sync"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

type windowsUI struct {
	port  int
	icon  []byte
	quitC chan struct{}

	mu      sync.Mutex
	webview webview2.WebView
}

func newPlatformUI(port int, icon []byte) UI {
	return &windowsUI{
		port:  port,
		icon:  icon,
		quitC: make(chan struct{}),
	}
}

func (u *windowsUI) Run(onReady func()) {
	systray.Run(func() {
		systray.SetIcon(u.icon)
		systray.SetTitle("cc-go")
		systray.SetTooltip("cc-go - Claude Code remote manager")

		mOpen := systray.AddMenuItem("打开 Web 管理面板", "在浏览器中打开管理面板")
		mShow := systray.AddMenuItem("显示窗口", "显示状态窗口")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "停止服务并退出")

		// Show status window on start
		u.showStatusWindow()

		if onReady != nil {
			onReady()
		}

		url := fmt.Sprintf("http://localhost:%d", u.port)

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mShow.ClickedCh:
					u.showStatusWindow()
				case <-mQuit.ClickedCh:
					u.closeWebview()
					systray.Quit()
					return
				case <-u.quitC:
					u.closeWebview()
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		// onExit
	})
}

func (u *windowsUI) showStatusWindow() {
	u.mu.Lock()
	if u.webview != nil {
		u.mu.Unlock()
		return // already showing
	}
	u.mu.Unlock()

	go func() {
		w := webview2.New(false)
		defer func() {
			u.mu.Lock()
			u.webview = nil
			u.mu.Unlock()
			w.Destroy()
		}()

		u.mu.Lock()
		u.webview = w
		u.mu.Unlock()

		w.SetTitle("cc-go")
		w.SetSize(480, 320, webview2.HintFixed)
		w.Navigate(fmt.Sprintf("http://localhost:%d/status", u.port))

		// When the window is closed, just destroy — don't exit process.
		// The systray keeps the process alive.
		w.Run()
	}()
}

func (u *windowsUI) closeWebview() {
	u.mu.Lock()
	w := u.webview
	u.mu.Unlock()
	if w != nil {
		w.Terminate()
	}
}

func (u *windowsUI) Quit() {
	select {
	case u.quitC <- struct{}{}:
	default:
	}
}