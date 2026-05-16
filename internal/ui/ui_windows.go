//go:build windows

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazyDLL("user32.dll")
	procFindWindowW = user32.NewProc("FindWindowW")
	procSendMessageW = user32.NewProc("SendMessageW")
	procLoadImageW  = user32.NewProc("LoadImageW")
)

const (
	WM_SETICON      = 0x0080
	ICON_BIG        = 1
	ICON_SMALL      = 0
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x0010
)

type windowsUI struct {
	port  int
	icon  []byte
	quitC chan struct{}

	mu       sync.Mutex
	webview  webview2.WebView
	showReq  chan struct{}
	started  bool
	iconPath string
}

func newPlatformUI(port int, icon []byte) UI {
	u := &windowsUI{
		port:    port,
		icon:    icon,
		quitC:   make(chan struct{}),
		showReq: make(chan struct{}, 1),
	}
	// Write icon to temp file so LoadImageW can load it.
	if len(icon) > 0 {
		tmp := filepath.Join(os.TempDir(), "cc-go-icon.ico")
		if err := os.WriteFile(tmp, icon, 0644); err == nil {
			u.iconPath = tmp
		}
	}
	return u
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

			// Set window icon from the temp ICO file.
			if u.iconPath != "" {
				go setWindowIconFromFile("cc-go", u.iconPath)
			}

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

func openBrowserCmd(url string) *exec.Cmd {
	cmd := exec.Command("cmd", "/c", "start", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

func setWindowIconFromFile(title, icoPath string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	icoPathPtr, _ := windows.UTF16PtrFromString(icoPath)

	for i := 0; i < 50; i++ {
		hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
		if hwnd != 0 {
			hSmall, _, _ := procLoadImageW.Call(
				0, uintptr(unsafe.Pointer(icoPathPtr)),
				IMAGE_ICON, 16, 16, LR_LOADFROMFILE,
			)
			hBig, _, _ := procLoadImageW.Call(
				0, uintptr(unsafe.Pointer(icoPathPtr)),
				IMAGE_ICON, 256, 256, LR_LOADFROMFILE,
			)
			if hSmall != 0 {
				procSendMessageW.Call(hwnd, WM_SETICON, ICON_SMALL, hSmall)
			}
			if hBig != 0 {
				procSendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, hBig)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}