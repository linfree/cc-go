package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type BotCommand struct {
	Key         string `json:"key"`
	Keyword     string `json:"keyword"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type SkillConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Builtin     bool   `json:"builtin"`
}

type Config struct {
	ClaudeCLIPath   string       `json:"claude_cli_path"`
	AutoFindClaude  bool         `json:"auto_find_claude"`
	PermissionMode  string       `json:"permission_mode"`
	Language        string       `json:"language"`
	WebPort         int          `json:"web_port"`
	AutoOpenBrowser  bool         `json:"auto_open_browser"`
	AutoResumeLatest bool         `json:"auto_resume_latest"`
	ClaudeEnvVars    string       `json:"claude_env_vars"`
	Wechat          WechatConfig `json:"wechat"`
	PushTypes       []string     `json:"push_types"`
	BotCommands     []BotCommand `json:"bot_commands"`
	Skills          []SkillConfig `json:"skills"`
	ActualPort      int          `json:"actual_port"` // runtime: actual listening port, may differ from WebPort
}

type WechatConfig struct {
	BotToken                  string `json:"bot_token"`
	BaseURL                   string `json:"base_url"`
	CDNBaseUrl                string `json:"cdn_base_url"`
	LoginTime                 string `json:"login_time"`
	LastFromID                string `json:"last_from_id"`
	LastContextToken          string `json:"last_context_token"`
	SendBudgetLimit           int    `json:"send_budget_limit"`
	MaxBufferedMessages       int    `json:"max_buffered_messages"`
	ActivationWarningHours    int    `json:"activation_warning_hours"`
	ActivationReminderMinutes int    `json:"activation_reminder_minutes"`
}

func (w WechatConfig) GetSendBudgetLimit() int {
	if w.SendBudgetLimit <= 0 {
		return 7
	}
	return w.SendBudgetLimit
}
func (w WechatConfig) GetMaxBufferedMessages() int {
	if w.MaxBufferedMessages <= 0 {
		return 100
	}
	return w.MaxBufferedMessages
}
func (w WechatConfig) GetActivationWarningHours() int {
	if w.ActivationWarningHours <= 0 {
		return 20
	}
	return w.ActivationWarningHours
}
func (w WechatConfig) GetActivationReminderMinutes() int {
	if w.ActivationReminderMinutes <= 0 {
		return 60
	}
	return w.ActivationReminderMinutes
}

func (w WechatConfig) GetCDNBaseUrl() string {
	if w.CDNBaseUrl == "" {
		return "https://novac2c.cdn.weixin.qq.com/c2c"
	}
	return w.CDNBaseUrl
}

func (w WechatConfig) Normalize() WechatConfig {
	w.SendBudgetLimit = w.GetSendBudgetLimit()
	w.MaxBufferedMessages = w.GetMaxBufferedMessages()
	w.ActivationWarningHours = w.GetActivationWarningHours()
	w.ActivationReminderMinutes = w.GetActivationReminderMinutes()
	return w
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
			BotToken:                  "",
			BaseURL:                   "https://ilinkai.weixin.qq.com",
			LoginTime:                 "",
			SendBudgetLimit:           7,
			MaxBufferedMessages:       100,
			ActivationWarningHours:    21,
			ActivationReminderMinutes: 60,
		},
		PushTypes:   []string{"permission", "claude_response", "tool_use", "session_status"},
		BotCommands: DefaultBotCommands(),
	}
}

func DefaultBotCommands() []BotCommand {
	return []BotCommand{
		{Key: "help", Keyword: "/help", Description: "查看帮助", Enabled: true},
		{Key: "sessions", Keyword: "/sessions", Description: "列出会话", Enabled: true},
		{Key: "switch", Keyword: "/switch", Description: "切换会话 (例: /switch 3)", Enabled: true},
		{Key: "status", Keyword: "/status", Description: "查看当前会话状态", Enabled: true},
		{Key: "stop", Keyword: "/stop", Description: "停止当前会话", Enabled: true},
		{Key: "y", Keyword: "/y", Description: "批准权限请求 (例: /y 或 /y 3 或 /y all)", Enabled: true},
		{Key: "n", Keyword: "/n", Description: "拒绝权限请求 (例: /n 或 /n 3 或 /n all)", Enabled: true},
		{Key: "r", Keyword: "/r", Description: "回答提问 (例: /r 是的)", Enabled: true},
		{Key: "activate", Keyword: "/", Description: "激活消息轮次", Enabled: true},
		{Key: "relogin", Keyword: "/relogin", Description: "重新登录机器人", Enabled: true},
	}
}


func (c *Config) BotCommandByKeyword(keyword string) *BotCommand {
	for i := range c.BotCommands {
		if c.BotCommands[i].Keyword == keyword {
			return &c.BotCommands[i]
		}
	}
	return nil
}

func (c *Config) BotCommandByKey(key string) *BotCommand {
	for i := range c.BotCommands {
		if c.BotCommands[i].Key == key {
			return &c.BotCommands[i]
		}
	}
	return nil
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
			SeedDefaultSkills()
			cfg := DefaultConfig()
			cfg.Skills = ensureSkills(cfg.Skills)
			return cfg, cfg.Save()
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	SeedDefaultSkills()
	cfg.Wechat = cfg.Wechat.Normalize()
	cfg.PushTypes = ensurePermission(cfg.PushTypes)
	cfg.BotCommands = ensureBotCommands(cfg.BotCommands)
	cfg.Skills = ensureSkills(cfg.Skills)
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
	c.PushTypes = ensurePermission(c.PushTypes)
	c.BotCommands = ensureBotCommands(c.BotCommands)
	c.Skills = ensureSkills(c.Skills)
	c.Wechat = c.Wechat.Normalize()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

func (c *Config) IsPushEnabled(t string) bool {
	for _, pt := range c.PushTypes {
		if pt == t {
			return true
		}
	}
	return false
}

// ParseEnvVars parses the ClaudeEnvVars dotenv string into KEY=VALUE pairs.
// Blank lines and # comments are skipped.
func (c *Config) ParseEnvVars() []string {
	return ParseDotEnv(c.ClaudeEnvVars)
}

func ParseDotEnv(raw string) []string {
	var result []string
	for _, line := range splitLines(raw) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		idx := indexEqual(line)
		if idx < 0 {
			continue
		}
		key := trimSpace(line[:idx])
		val := trimSpace(line[idx+1:])
		// Strip optional surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" {
			result = append(result, key+"="+val)
		}
	}
	return result
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func indexEqual(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return i
		}
	}
	return -1
}

func ensurePermission(types []string) []string {
	for _, t := range types {
		if t == "permission" {
			return types
		}
	}
	return append([]string{"permission"}, types...)
}

func ensureBotCommands(cmds []BotCommand) []BotCommand {
	if len(cmds) == 0 {
		return DefaultBotCommands()
	}
	// Merge missing default commands into existing config
	defaults := DefaultBotCommands()
	for _, d := range defaults {
		found := false
		for _, c := range cmds {
			if c.Key == d.Key {
				found = true
				break
			}
		}
		if !found {
			cmds = append(cmds, d)
		}
	}
	return cmds
}

func ensureSkills(skills []SkillConfig) []SkillConfig {
	discovered := DiscoverSkills()
	if len(discovered) == 0 {
		return nil
	}
	// Build result from discovered skills, preserving enabled state from config
	enabledMap := make(map[string]bool)
	for _, s := range skills {
		enabledMap[s.Name] = s.Enabled
	}
	result := make([]SkillConfig, 0, len(discovered))
	for _, d := range discovered {
		enabled := true
		if e, ok := enabledMap[d.Name]; ok {
			enabled = e
		}
		result = append(result, SkillConfig{Name: d.Name, Description: d.Description, Enabled: enabled, Builtin: d.Builtin})
	}
	return result
}

func (c *Config) SyncSkills() {
	c.Skills = ensureSkills(c.Skills)
}
