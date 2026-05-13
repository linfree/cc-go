//go:build !windows

package ui

import (
	"fmt"
	"os/exec"

	"github.com/getlantern/systray"
)

func openBrowserCmd(url string) *exec.Cmd {
	return nil
}

type otherUI struct {
	port  int
	icon  []byte
	quitC chan struct{}
}

func newPlatformUI(port int, icon []byte) UI {
	return &otherUI{port: port, icon: icon, quitC: make(chan struct{})}
}

func (u *otherUI) Run(onReady func()) {
	systray.Run(func() {
		systray.SetIcon(u.icon)
		systray.SetTitle("cc-go")
		systray.SetTooltip("cc-go - Claude Code remote manager")

		mOpen := systray.AddMenuItem("打开 Web 管理面板", "在浏览器中打开管理面板")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "停止服务并退出")

		// Auto-open browser on start
		url := fmt.Sprintf("http://localhost:%d", u.port)
		openBrowser(url)

		if onReady != nil {
			onReady()
		}

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				case <-u.quitC:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		// onExit — nothing extra needed
	})
}

func (u *otherUI) Quit() {
	select {
	case u.quitC <- struct{}{}:
	default:
	}
}