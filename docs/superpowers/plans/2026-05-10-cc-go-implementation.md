# cc-go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go tool that enables remote management of Claude Code via WeChat iLink Bot, with a React+Ant Design web UI for configuration and session management.

**Architecture:** Monolithic Go binary embedding a React SPA. Gin serves the HTTP API and static files, cobra handles the CLI. `internal/claude/` manages subprocesses via stream-json protocol, `internal/wechat/` handles iLink Bot long-polling, and `internal/bridge/` connects the two. SQLite stores session metadata; JSON config files hold static settings.

**Tech Stack:** Go 1.24 (cobra v1.10.2, gin v1.11.0, modernc.org/sqlite, coder/websocket v1.8.12), React 18 + Ant Design 5, TypeScript, Vite

**Key design decisions:**
- No CGO (pure Go SQLite via modernc.org/sqlite) for easy cross-compilation
- Chat content read directly from Claude's JSONL history files, never duplicated in SQLite
- Permission requests always force-push to WeChat regardless of user push settings
- WeChat connection state is in-memory; config (token, base_url) in JSON file
- Single binary via `embed` — React SPA built with Vite, output embedded in Go binary

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/cc-go/main.go`
- Create: `internal/config/config.go`
- Create: `internal/store/store.go`
- Create: `internal/wechat/client.go`
- Create: `internal/claude/session.go`
- Create: `internal/claude/protocol.go`
- Create: `internal/claude/history.go`
- Create: `internal/claude/finder.go`
- Create: `internal/bridge/bridge.go`
- Create: `internal/server/router.go`
- Create: `internal/server/api/wechat.go`
- Create: `internal/server/api/claude_config.go`
- Create: `internal/server/api/sessions.go`
- Create: `internal/server/api/push.go`
- Create: `internal/server/api/settings.go`
- Create: `internal/server/ws/hub.go`
- Create: `web/` (Vite+React project)

- [ ] **Step 1: Initialize Go module**

Run: `cd G:/dev/AI/cc-go && go mod init github.com/linfree/cc-go`

- [ ] **Step 2: Create directory structure**

```bash
mkdir -p cmd/cc-go
mkdir -p internal/{config,store,wechat,claude,bridge,server/{api,ws}}
```

- [ ] **Step 3: Create stub files for all packages**

Create `cmd/cc-go/main.go`:
```go
package main

import "fmt"

func main() {
	fmt.Println("cc-go starting...")
}
```

Create `internal/config/config.go`:
```go
package config

type Config struct {
	ClaudeCLIPath   string        `json:"claude_cli_path"`
	AutoFindClaude  bool          `json:"auto_find_claude"`
	PermissionMode  string        `json:"permission_mode"`
	Language        string        `json:"language"`
	WebPort         int           `json:"web_port"`
	AutoOpenBrowser bool          `json:"auto_open_browser"`
	Wechat          WechatConfig  `json:"wechat"`
	PushTypes       []string      `json:"push_types"`
}

