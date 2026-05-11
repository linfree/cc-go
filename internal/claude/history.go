package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type HistoryMessage struct {
	Type          string        `json:"type"`
	Role          string        `json:"role"`
	Content       string        `json:"content"`
	Thinking      string        `json:"thinking,omitempty"`
	ToolUse       *ToolUseBlock `json:"tool_use,omitempty"`
	ToolResult    string        `json:"tool_result,omitempty"`
	ToolUseID     string        `json:"tool_use_id,omitempty"`
	Subtype       string        `json:"subtype,omitempty"`
	Attachment    string        `json:"attachment,omitempty"`
	Timestamp     string        `json:"timestamp"`
}

type ToolUseBlock struct {
	Name  string                 `json:"name"`
	ID    string                 `json:"id,omitempty"`
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

type dirEntryInfo struct {
	name    string
	modTime time.Time
}

type sessionsCache struct {
	mu        sync.RWMutex
	sessions  []HistorySession
	dirModMap map[string]time.Time // projectDir -> last mod time of the dir
	expiresAt time.Time
}

var cache = &sessionsCache{}

func ClaudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

func DiscoverSessions() ([]HistorySession, error) {
	cache.mu.RLock()
	if time.Now().Before(cache.expiresAt) && cache.sessions != nil {
		sessions := cache.sessions
		cache.mu.RUnlock()
		return sessions, nil
	}
	cache.mu.RUnlock()

	return refreshCache()
}

func refreshCache() ([]HistorySession, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if time.Now().Before(cache.expiresAt) && cache.sessions != nil {
		return cache.sessions, nil
	}

	projectsDir, err := ClaudeProjectsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	dirMods := make(map[string]time.Time)
	var dirInfos []dirEntryInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		dirInfos = append(dirInfos, dirEntryInfo{name: entry.Name(), modTime: info.ModTime()})
		dirMods[entry.Name()] = info.ModTime()
	}

	// Fast path: directory mod times unchanged → reuse cache
	if len(dirMods) > 0 && len(cache.dirModMap) == len(dirMods) {
		same := true
		for name, mod := range dirMods {
			if cached, ok := cache.dirModMap[name]; !ok || !cached.Equal(mod) {
				same = false
				break
			}
		}
		if same && cache.sessions != nil {
			cache.expiresAt = time.Now().Add(30 * time.Second)
			return cache.sessions, nil
		}
	}

	var sessions []HistorySession

	for _, di := range dirInfos {
		fallbackPath := DecodeProjectName(di.name)
		projectDir := filepath.Join(projectsDir, di.name)

		jsonlFiles, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, jsonlPath := range jsonlFiles {
			sessionID := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
			if sessionID == "" {
				continue
			}

			// Extract real cwd from JSONL content (avoids DecodeProjectName encoding loss,
			// e.g. G--dev-AI-plant-explorer → G:\dev\AI\plant-explorer instead of plant_explorer)
			projectPath := fallbackPath
			firstPrompt, msgCount, created, modified, gitBranch, isSidechain := scanSessionFile(jsonlPath, &projectPath)
			if isSidechain {
				continue
			}

			sessions = append(sessions, HistorySession{
				ID:           sessionID,
				FirstPrompt:  firstPrompt,
				MessageCount: msgCount,
				Created:      created,
				Modified:     modified,
				ProjectPath:  projectPath,
				FilePath:     jsonlPath,
				GitBranch:    gitBranch,
			})
		}
	}

	cache.sessions = sessions
	cache.dirModMap = dirMods
	cache.expiresAt = time.Now().Add(30 * time.Second)
	return sessions, nil
}

func FindSession(id string) *HistorySession {
	sessions, _ := DiscoverSessions()
	for i := range sessions {
		if sessions[i].ID == id {
			return &sessions[i]
		}
	}
	return nil
}

func InvalidateSessionCache() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.expiresAt = time.Time{}
}

