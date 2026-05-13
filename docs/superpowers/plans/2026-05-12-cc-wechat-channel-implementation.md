# cc-wechat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Claude Code channel plugin that bridges WeChat iLink Bot messages to Claude Code sessions, enabling bidirectional chat via WeChat.

**Architecture:** A Bun TypeScript project. `WeChatClient` handles iLink HTTP API, `Channel` wraps it as an MCP stdio server declaring `claude/channel` capability + `reply` tool. `Store` persists session state to JSON. `index.ts` wires everything together.

**Tech Stack:** Bun runtime, `@modelcontextprotocol/sdk` (v1.x branch for channels compatibility), TypeScript.

---

## File Structure

```
cc-wechat/
├── package.json          # deps: @modelcontextprotocol/sdk
├── tsconfig.json
├── .gitignore            # state/, node_modules/
├── src/
│   ├── index.ts          # Entry: load/store → login → MCP → poll
│   ├── types.ts          # Shared types
│   ├── wechat-client.ts  # WeChat iLink Bot HTTP client
│   ├── channel.ts        # MCP channel server
│   └── store.ts          # JSON file persistence
└── state/                # git-ignored runtime data
```

---

### Task 1: Project Setup

**Files:**
- Create: `cc-wechat/package.json`
- Create: `cc-wechat/tsconfig.json`
- Create: `cc-wechat/.gitignore`

- [ ] **Step 1: Create `cc-wechat/package.json`**

```json
{
  "name": "cc-wechat",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "start": "bun run src/index.ts"
  },
  "dependencies": {
    "@modelcontextprotocol/sdk": "^1.25.0"
  }
}
```

- [ ] **Step 2: Create `cc-wechat/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ESNext",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "outDir": "dist",
    "rootDir": "src"
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Create `cc-wechat/.gitignore`**

```
node_modules/
state/
dist/
```

- [ ] **Step 4: Install dependencies**

Run: `cd G:/dev/AI/cc-go/cc-wechat && bun install`

- [ ] **Step 5: Create `cc-wechat/src/` directory**

Run: `mkdir -p G:/dev/AI/cc-go/cc-wechat/src`

- [ ] **Step 6: Commit**

```bash
git add cc-wechat/
git commit -m "feat: scaffold cc-wechat project with Bun + MCP SDK"
```

---

### Task 2: Define Shared Types

**Files:**
- Create: `cc-wechat/src/types.ts`

- [ ] **Step 1: Write `cc-wechat/src/types.ts`**

```typescript
// WeChat message from iLink API
export interface WeChatMessage {
  from_user_id: string;
  to_user_id: string;
  message_type: number;
  message_state: number;
  context_token: string;
  text: string;
}

// Poll response from /ilink/bot/getupdates
export interface PollResponse {
  ret: number;
  msgs: Record<string, unknown>[];
  get_updates_buf: string;
  longpolling_timeout_ms: number;
}

// Session data persisted to state/session.json
export interface SessionState {
  bot_token: string;
  base_url: string;
  get_updates_buf: string;
  login_time: string; // ISO 8601
}

