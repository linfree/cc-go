# cc-go 设计文档

> 通过微信机器人远程管理 Claude Code 的 Go 工具
> 版本：1.0 | 日期：2026-05-10

---

## 1. 概述

cc-go 是一个单二进制的 Go 工具，通过微信 iLink Bot 官方协议接入微信，以子进程方式管理 Claude Code CLI 会话，实现"人在电脑前用 Claude Code 干活，走开后通过微信机器人继续同一个会话"的场景。

### 核心特性

- 单用户、单二进制分发（Go embed 嵌入 React SPA）
- 微信 iLink Bot 官方协议（扫码绑定、长轮询收消息）
- Claude Code 子进程管理（新建会话 / `--resume` 接管历史会话）
- 权限请求全部推送微信审批（用户回复 Y/N）
- Web 界面管理配置、查看会话、聊天记录
- CLI 启动后自动打开浏览器

### 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go (cobra + gin) |
| 前端 | React + Ant Design |
| 存储 | SQLite (动态状态) + JSON 配置文件 |
| 通信 | Claude stream-json 协议 / 微信 iLink HTTP API / WebSocket |

---

## 2. 架构

单体架构，所有模块编译到一个二进制文件中。

```
cc-go (单个 Go 二进制)
├── CLI 入口 (cobra)
├── Web Server (gin) + embedded React SPA
├── WeChat Bot Service (iLink 长轮询)
├── Claude Session Manager (子进程管理)
├── Message Bridge (WeChat <-> Claude 消息路由)
├── SQLite (会话动态状态)
└── JSON Config (~/.cc-go/config.json)
```

模块间依赖关系：

```
CLI ──> Server ──> Bridge ──> Claude (子进程)
                 ──> WeChat (HTTP)
                 ──> Store (SQLite)
                 ──> Config (JSON)
```

---

## 3. 项目结构

```
cc-go/
├── cmd/
│   └── cc-go/
│       └── main.go
├── internal/
│   ├── config/              # 配置管理 (~/.cc-go/config.json)
│   ├── store/               # SQLite 存储 (sessions 动态状态)
│   ├── wechat/              # 微信 iLink Bot 客户端
│   │   ├── client.go        # HTTP 客户端、鉴权、长轮询
│   │   ├── message.go       # 消息收发、类型解析
│   │   └── reconnect.go     # 自动重连逻辑
│   ├── claude/              # Claude Code 子进程管理
│   │   ├── session.go       # 会话生命周期 (spawn/kill/resume)
│   │   ├── protocol.go      # stream-json 协议解析
│   │   ├── history.go       # 读取 Claude 历史会话文件
│   │   └── finder.go        # 自动查找 claude CLI 路径
│   ├── bridge/              # 消息桥接层 (WeChat <-> Claude)
│   │   └── bridge.go        # 消息路由、权限请求推送、响应回传
│   └── server/              # Web 服务
│       ├── router.go        # HTTP 路由 (gin)
│       ├── api/             # REST API handlers
│       └── ws/              # WebSocket (实时事件推送)
├── web/                     # React SPA 前端源码
│   ├── src/
│   │   ├── pages/
│   │   │   ├── WechatBind.tsx
│   │   │   ├── SessionList.tsx
│   │   │   ├── SessionChat.tsx
│   │   │   ├── ClaudeConfig.tsx
│   │   │   ├── PushSettings.tsx
│   │   │   └── Settings.tsx
│   │   └── components/
│   └── package.json
├── migrations/              # SQLite 迁移脚本
└── Makefile
```

---

## 4. 数据模型与存储

### 4.1 SQLite `~/.cc-go/cc-go.db`

只存动态状态数据，一张表：

**sessions 表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PRIMARY KEY | Claude Code 的 session_id (UUID) |
| name | TEXT | 会话显示名称 |
| work_dir | TEXT | 工作目录 |
| model | TEXT | 使用的模型 |
| status | TEXT | active / idle / stopped / error |
| claude_pid | INTEGER | 当前子进程 PID (0 = 未启动) |
| created_at | DATETIME | 创建时间 |
| last_active_at | DATETIME | 最后活跃时间 |
| history_path | TEXT | Claude 历史文件路径 |

### 4.2 JSON 配置 `~/.cc-go/config.json`

```json
{
  "claude_cli_path": "/usr/local/bin/claude",
  "auto_find_claude": true,
  "permission_mode": "default",
  "language": "zh-CN",
  "web_port": 18080,
  "auto_open_browser": true,
  "wechat": {
    "bot_token": "",
    "base_url": "https://ilinkai.weixin.qq.com",
    "login_time": ""
  },
  "push_types": ["permission", "claude_response", "tool_use", "session_status"]
}
```

