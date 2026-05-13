//go:build windows

package ui

import (
	"fmt"
	"runtime"
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
	showReq chan struct{}
	started bool
}

func newPlatformUI(port int, icon []byte) UI {
	return &windowsUI{
		port:    port,
		icon:    icon,
		quitC:   make(chan struct{}),
		showReq: make(chan struct{}, 1),
	}
}

func (u *windowsUI) Run(onReady func()) {
	systray.Run(func() {
		systray.SetIcon(u.icon)
		systray.SetTitle("cc-go")
		systray.SetTooltip("cc-go - Claude Code remote manager")

		mOpen := systray.AddMenuItem("在浏览器中打开", "在外部浏览器中打开管理面板")
		mShow := systray.AddMenuItem("显示窗口", "显示管理面板窗口")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "停止服务并退出")

		if onReady != nil {
			onReady()
		}

		u.startWebviewLoop()
		u.showWindow()

		url := fmt.Sprintf("http://localhost:%d", u.port)

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mShow.ClickedCh:
					u.showWindow()
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

// startWebviewLoop spawns a single OS-thread-locked goroutine that owns all
// WebView2 windows. Win32 windows and COM objects must stay on their creating
// thread, otherwise the message pump deadlocks.
func (u *windowsUI) startWebviewLoop() {
	u.mu.Lock()
	if u.started {
		u.mu.Unlock()
		return
	}
	u.started = true
	u.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		for range u.showReq {
			w := webview2.New(false)
			if w == nil {
				continue
			}

			u.mu.Lock()
			u.webview = w
			u.mu.Unlock()

			w.SetTitle("cc-go")
			w.SetSize(1400, 900, webview2.HintNone)
			w.Navigate(fmt.Sprintf("http://localhost:%d", u.port))

			// w.Run blocks until the window is destroyed (user clicks X
			// or closeWebview calls w.Destroy from another goroutine).
			w.Run()

			u.mu.Lock()
			u.webview = nil
			u.mu.Unlock()
		}
	}()
}

func (u *windowsUI) showWindow() {
	select {
	case u.showReq <- struct{}{}:
	default:
	}
}

// closeWebview posts WM_CLOSE from the calling goroutine. This is thread-safe
// because Destroy() uses PostMessageW, which can be called from any thread.
func (u *windowsUI) closeWebview() {
	u.mu.Lock()
	w := u.webview
	u.mu.Unlock()
	if w != nil {
		w.Destroy()
	}
}

func (u *windowsUI) Quit() {
	select {
	case u.quitC <- struct{}{}:
	default:
	}
}