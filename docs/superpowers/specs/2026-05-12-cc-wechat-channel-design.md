# cc-wechat 微信通道设计

## 概述

`cc-wechat` 是一个 Claude Code Channel 插件，基于微信 iLink Bot API，让用户通过微信消息与 Claude Code 进行双向对话。

通道是 MCP 服务器，Claude Code 通过 stdio 启动并管理它。`cc-wechat` 使用 Bun 运行时和 `@modelcontextprotocol/sdk`。

## 项目结构

```
cc-wechat/
├── package.json
├── tsconfig.json
├── src/
│   ├── index.ts          # 入口，串联 WeChatClient + MCP Server + 轮询
│   ├── wechat-client.ts  # 微信 iLink Bot HTTP API 客户端
│   ├── channel.ts        # MCP 通道服务器，stdio transport
│   ├── types.ts          # 共享类型定义
│   └── store.ts          # session/state 持久化（JSON 文件）
└── state/                # .gitignore，运行时数据
    ├── session.json      # bot_token, get_updates_buf, login_time
    └── qrcode.txt        # 登录二维码 URL（登录期间）
```

## 数据流

```
微信用户 → iLink API (长轮询) → WeChatClient.pollLoop()
  → channel.sendNotification('notifications/claude/channel')
  → Claude Code → Claude 处理

Claude 调用 reply 工具 → channel.toolHandler('reply')
  → WeChatClient.sendMessage() → iLink API → 微信用户
```

通道实例事件包含 meta 信息：
- `chat_id`: 微信用户 ID（如 `o9cq800kum_xxx@im.wechat`），Claude reply 时回传
- `context_token`：微信消息上下文 token，reply 时必填

## 核心组件

### wechat-client.ts — 微信 API 封装

纯 HTTP 客户端，不依赖 MCP 框架。直接翻译自 Go 版 `internal/wechat/client.go`。

核心方法：
- `login()`: 获取二维码 → 轮询扫码状态 → 返回 bot_token
- `pollMessages()`: POST `/ilink/bot/getupdates`，长轮询 35s
- `sendMessage(toID, contextToken, text)`: POST `/ilink/bot/sendmessage`
- `startPolling(onMessage)`: 启动 poll loop，收到消息回调

Auth：每次请求携带 `X-WECHAT-UIN`（随机 uint32 base64）、`AuthorizationType: ilink_bot_token`、`Authorization: Bearer <token>`。

### channel.ts — MCP 通道服务器

声明 capabilities：
- `experimental['claude/channel']: {}` — 注册通道
- `tools: {}` — 公开 reply 工具

ServerOptions.instructions 告诉 Claude：
> Messages arrive as `<channel source="cc-wechat" chat_id="..." context_token="...">`. Reply with the `reply` tool, passing `chat_id` and `context_token` from the tag attributes. Keep replies concise.

reply 工具 schema:
```typescript
{
  name: 'reply',
  inputSchema: {
    type: 'object',
    properties: {
      chat_id: { type: 'string' },
      context_token: { type: 'string' },
      text: { type: 'string' },
    },
    required: ['chat_id', 'context_token', 'text'],
  },
}
```

### store.ts — 状态持久化

线程安全的 JSON 文件读写（`state/session.json`）：
- `bot_token`: 登录后获取
- `get_updates_buf`: 长轮询游标
- `login_time`: ISO 8601，用于判断 session 是否过期
- `base_url`: iLink 服务器地址

### index.ts — 入口

1. 加载 `state/session.json`
2. 若无 token → 登录流程：
   - 调用 `get_bot_qrcode?bot_type=3` 获取二维码 URL
   - 写入 `state/qrcode.txt`，打印到 stderr 提示用户扫码
   - 轮询 `get_qrcode_status` 等待确认
   - 保存 token 到 session.json
3. 若有有效 token → 创建 WeChatClient → 创建 MCP Server → `mcp.connect(new StdioServerTransport())`
4. 启动 `wechatClient.startPolling()`，收到消息时调用 `mcp.notification()`

## 依赖

- `@modelcontextprotocol/sdk` — MCP server transport + types
- Bun runtime — HTTPS、文件 IO、TypeScript 原生支持
- 零外部网络依赖（除了 iLink API）

## 和 Go 项目的关系

`cc-wechat` 与 `cc-go` 同仓库并存，互不依赖：
- Go 项目 (`internal/wechat/`) — cc-go 桌面的内建微信支持
- TS 通道 (`cc-wechat/`) — Claude Code 原生通道插件

微信 API 调用逻辑从 Go 翻译到 TS，两套代码独立维护。

## 后续扩展

设计已为方案 C（完整插件包）预留结构。添加 `.claude-plugin/plugin.json` + channels 声明 + userConfig 即可升级。