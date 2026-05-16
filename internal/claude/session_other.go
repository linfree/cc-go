//go:build !windows

package claude

import "os/exec"

func setHideWindow(cmd *exec.Cmd) {}