func scanSessionFile(path string, outProjectPath *string) (firstPrompt string, msgCount int, created, modified, gitBranch string, isSidechain bool) {
	info, err := os.Stat(path)
	if err == nil {
		created = info.ModTime().Format(time.RFC3339)
		modified = created
	}

	f, err := os.Open(path)
	if err != nil {
		return "", 0, created, modified, "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var firstTS, lastTS string
	var cwdFound bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		t, _ := raw["type"].(string)

		if sc, ok := raw["isSidechain"].(bool); ok && sc {
			isSidechain = true
		}
		if gb, ok := raw["gitBranch"].(string); ok && gb != "" {
			gitBranch = gb
		}

		// Extract real cwd from JSONL to fix DecodeProjectName encoding bugs.
		// Use only the FIRST cwd — it's the original project directory.
		// Subsequent cwds may reflect cd commands and are not the real project root.
		if !cwdFound {
			if cwd, ok := raw["cwd"].(string); ok && cwd != "" && filepath.IsAbs(cwd) {
				if info, err := os.Stat(cwd); err == nil && info.IsDir() {
					if outProjectPath != nil {
						*outProjectPath = cwd
					}
					cwdFound = true
				}
			}
		}

		if t == "user" && firstPrompt == "" {
			if message, ok := raw["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					firstPrompt = content
				} else if contentArr, ok := message["content"].([]interface{}); ok {
					for _, c := range contentArr {
						if cm, ok := c.(map[string]interface{}); ok {
							if cm["type"] == "text" {
								if txt, ok := cm["text"].(string); ok {
									firstPrompt = txt
									break
								}
							}
						}
					}
				}
			}
			if len(firstPrompt) > 100 {
				firstPrompt = firstPrompt[:100] + "..."
			}
		}

		if t == "user" || t == "assistant" {
			msgCount++
		}

		if ts, ok := raw["timestamp"].(string); ok && ts != "" {
			if firstTS == "" {
				firstTS = ts
			}
			lastTS = ts
		}
	}

	if firstTS != "" {
		created = firstTS
	}
	if lastTS != "" {
		modified = lastTS
	}

	return firstPrompt, msgCount, created, modified, gitBranch, isSidechain
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

func extractToolResultContent(val interface{}) string {
	switch v := val.(type) {
	case string:
		return truncate(v, 300)
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						return truncate(t, 300)
					}
				}
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
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
			} else if contentArr, ok := message["content"].([]interface{}); ok {
				hasText := false
				for _, c := range contentArr {
					cm, _ := c.(map[string]interface{})
					ct, _ := cm["type"].(string)
					switch ct {
					case "text":
						txt, _ := cm["text"].(string)
						msg.Content += txt
						hasText = true
					case "tool_result":
						msg.ToolResult = extractToolResultContent(cm["content"])
						if id, ok := cm["tool_use_id"].(string); ok {
							msg.ToolUseID = id
						}
					}
				}
				if !hasText && msg.ToolResult != "" {
					msg.Role = "tool_result"
				}
			}
		}
		if tur, ok := raw["toolUseResult"].([]interface{}); ok && msg.ToolResult == "" {
			msg.ToolResult = extractToolResultContent(tur)
		}
	case "assistant":
		msg.Role = "assistant"
		if message, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				msg.Content = content
			} else if contentArr, ok := message["content"].([]interface{}); ok {
				for _, c := range contentArr {
					cm, _ := c.(map[string]interface{})
					ct, _ := cm["type"].(string)
					switch ct {
					case "text":
						txt, _ := cm["text"].(string)
						msg.Content += txt
					case "thinking":
						think, _ := cm["thinking"].(string)
						msg.Thinking = think
					case "tool_use":
						name, _ := cm["name"].(string)
						id, _ := cm["id"].(string)
						input, _ := cm["input"].(map[string]interface{})
						msg.ToolUse = &ToolUseBlock{Name: name, ID: id, Input: input}
					}
				}
			}
		}
	case "attachment":
		if att, ok := raw["attachment"].(map[string]interface{}); ok {
			at, _ := att["type"].(string)
			msg.Attachment = at
		}
	case "system":
		msg.Role = "system"
		if sub, ok := raw["subtype"].(string); ok {
			msg.Subtype = sub
		}
	case "file-history-snapshot":
		msg.Role = "system"
		msg.Content = "[文件历史快照]"
	case "summary":
		msg.Role = "system"
		if s, ok := raw["summary"].(string); ok && s != "" {
			msg.Content = s
			if len(msg.Content) > 200 {
				msg.Content = msg.Content[:200] + "..."
			}
		} else {
			msg.Content = "[会话摘要]"
		}
	case "progress":
		msg.Role = "system"
		if d, ok := raw["data"].(map[string]interface{}); ok {
			if pct, ok := d["percentage"]; ok {
				msg.Content = fmt.Sprintf("进度: %v%%", pct)
			}
		}
		if msg.Content == "" {
			msg.Content = "[进度更新]"
		}
	case "queue-operation":
		msg.Role = "system"
		op, _ := raw["operation"].(string)
		if content, ok := raw["content"].(string); ok && content != "" {
			msg.Content = fmt.Sprintf("队列 %s: %s", op, content)
		} else {
			msg.Content = fmt.Sprintf("[队列操作: %s]", op)
		}
	default:
		return nil
	}
	return msg
}

func DecodeProjectName(encoded string) string {
	if runtime.GOOS == "windows" {
		parts := strings.Split(encoded, "--")
		if len(parts) < 2 {
			return encoded
		}
		drive := parts[0] + ":"
		rest := strings.Join(parts[1:], "\\")
		return drive + "\\" + rest
	}
	return strings.ReplaceAll(encoded, "--", "/")
}