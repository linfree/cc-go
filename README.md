# cc-go

> 通过微信机器人接管 Claude Code，实现随时随地编码。

cc-go 是一款基于 [Claude Code](https://claude.ai/code) 的远程编码工具。通过微信机器人接管 Claude Code 会话，你可以在手机上批准权限请求、查看 AI 回复、启动/切换会话，真正做到随时随地编码。

## Architecture

```
┌─────────────┐     HTTP/WS      ┌──────────────┐    stdin/stdout    ┌────────────┐
│  微信客户端   │ ◄──────────────► │   cc-go      │ ◄────────────────► │ Claude Code │
│  (手机)      │                  │  (Web + Bot)  │   stream-json     │ (CLI进程)   │
└─────────────┘                  └──────┬───────┘                    └────────────┘
                                        │
                                        ▼
                                 ┌──────────────┐
                                 │  Web 管理界面   │
                                 │  (React + SPA) │
                                 └──────────────┘
```

## Features

- **微信远程控制** — 通过微信消息启动/停止/切换 Claude Code 会话
- **权限审批** — 实时处理 Claude Code 的工具使用权限请求（批准/拒绝/回答提问）
- **Web 管理面板** — 仪表盘、会话列表、实时聊天、日志查看、系统设置
- **会话管理** — 新建/恢复/删除会话，查看对话历史和消息数量
- **Claude 输出推送** — 将 AI 回答、工具调用实时推送到微信
- **WebSocket 实时推送** — 浏览器实时接收会话状态和权限事件

## Screenshots

<!-- TODO: Add screenshots -->
<!--
![Dashboard](docs/screenshots/dashboard.png)
![Session Chat](docs/screenshots/session-chat.png)
![WeChat Bind](docs/screenshots/wechat-bind.png)
![Settings](docs/screenshots/settings.png)
-->

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- [Claude Code](https://claude.ai/code) installed and authenticated
- A WeChat account with iLink Bot API access

### Build

```bash
# Install frontend dependencies
cd web && npm install

# Build frontend
npm run build

# Build Go binary
cd .. && go build -o cc-go.exe ./cmd/cc-go/
```

### Run

```bash
# First run (will create default config at ~/.cc-go/config.json)
./cc-go.exe start

# Service will start on http://localhost:18080
# Open browser to scan WeChat QR code and configure Claude CLI path
```

## Configuration

Config file: `~/.cc-go/config.json`

| Field | Type | Description |
|-------|------|-------------|
| `web_port` | int | Web server port (default: 18080) |
| `permission_mode` | string | Claude Code permission mode (default/acceptEdits/bypass) |
| `claude_cli_path` | string | Claude CLI executable path |
| `auto_find_claude` | bool | Auto-detect Claude CLI on startup |
| `claude_env_vars` | string | Additional env vars for Claude (dotenv format) |
| `push_types` | string[] | Push notification types: permission, claude_response, tool_use, session_status |

### Bot Commands (WeChat)

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/sessions` | List recent sessions |
| `/switch <id>` | Switch to a session |
| `/status` | Check current session status |
| `/stop` | Stop current session |
| `/y [n/all]` | Approve permission request(s) |
| `/n [n/all]` | Reject permission request(s) |
| `/r <answer>` | Answer AskUserQuestion |

## Tech Stack

- **Backend**: Go, Gin, gorilla/websocket (coder/websocket)
- **Frontend**: React 19, TypeScript, Vite, Tailwind CSS 4
- **Protocol**: Claude Code stream-json (stdin/stdout)
- **WeChat Bot**: iLink Bot API (`ilinkai.weixin.qq.com`)
- **Storage**: BoltDB (embedded key-value store)