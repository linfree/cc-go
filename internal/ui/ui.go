package ui

import (
	"os/exec"
	"runtime"
)

// UI wraps platform-specific desktop UI behaviour.
type UI interface {
	// Run starts the UI and blocks until the user requests exit.
	// onReady is called once the UI is set up (e.g. menu items registered).
	Run(onReady func())

	// Quit triggers a programmatic exit.
	Quit()
}

// New returns the platform-appropriate UI implementation.
// port is the web server port; icon is the raw bytes of the app icon (PNG or ICO).
func New(port int, icon []byte) UI {
	return newPlatformUI(port, icon)
}

// openBrowser opens a URL in the default system browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = openBrowserCmd(url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}