**push_types 列表**：在列表里的消息类型就推送，不在就不推送。`permission` 硬编码强制存在，用户无法移除。后续新增消息类型只需加字符串。

### 4.3 内存数据

- 微信连接状态（connected / disconnected）
- 长轮询游标 `get_updates_buf`
- 最近联系人信息
- 当前活跃会话 ID 映射

---

## 5. 核心流程

### 5.1 启动流程

```
cc-go start (CLI)
  │
  ├─ 1. 读取 ~/.cc-go/config.json
  │     ├─ 不存在 → 创建默认配置
  │     └─ 存在 → 加载
  │
  ├─ 2. 检查 claude CLI 路径
  │     ├─ config.claude_cli_path 有效 → 跳过
  │     ├─ auto_find_claude=true → 搜索 PATH / 常见路径
  │     └─ 找不到 → 标记 need_setup_claude=true
  │
  ├─ 3. 检查 wechat 配置
  │     ├─ bot_token 存在且有效 → 后台启动长轮询
  │     ├─ bot_token 为空或过期 → need_setup_wechat=true
  │     └─ 连接失败 → need_setup_wechat=true
  │
  ├─ 4. 启动 Web Server
  │     └─ 自动打开浏览器
  │
  └─ 5. 前端根据 need_setup 跳转
        ├─ need_setup_wechat → /wechat
        ├─ need_setup_claude → /claude
        └─ 都正常 → /sessions
```

### 5.2 微信消息 → Claude Code

```
用户在微信发消息
  │
  ▼
wechat/client.go 长轮询收到消息
  │
  ▼
bridge.go 判断当前是否有活跃会话
  │
  ├─ 无活跃会话 → 回复"请先在 Web 界面选择/启动会话"
  │
  └─ 有活跃会话
       │
       ▼
    claude/protocol.go 写入 stdin
       {"type":"user","message":{"role":"user","content":"..."}}
       │
       ▼
    读取 stdout 事件流
       ├─ assistant (text) → 微信发送回复
       ├─ assistant (tool_use) → 按 push_types 配置决定是否推送
       ├─ control_request → 强制推送权限审批到微信
       │   用户回复 Y → control_response (allow)
       │   用户回复 N → control_response (deny)
       ├─ result → 按 push_types 配置推送状态
       └─ error → 推送错误信息
```

### 5.3 权限审批交互

```
Claude Code 需要执行 Bash: "rm -rf /tmp/test"
  │
  ▼
bridge 收到 control_request
  │
  ▼
微信推送:
  "⚠️ 权限请求
   工具: Bash
   命令: rm -rf /tmp/test
   回复 Y 批准 / N 拒绝"
  │
  ▼
用户回复 "Y" 或 "N"
  │
  ▼
bridge 写入 control_response 到 claude stdin
  │
  ▼
Claude Code 执行或跳过
```

### 5.4 Web 界面接管会话

```
用户在 Web 界面点击"接管会话"
  │
  ├─ 选择已有历史会话
  │     → spawn: claude --resume <session_id> --permission-prompt-tool stdio ...
  │
  └─ 启动新会话
        → spawn: claude -p --input-format stream-json ...
  │
  ▼
bridge 绑定为"活跃会话"
  │
  ▼
微信端收到通知: "已接管会话: xxx，工作目录: /path/to/project"
  │
  ▼
后续微信消息路由到该会话
```

### 5.5 微信重连

复用自动重连逻辑，二维码展示改为：
- 连接还在 → 推送二维码链接到微信
- 连接已断 → Web 界面展示二维码

---

## 6. 微信指令系统

| 指令 | 功能 |
|------|------|
| `/help` | 查看指令列表 |
| `/sessions` | 列出所有可用会话 |
| `/switch <id>` | 切换当前接管的会话 |
| `/status` | 查看当前会话状态 + 连接剩余时间 |
| `/stop` | 停止当前会话 |
| `/y` `/n` | 权限审批（上下文感知） |
| `/time` | 查询微信连接剩余时间 |
| `/reconnect` | 手动触发微信重连 |
| 其他文本 | 转发给当前活跃的 Claude Code 会话 |

---

## 7. Claude 子进程生命周期

状态机：`stopped → starting → active → stopping → stopped`，异常时 `active → error`。

