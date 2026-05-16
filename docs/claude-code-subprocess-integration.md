# Claude Code 子进程对接技术文档

> 版本：1.0 | Claude Code CLI v2.1.x | 更新：2026-05-10

本文档描述如何通过子进程方式对接 Claude Code CLI，实现**程序化调用**和**中途权限控制**。
适用于开发机器人、自动化工具、IDE 插件等需要与 Claude Code 交互的场景。

---

## 目录

1. [架构概览](#1-架构概览)
2. [启动参数配置](#2-启动参数配置)
3. [Stream-JSON 通信协议](#3-stream-json-通信协议)
4. [权限控制协议（核心）](#4-权限控制协议核心)
5. [完整事件类型参考](#5-完整事件类型参考)
6. [代码实现](#6-代码实现)
7. [踩坑记录](#7-踩坑记录)
8. [快速集成模板](#8-快速集成模板)

---

## 1. 架构概览

```
┌─────────────────────────────────────────────────┐
│              你的应用程序 (SDK)                    │
│                                                   │
│  ┌──────────────┐    ┌─────────────────────────┐ │
│  │ send(msg)    │───>│ stdin (JSON lines)       │ │
│  └──────────────┘    │                          │ │
│  ┌──────────────┐    │  {"type":"user",...}     │ │
│  │ respondTo    │───>│  {"type":"control_       │ │
│  │ Permission() │    │   response",...}         │ │
│  └──────────────┘    └──────────┬───────────────┘ │
│                                │                   │
│  ┌──────────────┐    ┌─────────▼───────────────┐ │
│  │ 事件处理      │<───│ stdout (JSON lines)      │ │
│  │ - TEXT        │    │                          │ │
│  │ - TOOL_USE    │    │  {"type":"system",...}   │ │
│  │ - RESULT      │    │  {"type":"assistant",...}│ │
│  │ - PERMISSION  │    │  {"type":"control_      │ │
│  │ - ERROR       │    │   request",...}          │ │
│  └──────────────┘    └──────────────────────────┘ │
│                                │                   │
└────────────────────────────────┼───────────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  Claude Code CLI 进程     │
                    │  claude -p [flags...]    │
                    │  (子进程，pipe 模式)       │
                    └─────────────────────────┘
```

**核心思路**：将 Claude Code CLI 作为子进程启动，通过 stdin/stdout 的 JSON 行协议进行双向通信。CLI 处理 AI 模型调用，你的程序负责发送用户消息和处理权限请求。

---

## 2. 启动参数配置

### 2.1 必需参数

```bash
claude -p \
  --input-format stream-json \
  --output-format stream-json \
  --verbose \
  --permission-mode default \
  --permission-prompt-tool stdio
```

| 参数 | 作用 |
|------|------|
| `-p` | 管道模式，从 stdin 读取输入 |
| `--input-format stream-json` | 输入格式为 JSON 行协议 |
| `--output-format stream-json` | 输出格式为 JSON 行协议 |
| `--verbose` | 启用详细输出（包含 stream_event 等） |
| `--permission-mode default` | 所有工具调用都需要权限审批 |
| `--permission-prompt-tool stdio` | **关键！** 将权限请求路由到 stdin/stdout 而非终端 |

### 2.2 可选参数

```bash
# 模型选择
--model sonnet              # claude-sonnet-4-6, opus, haiku

# 预算和轮次控制
--max-turns 10              # 最大 agent 轮次
--max-budget-usd 1.0        # 最大消费 USD

# 系统提示词
--system-prompt "你是一个..."         # 覆盖系统提示
--append-system-prompt "附加规则..."   # 追加到系统提示

# 会话管理
--resume <session-id>       # 恢复已有会话
--session-id <custom-id>    # 自定义会话 ID
-n "my-session"             # 会话显示名称

# 工具控制
--allowed-tools Bash,Read,Glob,Grep     # 白名单
--disallowed-tools Write,Edit           # 黑名单

# 其他
--bare                      # 禁用 CLAUDE.md 和 git 上下文
--effort high               # 推理努力程度：low/medium/high/xhigh/max
--include-hook-events       # 输出中包含 hook 生命周期事件
```

### 2.3 Permission Mode 说明

| Mode | 行为 | 适用场景 |
|------|------|---------|
| `default` | 每次工具调用都发 `control_request` | **推荐**：完整的权限控制 |
| `acceptEdits` | 自动批准文件编辑，危险操作仍需审批 | 半自动模式 |
| `auto` | 自动批准一切 | 全自动，不需要权限交互 |
| `bypassPermissions` | 跳过所有权限检查 | 危险，仅测试用 |
| `plan` | 只读模式，不允许任何写操作 | 安全分析 |

### 2.4 环境变量注意事项

```typescript
// 必须删除 CLAUDECODE 环境变量！
// 如果不删除，CLI 会检测到"嵌套会话"并拒绝正常工作
const env = { ...process.env };
delete env.CLAUDECODE;

const proc = spawn("claude", args, {
  cwd: workDir,
  env,
  stdio: ["pipe", "pipe", "pipe"],
});
```

---

## 3. Stream-JSON 通信协议

所有通信均为**单行 JSON**（NDJSON），一行一条消息，以 `\n` 分隔。

### 3.1 发送用户消息（stdin）

```jsonc
{
  "type": "user",
  "message": {
    "role": "user",
    "content": "请帮我列出当前目录的文件"
  }
}
```

**重要**：消息类型是 `"user"`，不是 `"user_message"`。

#### 带图片的消息

```jsonc
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "image",
        "source": {
          "type": "base64",
          "media_type": "image/png",
          "data": "<base64-encoded-image>"
        }
      },
      {
        "type": "text",
        "text": "请描述这张图片"
      }
    ]
  }
}
```

### 3.2 接收 CLI 事件（stdout）

所有事件以 JSON 行输出到 stdout。按时间顺序，一个完整的交互流程如下：

```
system (hook_started)    ← CLI 启动，session_id 在这里
system (hook_response)   ← hook 执行结果
assistant                ← AI 响应（包含 text/tool_use/thinking）
control_request          ← ⚠️ 权限请求（需要你回复）
assistant                ← AI 继续响应
result                   ← 轮次结束
```

---

## 4. 权限控制协议（核心）

这是本文档最重要的部分。通过 `--permission-prompt-tool stdio`，CLI 会将所有权限请求通过 stdout 发送，等待你通过 stdin 回复。

### 4.1 协议流程

```
                                    CLI                          SDK
                                     │                            │
  AI 决定使用 Bash 工具               │                            │
                                     │── control_request ────────>│
                                     │   (can_use_tool)           │
                                     │                            │
                                     │                     你的逻辑判断
                                     │                   approve/deny
                                     │                            │
                                     │<── control_response ───────│
                                     │   (allow/deny)             │
                                     │                            │
  CLI 执行/跳过工具                    │                            │
```

### 4.2 CLI 发送的权限请求

当 Claude Code 需要执行需要权限的工具时，CLI 通过 stdout 发送：

```jsonc
{
  "type": "control_request",
  "request_id": "baff0cbe-e0a3-44fd-bbd2-756a37ed490e",
  "request": {
    "subtype": "can_use_tool",
    "tool_name": "Bash",
    "input": {
      "command": "curl -s https://httpbin.org/get"
    }
  }
}
```

**字段说明**：

| 字段 | 说明 |
|------|------|
| `type` | 固定为 `"control_request"` |
| `request_id` | UUID，回复时必须原样返回 |
| `request.subtype` | 固定为 `"can_use_tool"` |
| `request.tool_name` | 工具名称：`Bash`, `Write`, `Edit`, `NotebookEdit` 等 |
| `request.input` | 工具的输入参数（对象） |

### 4.3 批准权限（stdin 回复）

```jsonc
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "baff0cbe-e0a3-44fd-bbd2-756a37ed490e",
    "response": {
      "behavior": "allow",
      "updatedInput": {}
    }
  }
}
```

**注意**：
- `request_id` 必须与请求中的 `request_id` 完全匹配
- `updatedInput` 字段**必须存在**（可以是空对象 `{}`），用于修改工具参数
- 如果你想修改工具的输入参数，可以在这里提供新的 input

### 4.4 拒绝权限（stdin 回复）

```jsonc
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "baff0cbe-e0a3-44fd-bbd2-756a37ed490e",
    "response": {
      "behavior": "deny",
      "message": "安全策略禁止执行此命令"
    }
  }
}
```

### 4.5 权限取消

CLI 可能主动取消一个权限请求：

```jsonc
{
  "type": "control_cancel_request",
  "request_id": "baff0cbe-e0a3-44fd-bbd2-756a37ed490e"
}
```

### 4.6 完整交互时序图

```
SDK                                    CLI (Claude Code)
 │                                        │
 │  spawn("claude", [...args...])         │
 │──────────────────────────────────────>│  进程启动
 │                                        │
 │  <── system (hook_started) ───────────│  session_id 在这里
 │  <── system (hook_response) ──────────│
 │                                        │
 │  ── {"type":"user","message":...} ──>│  发送用户消息
 │                                        │  AI 思考...
 │  <── assistant (text/thinking) ──────│  AI 输出
 │  <── assistant (tool_use: Bash) ─────│  AI 想用 Bash
 │  <── control_request ─────────────────│  ⚠️ 请求权限
 │                                        │
 │  ── control_response (allow) ────────>│  批准！
 │                                        │  执行 Bash
 │  <── assistant (tool_result) ─────────│  工具结果
 │  <── assistant (text) ────────────────│  AI 总结
 │  <── result ──────────────────────────│  轮次结束
 │                                        │
```

---

## 5. 完整事件类型参考

### 5.1 stdout 事件

| type | 说明 | 关键字段 |
|------|------|---------|
| `system` | 系统事件（init, hook_started, hook_response） | `subtype`, `session_id` |
| `assistant` | AI 响应消息 | `message.content[]`（text/tool_use/thinking） |
| `user` | 用户消息回显 | `message.content[]` |
| `result` | 轮次结束 | `result`, `stop_reason`, `duration_ms`, `num_turns` |
| `stream_event` | 细粒度流式事件 | `event.type`（content_block_start/delta/stop） |
| `tool_use` | 工具调用 | `name`, `input` |
| `tool_result` | 工具结果 | `content`, `is_error` |
| `hook` | Hook 生命周期 | `hook_event.hook_type`（PreToolUse/PostToolUse） |
| **`control_request`** | **权限请求** | `request_id`, `request.subtype/tool_name/input` |
| `control_cancel_request` | 取消权限请求 | `request_id` |
| `permission_request` | 旧版权限格式（可能仍有） | `permission.name/input` |
| `error` | 错误 | `error` |

### 5.2 assistant 事件中 content block 类型

```jsonc
// 文本输出
{ "type": "text", "text": "Hello world" }

// 工具调用
{ "type": "tool_use", "name": "Bash", "input": { "command": "ls" } }

// 思考（extended thinking）
{ "type": "thinking", "thinking": "Let me analyze..." }
```

### 5.3 result 事件结构

```jsonc
{
  "type": "result",
  "subtype": "success",
  "result": "The curl request succeeded...",
  "is_error": false,
  "stop_reason": "end_turn",
  "duration_ms": 20746,
  "duration_api_ms": 11061,
  "num_turns": 2,
  "session_id": "5d812a91-...",
  "usage": {
    "input_tokens": 1234,
    "output_tokens": 567,
    "cache_read_input_tokens": 890
  }
}
```

---

## 6. 代码实现

### 6.1 项目结构

```
connect_claudecode/
├── src/
│   ├── claude-session.ts   # 核心会话管理器（可复用）
│   ├── index.ts            # 交互式 CLI 工具
│   └── test-permission.ts  # 权限控制测试
├── package.json
└── tsconfig.json
```

### 6.2 核心代码：claude-session.ts

完整的 TypeScript 实现，可直接复制到项目中使用。以下是关键代码片段：

#### 启动会话

```typescript
import { spawn } from "node:child_process";
import * as readline from "node:readline";

// 构建参数
const args = [
  "-p",
  "--input-format", "stream-json",
  "--output-format", "stream-json",
  "--verbose",
  "--permission-mode", "default",
  "--permission-prompt-tool", "stdio",
];

// 过滤嵌套检测环境变量
const env = { ...process.env };
delete env.CLAUDECODE;

// 启动子进程
const proc = spawn("claude", args, {
  cwd: workDir,
  env,
  stdio: ["pipe", "pipe", "pipe"],
});

// 逐行读取 stdout
const rl = readline.createInterface({
  input: proc.stdout!,
  crlfDelay: Infinity,
});

rl.on("line", (line) => {
  if (!line.trim()) return;
  const event = JSON.parse(line);
  handleEvent(event);
});
```

#### 发送消息

```typescript
function sendMessage(proc: ChildProcess, text: string) {
  const payload = {
    type: "user",
    message: { role: "user", content: text },
  };
  proc.stdin!.write(JSON.stringify(payload) + "\n");
}
```

#### 处理权限请求

```typescript
let pendingRequestId: string | null = null;

function handleEvent(event: any) {
  switch (event.type) {
    case "control_request": {
      const req = event.request;
      if (req.subtype !== "can_use_tool") return;

      pendingRequestId = event.request_id;

      console.log(`权限请求: ${req.tool_name}`);
      console.log(`参数: ${JSON.stringify(req.input)}`);

      // 自动批准示例
      respondPermission(true);
      break;
    }
    // ... 其他事件处理
  }
}

function respondPermission(approved: boolean, reason?: string) {
  if (!pendingRequestId) return;

  const permResponse = approved
    ? { behavior: "allow", updatedInput: {} }
    : { behavior: "deny", message: reason || "Denied" };

  const envelope = {
    type: "control_response",
    response: {
      subtype: "success",
      request_id: pendingRequestId,
      response: permResponse,
    },
  };

  proc.stdin!.write(JSON.stringify(envelope) + "\n");
  pendingRequestId = null;
}
```

### 6.3 会话事件 API

```typescript
// 使用 EventEmitter 模式
session.on("init", (event) => {
  console.log("Session ID:", event.session_id);
});

session.on("text", (chunk) => {
  process.stdout.write(chunk);  // 流式文本
});

session.on("permission_request", (req) => {
  console.log(`Tool: ${req.tool}`);
  console.log(`Desc: ${req.description}`);
  // 手动审批
  session.respondToPermission(true);   // 批准
  // session.respondToPermission(false, "不安全");  // 拒绝
});

session.on("result", (event) => {
  console.log("Done:", event.result);
});

session.on("error", (err) => {
  console.error("Error:", err.message);
});

session.on("close", (code) => {
  console.log("Exit:", code);
});
```

### 6.4 回调模式

```typescript
const session = new ClaudeSession({
  cwd: process.cwd(),
  permissionMode: "default",
  onPermissionRequest: async (req) => {
    // 自动逻辑：编辑操作批准，Bash 操作拒绝
    if (["Edit", "Write", "NotebookEdit"].includes(req.tool)) {
      return true;  // 批准
    }
    if (req.tool === "Bash") {
      const input = req.input as { command?: string };
      if (input.command?.includes("rm ")) return false; // 拒绝删除
      return true;
    }
    return false;  // 其他拒绝
  },
});
```

---

## 7. 踩坑记录

### 7.1 消息格式错误（最大坑）

| 错误格式 | 正确格式 |
|---------|---------|
| `{"type":"user_message","content":"..."}` | `{"type":"user","message":{"role":"user","content":"..."}}` |
| `{"type":"human","content":"..."}` | 同上 |

**现象**：发送后 CLI 无任何输出，不报错，只是静默忽略。
**原因**：CLI 的 stream-json 解析器只识别 `"type":"user"` 格式，其他格式被静默丢弃。

### 7.2 权限事件类型错误

| 错误 | 正确 |
|------|------|
| `"type":"sdk_control_request"` | `"type":"control_request"` |
| `subtype:"permission"` | `subtype:"can_use_tool"` |
| `request.tool_input` | `request.input` |

### 7.3 不需要发送初始化消息

与一些文档描述不同，CLI **不需要** SDK 主动发送 `control_request` 初始化消息。
CLI 启动后会自动在需要时发送 `control_request` 给你。

### 7.4 CLAUDECODE 环境变量

如果从 Claude Code 内部启动子进程（如本项目中），**必须删除** `CLAUDECODE` 环境变量，否则 CLI 会检测到嵌套会话并异常。

```typescript
const env = { ...process.env };
delete env.CLAUDECODE;  // 必须！
```

### 7.5 `--permission-mode default` 在管道模式下的行为

单独使用 `--permission-mode default`（不带 `--permission-prompt-tool stdio`）时：
- CLI 会尝试在终端显示交互式提示
- 但 stdin 被 stream-json 占用，无法接收 y/n 输入
- **结果：进程挂起**

**必须配合** `--permission-prompt-tool stdio` 使用。

### 7.6 control_response 中 updatedInput 必须存在

批准权限时，`updatedInput` 字段**不能省略**，即使是空对象：

```jsonc
// ✅ 正确
{ "behavior": "allow", "updatedInput": {} }

// ❌ 错误（可能被 CLI 拒绝）
{ "behavior": "allow" }
```

### 7.7 session_id 的获取

在 `--permission-prompt-tool stdio` 模式下，CLI 不一定发送 `{"type":"system","subtype":"init"}` 事件。
`session_id` 可能出现在：
- `system` + `subtype: "hook_started"` 事件中
- `result` 事件中

建议从所有 `system` 事件中提取 `session_id`。

---

## 8. 快速集成模板

### 8.1 最小可用实现（Node.js）

```typescript
import { spawn } from "node:child_process";
import * as readline from "node:readline";

const proc = spawn("claude", [
  "-p",
  "--input-format", "stream-json",
  "--output-format", "stream-json",
  "--verbose",
  "--permission-mode", "default",
  "--permission-prompt-tool", "stdio",
  "--max-turns", "3",
], {
  cwd: process.cwd(),
  env: { ...process.env, CLAUDECODE: undefined },
  stdio: ["pipe", "pipe", "pipe"],
});

let pendingReqId: string | null = null;

const rl = readline.createInterface({ input: proc.stdout!, crlfDelay: Infinity });
rl.on("line", (line) => {
  if (!line.trim()) return;
  const evt = JSON.parse(line);

  switch (evt.type) {
    case "system":
      if (evt.session_id) console.log("Session:", evt.session_id);
      break;
    case "assistant":
      for (const block of evt.message?.content ?? []) {
        if (block.type === "text") process.stdout.write(block.text);
        if (block.type === "tool_use") console.log(`\n[Tool] ${block.name}`);
      }
      break;
    case "control_request":
      if (evt.request?.subtype === "can_use_tool") {
        pendingReqId = evt.request_id;
        console.log(`\n[Permission] ${evt.request.tool_name}: ${
          JSON.stringify(evt.request.input).slice(0, 100)
        }`);
        // 自动批准
        proc.stdin!.write(JSON.stringify({
          type: "control_response",
          response: {
            subtype: "success",
            request_id: pendingReqId,
            response: { behavior: "allow", updatedInput: {} },
          },
        }) + "\n");
        pendingReqId = null;
      }
      break;
    case "result":
      console.log("\n[Done]", evt.result?.slice(0, 200));
      proc.kill();
      break;
    case "error":
      console.error("[Error]", evt.error);
      break;
  }
});

proc.stderr?.on("data", (d) => process.stderr.write(d));

// 发送第一条消息
setTimeout(() => {
  proc.stdin!.write(JSON.stringify({
    type: "user",
    message: { role: "user", content: "Hello, list files in current directory" },
  }) + "\n");
}, 2000);
```

### 8.2 Python 实现

```python
import subprocess
import json
import sys
import threading

proc = subprocess.Popen(
    [
        "claude", "-p",
        "--input-format", "stream-json",
        "--output-format", "stream-json",
        "--verbose",
        "--permission-mode", "default",
        "--permission-prompt-tool", "stdio",
        "--max-turns", "3",
    ],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    env={**os.environ, "CLAUDECODE": ""},  # 清除嵌套检测
    cwd=os.getcwd(),
)

def read_stdout():
    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        evt = json.loads(line)

        if evt["type"] == "system" and evt.get("session_id"):
            print(f"Session: {evt['session_id']}")

        elif evt["type"] == "assistant":
            for block in evt.get("message", {}).get("content", []):
                if block["type"] == "text":
                    print(block["text"], end="")
                elif block["type"] == "tool_use":
                    print(f"\n[Tool] {block['name']}")

        elif evt["type"] == "control_request":
            req = evt.get("request", {})
            if req.get("subtype") == "can_use_tool":
                print(f"\n[Permission] {req['tool_name']}")
                # 自动批准
                response = json.dumps({
                    "type": "control_response",
                    "response": {
                        "subtype": "success",
                        "request_id": evt["request_id"],
                        "response": {"behavior": "allow", "updatedInput": {}},
                    },
                }) + "\n"
                proc.stdin.write(response.encode())
                proc.stdin.flush()

        elif evt["type"] == "result":
            print(f"\n[Done] {evt.get('result', '')[:200]}")
            proc.terminate()

        elif evt["type"] == "error":
            print(f"[Error] {evt.get('error')}", file=sys.stderr)

threading.Thread(target=read_stdout, daemon=True).start()

# 发送消息
import time
time.sleep(2)
msg = json.dumps({
    "type": "user",
    "message": {"role": "user", "content": "Hello, list the files"},
}) + "\n"
proc.stdin.write(msg.encode())
proc.stdin.flush()

proc.wait()
```

---

## 附录：参考项目

| 项目 | 语言 | 说明 |
|------|------|------|
| [cc-connect](https://github.com/chenhg5/cc-connect) | Go | 完整的 Claude Code 代理/网关，权限控制参考实现 |
| [claude-agent-sdk-go](https://github.com/anthropics/claude-agent-sdk-go) | Go | Anthropic 官方 Go SDK（协议参考） |

## 附录：工具名称对照

| 工具名 | 说明 | 风险级别 |
|--------|------|---------|
| `Bash` | 执行 shell 命令 | 🔴 高 |
| `Write` | 创建/覆盖文件 | 🟡 中 |
| `Edit` | 编辑文件内容 | 🟡 中 |
| `MultiEdit` | 批量编辑 | 🟡 中 |
| `NotebookEdit` | 编辑 Jupyter notebook | 🟡 中 |
| `Read` | 读取文件 | 🟢 低 |
| `Glob` | 文件搜索 | 🟢 低 |
| `Grep` | 内容搜索 | 🟢 低 |
| `LSP` | 语言服务协议 | 🟢 低 |
| `WebSearch` | 网络搜索 | 🟢 低 |
| `WebFetch` | 获取网页内容 | 🟢 低 |
| `TaskCreate/Update/List/Get` | 任务管理 | 🟢 低 |
| `Agent` | 子代理调用 | 🟡 中 |
