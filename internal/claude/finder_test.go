package claude

import (
	"runtime"
	"testing"
)

func TestFindClaudeCLI_LookPath(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skipf("claude not found: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	t.Logf("found claude at: %s", path)
}

func TestValidateClaudeCLI_InvalidPath(t *testing.T) {
	_, err := ValidateClaudeCLI("/nonexistent/claude")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestValidateClaudeCLI_ValidPath(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found, skipping validation test")
	}
	version, err := ValidateClaudeCLI(path)
	if err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
	if version == "" {
		t.Error("expected version output")
	}
	t.Logf("claude version: %s", version)
}

func TestCommonPaths_HasEntries(t *testing.T) {
	paths := commonPaths[runtime.GOOS]
	if len(paths) == 0 {
		t.Logf("no common paths for OS: %s", runtime.GOOS)
	}
}