// Contact info for reply routing
export interface ContactInfo {
  from_id: string;
  context_token: string;
}
```

- [ ] **Step 2: Commit**

```bash
git add cc-wechat/src/types.ts
git commit -m "feat: add shared types for cc-wechat"
```

---

### Task 3: Implement Store (Session Persistence)

**Files:**
- Create: `cc-wechat/src/store.ts`

- [ ] **Step 1: Write `cc-wechat/src/store.ts`**

```typescript
import { readFileSync, writeFileSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import type { SessionState } from "./types.js";

const STATE_DIR = join(import.meta.dirname || ".", "..", "state");
const SESSION_FILE = join(STATE_DIR, "session.json");

function ensureDir(): void {
  if (!existsSync(STATE_DIR)) {
    mkdirSync(STATE_DIR, { recursive: true });
  }
}

export function loadSession(): SessionState | null {
  try {
    if (!existsSync(SESSION_FILE)) return null;
    const raw = readFileSync(SESSION_FILE, "utf-8");
    return JSON.parse(raw) as SessionState;
  } catch {
    return null;
  }
}

export function saveSession(state: SessionState): void {
  ensureDir();
  writeFileSync(SESSION_FILE, JSON.stringify(state, null, 2), "utf-8");
}

export function clearSession(): void {
  try {
    if (existsSync(SESSION_FILE)) {
      writeFileSync(SESSION_FILE, "");
    }
  } catch {
    // ignore
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add cc-wechat/src/store.ts
git commit -m "feat: add session state persistence for cc-wechat"
```

---

### Task 4: Implement WeChat Client

**Files:**
- Create: `cc-wechat/src/wechat-client.ts`

- [ ] **Step 1: Write `cc-wechat/src/wechat-client.ts`**

```typescript
import type { PollResponse, WeChatMessage, SessionState, ContactInfo } from "./types.js";

const DEFAULT_BASE_URL = "https://ilinkai.weixin.qq.com";

export class WeChatClient {
  private baseUrl: string;
  private botToken: string;
  private loginTime: Date;
  private getUpdatesBuf: string;
  private stopFlag = false;
  private typingTickets: Map<string, string> = new Map();

  constructor(state?: SessionState | null) {
    this.baseUrl = state?.base_url || DEFAULT_BASE_URL;
    this.botToken = state?.bot_token || "";
    this.loginTime = state?.login_time ? new Date(state.login_time) : new Date(0);
    this.getUpdatesBuf = state?.get_updates_buf || "";
  }

  get token(): string {
    return this.botToken;
  }

  get isLoggedIn(): boolean {
    return this.botToken.length > 0;
  }

  get state(): SessionState {
    return {
      bot_token: this.botToken,
      base_url: this.baseUrl,
      get_updates_buf: this.getUpdatesBuf,
      login_time: this.loginTime.toISOString(),
    };
  }

  private randomUin(): string {
    return String(Math.floor(Math.random() * 0xffffffff));
  }

  private authHeaders(): Record<string, string> {
    const uin = btoa(this.randomUin());
    return {
      "Content-Type": "application/json",
      AuthorizationType: "ilink_bot_token",
      "X-WECHAT-UIN": uin,
      ...(this.botToken ? { Authorization: `Bearer ${this.botToken}` } : {}),
    };
  }

  private async doRequest(method: string, path: string, body?: unknown): Promise<Record<string, unknown>> {
    const url = `${this.baseUrl}/${path}`;
    const resp = await fetch(url, {
      method,
      headers: this.authHeaders(),
      body: body ? JSON.stringify(body) : undefined,
    });
    const text = await resp.text();
    try {
      return JSON.parse(text) as Record<string, unknown>;
    } catch {
      return {};
    }
  }

  async getQRCode(): Promise<{ qrcode: string; qrcode_img: string }> {
    const result = await this.doRequest("GET", "ilink/bot/get_bot_qrcode?bot_type=3");
    return {
      qrcode: (result.qrcode as string) || "",
      qrcode_img: (result.qrcode_img_content as string) || "",
    };
  }

  async checkQRCodeStatus(qrcode: string): Promise<{ confirmed: boolean; token: string; baseUrl: string }> {
    const result = await this.doRequest("GET", `ilink/bot/get_qrcode_status?qrcode=${encodeURIComponent(qrcode)}`);
    if (result.status === "confirmed") {
      return {
        confirmed: true,
        token: (result.bot_token as string) || "",
        baseUrl: (result.baseurl as string) || "",
      };
    }
    return { confirmed: false, token: "", baseUrl: "" };
  }

  async sendMessage(toID: string, contextToken: string, text: string): Promise<void> {
    const clientID = `cc-wechat-${Math.floor(Math.random() * 0xffffffff).toString(16).padStart(8, "0")}`;
    await this.doRequest("POST", "ilink/bot/sendmessage", {
      msg: {
        from_user_id: "",
        to_user_id: toID,
        client_id: clientID,
        message_type: 2,
        message_state: 2,
        context_token: contextToken,
        item_list: [{ type: 1, text_item: { text } }],
      },
      base_info: { channel_version: "1.0.2" },
    });
  }

  async pollMessages(): Promise<{ msgs: WeChatMessage[]; buf: string }> {
    const result = await this.doRequest("POST", "ilink/bot/getupdates", {
      get_updates_buf: this.getUpdatesBuf,
      base_info: { channel_version: "1.0.2" },
    });
    const buf = (result.get_updates_buf as string) || "";
    const rawMsgs = (result.msgs as Record<string, unknown>[]) || [];
    const msgs: WeChatMessage[] = [];
    for (const raw of rawMsgs) {
      if ((raw.message_type as number) !== 1) continue;
      const text = extractText(raw);
      msgs.push({
        from_user_id: (raw.from_user_id as string) || "",
        to_user_id: (raw.to_user_id as string) || "",
        message_type: raw.message_type as number,
        message_state: raw.message_state as number,
        context_token: (raw.context_token as string) || "",
        text,
      });
    }
    return { msgs, buf };
  }

  stop(): void {
    this.stopFlag = true;
  }

  async pollLoop(onMessage: (msg: WeChatMessage, contact: ContactInfo) => void): Promise<void> {
    this.stopFlag = false;
    while (!this.stopFlag) {
      try {
        const { msgs, buf } = await this.pollMessages();
        if (buf) this.getUpdatesBuf = buf;
        for (const msg of msgs) {
          onMessage(msg, { from_id: msg.from_user_id, context_token: msg.context_token });
        }
      } catch (err) {
        console.error("[wechat] poll error:", err);
        await sleep(2000);
      }
    }
  }
}

function extractText(raw: Record<string, unknown>): string {
  const items = (raw.item_list as Record<string, unknown>[]) || [];
  for (const item of items) {
    if (item.type === 1) {
      const textItem = item.text_item as Record<string, unknown> | undefined;
      return (textItem?.text as string) || "";
    }
  }
  return "";
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
```

- [ ] **Step 2: Commit**

```bash
git add cc-wechat/src/wechat-client.ts
git commit -m "feat: add WeChat iLink Bot HTTP client for cc-wechat"
```

---

### Task 5: Implement MCP Channel Server

**Files:**
- Create: `cc-wechat/src/channel.ts`

- [ ] **Step 1: Write `cc-wechat/src/channel.ts`**

The channel documentation uses v1.x SDK style (`Server`, `StdioServerTransport`, `ListToolsRequestSchema`, `CallToolRequestSchema`). Follow that pattern exactly.

```typescript
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

export interface ReplyHandler {
  sendMessage(toID: string, contextToken: string, text: string): Promise<void>;
}

export function createChannelServer(replyHandler: ReplyHandler): Server {
  const server = new Server(
    { name: "cc-wechat", version: "0.1.0" },
    {
      capabilities: {
        experimental: { "claude/channel": {} },
        tools: {},
      },
      instructions:
        "Messages arrive as <channel source=\"cc-wechat\" chat_id=\"...\" context_token=\"...\">. " +
        "Reply with the reply tool, passing chat_id and context_token from the tag attributes.",
    },
  );

  // Register reply tool
  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: [
      {
        name: "reply",
        description: "Send a message back to WeChat",
        inputSchema: {
          type: "object",
          properties: {
            chat_id: { type: "string", description: "The WeChat user ID to reply to" },
            context_token: { type: "string", description: "The context token from the incoming message" },
            text: { type: "string", description: "The message text to send" },
          },
          required: ["chat_id", "context_token", "text"],
        },
      },
    ],
  }));

  server.setRequestHandler(CallToolRequestSchema, async (req) => {
    if (req.params.name === "reply") {
      const args = req.params.arguments as { chat_id: string; context_token: string; text: string } | undefined;
      if (!args) {
        return { content: [{ type: "text", text: "error: missing arguments" }], isError: true };
      }
      await replyHandler.sendMessage(args.chat_id, args.context_token, args.text);
      return { content: [{ type: "text", text: "sent" }] };
    }
    throw new Error(`unknown tool: ${req.params.name}`);
  });

  return server;
}
```

- [ ] **Step 2: Commit**

```bash
git add cc-wechat/src/channel.ts
git commit -m "feat: add MCP channel server with reply tool for cc-wechat"
```

---

### Task 6: Implement Entry Point (Login + Wire Up)

**Files:**
- Create: `cc-wechat/src/index.ts`

- [ ] **Step 1: Write `cc-wechat/src/index.ts`**

```typescript
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { WeChatClient } from "./wechat-client.js";
import { createChannelServer } from "./channel.js";
import { loadSession, saveSession } from "./store.js";
import type { WeChatMessage, ContactInfo } from "./types.js";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

async function loginFlow(client: WeChatClient): Promise<void> {
  const { qrcode, qrcode_img } = await client.getQRCode();
  if (!qrcode) {
    throw new Error("Failed to get QR code from iLink API");
  }

  // Save QR code link to file so user can find it
  const qrPath = join(import.meta.dirname || ".", "..", "state", "qrcode.txt");
  writeFileSync(qrPath, `Scan this QR code to login:\n${qrcode}\n`, "utf-8");

  console.error("[cc-wechat] Please scan the QR code to login.");
  console.error("[cc-wechat] QR code URL saved to state/qrcode.txt");
  if (qrcode_img) {
    console.error("[cc-wechat] QR image (base64) received, length:", qrcode_img.length);
  }

  // Poll for confirmation
  for (let i = 0; i < 300; i++) {
    await sleep(2000);
    const result = await client.checkQRCodeStatus(qrcode);
    if (result.confirmed) {
      // Update client with new credentials
      (client as any).botToken = result.token;
      (client as any).baseUrl = result.baseUrl || (client as any).baseUrl;
      (client as any).loginTime = new Date();
      saveSession(client.state);
      console.error("[cc-wechat] Login successful!");
      return;
    }
    if (i % 15 === 0) {
      console.error("[cc-wechat] Waiting for scan...");
    }
  }
  throw new Error("QR code scan timed out after 10 minutes");
}

async function main(): Promise<void> {
  // Load persisted session
  const savedSession = loadSession();
  const client = new WeChatClient(savedSession);

  // Login if needed
  if (!client.isLoggedIn) {
    console.error("[cc-wechat] No saved session, starting login flow...");
    await loginFlow(client);
  } else {
    console.error("[cc-wechat] Restored session from state/session.json");
  }

  // Create MCP channel server
  const server = createChannelServer(client);

  // Connect stdio — blocks from here
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("[cc-wechat] Channel connected via stdio");

  // Start polling iLink for messages
  client.pollLoop((msg: WeChatMessage, contact: ContactInfo) => {
    // Persist cursor after each batch
    saveSession(client.state);

    // Push to Claude as channel event
    server.notification({
      method: "notifications/claude/channel",
      params: {
        content: msg.text,
        meta: {
          chat_id: contact.from_id,
          context_token: contact.context_token,
        },
      },
    } as any);
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((err) => {
  console.error("[cc-wechat] Fatal error:", err);
  process.exit(1);
});
```

- [ ] **Step 2: Commit**

```bash
git add cc-wechat/src/index.ts
git commit -m "feat: add cc-wechat entry point with login flow and channel wiring"
```

---

### Task 7: Verify the Build

**Files:**
- None (verification only)

- [ ] **Step 1: Run TypeScript type-check**

Run: `cd G:/dev/AI/cc-go/cc-wechat && npx tsc --noEmit`

Expected: No type errors.

- [ ] **Step 2: Verify all files exist**

Run: `ls -la G:/dev/AI/cc-go/cc-wechat/src/`

Expected: `index.ts`, `types.ts`, `wechat-client.ts`, `channel.ts`, `store.ts`

---

## Self-Review

**1. Spec coverage:**
- WeChatClient (HTTP API client) → Task 4
- MCP channel server with reply tool → Task 5
- Session persistence (store) → Task 3
- Entry point with login flow → Task 6
- Project setup → Task 1
- Types → Task 2
- All spec requirements covered

**2. Placeholder scan:** No TBD, TODO, or vague instructions. All steps have complete code.

**3. Type consistency:**
- `SessionState` defined in Task 2, used in Task 3 (`store.ts`) and Task 4 (`WeChatClient.state`)
- `WeChatMessage` defined in Task 2, used in Task 4 and Task 6
- `ContactInfo` defined in Task 2, used in Task 4 and Task 6
- `ReplyHandler` defined in Task 5, matches `WeChatClient.sendMessage()` signature from Task 4
- Imports across files consistent: `./types.js` for ESM