**启动参数**：
```bash
claude -p \
  --input-format stream-json \
  --output-format stream-json \
  --verbose \
  --permission-mode <config.permission_mode> \
  --permission-prompt-tool stdio \
  --resume <session_id> \    # 接管时才有
  --max-turns 0              # 0 = 无限制，持续交互
```

**关键约束**：
- 删除 `CLAUDECODE` 环境变量（避免嵌套检测）
- stdin/stdout pipe 模式
- stderr 记录日志

**停止流程**：
1. 关闭 stdin
2. 等待进程退出（最多 10s）
3. 超时则 Kill

---

## 8. Claude 历史会话发现

Claude Code 会话历史存储在 `~/.claude/projects/` 目录下，按项目路径组织。

1. 扫描 `$HOME/.claude/projects/` 下所有 JSONL 文件（Windows 为 `%USERPROFILE%\.claude\projects\`，需要 finder.go 和 history.go 做跨平台路径适配）
2. 解析头部获取 session_id、创建时间、工作目录
3. 将元信息同步写入 SQLite `sessions` 表
4. 聊天内容直接读取 JSONL 文件返回前端，不存 SQLite

---

## 9. 消息格式限制

微信单条消息约 4096 字符限制：
- 超长文本按 3500 字符分片发送
- 每片末尾标注 `[1/3]`、`[2/3]`...
- 工具调用 input JSON 超长时截断显示

---

## 10. Web API

```
基础路径: /api/v1

# 微信
GET  /wechat/qrcode          # 获取登录二维码
GET  /wechat/status          # 查询连接状态
POST /wechat/disconnect      # 断开连接

# Claude CLI
GET  /claude/path             # 获取当前路径
POST /claude/path             # 手动设置路径
POST /claude/auto-detect      # 自动查找

# 会话
GET    /sessions              # 列出所有会话
GET    /sessions/:id          # 会话详情
GET    /sessions/:id/history  # 聊天记录（读 JSONL）
POST   /sessions/start        # 启动新会话
POST   /sessions/:id/resume   # 接管历史会话
POST   /sessions/:id/stop     # 停止会话
DELETE /sessions/:id          # 删除会话记录

# 推送
GET  /push/types              # 所有可配置消息类型
GET  /push/settings           # 当前启用的推送类型
PUT  /push/settings           # 更新推送类型列表

# 设置
GET  /settings                # 获取全部设置
PUT  /settings                # 更新设置

# WebSocket
WS   /ws/events               # 实时事件流
```

**WebSocket 事件**：
```json
{"event": "session_status_changed", "session_id": "xxx", "status": "active"}
{"event": "wechat_status_changed", "status": "connected"}
{"event": "claude_output", "session_id": "xxx", "type": "text", "content": "..."}
{"event": "permission_request", "session_id": "xxx", "tool": "Bash", "input": {...}}
```

---

## 11. Web 界面

### 页面路由

```
/ (Layout: 左侧导航 + 右侧内容)
├── /wechat          微信连接
├── /sessions        会话管理（主页）
├── /sessions/:id    会话聊天记录
├── /claude          Claude CLI 配置
├── /push            推送设置
└── /settings        系统设置
```

### 各页面

**微信连接 `/wechat`**
- 连接状态指示灯（绿/红/灰）
- 二维码展示区
- 最近联系人、连接剩余时间倒计时
- 断开/重新连接按钮
- 未配置时自动跳转此页

**会话管理 `/sessions`（主页）**
- 当前被微信接管的会话卡片（高亮）
- 所有历史会话列表（表格）
- 操作：启动新会话、接管、停止、删除

**会话聊天记录 `/sessions/:id`**
- 聊天气泡界面（读 Claude 历史文件渲染）
- 用户消息靠右，Claude 靠左
- 工具调用折叠展示
- 活跃会话通过 WebSocket 实时追加

**Claude CLI 配置 `/claude`**
- 当前路径显示
- 自动检测 / 手动输入
- 路径验证（执行 `claude --version`）
- 找不到 claude 时自动跳转此页

**推送设置 `/push`**
- 消息类型列表，每行一个开关
- `permission` 固定开启，开关禁用灰显
- 保存按钮

**系统设置 `/settings`**
- 语言、端口、自动打开浏览器
- 权限模式选择
- 配置导入/导出

---

## 12. 参考资源

- [WeChat iLink Bot API](https://github.com/hao-ji-xing/openclaw-weixin)
- [Claude Code stream-json 协议](../claude-code-subprocess-integration.md)
- [cc-connect (Go 实现)](https://github.com/chenhg5/cc-connect)
- [claude-agent-sdk-go](https://github.com/anthropics/claude-agent-sdk-go)
