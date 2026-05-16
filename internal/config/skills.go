package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var builtinSkills = map[string]bool{
	"wechat-notify": true,
}

func IsBuiltinSkill(name string) bool {
	return builtinSkills[name]
}

func SkillsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "skills"), nil
}

func ClaudeCodeSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "skills")
}

func SeedDefaultSkills() {
	dir, err := SkillsDir()
	if err != nil {
		return
	}

	wechatNotify := filepath.Join(dir, "wechat-notify", "SKILL.md")
	if _, err := os.Stat(wechatNotify); err == nil {
		return
	}

	content := `---
name: wechat-notify
description: 通过微信机器人发送通知消息（文本、图片、文件、视频）
allowed-tools: Bash(curl *)
---

通过本地 Bot API 向用户发送微信通知。

## 获取端口

端口配置在 ` + "`~/.cc-go/config.json`" + ` 的 ` + "`web_port`" + ` 字段中，发送请求前先读取：

` + "```" + `bash
PORT=$(grep -o '"web_port":[[:space:]]*[0-9]*' ~/.cc-go/config.json | grep -o '[0-9]*')
` + "```" + `

## 发送文本消息

` + "```" + `bash
curl -s -X POST http://localhost:$PORT/api/v1/wechat-bot/send/text \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"text":"通知内容"}'
` + "```" + `

## 发送图片

` + "```" + `bash
curl -s -X POST http://localhost:$PORT/api/v1/wechat-bot/send/image \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"file_path":"C:/path/to/image.png"}'
` + "```" + `

## 发送文件

` + "```" + `bash
curl -s -X POST http://localhost:$PORT/api/v1/wechat-bot/send/file \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"file_path":"C:/path/to/document.pdf"}'
` + "```" + `

## 发送视频

` + "```" + `bash
curl -s -X POST http://localhost:$PORT/api/v1/wechat-bot/send/video \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"file_path":"C:/path/to/video.mp4"}'
` + "```" + `

## 返回值

- ` + "`{\"status\":\"sent\"}`" + ` — 已发送
- ` + "`{\"status\":\"buffered\"}`" + ` — 已排队，等待预算重置后发送

## 适用场景

- 长时间任务完成通知
- 错误或重要状态变更提醒
- 主动推送结果或摘要
`
	os.MkdirAll(filepath.Dir(wechatNotify), 0755)
	os.WriteFile(wechatNotify, []byte(content), 0644)
}

func parseFrontmatterField(skillFile, field string) string {
	f, err := os.Open(skillFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if inFrontmatter && strings.HasPrefix(line, field+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, field+":"))
		}
	}
	return ""
}

func discoverSkillsFrom(dir string) []SkillConfig {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []SkillConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}

		name := parseFrontmatterField(skillFile, "name")
		if name == "" {
			name = e.Name()
		}
		desc := parseFrontmatterField(skillFile, "description")

		skills = append(skills, SkillConfig{
			Name:        name,
			Description: desc,
			Enabled:     true,
				Builtin:     IsBuiltinSkill(name),
		})
	}
	return skills
}

func DiscoverSkills() []SkillConfig {
	dir, err := SkillsDir()
	if err != nil {
		return nil
	}
	return discoverSkillsFrom(dir)
}

func DiscoverClaudeCodeSkills() []SkillConfig {
	dir := ClaudeCodeSkillsDir()
	if dir == "" {
		return nil
	}
	return discoverSkillsFrom(dir)
}

func DeleteSkill(name string) error {
	dir, err := SkillsDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir, name))
}

func ImportSkills(names []string) error {
	srcDir := ClaudeCodeSkillsDir()
	dstDir, err := SkillsDir()
	if err != nil {
		return err
	}
	for _, name := range names {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(dstDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		os.RemoveAll(dst)
		if err := copyDir(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) InjectSkills(workDir string) {
	if workDir == "" {
		return
	}

	skillsSrcDir, err := SkillsDir()
	if err != nil {
		return
	}
	projectSkillsDir := filepath.Join(workDir, ".claude", "skills")

	enabledMap := make(map[string]bool)
	for _, s := range c.Skills {
		enabledMap[s.Name] = s.Enabled
	}

	discovered := DiscoverSkills()
	for _, d := range discovered {
		targetPath := filepath.Join(projectSkillsDir, d.Name)
		enabled := enabledMap[d.Name]
		if !enabled {
			if _, ok := enabledMap[d.Name]; ok {
				os.RemoveAll(targetPath)
			}
			continue
		}
		if _, err := os.Stat(targetPath); err == nil {
			continue
		}
		srcPath := filepath.Join(skillsSrcDir, d.Name)
		copyDir(srcPath, targetPath)
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
