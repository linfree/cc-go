//go:build linux

package ui

import (
	"fmt"
	"log"
	"os/exec"
)

func openBrowserCmd(url string) *exec.Cmd {
	return nil
}

type linuxUI struct {
	port  int
	icon  []byte
	quitC chan struct{}
}

func newPlatformUI(port int, icon []byte) UI {
	return &linuxUI{port: port, icon: icon, quitC: make(chan struct{})}
}

func (u *linuxUI) Run(onReady func()) {
	url := fmt.Sprintf("http://localhost:%d", u.port)
	log.Printf("Running in headless mode, web UI at %s", url)
	openBrowser(url)
	if onReady != nil {
		onReady()
	}
	<-u.quitC
}

func (u *linuxUI) ShowMessage(title, message string) {
	log.Printf("[%s] %s", title, message)
}

func (u *linuxUI) Quit() {
	select {
	case u.quitC <- struct{}{}:
	default:
	}
}