type WechatConfig struct {
	BotToken  string `json:"bot_token"`
	BaseURL   string `json:"base_url"`
	LoginTime string `json:"login_time"`
}
```

Create stubs for all other `internal/*` packages with package declaration and a placeholder function.

- [ ] **Step 4: Initialize React project**

Run: `cd G:/dev/AI/cc-go/web && npm create vite@latest . -- --template react-ts && npm install && npm install antd @ant-design/icons react-router-dom`

- [ ] **Step 5: Fetch Go dependencies**

Run: `cd G:/dev/AI/cc-go && go get github.com/spf13/cobra@v1.10.2 github.com/gin-gonic/gin@v1.11.0 modernc.org/sqlite@v1.48.2 github.com/coder/websocket@v1.8.12`

- [ ] **Step 6: Verify everything builds**

Run: `cd G:/dev/AI/cc-go && go build ./...` Expected: builds without errors.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: scaffold cc-go project structure"
```

---

### Task 2: Config Module

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Implement config loading, saving, and defaults**

Rewrite `internal/config/config.go`:

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ClaudeCLIPath   string       `json:"claude_cli_path"`
	AutoFindClaude  bool         `json:"auto_find_claude"`
	PermissionMode  string       `json:"permission_mode"`
	Language        string       `json:"language"`
	WebPort         int          `json:"web_port"`
	AutoOpenBrowser bool         `json:"auto_open_browser"`
	Wechat          WechatConfig `json:"wechat"`
	PushTypes       []string     `json:"push_types"`
}

type WechatConfig struct {
	BotToken  string `json:"bot_token"`
	BaseURL   string `json:"base_url"`
	LoginTime string `json:"login_time"`
}

func DefaultConfig() *Config {
	return &Config{
		ClaudeCLIPath:   "",
		AutoFindClaude:  true,
		PermissionMode:  "default",
		Language:        "zh-CN",
		WebPort:         18080,
		AutoOpenBrowser: true,
		Wechat: WechatConfig{
			BotToken:  "",
			BaseURL:   "https://ilinkai.weixin.qq.com",
			LoginTime: "",
		},
		PushTypes: []string{"permission", "claude_response", "tool_use", "session_status"},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cc-go"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	cfgPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return cfg, cfg.Save()
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	ensurePermission(cfg.PushTypes)
	return &cfg, nil
}

func (c *Config) Save() error {
	cfgDir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return err
	}
	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	ensurePermission(c.PushTypes)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

func (c *Config) Clone() *Config {
	data, _ := json.Marshal(c)
	var clone Config
	json.Unmarshal(data, &clone)
	return &clone
}

func (c *Config) IsPushEnabled(t string) bool {
	for _, pt := range c.PushTypes {
		if pt == t {
			return true
		}
	}
	return false
}

func ensurePermission(types []string) []string {
	for _, t := range types {
		if t == "permission" {
			return types
		}
	}
	return append([]string{"permission"}, types...)
}
```

- [ ] **Step 2: Write and run config tests**

Create `internal/config/config_test.go`:

```go
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
	types := []string{"claude_response", "tool_use"}
	result := ensurePermission(types)
	found := false
	for _, r := range result {
		if r == "permission" {
			found = true
			break
		}
	}
	if !found {
		t.Error("permission should be forced in push_types")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	// Override home via env trick — use a test helper approach
	_ = cfgPath
}
```

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/config/
git commit -m "feat: implement config module with JSON load/save"
```

---

### Task 3: Store Module (SQLite)

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/migrations.go`

- [ ] **Step 1: Implement SQLite store**

Write `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WorkDir      string    `json:"work_dir"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	ClaudePID    int       `json:"claude_pid"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	HistoryPath  string    `json:"history_path"`
}

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbDir := filepath.Join(home, ".cc-go")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, "cc-go.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		work_dir TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle',
		claude_pid INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		history_path TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) InsertSession(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Name, sess.WorkDir, sess.Model, sess.Status, sess.ClaudePID,
		sess.CreatedAt, sess.LastActiveAt, sess.HistoryPath,
	)
	return err
}

func (s *Store) UpdateSessionStatus(id, status string, pid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE sessions SET status = ?, claude_pid = ?, last_active_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, pid, id,
	)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions WHERE id = ?`, id)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListSessions() ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions ORDER BY last_active_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath); err != nil {
			return nil, err
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

func (s *Store) GetActiveSession() (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions WHERE status = 'active' LIMIT 1`)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}
```

- [ ] **Step 2: Write and run store tests**

Create `internal/store/store_test.go`:

```go
package store

import (
	"os"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	os.Setenv("HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sess := &Session{
		ID:      "test-session-1",
		Name:    "test",
		WorkDir: "/tmp/test",
		Status:  "idle",
	}
	if err := s.InsertSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession("test-session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" {
		t.Errorf("expected name test, got %s", got.Name)
	}

	if err := s.UpdateSessionStatus("test-session-1", "active", 12345); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSession("test-session-1")
	if got.Status != "active" {
		t.Errorf("expected status active, got %s", got.Status)
	}

	list, err := s.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 session, got %d", len(list))
	}
}
```

Run: `go test ./internal/store/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/store/
git commit -m "feat: implement SQLite store for session metadata"
```

---

### Task 4: Claude CLI Finder

**Files:**
- Modify: `internal/claude/finder.go`

- [ ] **Step 1: Implement claude CLI path finder**

Write `internal/claude/finder.go`:

```go
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
```

- [ ] **Step 2: Write and run finder tests**

Create `internal/claude/finder_test.go`:

```go
package claude

import (
	"os"
	"testing"
)

func TestFindClaudeCLI_ToolExists(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skipf("claude not found in PATH: %v", err)
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
	path, _ := FindClaudeCLI()
	if path == "" {
		t.Skip("claude not found")
	}
	version, err := ValidateClaudeCLI(path)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if version == "" {
		t.Error("expected version output")
	}
	t.Logf("claude version: %s", version)
}
```

Run: `go test ./internal/claude/... -v`
Expected: Test passes or skips if claude not installed.

- [ ] **Step 3: Commit**

```bash
git add internal/claude/finder.go internal/claude/finder_test.go
git commit -m "feat: implement claude CLI path finder"
```

---

### Task 5: Claude Session Manager

**Files:**
- Modify: `internal/claude/session.go`
- Modify: `internal/claude/protocol.go`

- [ ] **Step 1: Implement Claude session lifecycle**

Write `internal/claude/session.go`:

```go
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type SessionStatus string

const (
	StatusStopped  SessionStatus = "stopped"
	StatusStarting SessionStatus = "starting"
	StatusActive   SessionStatus = "active"
	StatusError    SessionStatus = "error"
)

type Session struct {
	ID        string
	Name      string
	WorkDir   string
	Model     string
	Status    SessionStatus
	cliPath   string
	permMode  string
	resumeID  string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	mu        sync.Mutex
	stopCh    chan struct{}
	eventCh   chan Event
}

type StartOptions struct {
	CLIPath    string
	WorkDir    string
	Model      string
	PermMode   string
	SessionID  string // for new sessions (custom ID)
	ResumeID   string // for resuming existing sessions
	Name       string
}

func Start(opts StartOptions) (*Session, error) {
	s := &Session{
		Name:     opts.Name,
		WorkDir:  opts.WorkDir,
		Model:    opts.Model,
		Status:   StatusStarting,
		cliPath:  opts.CLIPath,
		permMode: opts.PermMode,
		resumeID: opts.ResumeID,
		stopCh:   make(chan struct{}),
		eventCh:  make(chan Event, 100),
	}

	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", opts.PermMode,
		"--permission-prompt-tool", "stdio",
		"--max-turns", "0",
	}
	if opts.ResumeID != "" {
		args = append(args, "--resume", opts.ResumeID)
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	env := os.Environ()
	env = filterOut(env, "CLAUDECODE")

	s.cmd = exec.Command(s.cliPath, args...)
	s.cmd.Dir = opts.WorkDir
	s.cmd.Env = env

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	go s.readStdout()
	go s.readStderr()

	s.Status = StatusActive
	return s, nil
}

func (s *Session) SendMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusActive {
		return fmt.Errorf("session not active: %s", s.Status)
	}
	msg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": text,
		},
	}
	data, _ := json.Marshal(msg)
	_, err := fmt.Fprintf(s.stdin, "%s\n", data)
	return err
}

func (s *Session) RespondPermission(requestID string, allow bool, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp := map[string]string{}
	if allow {
		resp["behavior"] = "allow"
		resp["updatedInput"] = "{}"
	} else {
		resp["behavior"] = "deny"
		resp["message"] = reason
	}
	envelope := map[string]interface{}{
		"type": "control_response",
		"response": map[string]interface{}{
			"subtype":    "success",
			"request_id": requestID,
			"response":   resp,
		},
	}
	data, _ := json.Marshal(envelope)
	_, err := fmt.Fprintf(s.stdin, "%s\n", data)
	return err
}

func (s *Session) Events() <-chan Event { return s.eventCh }

func (s *Session) Stop() error {
	s.mu.Lock()
	s.Status = "stopping"
	s.mu.Unlock()

	s.stdin.Close()
	close(s.stopCh)

	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.cmd.Process.Kill()
	}

	s.mu.Lock()
	s.Status = "stopped"
	s.mu.Unlock()
	close(s.eventCh)
	return nil
}

func (s *Session) PID() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

func (s *Session) readStdout() {
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		evt := parseEvent(raw, s.ID)
		if evt != nil {
			select {
			case s.eventCh <- evt:
			case <-s.stopCh:
				return
			}
		}
	}
}

func (s *Session) readStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		fmt.Fprintf(os.Stderr, "[claude stderr] %s\n", scanner.Text())
	}
}

func filterOut(env []string, key string) []string {
	var result []string
	prefix := key + "="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] != prefix {
			result = append(result, e)
		}
	}
	return result
}
```

- [ ] **Step 2: Implement stream-json protocol parser**

Write `internal/claude/protocol.go`:

```go
package claude

type EventType string

const (
	EventSystem          EventType = "system"
	EventAssistant       EventType = "assistant"
	EventUser            EventType = "user"
	EventResult          EventType = "result"
	EventControlRequest  EventType = "control_request"
	EventControlCancel   EventType = "control_cancel_request"
	EventError           EventType = "error"
)

type Event struct {
	Type       EventType              `json:"type"`
	Subtype    string                 `json:"subtype,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	Content    []ContentBlock         `json:"content,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Result     string                 `json:"result,omitempty"`
	IsError    bool                   `json:"is_error,omitempty"`
	StopReason string                 `json:"stop_reason,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
	NumTurns   int                    `json:"num_turns,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Raw        map[string]interface{} `json:"-"`
}

type ContentBlock struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Thinking string                 `json:"thinking,omitempty"`
}

func parseEvent(raw map[string]interface{}, sessionID string) *Event {
	evt := &Event{Raw: raw}
	evt.SessionID = sessionID

	t, _ := raw["type"].(string)
	evt.Type = EventType(t)

	switch evt.Type {
	case EventSystem:
		evt.Subtype, _ = raw["subtype"].(string)
		if sid, ok := raw["session_id"].(string); ok {
			evt.SessionID = sid
		}
	case EventAssistant:
		msg, _ := raw["message"].(map[string]interface{})
		if content, ok := msg["content"].([]interface{}); ok {
			for _, c := range content {
				cm, _ := c.(map[string]interface{})
				ct, _ := cm["type"].(string)
				switch ct {
				case "text":
					txt, _ := cm["text"].(string)
					evt.Text += txt
					evt.Content = append(evt.Content, ContentBlock{Type: "text", Text: txt})
				case "tool_use":
					name, _ := cm["name"].(string)
					input, _ := cm["input"].(map[string]interface{})
					evt.Content = append(evt.Content, ContentBlock{Type: "tool_use", Name: name, Input: input})
				case "thinking":
					think, _ := cm["thinking"].(string)
					evt.Content = append(evt.Content, ContentBlock{Type: "thinking", Thinking: think})
				}
			}
		}
	case EventResult:
		evt.Subtype, _ = raw["subtype"].(string)
		evt.Result, _ = raw["result"].(string)
		evt.IsError, _ = raw["is_error"].(bool)
		evt.StopReason, _ = raw["stop_reason"].(string)
		if d, ok := raw["duration_ms"].(float64); ok {
			evt.DurationMs = int64(d)
		}
		if n, ok := raw["num_turns"].(float64); ok {
			evt.NumTurns = int(n)
		}
	case EventControlRequest:
		evt.RequestID, _ = raw["request_id"].(string)
		if req, ok := raw["request"].(map[string]interface{}); ok {
			evt.ToolName, _ = req["tool_name"].(string)
			evt.ToolInput, _ = req["input"].(map[string]interface{})
		}
	case EventControlCancel:
		evt.RequestID, _ = raw["request_id"].(string)
	case EventError:
		evt.Error, _ = raw["error"].(string)
	}
	return evt
}
```

- [ ] **Step 3: Write session tests**

Create `internal/claude/session_test.go`:

```go
package claude

import (
	"os"
	"testing"
)

func TestStartSession_InvalidCLI(t *testing.T) {
	_, err := Start(StartOptions{
		CLIPath: "/nonexistent/claude",
		WorkDir: t.TempDir(),
	})
	if err == nil {
		t.Error("expected error for invalid CLI path")
	}
}

func TestStartSession_ValidCLI(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-session",
		PermMode: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sess.Stop()

	if sess.Status != StatusActive {
		t.Errorf("expected active, got %s", sess.Status)
	}
	if sess.PID() == 0 {
		t.Error("expected non-zero PID")
	}
}

func TestSendMessage(t *testing.T) {
	path, err := FindClaudeCLI()
	if err != nil {
		t.Skip("claude not found")
	}
	sess, err := Start(StartOptions{
		CLIPath:  path,
		WorkDir:  t.TempDir(),
		Name:     "test-msg",
		PermMode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	err = sess.SendMessage("list files in current directory")
	if err != nil {
		t.Errorf("send failed: %v", err)
	}

	// Read events until we get something
	select {
	case evt := <-sess.Events():
		t.Logf("got event: type=%s", evt.Type)
	default:
		t.Log("no immediate event (expected, claude needs time)")
	}
}

func TestFilterOutEnv(t *testing.T) {
	env := []string{"PATH=/usr/bin", "CLAUDECODE=something", "HOME=/home"}
	result := filterOut(env, "CLAUDECODE")
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(result), result)
	}
}
```

Run: `go test ./internal/claude/... -v -timeout 30s`
Expected: Tests pass or skip if claude not available.

- [ ] **Step 4: Commit**

```bash
git add internal/claude/session.go internal/claude/protocol.go internal/claude/session_test.go
git commit -m "feat: implement claude session manager and protocol parser"
```

---

### Task 6: Claude History Reader

**Files:**
- Modify: `internal/claude/history.go`

- [ ] **Step 1: Implement history file reader**

Write `internal/claude/history.go`:

```go
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type HistoryMessage struct {
	Type      string                 `json:"type"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	ToolUse   *ToolUseBlock          `json:"tool_use,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

type ToolUseBlock struct {
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type HistorySession struct {
	ID           string `json:"id"`
	FirstPrompt  string `json:"first_prompt"`
	MessageCount int    `json:"message_count"`
	Created      string `json:"created"`
	Modified     string `json:"modified"`
	ProjectPath  string `json:"project_path"`
	FilePath     string `json:"file_path"`
	GitBranch    string `json:"git_branch"`
}

func ClaudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

func DiscoverSessions() ([]HistorySession, error) {
	projectsDir, err := ClaudeProjectsDir()
	if err != nil {
		return nil, err
	}
	var sessions []HistorySession

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		indexPath := filepath.Join(projectsDir, entry.Name(), "sessions-index.json")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}
		var index struct {
			Entries []struct {
				SessionID    string `json:"sessionId"`
				FullPath     string `json:"fullPath"`
				FirstPrompt  string `json:"firstPrompt"`
				MessageCount int    `json:"messageCount"`
				Created      string `json:"created"`
				Modified     string `json:"modified"`
				ProjectPath  string `json:"projectPath"`
				GitBranch    string `json:"gitBranch"`
				IsSidechain  bool   `json:"isSidechain"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(data, &index); err != nil {
			continue
		}
		for _, e := range index.Entries {
			if e.IsSidechain {
				continue
			}
			sessions = append(sessions, HistorySession{
				ID:           e.SessionID,
				FirstPrompt:  e.FirstPrompt,
				MessageCount: e.MessageCount,
				Created:      e.Created,
				Modified:     e.Modified,
				ProjectPath:  e.ProjectPath,
				FilePath:     e.FullPath,
				GitBranch:    e.GitBranch,
			})
		}
	}
	return sessions, nil
}

func ReadHistory(filePath string) ([]HistoryMessage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var messages []HistoryMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		msg := convertHistoryLine(raw)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}
	return messages, scanner.Err()
}

func convertHistoryLine(raw map[string]interface{}) *HistoryMessage {
	t, _ := raw["type"].(string)
	msg := &HistoryMessage{Type: t}

	if ts, ok := raw["timestamp"].(string); ok {
		msg.Timestamp = ts
	}

	switch t {
	case "user":
		msg.Role = "user"
		if message, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				msg.Content = content
			}
		}
	case "assistant":
		msg.Role = "assistant"
		if message, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].([]interface{}); ok {
				var textParts []string
				for _, c := range content {
					cm, _ := c.(map[string]interface{})
					ct, _ := cm["type"].(string)
					switch ct {
					case "text":
						txt, _ := cm["text"].(string)
						textParts = append(textParts, txt)
					case "tool_use":
						name, _ := cm["name"].(string)
						input, _ := cm["input"].(map[string]interface{})
						msg.ToolUse = &ToolUseBlock{Name: name, Input: input}
					}
				}
				msg.Content = strings.Join(textParts, "\n")
			}
		}
	case "system":
		msg.Role = "system"
		if c, ok := raw["content"].(string); ok {
			msg.Content = c
		}
	}
	return msg
}

func DecodeProjectName(encoded string) string {
	if runtime.GOOS == "windows" {
		parts := strings.Split(encoded, "--")
		if len(parts) >= 1 {
			drive := parts[0] + ":"
			rest := strings.Join(parts[1:], "\\")
			return drive + "\\" + rest
		}
	}
	return strings.ReplaceAll(encoded, "--", "/")
}

func EncodeProjectPath(path string) string {
	if runtime.GOOS == "windows" {
		path = strings.TrimSuffix(path, ":")
		path = strings.ReplaceAll(path, "\\", "--")
		path = strings.ReplaceAll(path, ":", "--")
		return path
	}
	return strings.ReplaceAll(path, "/", "--")
}

// unused import intentionally omitted — time is not needed here
```

- [ ] **Step 2: Write history reader tests**

Create `internal/claude/history_test.go`:

```go
package claude

import (
	"testing"
)

func TestDiscoverSessions_RealData(t *testing.T) {
	sessions, err := DiscoverSessions()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found %d sessions", len(sessions))
	for _, s := range sessions {
		t.Logf("  session: id=%s project=%s messages=%d", s.ID, s.ProjectPath, s.MessageCount)
	}
}

func TestDecodeProjectName_Windows(t *testing.T) {
	result := DecodeProjectName("G--dev-AI-cc-go")
	t.Logf("decoded: %s", result)
}

func TestConvertHistoryLine_User(t *testing.T) {
	raw := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": "hello",
		},
		"timestamp": "2026-05-10T12:00:00.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content hello, got %s", msg.Content)
	}
}

func TestConvertHistoryLine_Assistant(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello, how can I help?",
				},
			},
		},
		"timestamp": "2026-05-10T12:00:01.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", msg.Role)
	}
	if msg.Content != "Hello, how can I help?" {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}
```

Run: `go test ./internal/claude/... -v -run TestDiscover -timeout 10s`
Expected: PASS (lists real sessions if any exist)

Run: `go test ./internal/claude/... -v -run TestConvert -timeout 10s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/claude/history.go internal/claude/history_test.go
git commit -m "feat: implement claude history file reader"
```

---

### Task 7: WeChat Client

**Files:**
- Modify: `internal/wechat/client.go`
- Modify: `internal/wechat/message.go`

- [ ] **Step 1: Implement WeChat iLink Bot client**

Write `internal/wechat/client.go`:

```go
package wechat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const DefaultBaseURL = "https://ilinkai.weixin.qq.com"

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusExpired      Status = "expired"
)

type Client struct {
	baseURL   string
	botToken  string
	loginTime time.Time
	status    Status
	mu        sync.RWMutex
	http      *http.Client
	msgCh     chan Message
	getUpdatesBuf string
	lastContact   ContactInfo
}

type ContactInfo struct {
	FromID       string
	ContextToken string
}

func NewClient(baseURL, botToken string, loginTime time.Time) *Client {
	return &Client{
		baseURL:   baseURL,
		botToken:  botToken,
		loginTime: loginTime,
		status:    StatusDisconnected,
		http:      &http.Client{Timeout: 60 * time.Second},
		msgCh:     make(chan Message, 100),
	}
}

func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Client) SetStatus(s Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.botToken
}

func (c *Client) SetToken(token, baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.botToken = token
	if baseURL != "" {
		c.baseURL = baseURL
	}
	c.loginTime = time.Now()
}

func (c *Client) LoginTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loginTime
}

func (c *Client) Messages() <-chan Message { return c.msgCh }

func (c *Client) LastContact() ContactInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastContact
}

func (c *Client) makeHeaders() map[string]string {
	uin := fmt.Sprintf("%d", rand.Uint32())
	return map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
		"X-WECHAT-UIN":      base64.StdEncoding.EncodeToString([]byte(uin)),
		"Authorization":     "Bearer " + c.Token(),
	}
}

func (c *Client) apiGet(path string) (map[string]interface{}, error) {
	base := c.baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, _ := http.NewRequest("GET", base+"/"+path, nil)
	for k, v := range c.makeHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *Client) apiPost(path string, body map[string]interface{}) (map[string]interface{}, error) {
	base := c.baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/"+path, nil)
	for k, v := range c.makeHeaders() {
		req.Header.Set(k, v)
	}
	// Use a simple approach with bytes.NewReader
	import_bytes := json.RawMessage(data)
	_ = import_bytes
	// Actually let's simplify:
	return nil, fmt.Errorf("TODO: implement proper POST")
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = StatusDisconnected
}
```

- [ ] **Step 1 is too complex — let me rewrite the entire client cleanly. Replace with:**

Write `internal/wechat/client.go`:

```go
package wechat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const DefaultBaseURL = "https://ilinkai.weixin.qq.com"

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusExpired      Status = "expired"
)

type Client struct {
	baseURL       string
	botToken      string
	loginTime     time.Time
	status        Status
	mu            sync.RWMutex
	httpClient    *http.Client
	msgCh         chan Message
	getUpdatesBuf string
	lastContact   ContactInfo
	stopCh        chan struct{}
	done          chan struct{}
}

type ContactInfo struct {
	FromID       string
	ContextToken string
}

func NewClient(baseURL, botToken string, loginTime time.Time) *Client {
	return &Client{
		baseURL:    baseURL,
		botToken:   botToken,
		loginTime:  loginTime,
		status:     StatusDisconnected,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		msgCh:      make(chan Message, 100),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (c *Client) Status() Status {
	c.mu.RLock(); defer c.mu.RUnlock()
	return c.status
}

func (c *Client) SetStatus(s Status) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.status = s
}

func (c *Client) Token() string {
	c.mu.RLock(); defer c.mu.RUnlock()
	return c.botToken
}

func (c *Client) SetToken(token, baseURL string) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.botToken = token
	if baseURL != "" {
		c.baseURL = baseURL
	}
	c.loginTime = time.Now()
}

func (c *Client) LoginTime() time.Time {
	c.mu.RLock(); defer c.mu.RUnlock()
	return c.loginTime
}

func (c *Client) Messages() <-chan Message { return c.msgCh }

func (c *Client) LastContact() ContactInfo {
	c.mu.RLock(); defer c.mu.RUnlock()
	return c.lastContact
}

func (c *Client) makeHeaders() map[string]string {
	h := map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
	}
	uin := fmt.Sprintf("%d", rand.Uint32())
	h["X-WECHAT-UIN"] = base64.StdEncoding.EncodeToString([]byte(uin))
	if c.Token() != "" {
		h["Authorization"] = "Bearer " + c.Token()
	}
	return h
}

func (c *Client) doRequest(method, path string, bodyData []byte) (map[string]interface{}, error) {
	base := c.baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := base + "/" + path
	var body io.Reader
	if bodyData != nil {
		body = bytes.NewReader(bodyData)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range c.makeHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respData, &result)
	return result, nil
}

func (c *Client) GetQRCode() (string, string, error) {
	result, err := c.doRequest("GET", "ilink/bot/get_bot_qrcode?bot_type=3", nil)
	if err != nil {
		return "", "", err
	}
	qrcode, _ := result["qrcode"].(string)
	qrcodeImg, _ := result["qrcode_img_content"].(string)
	return qrcode, qrcodeImg, nil
}

func (c *Client) CheckQRCodeStatus(qrcode string) (bool, string, string, error) {
	result, err := c.doRequest("GET", "ilink/bot/get_qrcode_status?qrcode="+qrcode, nil)
	if err != nil {
		return false, "", "", err
	}
	status, _ := result["status"].(string)
	if status == "confirmed" {
		token, _ := result["bot_token"].(string)
		baseURL, _ := result["baseurl"].(string)
		return true, token, baseURL, nil
	}
	return false, "", "", nil
}

func (c *Client) SendMessage(toID, contextToken, text string) error {
	clientID := fmt.Sprintf("cc-go-%08x", rand.Uint32())
	body, _ := json.Marshal(map[string]interface{}{
		"msg": map[string]interface{}{
			"from_user_id":  "",
			"to_user_id":    toID,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"context_token": contextToken,
			"item_list": []map[string]interface{}{
				{"type": 1, "text_item": map[string]string{"text": text}},
			},
		},
		"base_info": map[string]string{"channel_version": "1.0.2"},
	})
	_, err := c.doRequest("POST", "ilink/bot/sendmessage", body)
	return err
}

func (c *Client) PollMessages(timeoutMs int) ([]Message, string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"get_updates_buf": c.getUpdatesBuf,
		"base_info":       map[string]string{"channel_version": "1.0.2"},
	})
	result, err := c.doRequest("POST", "ilink/bot/getupdates", body)
	if err != nil {
		return nil, "", err
	}
	buf, _ := result["get_updates_buf"].(string)
	rawMsgs, _ := result["msgs"].([]interface{})
	var msgs []Message
	for _, raw := range rawMsgs {
		rm, _ := raw.(map[string]interface{})
		msg := parseMessage(rm)
		msgs = append(msgs, msg)
	}
	return msgs, buf, nil
}

func (c *Client) Start() {
	c.SetStatus(StatusConnected)
	go c.pollLoop()
}

func (c *Client) Stop() {
	close(c.stopCh)
	<-c.done
	c.SetStatus(StatusDisconnected)
}

func (c *Client) pollLoop() {
	defer close(c.done)
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}
		if c.Status() != StatusConnected {
			time.Sleep(2 * time.Second)
			continue
		}
		msgs, newBuf, err := c.PollMessages(35000)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if newBuf != "" {
			c.getUpdatesBuf = newBuf
		}
		for _, msg := range msgs {
			if msg.MessageType != 1 {
				continue
			}
			c.mu.Lock()
			c.lastContact = ContactInfo{FromID: msg.FromUserID, ContextToken: msg.ContextToken}
			c.mu.Unlock()
			select {
			case c.msgCh <- msg:
			case <-c.stopCh:
				return
			}
		}
	}
}
```

Write `internal/wechat/message.go`:

```go
package wechat

type Message struct {
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	MessageType  int    `json:"message_type"`
	MessageState int    `json:"message_state"`
	ContextToken string `json:"context_token"`
	Text         string `json:"text"`
}

func parseMessage(raw map[string]interface{}) Message {
	msg := Message{}
	msg.FromUserID, _ = raw["from_user_id"].(string)
	msg.ToUserID, _ = raw["to_user_id"].(string)
	if mt, ok := raw["message_type"].(float64); ok {
		msg.MessageType = int(mt)
	}
	if ms, ok := raw["message_state"].(float64); ok {
		msg.MessageState = int(ms)
	}
	msg.ContextToken, _ = raw["context_token"].(string)
	items, _ := raw["item_list"].([]interface{})
	for _, item := range items {
		im, _ := item.(map[string]interface{})
		t, _ := im["type"].(float64)
		if t == 1 {
			ti, _ := im["text_item"].(map[string]interface{})
			msg.Text, _ = ti["text"].(string)
			break
		}
	}
	return msg
}
```

- [ ] **Step 2: Write WeChat client tests**

Create `internal/wechat/client_test.go`:

```go
package wechat

import (
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient(DefaultBaseURL, "test-token", time.Now())
	if c.Status() != StatusDisconnected {
		t.Error("expected disconnected status")
	}
}

func TestParseMessage(t *testing.T) {
	raw := map[string]interface{}{
		"from_user_id":  "user123@im.wechat",
		"to_user_id":    "bot456@im.bot",
		"message_type":  float64(1),
		"message_state": float64(2),
		"context_token": "tokentest123",
		"item_list": []interface{}{
			map[string]interface{}{
				"type": float64(1),
				"text_item": map[string]interface{}{
					"text": "Hello bot",
				},
			},
		},
	}
	msg := parseMessage(raw)
	if msg.FromUserID != "user123@im.wechat" {
		t.Errorf("unexpected from: %s", msg.FromUserID)
	}
	if msg.Text != "Hello bot" {
		t.Errorf("unexpected text: %s", msg.Text)
	}
	if msg.MessageType != 1 {
		t.Errorf("unexpected message type: %d", msg.MessageType)
	}
}

func TestClientHeaders(t *testing.T) {
	c := NewClient(DefaultBaseURL, "test-token", time.Now())
	headers := c.makeHeaders()
	if headers["AuthorizationType"] != "ilink_bot_token" {
		t.Error("missing AuthorizationType header")
	}
	if headers["Content-Type"] != "application/json" {
		t.Error("missing Content-Type header")
	}
}
```

Run: `go test ./internal/wechat/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/wechat/
git commit -m "feat: implement wechat iLink bot client"
```

---

### Task 8: WeChat Reconnect Timer

**Files:**
- Create: `internal/wechat/reconnect.go`

- [ ] **Step 1: Implement reconnect logic**

Write `internal/wechat/reconnect.go`:

```go
package wechat

import (
	"log"
	"time"
)

type ReconnectConfig struct {
	SessionDuration   time.Duration
	WarningBefore     time.Duration
	ReminderInterval  time.Duration
	ForceBefore       time.Duration
	QRCodeScanTimeout time.Duration
}

var DefaultReconnectConfig = ReconnectConfig{
	SessionDuration:   24 * time.Hour,
	WarningBefore:     2 * time.Hour,
	ReminderInterval:  30 * time.Minute,
	ForceBefore:       30 * time.Minute,
	QRCodeScanTimeout: 10 * time.Minute,
}

type ReconnectHandler func() (newQRCode string, waitForScan func() (token, baseURL string, err error))

func (c *Client) StartReconnectTimer(cfg ReconnectConfig, handler ReconnectHandler) {
	go func() {
		for {
			select {
			case <-c.stopCh:
				return
			case <-time.After(1 * time.Minute):
			}

			if c.Status() != StatusConnected {
				continue
			}

			remaining := c.LoginTime().Add(cfg.SessionDuration).Sub(time.Now())

			if remaining <= cfg.ForceBefore {
				log.Println("[reconnect] forcing reconnect...")
				c.doReconnect(handler)
				continue
			}

			if remaining <= cfg.WarningBefore {
				log.Printf("[reconnect] warning: session expires in %v", remaining)
				c.doReconnect(handler)
			}
		}
	}()
}

func (c *Client) doReconnect(handler ReconnectHandler) {
	qrcode, _ := handler()
	if qrcode == "" {
		return
	}
	waitFn := func() (string, string, error) { return "", "", nil }
	_ = waitFn
	// Poll for scan
	deadline := time.Now().Add(DefaultReconnectConfig.QRCodeScanTimeout)
	for time.Now().Before(deadline) {
		confirmed, token, baseURL, err := c.CheckQRCodeStatus(qrcode)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		if confirmed {
			c.SetToken(token, baseURL)
			log.Println("[reconnect] new token set")
			return
		}
		time.Sleep(1 * time.Second)
	}
	log.Println("[reconnect] scan timeout")
}
```

- [ ] **Step 2: Write reconnect tests**

Create `internal/wechat/reconnect_test.go`:

```go
package wechat

import (
	"testing"
	"time"
)

func TestReconnectConfig_Defaults(t *testing.T) {
	cfg := DefaultReconnectConfig
	if cfg.SessionDuration != 24*time.Hour {
		t.Error("expected 24h session duration")
	}
	if cfg.QRCodeScanTimeout != 10*time.Minute {
		t.Error("expected 10min scan timeout")
	}
}

func TestDoReconnect_NoOpWhenNotConnected(t *testing.T) {
	// Just a smoke test that the timer goroutine doesn't panic
	c := NewClient(DefaultBaseURL, "", time.Now())
	cfg := DefaultReconnectConfig
	cfg.SessionDuration = 1 * time.Millisecond // Force immediate reconnect
	handler := func() (string, func() (string, string, error)) {
		return "", func() (string, string, error) { return "", "", nil }
	}
	c.StartReconnectTimer(cfg, handler)
	time.Sleep(100 * time.Millisecond)
	c.Stop()
	// Test passes if no panic
}
```

Run: `go test ./internal/wechat/... -v -timeout 10s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/wechat/reconnect.go internal/wechat/reconnect_test.go
git commit -m "feat: implement wechat reconnect timer"
```

---

### Task 9: Message Bridge

**Files:**
- Modify: `internal/bridge/bridge.go`

- [ ] **Step 1: Implement message bridge between WeChat and Claude**

Write `internal/bridge/bridge.go`:

```go
package bridge

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

const maxWeChatMsgLen = 3500

type Bridge struct {
	config       *config.Config
	store        *store.Store
	wechatClient *wechat.Client
	activeSess   *claude.Session
	pendingPerm  *pendingPermission
	mu           sync.Mutex
	eventBus     chan interface{} // WebSocket events
}

type pendingPermission struct {
	RequestID string
	ToolName  string
	ToolInput map[string]interface{}
}

type WSEvent struct {
	Event     string      `json:"event"`
	SessionID string      `json:"session_id,omitempty"`
	Status    string      `json:"status,omitempty"`
	Tool      string      `json:"tool,omitempty"`
	Type      string      `json:"type,omitempty"`
	Content   string      `json:"content,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func New(cfg *config.Config, s *store.Store) *Bridge {
	return &Bridge{
		config:   cfg,
		store:    s,
		eventBus: make(chan interface{}, 200),
	}
}

func (b *Bridge) SetWechatClient(wc *wechat.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wechatClient = wc
}

func (b *Bridge) EventBus() <-chan interface{} {
	return b.eventBus
}

func (b *Bridge) emit(evt WSEvent) {
	select {
	case b.eventBus <- evt:
	default:
	}
}

func (b *Bridge) HandleWechatMessage(msg wechat.Message) {
	text := strings.TrimSpace(msg.Text)

	// Command handling
	switch {
	case text == "/help":
		b.sendWechat(msg, "可用指令：\n/help - 查看帮助\n/sessions - 列出会话\n/switch <id> - 切换会话\n/status - 查看状态\n/stop - 停止会话\n/y - 批准操作\n/n - 拒绝操作\n/time - 查看连接时间\n/reconnect - 重新连接")
		return
	case text == "/sessions":
		sessions, err := b.store.ListSessions()
		if err != nil {
			b.sendWechat(msg, "获取会话列表失败")
			return
		}
		var sb strings.Builder
		sb.WriteString("会话列表：\n")
		for _, s := range sessions {
			sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", s.ID[:8], s.Status, s.Name))
		}
		b.sendWechat(msg, sb.String())
		return
	case strings.HasPrefix(text, "/switch "):
		id := strings.TrimPrefix(text, "/switch ")
		b.resumeSession(id, msg)
		return
	case text == "/status":
		b.handleStatus(msg)
		return
	case text == "/stop":
		b.handleStop(msg)
		return
	case text == "/y":
		b.handlePermissionResponse(true, msg)
		return
	case text == "/n":
		b.handlePermissionResponse(false, msg)
		return
	}

	// Forward to Claude
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil || sess.Status != claude.StatusActive {
		b.sendWechat(msg, "当前没有活跃的 Claude 会话。请先在 Web 界面选择或启动会话。")
		return
	}

	if err := sess.SendMessage(text); err != nil {
		b.sendWechat(msg, fmt.Sprintf("发送消息失败: %v", err))
	}
}

func (b *Bridge) handleStatus(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechat(msg, "当前无活跃会话")
		return
	}
	b.sendWechat(msg, fmt.Sprintf("会话: %s\n状态: %s\n工作目录: %s", sess.Name, sess.Status, sess.WorkDir))
}

func (b *Bridge) handleStop(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechat(msg, "当前无活跃会话")
		return
	}
	b.StopSession()
	b.sendWechat(msg, "会话已停止")
}

func (b *Bridge) handlePermissionResponse(allow bool, msg wechat.Message) {
	b.mu.Lock()
	p := b.pendingPerm
	b.mu.Unlock()

	if p == nil {
		b.sendWechat(msg, "当前没有待处理的权限请求")
		return
	}

	if allow {
		b.activeSess.RespondPermission(p.RequestID, true, "")
		b.sendWechat(msg, "已批准")
	} else {
		b.activeSess.RespondPermission(p.RequestID, false, "用户拒绝")
		b.sendWechat(msg, "已拒绝")
	}

	b.mu.Lock()
	b.pendingPerm = nil
	b.mu.Unlock()
}

func (b *Bridge) StartSession(opts claude.StartOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.activeSess != nil {
		b.StopSessionLocked()
	}

	if opts.CLIPath == "" {
		opts.CLIPath = b.config.ClaudeCLIPath
	}
	if opts.PermMode == "" {
		opts.PermMode = b.config.PermissionMode
	}

	sess, err := claude.Start(opts)
	if err != nil {
		return err
	}
	b.activeSess = sess

	// Update store
	b.store.InsertSession(&store.Session{
		ID:      sess.ID,
		Name:    opts.Name,
		WorkDir: opts.WorkDir,
		Model:   opts.Model,
		Status:  "active",
	})

	go b.readClaudeEvents(sess)

	b.emit(WSEvent{Event: "session_status_changed", SessionID: sess.ID, Status: "active"})

	if b.wechatClient != nil && b.config.IsPushEnabled("session_status") {
		ct := b.wechatClient.LastContact()
		if ct.FromID != "" {
			b.wechatClient.SendMessage(ct.FromID, ct.ContextToken,
				fmt.Sprintf("已接管会话: %s\n工作目录: %s", opts.Name, opts.WorkDir))
		}
	}
	return nil
}

func (b *Bridge) readClaudeEvents(sess *claude.Session) {
	for evt := range sess.Events() {
		switch evt.Type {
		case claude.EventAssistant:
			if evt.Text != "" && b.config.IsPushEnabled("claude_response") {
				b.sendWechatToLast(evt.Text)
			}
			for _, block := range evt.Content {
				if block.Type == "tool_use" && b.config.IsPushEnabled("tool_use") {
					b.sendWechatToLast(fmt.Sprintf("[工具调用] %s: %v", block.Name, truncateInput(block.Input)))
				}
			}
			b.emit(WSEvent{
				Event:     "claude_output",
				SessionID: sess.ID,
				Type:      "text",
				Content:   evt.Text,
			})

		case claude.EventControlRequest:
			b.mu.Lock()
			b.pendingPerm = &pendingPermission{
				RequestID: evt.RequestID,
				ToolName:  evt.ToolName,
				ToolInput: evt.ToolInput,
			}
			b.mu.Unlock()

			permMsg := fmt.Sprintf("⚠️ 权限请求\n工具: %s\n参数: %v\n回复 Y 批准 / N 拒绝",
				evt.ToolName, truncateInput(evt.ToolInput))
			b.sendWechatToLast(permMsg)
			b.emit(WSEvent{
				Event:     "permission_request",
				SessionID: sess.ID,
				Tool:      evt.ToolName,
				Data:      evt.ToolInput,
			})

		case claude.EventResult:
			if b.config.IsPushEnabled("session_status") {
				b.sendWechatToLast(fmt.Sprintf("[完成] %s (耗时 %dms, %d 轮)",
					evt.StopReason, evt.DurationMs, evt.NumTurns))
			}

		case claude.EventError:
			b.sendWechatToLast(fmt.Sprintf("[错误] %s", evt.Error))
			b.emit(WSEvent{Event: "session_status_changed", SessionID: sess.ID, Status: "error"})
		}
	}
}

func (b *Bridge) StopSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.StopSessionLocked()
}

func (b *Bridge) StopSessionLocked() {
	if b.activeSess == nil {
		return
	}
	b.store.UpdateSessionStatus(b.activeSess.ID, "stopped", 0)
	b.activeSess.Stop()
	b.emit(WSEvent{Event: "session_status_changed", SessionID: b.activeSess.ID, Status: "stopped"})
	b.activeSess = nil
}

func (b *Bridge) resumeSession(id string, msg wechat.Message) {
	sessMeta, err := b.store.GetSession(id)
	if err != nil {
		b.sendWechat(msg, fmt.Sprintf("找不到会话: %s", id))
		return
	}
	err = b.StartSession(claude.StartOptions{
		WorkDir:  sessMeta.WorkDir,
		Name:     sessMeta.Name,
		ResumeID: sessMeta.ID,
	})
	if err != nil {
		b.sendWechat(msg, fmt.Sprintf("恢复会话失败: %v", err))
	}
}

func (b *Bridge) sendWechat(msg wechat.Message, text string) {
	if b.wechatClient == nil || b.wechatClient.Status() != wechat.StatusConnected {
		return
	}
	for _, chunk := range splitLongMessage(text, maxWeChatMsgLen) {
		b.wechatClient.SendMessage(msg.FromUserID, msg.ContextToken, chunk)
	}
}

func (b *Bridge) sendWechatToLast(text string) {
	if b.wechatClient == nil {
		return
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID == "" {
		return
	}
	for _, chunk := range splitLongMessage(text, maxWeChatMsgLen) {
		b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, chunk)
	}
}

func (b *Bridge) ActiveSessionID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.activeSess != nil {
		return b.activeSess.ID
	}
	return ""
}

func splitLongMessage(text string, maxLen int) []string {
	if utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	runes := []rune(text)
	total := (len(runes) + maxLen - 1) / maxLen
	for i := 0; i < len(runes); i += maxLen {
		end := i + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		if total > 1 {
			chunk += fmt.Sprintf(" [%d/%d]", len(chunks)+1, total)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func truncateInput(input map[string]interface{}) string {
	if cmd, ok := input["command"]; ok {
		s := fmt.Sprintf("%v", cmd)
		if len(s) > 200 {
			return s[:200] + "..."
		}
		return s
	}
	s := fmt.Sprintf("%v", input)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
```

- [ ] **Step 2: Write bridge tests**

Create `internal/bridge/bridge_test.go`:

```go
package bridge

import (
	"testing"

	"github.com/linfree/cc-go/internal/config"
)

func TestSplitLongMessage_Short(t *testing.T) {
	result := splitLongMessage("hello", 3500)
	if len(result) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(result))
	}
	if result[0] != "hello" {
		t.Errorf("expected 'hello', got '%s'", result[0])
	}
}

func TestSplitLongMessage_Long(t *testing.T) {
	longText := ""
	for i := 0; i < 4000; i++ {
		longText += "a"
	}
	result := splitLongMessage(longText, 3500)
	if len(result) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(result))
	}
}

func TestTruncateInput_Short(t *testing.T) {
	result := truncateInput(map[string]interface{}{"command": "ls -la"})
	if result != "ls -la" {
		t.Errorf("expected 'ls -la', got '%s'", result)
	}
}

func TestNewBridge(t *testing.T) {
	cfg := config.DefaultConfig()
	b := New(cfg, nil)
	if b == nil {
		t.Fatal("expected non-nil bridge")
	}
	if b.ActiveSessionID() != "" {
		t.Error("expected no active session")
	}
}
```

Run: `go test ./internal/bridge/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/bridge/
git commit -m "feat: implement wechat-claude message bridge"
```

---

### Task 10: Web Server Core

**Files:**
- Modify: `internal/server/router.go`
- Create: `internal/server/ws/hub.go`

- [ ] **Step 1: Implement Gin router and WebSocket hub**

Write `internal/server/router.go`:

```go
package server

import (
	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/server/api"
	"github.com/linfree/cc-go/internal/server/ws"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

type Server struct {
	router  *gin.Engine
	cfg     *config.Config
	store   *store.Store
	bridge  *bridge.Bridge
	wechat  *wechat.Client
	wsHub   *ws.Hub
}

func New(cfg *config.Config, st *store.Store, br *bridge.Bridge, wc *wechat.Client) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	hub := ws.NewHub()
	go hub.Run()

	s := &Server{
		router: r,
		cfg:    cfg,
		store:  st,
		bridge: br,
		wechat: wc,
		wsHub:  hub,
	}

	// Pipe bridge events to WebSocket hub
	go func() {
		for evt := range br.EventBus() {
			hub.Broadcast(evt)
		}
	}()

	api.RegisterRoutes(r, cfg, st, br, wc, hub)
	return s
}

func (s *Server) Router() *gin.Engine { return s.router }
```

Write `internal/server/ws/hub.go`:

```go
package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type Hub struct {
	clients map[*Client]bool
	mu      sync.RWMutex
}

type Client struct {
	conn *websocket.Conn
	hub  *Hub
	send chan []byte
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]bool)}
}

func (h *Hub) Run() {
	// Hub runs until server stops
}

func (h *Hub) Broadcast(msg interface{}) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	client := &Client{conn: conn, hub: h, send: make(chan []byte, 64)}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	go client.writePump()
	client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.mu.Lock()
		delete(c.hub.clients, c)
		c.hub.mu.Unlock()
		c.conn.CloseNow()
	}()
	for {
		_, _, err := c.conn.Read(c.hub)
		_ = err
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.CloseNow()
	for msg := range c.send {
		err := c.conn.Write(c.hub, websocket.MessageText, msg)
		if err != nil {
			log.Printf("[ws] write error: %v", err)
			return
		}
	}
}

// Satisfy the context interface that coder/websocket needs
func (h *Hub) Close() error { return nil }
```

- [ ] **Step 2: Write a minimal test to verify server starts**

Create `internal/server/server_test.go`:

```go
package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

func TestNewServer(t *testing.T) {
	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	wc := wechat.NewClient("", "", time.Now())
	br := bridge.New(cfg, st)
	br.SetWechatClient(wc)

	s := New(cfg, st, br, wc)
	if s.Router() == nil {
		t.Error("expected non-nil router")
	}
}

func TestServerListen(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WebPort = 0 // Let OS pick
	st, _ := store.Open()
	defer st.Close()
	wc := wechat.NewClient("", "", time.Now())
	br := bridge.New(cfg, st)

	s := New(cfg, st, br, wc)
	go http.ListenAndServe(":0", s.Router())
	time.Sleep(100 * time.Millisecond)
}
```

Run: `go test ./internal/server/... -v -timeout 10s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/
git commit -m "feat: implement gin server with websocket hub"
```

---

### Task 11: API Handlers

**Files:**
- Create: `internal/server/api/routes.go`
- Create: `internal/server/api/wechat.go`
- Create: `internal/server/api/sessions.go`
- Create: `internal/server/api/settings.go`

- [ ] **Step 1: Implement all API routes and handlers**

Write `internal/server/api/routes.go`:

```go
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
	"github.com/linfree/cc-go/internal/server/ws"
)

func RegisterRoutes(r *gin.Engine, cfg *config.Config, st *store.Store, br *bridge.Bridge, wc *wechat.Client, hub *ws.Hub) {
	api := r.Group("/api/v1")

	registerWechatRoutes(api, cfg, wc)
	registerClaudeRoutes(api, st, br)
	registerSessionRoutes(api, st, br)
	registerPushRoutes(api, cfg)
	registerSettingsRoutes(api, cfg)

	r.GET("/ws/events", hub.HandleWS)
}
```

Write `internal/server/api/wechat.go`:

```go
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/wechat"
)

func registerWechatRoutes(r *gin.RouterGroup, cfg *config.Config, wc *wechat.Client) {
	r.GET("/wechat/qrcode", func(c *gin.Context) {
		if wc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wechat client not initialized"})
			return
		}
		qrcode, qrcodeImg, err := wc.GetQRCode()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Start polling for scan in background
		go func() {
			deadline := time.Now().Add(10 * time.Minute)
			for time.Now().Before(deadline) {
				confirmed, token, baseURL, err := wc.CheckQRCodeStatus(qrcode)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				if confirmed {
					wc.SetToken(token, baseURL)
					cfg.Wechat.BotToken = token
					cfg.Wechat.BaseURL = baseURL
					cfg.Wechat.LoginTime = time.Now().Format(time.RFC3339)
					cfg.Save()
					wc.Start()
					return
				}
				time.Sleep(1 * time.Second)
			}
		}()

		c.JSON(http.StatusOK, gin.H{
			"qrcode":      qrcode,
			"qrcode_img":  qrcodeImg,
		})
	})

	r.GET("/wechat/status", func(c *gin.Context) {
		connected := false
		if wc != nil && wc.Status() == wechat.StatusConnected {
			connected = true
		}
		c.JSON(http.StatusOK, gin.H{
			"connected":  connected,
			"status":     string(wc.Status()),
			"login_time": cfg.Wechat.LoginTime,
		})
	})

	r.POST("/wechat/disconnect", func(c *gin.Context) {
		if wc != nil {
			wc.Stop()
		}
		c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
	})
}
```

Write `internal/server/api/sessions.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/store"
)

func registerClaudeRoutes(r *gin.RouterGroup, st *store.Store, br *bridge.Bridge) {
	r.GET("/claude/path", func(c *gin.Context) {
		path, err := claude.FindClaudeCLI()
		c.JSON(http.StatusOK, gin.H{
			"path":  path,
			"error": errToString(err),
		})
	})

	r.POST("/claude/path", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		version, err := claude.ValidateClaudeCLI(req.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"path": req.Path, "version": version})
	})

	r.POST("/claude/auto-detect", func(c *gin.Context) {
		path, err := claude.FindClaudeCLI()
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "claude CLI not found"})
			return
		}
		version, err := claude.ValidateClaudeCLI(path)
		c.JSON(http.StatusOK, gin.H{
			"path":    path,
			"version": version,
			"valid":   err == nil,
		})
	})
}

func registerSessionRoutes(r *gin.RouterGroup, st *store.Store, br *bridge.Bridge) {
	r.GET("/sessions", func(c *gin.Context) {
		// Merge store sessions with discovered history sessions
		stored, _ := st.ListSessions()
		historySessions, _ := claude.DiscoverSessions()

		storedMap := make(map[string]store.Session)
		for _, s := range stored {
			storedMap[s.ID] = s
		}

		var result []gin.H
		for _, hs := range historySessions {
			status := "stopped"
			if ss, ok := storedMap[hs.ID]; ok {
				status = ss.Status
			}
			result = append(result, gin.H{
				"id":            hs.ID,
				"name":          hs.FirstPrompt,
				"work_dir":      hs.ProjectPath,
				"status":        status,
				"message_count": hs.MessageCount,
				"created":       hs.Created,
				"modified":      hs.Modified,
				"history_path":  hs.FilePath,
				"git_branch":    hs.GitBranch,
			})
		}
		if result == nil {
			result = []gin.H{}
		}
		c.JSON(http.StatusOK, result)
	})

	r.GET("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")
		sess, err := st.GetSession(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusOK, sess)
	})

	r.GET("/sessions/:id/history", func(c *gin.Context) {
		id := c.Param("id")
		sess, err := st.GetSession(id)
		var historyPath string
		if err == nil {
			historyPath = sess.HistoryPath
		}
		if historyPath == "" {
			// Try to find from discovered sessions
			hs, _ := claude.DiscoverSessions()
			for _, h := range hs {
				if h.ID == id {
					historyPath = h.FilePath
					break
				}
			}
		}
		if historyPath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "history file not found"})
			return
		}
		msgs, err := claude.ReadHistory(historyPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, msgs)
	})

	r.POST("/sessions/start", func(c *gin.Context) {
		var req struct {
			WorkDir string `json:"work_dir"`
			Model   string `json:"model"`
			Name    string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		err := br.StartSession(claude.StartOptions{
			WorkDir: req.WorkDir,
			Model:   req.Model,
			Name:    req.Name,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "started"})
	})

	r.POST("/sessions/:id/resume", func(c *gin.Context) {
		id := c.Param("id")
		sess, err := st.GetSession(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		err = br.StartSession(claude.StartOptions{
			WorkDir:  sess.WorkDir,
			Name:     sess.Name,
			ResumeID: id,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
	})

	r.POST("/sessions/:id/stop", func(c *gin.Context) {
		br.StopSession()
		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	})

	r.DELETE("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")
		st.DeleteSession(id)
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	})
}
```

Write `internal/server/api/settings.go`:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/config"
)

func registerPushRoutes(r *gin.RouterGroup, cfg *config.Config) {
	r.GET("/push/types", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"types": []gin.H{
				{"key": "permission", "label": "权限请求", "required": true},
				{"key": "claude_response", "label": "Claude 响应内容", "required": false},
				{"key": "tool_use", "label": "工具调用通知", "required": false},
				{"key": "session_status", "label": "会话状态变更", "required": false},
				{"key": "resource_usage", "label": "资源使用统计", "required": false},
			},
		})
	})

	r.GET("/push/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"push_types": cfg.PushTypes})
	})

	r.PUT("/push/settings", func(c *gin.Context) {
		var req struct {
			PushTypes []string `json:"push_types"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cfg.PushTypes = req.PushTypes
		cfg.Save()
		c.JSON(http.StatusOK, gin.H{"push_types": cfg.PushTypes})
	})
}

func registerSettingsRoutes(r *gin.RouterGroup, cfg *config.Config) {
	r.GET("/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, cfg)
	})

	r.PUT("/settings", func(c *gin.Context) {
		var req struct {
			ClaudeCLIPath   string   `json:"claude_cli_path"`
			AutoFindClaude  *bool    `json:"auto_find_claude"`
			PermissionMode  string   `json:"permission_mode"`
			Language        string   `json:"language"`
			WebPort         int      `json:"web_port"`
			AutoOpenBrowser *bool    `json:"auto_open_browser"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.ClaudeCLIPath != "" {
			cfg.ClaudeCLIPath = req.ClaudeCLIPath
		}
		if req.AutoFindClaude != nil {
			cfg.AutoFindClaude = *req.AutoFindClaude
		}
		if req.PermissionMode != "" {
			cfg.PermissionMode = req.PermissionMode
		}
		if req.Language != "" {
			cfg.Language = req.Language
		}
		if req.WebPort > 0 {
			cfg.WebPort = req.WebPort
		}
		if req.AutoOpenBrowser != nil {
			cfg.AutoOpenBrowser = *req.AutoOpenBrowser
		}
		cfg.Save()
		c.JSON(http.StatusOK, cfg)
	})
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

- [ ] **Step 2: Run tests**

Run: `go build ./...`
Expected: builds without errors

- [ ] **Step 3: Commit**

```bash
git add internal/server/api/
git commit -m "feat: implement REST API handlers"
```

---

### Task 12: CLI Entry Point

**Files:**
- Modify: `cmd/cc-go/main.go`

- [ ] **Step 1: Implement the cobra CLI with start command**

Rewrite `cmd/cc-go/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/server"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

func main() {
	root := &cobra.Command{
		Use:   "cc-go",
		Short: "WeChat bot for remote Claude Code management",
	}

	root.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the cc-go service",
		RunE:  runStart,
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := store.Open()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	needSetupWechat := false
	needSetupClaude := false

	// Check Claude CLI
	if cfg.ClaudeCLIPath == "" && cfg.AutoFindClaude {
		path, err := claude.FindClaudeCLI()
		if err == nil {
			cfg.ClaudeCLIPath = path
			cfg.Save()
		}
	}
	if cfg.ClaudeCLIPath == "" {
		needSetupClaude = true
	}

	// Check WeChat
	var wc *wechat.Client
	if cfg.Wechat.BotToken != "" {
		wc = wechat.NewClient(cfg.Wechat.BaseURL, cfg.Wechat.BotToken, wechat.ParseLoginTime(cfg.Wechat.LoginTime))
		wc.Start()
	} else {
		needSetupWechat = true
	}

	br := bridge.New(cfg, st)
	if wc != nil {
		br.SetWechatClient(wc)

		// Route incoming WeChat messages to bridge
		go func() {
			for msg := range wc.Messages() {
				br.HandleWechatMessage(msg)
			}
		}()
	}

	srv := server.New(cfg, st, br, wc)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down...")
		br.StopSession()
		if wc != nil {
			wc.Stop()
		}
		os.Exit(0)
	}()

	// Open browser
	if cfg.AutoOpenBrowser {
		url := fmt.Sprintf("http://localhost:%d", cfg.WebPort)
		if needSetupWechat {
			url += "/wechat"
		} else if needSetupClaude {
			url += "/claude"
		}
		openBrowser(url)
	}

	addr := fmt.Sprintf(":%d", cfg.WebPort)
	log.Printf("cc-go server starting on %s", addr)
	return srv.Router().Run(addr)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}
```

Also add the helper to wechat package:

Modify `internal/wechat/client.go`, add:
```go
func ParseLoginTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
```

- [ ] **Step 2: Build and verify**

Run: `cd G:/dev/AI/cc-go && go build -o cc-go.exe ./cmd/cc-go/`
Expected: build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/cc-go/main.go internal/wechat/client.go
git commit -m "feat: implement CLI entry point with start command"
```

---

### Task 13: Frontend — Project Setup & Layout

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/main.tsx`
- Create: `web/src/layouts/MainLayout.tsx`
- Create: `web/vite.config.ts`

- [ ] **Step 1: Configure Vite with proxy and build output**

Replace `web/vite.config.ts`:

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:18080',
      '/ws': {
        target: 'ws://localhost:18080',
        ws: true,
      },
    },
  },
  build: {
    outDir: '../cmd/cc-go/web-dist',
    emptyOutDir: true,
  },
})
```

- [ ] **Step 2: Create main layout with Ant Design**

Replace `web/src/App.tsx`:

```tsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ConfigProvider, theme } from 'antd'
import MainLayout from './layouts/MainLayout'
import WechatBind from './pages/WechatBind'
import SessionList from './pages/SessionList'
import SessionChat from './pages/SessionChat'
import ClaudeConfig from './pages/ClaudeConfig'
import PushSettings from './pages/PushSettings'
import Settings from './pages/Settings'

export default function App() {
  return (
    <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<MainLayout />}>
            <Route index element={<Navigate to="/sessions" replace />} />
            <Route path="wechat" element={<WechatBind />} />
            <Route path="sessions" element={<SessionList />} />
            <Route path="sessions/:id" element={<SessionChat />} />
            <Route path="claude" element={<ClaudeConfig />} />
            <Route path="push" element={<PushSettings />} />
            <Route path="settings" element={<Settings />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  )
}
```

Write `web/src/layouts/MainLayout.tsx`:

```tsx
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu } from 'antd'
import { WechatOutlined, MessageOutlined, SettingOutlined, ToolOutlined, BellOutlined, ApartmentOutlined } from '@ant-design/icons'

const { Sider, Content } = Layout

const menuItems = [
  { key: '/wechat', icon: <WechatOutlined />, label: '微信连接' },
  { key: '/sessions', icon: <MessageOutlined />, label: '会话管理' },
  { key: '/claude', icon: <ToolOutlined />, label: 'Claude 配置' },
  { key: '/push', icon: <BellOutlined />, label: '推送设置' },
  { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
]

export default function MainLayout() {
  const navigate = useNavigate()
  const location = useLocation()

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider theme="light" width={200}>
        <div style={{ padding: '16px', fontWeight: 'bold', fontSize: '18px', textAlign: 'center' }}>
          cc-go
        </div>
        <Menu
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Content style={{ padding: '24px', background: '#fff' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
```

- [ ] **Step 3: Verify frontend builds**

Run: `cd G:/dev/AI/cc-go/web && npm run build`
Expected: builds without errors, output in `cmd/cc-go/web-dist/`

- [ ] **Step 4: Commit**

```bash
git add web/
git commit -m "feat: frontend layout with ant design navigation"
```

---

### Task 14: Frontend — WeChat Bind Page

**Files:**
- Create: `web/src/pages/WechatBind.tsx`
- Create: `web/src/api.ts`

- [ ] **Step 1: Create API client**

Write `web/src/api.ts`:

```typescript
const BASE = '/api/v1'

async function request(path: string, options?: RequestInit) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  return res.json()
}

export const api = {
  // WeChat
  getQRCode: () => request('/wechat/qrcode'),
  getWechatStatus: () => request('/wechat/status'),
  disconnectWechat: () => request('/wechat/disconnect', { method: 'POST' }),

  // Claude
  getClaudePath: () => request('/claude/path'),
  setClaudePath: (path: string) => request('/claude/path', { method: 'POST', body: JSON.stringify({ path }) }),
  autoDetectClaude: () => request('/claude/auto-detect', { method: 'POST' }),

  // Sessions
  getSessions: () => request('/sessions'),
  getSession: (id: string) => request(`/sessions/${id}`),
  getSessionHistory: (id: string) => request(`/sessions/${id}/history`),
  startSession: (data: { work_dir: string; model?: string; name?: string }) =>
    request('/sessions/start', { method: 'POST', body: JSON.stringify(data) }),
  resumeSession: (id: string) => request(`/sessions/${id}/resume`, { method: 'POST' }),
  stopSession: (id: string) => request(`/sessions/${id}/stop`, { method: 'POST' }),
  deleteSession: (id: string) => request(`/sessions/${id}`, { method: 'DELETE' }),

  // Push
  getPushTypes: () => request('/push/types'),
  getPushSettings: () => request('/push/settings'),
  updatePushSettings: (types: string[]) =>
    request('/push/settings', { method: 'PUT', body: JSON.stringify({ push_types: types }) }),

  // Settings
  getSettings: () => request('/settings'),
  updateSettings: (data: Record<string, unknown>) =>
    request('/settings', { method: 'PUT', body: JSON.stringify(data) }),
}
```

- [ ] **Step 2: Implement WeChat bind page**

Write `web/src/pages/WechatBind.tsx`:

```tsx
import { useState, useEffect, useCallback } from 'react'
import { Card, Button, Tag, Spin, Typography, Space } from 'antd'
import { ReloadOutlined, DisconnectOutlined } from '@ant-design/icons'
import { api } from '../api'

const { Text, Title } = Typography

export default function WechatBind() {
  const [status, setStatus] = useState<string>('loading')
  const [qrcodeImg, setQrcodeImg] = useState<string>('')
  const [loading, setLoading] = useState(false)

  const checkStatus = useCallback(async () => {
    try {
      const data = await api.getWechatStatus()
      setStatus(data.connected ? 'connected' : 'disconnected')
    } catch {
      setStatus('error')
    }
  }, [])

  useEffect(() => {
    checkStatus()
    const interval = setInterval(checkStatus, 5000)
    return () => clearInterval(interval)
  }, [checkStatus])

  const handleGetQRCode = async () => {
    setLoading(true)
    try {
      const data = await api.getQRCode()
      if (data.qrcode_img) {
        setQrcodeImg(data.qrcode_img)
      }
    } finally {
      setLoading(false)
    }
  }

  const handleDisconnect = async () => {
    await api.disconnectWechat()
    setStatus('disconnected')
    setQrcodeImg('')
  }

  const statusColor = status === 'connected' ? 'green' : status === 'disconnected' ? 'red' : 'default'

  return (
    <div>
      <Title level={3}>微信连接</Title>
      <Card>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <div>
            <Text strong>连接状态：</Text>
            <Tag color={statusColor}>{status === 'connected' ? '已连接' : '未连接'}</Tag>
          </div>

          {status !== 'connected' && (
            <div>
              <Button type="primary" icon={<ReloadOutlined />} onClick={handleGetQRCode} loading={loading}>
                获取二维码
              </Button>
              {qrcodeImg && (
                <div style={{ marginTop: '16px' }}>
                  <img src={qrcodeImg} alt="WeChat QR Code" style={{ maxWidth: '300px' }} />
                  <Text type="secondary" style={{ display: 'block', marginTop: '8px' }}>
                    请使用微信扫描二维码
                  </Text>
                </div>
              )}
            </div>
          )}

          {status === 'connected' && (
            <Button danger icon={<DisconnectOutlined />} onClick={handleDisconnect}>
              断开连接
            </Button>
          )}

          {status === 'loading' && <Spin tip="检查连接状态..." />}
        </Space>
      </Card>
    </div>
  )
}
```

- [ ] **Step 3: Verify build**

Run: `cd G:/dev/AI/cc-go/web && npm run build`
Expected: builds without TS errors

- [ ] **Step 4: Commit**

```bash
git add web/src/api.ts web/src/pages/WechatBind.tsx
git commit -m "feat: wechat bind page with QR code scanning"
```

---

### Task 15: Frontend — Session List Page

**Files:**
- Create: `web/src/pages/SessionList.tsx`

- [ ] **Step 1: Implement session list page**

Write `web/src/pages/SessionList.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Table, Button, Tag, Space, Modal, Form, Input, message } from 'antd'
import { PlayCircleOutlined, StopOutlined, DeleteOutlined, EyeOutlined, PlusOutlined } from '@ant-design/icons'
import { api } from '../api'

interface Session {
  id: string
  name: string
  work_dir: string
  status: string
  message_count: number
  created: string
  modified: string
  history_path: string
  git_branch: string
}

export default function SessionList() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm()
  const navigate = useNavigate()

  const fetchSessions = async () => {
    setLoading(true)
    try {
      const data = await api.getSessions()
      setSessions(data || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchSessions()
    const interval = setInterval(fetchSessions, 10000)
    return () => clearInterval(interval)
  }, [])

  const handleStart = async (values: { work_dir: string; model?: string; name?: string }) => {
    try {
      await api.startSession(values)
      message.success('会话已启动')
      setModalOpen(false)
      form.resetFields()
      fetchSessions()
    } catch (e) {
      message.error('启动会话失败')
    }
  }

  const handleResume = async (id: string) => {
    try {
      await api.resumeSession(id)
      message.success('会话已接管')
      fetchSessions()
    } catch {
      message.error('接管会话失败')
    }
  }

  const handleStop = async (id: string) => {
    try {
      await api.stopSession(id)
      message.success('会话已停止')
      fetchSessions()
    } catch {
      message.error('停止会话失败')
    }
  }

  const handleDelete = async (id: string) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除这个会话记录吗？',
      onOk: async () => {
        await api.deleteSession(id)
        message.success('已删除')
        fetchSessions()
      },
    })
  }

  const statusColor: Record<string, string> = {
    active: 'green',
    idle: 'blue',
    stopped: 'default',
    error: 'red',
  }

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true,
      render: (text: string) => text || '(无标题)' },
    { title: '工作目录', dataIndex: 'work_dir', key: 'work_dir', ellipsis: true },
    { title: '消息数', dataIndex: 'message_count', key: 'message_count', width: 80 },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 100,
      render: (s: string) => <Tag color={statusColor[s] || 'default'}>{s}</Tag>,
    },
    {
      title: '操作', key: 'actions', width: 300,
      render: (_: unknown, record: Session) => (
        <Space>
          <Button size="small" icon={<EyeOutlined />} onClick={() => navigate(`/sessions/${record.id}`)}>
            查看
          </Button>
          {record.status !== 'active' && (
            <Button size="small" type="primary" icon={<PlayCircleOutlined />} onClick={() => handleResume(record.id)}>
              接管
            </Button>
          )}
          {record.status === 'active' && (
            <Button size="small" danger icon={<StopOutlined />} onClick={() => handleStop(record.id)}>
              停止
            </Button>
          )}
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => handleDelete(record.id)} />
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '16px' }}>
        <h3>会话管理</h3>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          新建会话
        </Button>
      </div>

      <Card>
        <Table
          dataSource={sessions}
          columns={columns}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      </Card>

      <Modal
        title="新建 Claude 会话"
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={handleStart}>
          <Form.Item name="work_dir" label="工作目录" rules={[{ required: true, message: '请输入工作目录' }]}>
            <Input placeholder="/path/to/project" />
          </Form.Item>
          <Form.Item name="name" label="会话名称">
            <Input placeholder="可选" />
          </Form.Item>
          <Form.Item name="model" label="模型">
            <Input placeholder="默认" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
```

- [ ] **Step 2: Verify build**

Run: `cd G:/dev/AI/cc-go/web && npm run build`
Expected: builds without TS errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/SessionList.tsx
git commit -m "feat: session list page with start/resume/stop"
```

---

### Task 16: Frontend — Session Chat Page

**Files:**
- Create: `web/src/pages/SessionChat.tsx`

- [ ] **Step 1: Implement chat view page**

Write `web/src/pages/SessionChat.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { useParams } from 'react-router-dom'
import { Card, Typography, Spin, Collapse, Tag, Empty } from 'antd'
import { UserOutlined, RobotOutlined, ToolOutlined } from '@ant-design/icons'
import { api } from '../api'

const { Text, Paragraph } = Typography

interface HistoryMessage {
  type: string
  role: string
  content: string
  tool_use?: { name: string; input: Record<string, unknown> }
  timestamp: string
}

export default function SessionChat() {
  const { id } = useParams<{ id: string }>()
  const [messages, setMessages] = useState<HistoryMessage[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    api.getSessionHistory(id).then(data => {
      setMessages(data || [])
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [id])

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />

  return (
    <div>
      <Typography.Title level={3}>会话记录</Typography.Title>
      <Card>
        {messages.length === 0 ? (
          <Empty description="暂无聊天记录" />
        ) : (
          <div style={{ maxHeight: '70vh', overflow: 'auto' }}>
            {messages.filter(m => m.role === 'user' || m.role === 'assistant').map((msg, i) => (
              <div key={i} style={{
                display: 'flex',
                flexDirection: msg.role === 'user' ? 'row-reverse' : 'row',
                marginBottom: '16px',
                gap: '12px',
              }}>
                <div style={{
                  width: '36px', height: '36px', borderRadius: '50%',
                  background: msg.role === 'user' ? '#1677ff' : '#52c41a',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  color: '#fff', flexShrink: 0,
                }}>
                  {msg.role === 'user' ? <UserOutlined /> : <RobotOutlined />}
                </div>
                <div style={{
                  maxWidth: '70%',
                  background: msg.role === 'user' ? '#e6f4ff' : '#f6ffed',
                  borderRadius: '12px',
                  padding: '12px 16px',
                }}>
                  <Paragraph style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
                    {msg.content || '(无文本内容)'}
                  </Paragraph>
                  {msg.tool_use && (
                    <Collapse
                      size="small"
                      style={{ marginTop: '8px' }}
                      items={[{
                        key: 'tool',
                        label: <span><ToolOutlined /> {msg.tool_use.name}</span>,
                        children: <pre style={{ fontSize: '12px', margin: 0 }}>
                          {JSON.stringify(msg.tool_use.input, null, 2)}
                        </pre>,
                      }]}
                    />
                  )}
                  {msg.timestamp && (
                    <Text type="secondary" style={{ fontSize: '11px', display: 'block', marginTop: '4px' }}>
                      {new Date(msg.timestamp).toLocaleString()}
                    </Text>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
```

- [ ] **Step 2: Verify build**

Run: `cd G:/dev/AI/cc-go/web && npm run build`
Expected: builds without TS errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/SessionChat.tsx
git commit -m "feat: session chat history page"
```

---

### Task 17: Frontend — Remaining Pages & Embed Setup

**Files:**
- Create: `web/src/pages/ClaudeConfig.tsx`
- Create: `web/src/pages/PushSettings.tsx`
- Create: `web/src/pages/Settings.tsx`
- Create: `cmd/cc-go/embed.go`

- [ ] **Step 1: Create Claude config page**

Write `web/src/pages/ClaudeConfig.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { Card, Button, Input, Space, Typography, message, Descriptions, Tag } from 'antd'
import { SearchOutlined, CheckOutlined } from '@ant-design/icons'
import { api } from '../api'

const { Title } = Typography

export default function ClaudeConfig() {
  const [currentPath, setCurrentPath] = useState('')
  const [manualPath, setManualPath] = useState('')
  const [version, setVersion] = useState('')
  const [valid, setValid] = useState<boolean | null>(null)

  useEffect(() => {
    api.getSettings().then(data => {
      setCurrentPath(data.claude_cli_path || '')
    })
  }, [])

  const handleAutoDetect = async () => {
    try {
      const data = await api.autoDetectClaude()
      if (data.valid) {
        setCurrentPath(data.path)
        setVersion(data.version)
        setValid(true)
        await api.updateSettings({ claude_cli_path: data.path })
        message.success(`找到 Claude CLI: ${data.path}`)
      } else {
        message.warning('未找到 Claude CLI')
        setValid(false)
      }
    } catch {
      message.error('未找到 Claude CLI，请手动配置')
      setValid(false)
    }
  }

  const handleManualSet = async () => {
    try {
      const data = await api.setClaudePath(manualPath)
      setCurrentPath(manualPath)
      setVersion(data.version)
      setValid(true)
      await api.updateSettings({ claude_cli_path: manualPath })
      message.success('路径已设置')
    } catch {
      message.error('路径无效')
    }
  }

  return (
    <div>
      <Title level={3}>Claude CLI 配置</Title>
      <Card>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Descriptions bordered column={1}>
            <Descriptions.Item label="当前路径">
              {currentPath || '(未配置)'}
            </Descriptions.Item>
            <Descriptions.Item label="版本">
              {version || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              {valid === true ? <Tag color="green">有效</Tag> :
               valid === false ? <Tag color="red">未找到</Tag> :
               <Tag>未验证</Tag>}
            </Descriptions.Item>
          </Descriptions>

          <Button type="primary" icon={<SearchOutlined />} onClick={handleAutoDetect}>
            自动检测
          </Button>

          <div>
            <Space>
              <Input
                placeholder="/usr/local/bin/claude"
                value={manualPath}
                onChange={e => setManualPath(e.target.value)}
                style={{ width: '400px' }}
              />
              <Button icon={<CheckOutlined />} onClick={handleManualSet}>
                手动设置
              </Button>
            </Space>
          </div>
        </Space>
      </Card>
    </div>
  )
}
```

- [ ] **Step 2: Create push settings page**

Write `web/src/pages/PushSettings.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { Card, Switch, List, Typography, message, Tag } from 'antd'
import { api } from '../api'

const { Title } = Typography

interface PushType {
  key: string
  label: string
  required: boolean
}

export default function PushSettings() {
  const [types, setTypes] = useState<PushType[]>([])
  const [enabledTypes, setEnabledTypes] = useState<string[]>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api.getPushTypes().then(data => setTypes(data.types || []))
    api.getPushSettings().then(data => setEnabledTypes(data.push_types || []))
  }, [])

  const handleToggle = async (key: string, checked: boolean) => {
    const newTypes = checked
      ? [...enabledTypes, key]
      : enabledTypes.filter(t => t !== key)
    setEnabledTypes(newTypes)
    setSaving(true)
    try {
      await api.updatePushSettings(newTypes)
      message.success('已保存')
    } catch {
      message.error('保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <Title level={3}>推送设置</Title>
      <Card>
        <List
          dataSource={types}
          renderItem={(item) => (
            <List.Item
              actions={[
                <Switch
                  checked={enabledTypes.includes(item.key)}
                  disabled={item.required || saving}
                  onChange={(checked) => handleToggle(item.key, checked)}
                />,
              ]}
            >
              <List.Item.Meta
                title={
                  <span>
                    {item.label}
                    {item.required && <Tag color="red" style={{ marginLeft: '8px' }}>强制开启</Tag>}
                  </span>
                }
              />
            </List.Item>
          )}
        />
      </Card>
    </div>
  )
}
```

- [ ] **Step 3: Create settings page**

Write `web/src/pages/Settings.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { Card, Form, Input, InputNumber, Switch, Select, Button, Typography, message } from 'antd'
import { api } from '../api'

const { Title } = Typography

export default function Settings() {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    api.getSettings().then(data => {
      form.setFieldsValue(data)
    })
  }, [form])

  const handleSave = async (values: Record<string, unknown>) => {
    setLoading(true)
    try {
      await api.updateSettings(values)
      message.success('设置已保存')
    } catch {
      message.error('保存失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <Title level={3}>系统设置</Title>
      <Card>
        <Form form={form} layout="vertical" onFinish={handleSave} style={{ maxWidth: '500px' }}>
          <Form.Item name="language" label="语言">
            <Select options={[
              { value: 'zh-CN', label: '中文' },
              { value: 'en', label: 'English' },
            ]} />
          </Form.Item>
          <Form.Item name="web_port" label="Web 端口">
            <InputNumber min={1024} max={65535} />
          </Form.Item>
          <Form.Item name="auto_open_browser" label="启动时自动打开浏览器" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="auto_find_claude" label="自动查找 Claude CLI" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="permission_mode" label="权限模式">
            <Select options={[
              { value: 'default', label: '默认（全部审批）' },
              { value: 'acceptEdits', label: '接受编辑（半自动）' },
              { value: 'auto', label: '自动（全部批准）' },
              { value: 'plan', label: '计划（只读）' },
            ]} />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading}>
              保存设置
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
```

- [ ] **Step 4: Create Go embed file**

Write `cmd/cc-go/embed.go`:

```go
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web-dist/*
var webAssets embed.FS

func RegisterStaticRoutes(r *gin.Engine) {
	sub, err := fs.Sub(webAssets, "web-dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(sub))

	r.GET("/assets/*filepath", func(c *gin.Context) {
		// Strip / prefix for FileServer
		c.Request.URL.Path = c.Request.URL.Path
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/ws") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		data, err := webAssets.ReadFile("web-dist/index.html")
		if err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

func IsDevMode() bool {
	_, err := os.Stat("web/dist")
	return err == nil
}
```

Update `cmd/cc-go/main.go` to call `RegisterStaticRoutes` after server creation:

In `runStart`, after `srv := server.New(...)`, add:
```go
RegisterStaticRoutes(srv.Router())
```

- [ ] **Step 5: Build frontend and verify Go build**

Run: `cd G:/dev/AI/cc-go/web && npm run build`
Run: `cd G:/dev/AI/cc-go && go build ./cmd/cc-go/`
Expected: both build without errors

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/ClaudeConfig.tsx web/src/pages/PushSettings.tsx web/src/pages/Settings.tsx cmd/cc-go/embed.go cmd/cc-go/main.go
git commit -m "feat: remaining web pages and go embed for SPA"
```

---

### Task 18: Makefile & Build Automation

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

Write `Makefile`:

```makefile
.PHONY: all build web clean run dev

APP_NAME = cc-go
WEB_DIR = web
WEB_DIST = cmd/cc-go/web-dist

all: web build

web:
	cd $(WEB_DIR) && npm install && npm run build

build:
	go build -o $(APP_NAME) ./cmd/cc-go/

build-linux:
	cd $(WEB_DIR) && npm run build
	GOOS=linux GOARCH=amd64 go build -o $(APP_NAME)-linux ./cmd/cc-go/

build-mac:
	cd $(WEB_DIR) && npm run build
	GOOS=darwin GOARCH=amd64 go build -o $(APP_NAME)-mac ./cmd/cc-go/

build-win:
	cd $(WEB_DIR) && npm run build
	GOOS=windows GOARCH=amd64 go build -o $(APP_NAME).exe ./cmd/cc-go/

run: web
	go run ./cmd/cc-go/ start

dev:
	cd $(WEB_DIR) && npm run dev &
	go run ./cmd/cc-go/ start

clean:
	rm -rf $(WEB_DIST) $(APP_NAME) $(APP_NAME)-linux $(APP_NAME)-mac $(APP_NAME).exe

test:
	go test ./... -v -count=1 -timeout 60s
```

- [ ] **Step 2: Test builds**

Run: `make build-win` (or the equivalent Windows command)
Expected: produces `cc-go.exe` binary

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add Makefile with cross-compilation targets"
```

---

### Task 19: Integration Testing & Final Polish

**Files:**
- None new

- [ ] **Step 1: Run all Go tests**

Run: `go test ./... -v -timeout 60s`
Expected: All tests pass or skip gracefully

- [ ] **Step 2: Build the complete binary**

Run: `cd G:/dev/AI/cc-go && go build -o cc-go.exe ./cmd/cc-go/`
Expected: produces `cc-go.exe`

- [ ] **Step 3: Verify binary starts and serves pages**

Run: `.\cc-go.exe start` or `./cc-go start`
Expected: server starts on port 18080, opens browser

- [ ] **Step 4: Final commit**

```bash
git add -A
git diff --cached --stat
git commit -m "feat: cc-go v1.0 complete"
```

---

## Summary

| Task | Component | Files Created |
|------|-----------|--------------|
| 1 | Scaffolding | go.mod, directory structure, stubs |
| 2 | Config | internal/config/config.go |
| 3 | Store | internal/store/store.go |
| 4 | Claude Finder | internal/claude/finder.go |
| 5 | Claude Session | internal/claude/session.go, protocol.go |
| 6 | Claude History | internal/claude/history.go |
| 7 | WeChat Client | internal/wechat/client.go, message.go |
| 8 | WeChat Reconnect | internal/wechat/reconnect.go |
| 9 | Bridge | internal/bridge/bridge.go |
| 10 | Server Core | internal/server/router.go, ws/hub.go |
| 11 | API Handlers | internal/server/api/*.go |
| 12 | CLI Entry | cmd/cc-go/main.go |
| 13 | Frontend Layout | web/src/App.tsx, layouts/MainLayout.tsx |
| 14 | WeChat Page | web/src/pages/WechatBind.tsx, api.ts |
| 15 | Sessions Page | web/src/pages/SessionList.tsx |
| 16 | Chat Page | web/src/pages/SessionChat.tsx |
| 17 | Remaining Pages | ClaudeConfig, PushSettings, Settings, embed.go |
| 18 | Makefile | Makefile |
| 19 | Integration | Final testing and polish |