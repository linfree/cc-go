package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WebPort != 18080 {
		t.Errorf("expected port 18080, got %d", cfg.WebPort)
	}
	if cfg.Language != "zh-CN" {
		t.Errorf("expected zh-CN, got %s", cfg.Language)
	}
}

func TestEnsurePermission(t *testing.T) {
	// permission already present
	types := []string{"permission", "claude_response"}
	result := ensurePermission(types)
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}

	// permission missing
	types2 := []string{"claude_response", "tool_use"}
	result2 := ensurePermission(types2)
	found := false
	for _, r := range result2 {
		if r == "permission" {
			found = true
			break
		}
	}
	if !found {
		t.Error("permission should be forced in push_types")
	}
	if len(result2) != 3 {
		t.Errorf("expected 3 items, got %d", len(result2))
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Override home dir on Windows (uses USERPROFILE) and Unix (uses HOME)
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	cfg := DefaultConfig()
	cfg.WebPort = 9999
	if err := cfg.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.WebPort != 9999 {
		t.Errorf("expected WebPort 9999, got %d", loaded.WebPort)
	}
	if loaded.Language != "zh-CN" {
		t.Errorf("expected zh-CN, got %s", loaded.Language)
	}
}

func TestIsPushEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.IsPushEnabled("permission") {
		t.Error("permission should be enabled by default")
	}
	if cfg.IsPushEnabled("nonexistent") {
		t.Error("nonexistent should not be enabled")
	}
}

func TestConfigDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmpDir, ".cc-go")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}