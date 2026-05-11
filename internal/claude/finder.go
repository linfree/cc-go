package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var commonPaths = map[string][]string{
	"windows": {
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Claude", "claude.exe"),
		filepath.Join(os.Getenv("APPDATA"), "npm", "claude.cmd"),
		filepath.Join(os.Getenv("ProgramFiles"), "Claude", "claude.exe"),
	},
	"darwin": {
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
	},
	"linux": {
		"/usr/local/bin/claude",
		"/usr/bin/claude",
		filepath.Join(os.Getenv("HOME"), ".local/bin/claude"),
		filepath.Join(os.Getenv("HOME"), ".npm-global/bin/claude"),
	},
}

func FindClaudeCLI() (string, error) {
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	paths := commonPaths[runtime.GOOS]
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func ValidateClaudeCLI(path string) (string, error) {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}