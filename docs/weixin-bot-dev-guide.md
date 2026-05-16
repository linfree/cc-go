# 微信 iLink Bot 开发指导说明

> 基于腾讯 iLink Bot 协议（`ilinkai.weixin.qq.com`），使用 Python 异步实现的微信机器人开发指南。

---

## 一、协议概述

腾讯通过 OpenClaw 框架开放了微信个人账号的 Bot API，底层协议为 **iLink（智联）**，接入域名：

```
https://ilinkai.weixin.qq.com
```

所有接口均为 HTTP/JSON，无需 SDK，可直接用任意 HTTP 客户端调用。

### 1.1 与旧方案的区别

| 维度 | 旧方案（iPad协议/Hook） | iLink Bot API |
|------|------------------------|---------------|
| 合法性 | 违反微信协议，灰色地带 | 官方开放，合法 |
| 稳定性 | 微信更新可能失效 | 服务端 API，稳定 |
| 封号风险 | 极高 | 正常使用无风险 |
| 协议层 | 模拟客户端协议 | 标准 HTTP/JSON |

---

## 二、鉴权流程

### 2.1 时序图

```
开发者                    iLink 服务器                 微信用户
 │                           │                           │
 │── GET get_bot_qrcode ────▶│                           │
 │◀─── { qrcode, url } ─────│                           │
 │                           │◀──── 用户扫码 ────────────│
 │── GET get_qrcode_status ─▶│  (轮询)                   │
 │◀── { confirmed, token } ─│                           │
 │                           │                           │
 │  持久化 bot_token，后续请求 Bearer 鉴权                 │
```

### 2.2 请求头规范

每次请求必须携带以下固定头：

```python
headers = {
    "Content-Type": "application/json",
    "AuthorizationType": "ilink_bot_token",
    "X-WECHAT-UIN": base64.b64encode(str(random_uint32).encode()).decode(),
    "Authorization": f"Bearer {bot_token}",  # 登录后才有
}
```

- `X-WECHAT-UIN`：随机 uint32 的 base64 编码，每次请求都变，用于防重放
- `Authorization`：登录成功后获取的 `bot_token`

### 2.3 登录实现

```python
import base64, random, aiohttp

BASE_URL = "https://ilinkai.weixin.qq.com"

def make_headers(token=None):
    uin = str(random.randint(0, 0xFFFFFFFF))
    headers = {
        "Content-Type": "application/json",
        "AuthorizationType": "ilink_bot_token",
        "X-WECHAT-UIN": base64.b64encode(uin.encode()).decode(),
    }
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers

async def login(session):
    # 1. 获取二维码
    async with session.get(
        f"{BASE_URL}/ilink/bot/get_bot_qrcode?bot_type=3"
    ) as res:
        data = await res.json(content_type=None)
    qrcode = data["qrcode"]

    # 2. 轮询扫码状态
    while True:
        async with session.get(
            f"{BASE_URL}/ilink/bot/get_qrcode_status?qrcode={qrcode}"
        ) as res:
            status = await res.json(content_type=None)
        if status.get("status") == "confirmed":
            return status["bot_token"], status.get("baseurl", "")
        await asyncio.sleep(1)
```

**注意：** `bot_type=3` 是固定参数，不可更改。

---

## 三、完整 API 列表

| Endpoint | Method | 功能 |
|----------|--------|------|
| `/ilink/bot/get_bot_qrcode` | GET | 获取登录二维码（`?bot_type=3`） |
| `/ilink/bot/get_qrcode_status` | GET | 轮询扫码状态（`?qrcode=xxx`） |
| `/ilink/bot/getupdates` | POST | **长轮询收消息**（核心） |
| `/ilink/bot/sendmessage` | POST | 发送消息（文字/图片/文件/视频/语音） |
| `/ilink/bot/getuploadurl` | POST | 获取 CDN 预签名上传地址 |
| `/ilink/bot/getconfig` | POST | 获取 typing_ticket |
| `/ilink/bot/sendtyping` | POST | 发送"正在输入"状态 |

---

## 四、消息收取（长轮询）

### 4.1 请求

```python
result = await api_post(
    session,
    "ilink/bot/getupdates",
    {
        "get_updates_buf": get_updates_buf,  # 上次返回的游标
        "base_info": {"channel_version": "1.0.2"},
    },
    bot_token,
)
```

服务器会 **hold 住连接最多 35 秒**，直到有新消息才返回。

### 4.2 响应结构

```json
{
    "ret": 0,
    "msgs": [...],
    "get_updates_buf": "<新游标>",
    "longpolling_timeout_ms": 35000
}
```

**关键：** `get_updates_buf` 是游标（类似数据库 cursor），必须每次更新，否则会重复收到消息。

```python
get_updates_buf = result.get("get_updates_buf") or get_updates_buf
```

---

## 五、消息结构

### 5.1 消息字段

```json
{
    "from_user_id": "o9cq800kum_xxx@im.wechat",
    "to_user_id": "e06c1ceea05e@im.bot",
    "message_type": 1,
    "message_state": 2,
    "context_token": "AARzJWAFAAABAAAAAAAp...",
    "item_list": [
        { "type": 1, "text_item": { "text": "你好" } }
    ]
}
```

### 5.2 ID 格式

- 用户 ID：`xxx@im.wechat`
- Bot ID：`xxx@im.bot`

### 5.3 消息类型（item_list[].type）

| type | 含义 |
|------|------|
| 1 | 文本 |
| 2 | 图片（CDN 加密存储） |
| 3 | 语音（silk 编码） |
| 4 | 文件附件 |
| 5 | 视频 |

---

## 六、发送消息

### 6.1 context_token（核心必填）

回复消息时**必须**原样携带收到消息的 `context_token`，否则消息不会关联到正确的对话窗口。

```python
await api_post(session, "ilink/bot/sendmessage", {
    "msg": {
        "from_user_id": "",
        "to_user_id": from_id,
        "client_id": f"openclaw-weixin-{random.randint(0, 0xFFFFFFFF):08x}",
        "message_type": 2,       # Bot 发出
        "message_state": 2,      # FINISH（完整消息）
        "context_token": context_token,  # 必填！
        "item_list": [
            {"type": 1, "text_item": {"text": "回复内容"}}
        ],
    },
    "base_info": {"channel_version": "1.0.2"},
}, bot_token)
```

### 6.2 字段说明

| 字段 | 值 | 含义 |
|------|-----|------|
| `message_type` | 2 | Bot 发出的消息 |
| `message_state` | 2 | 完整消息（非流式） |
| `client_id` | 随机字符串 | 消息唯一标识 |
| `from_user_id` | `""` | Bot 发送时留空 |

---

## 七、正在输入状态

发送"正在输入"提示，需要在发送消息前后各调一次：

```python
# 1. 获取 typing_ticket（每个用户缓存一次）
if from_id not in typing_ticket_cache:
    cfg = await api_post(session, "ilink/bot/getconfig", {
        "ilink_user_id": from_id,
        "context_token": context_token,
        "base_info": {"channel_version": "1.0.2"},
    }, bot_token)
    typing_ticket_cache[from_id] = cfg.get("typing_ticket", "")

ticket = typing_ticket_cache[from_id]

# 2. 发送"正在输入"
if ticket:
    await api_post(session, "ilink/bot/sendtyping", {
        "ilink_user_id": from_id,
        "typing_ticket": ticket,
        "status": 1,  # 1=开始输入
    }, bot_token)

# ... 处理消息 ...

# 3. 取消"正在输入"
if ticket:
    await api_post(session, "ilink/bot/sendtyping", {
        "ilink_user_id": from_id,
        "typing_ticket": ticket,
        "status": 2,  # 2=停止输入
    }, bot_token)
```

---

## 八、媒体文件处理

CDN 上的所有媒体文件都经过 **AES-128-ECB** 加密。

### 发送图片流程

1. 生成随机 AES-128 key
2. 用 AES-128-ECB 加密文件
3. 调用 `getuploadurl` 获取预签名 URL
4. PUT 加密文件到 CDN（`https://novac2c.cdn.weixin.qq.com/c2c`）
5. 在 `sendmessage` 中带上 `aes_key`（base64）和 CDN 引用参数

---

## 九、自动重连机制

由于 `bot_token` 有会话时长限制（默认 24 小时），需要实现自动重连。

### 9.1 配置参数

```python
RECONNECT_CONFIG = {
    "session_duration":    24 * 3600,  # 会话总时长（秒）
    "warning_before":       2 * 3600,  # 提前多久发警告（秒）
    "reminder_interval":      30 * 60, # 用户回 N 后再提醒间隔（秒）
    "force_before":           30 * 60, # 最后多久强制重连（秒）
    "qrcode_scan_timeout":       600,  # 等待扫码最长时间（秒）
}
```

### 9.2 重连流程

```
开始 ──▶ 等待到 warning_before 时间点
              │
              ├─ 剩余时间 ≤ force_before ──▶ 强制重连
              │
              └─ 发送警告消息给用户
                    │
                    ├─ 用户回 Y ──▶ 执行重连
                    ├─ 用户回 N ──▶ 等待 reminder_interval 后再提醒
                    └─ 超时 ──▶ 检查是否到达 force_before
```

### 9.3 重连实现要点

1. **防重入**：用 `reconnect_in_progress` 标志防止并发重连
2. **原子替换 token**：重连成功后立即替换 `bot_token` 和 `base_url`
3. **清除缓存**：重连后清空 `typing_ticket_cache`
4. **优雅降级**：重连失败时重置计时器，不 crash

```python
async def do_reconnect(session, bot_token_ref, bot_base_url_ref, ...):
    if reconnect_in_progress[0]:
        return
    reconnect_in_progress[0] = True

    # 获取新二维码
    data = await session.get(f"{base_url}/ilink/bot/get_bot_qrcode?bot_type=3")
    qrcode = data["qrcode"]

    # 发送二维码给用户
    await send_msg_safe(session, from_id, ctx, f"请扫码：{qrcode}", ...)

    # 轮询扫码状态（带超时）
    deadline = time.time() + qrcode_scan_timeout
    while time.time() < deadline:
        status = await session.get(f".../get_qrcode_status?qrcode={qrcode}")
        if status.get("status") == "confirmed":
            # 原子替换
            bot_token_ref[0] = status["bot_token"]
            bot_base_url_ref[0] = status.get("baseurl")
            typing_ticket_cache.clear()
            login_time_ref[0] = time.time()
            break
        await asyncio.sleep(1)

    reconnect_in_progress[0] = False
```

---

## 十、AI 集成

### 10.1 OpenAI 兼容接口集成

```python
class OpenAIClient:
    def __init__(self, api_key, base_url, model, prompt):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.prompt = prompt

    def chat(self, text):
        payload = json.dumps({
            "model": self.model,
            "messages": [
                {"role": "system", "content": self.prompt},
                {"role": "user", "content": text},
            ],
            "temperature": 0.7,
            "max_tokens": 2048,
        }, ensure_ascii=False).encode("utf-8")

        req = urllib.request.Request(
            f"{self.base_url}/chat/completions",
            data=payload,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.api_key}",
            },
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data["choices"][0]["message"]["content"].strip()
```

### 10.2 异步调用

AI 接口通常是同步阻塞的，用 `run_in_executor` 放到线程池中异步执行：

```python
executor = ThreadPoolExecutor(max_workers=4)
loop = asyncio.get_event_loop()
reply = await loop.run_in_executor(executor, ai.chat, text)
```

### 10.3 配置文件

通过 `config.json` 管理 AI 配置，支持环境变量覆盖：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `OPENAI_API_KEY` | API 密钥 | `aaa` |
| `OPENAI_BASE_URL` | 接口地址 | `` |
| `OPENAI_MODEL` | 模型名称 | `` |
| `OPENAI_PROMPT` | 系统提示词 | 见代码默认值 |

---

## 十一、完整消息处理流程

```python
# 主循环
while True:
    # 1. 长轮询获取消息
    result = await api_post(session, "ilink/bot/getupdates", {...}, bot_token)

    # 2. 更新游标
    get_updates_buf = result.get("get_updates_buf") or get_updates_buf

    # 3. 遍历消息
    for msg in result.get("msgs") or []:
        if msg.get("message_type") != 1:
            continue  # 只处理用户消息

        text = msg["item_list"][0]["text_item"]["text"]
        from_id = msg["from_user_id"]
        context_token = msg["context_token"]

        # 4. 更新最近联系人（重连通知用）
        last_contact["from_id"] = from_id
        last_contact["context_token"] = context_token

        # 5. 处理指令 / AI 对话
        # ... 业务逻辑 ...

        # 6. 发送"正在输入"
        # 7. 调用 AI
        # 8. 发送回复
        # 9. 取消"正在输入"
```

---

## 十二、指令系统

| 指令 | 功能 |
|------|------|
| `/help` `/指令` | 查看指令列表 |
| `/time` | 查询连接剩余时间 |
| `/重新连接` | 手动触发重连（需 Y/N 确认） |
| 其他文本 | 作为 AI 对话输入 |

**手动重连流程：**

```
用户发送 /重新连接
    │
    ├─ 重连进行中 ──▶ 回复"请稍候"
    │
    └─ 标记 pending ──▶ 回复"确认？Y/N"
                          │
                          ├─ Y ──▶ 执行重连
                          └─ N ──▶ 取消
```

---

## 十三、开发注意事项

### 13.1 必须遵守

1. **`context_token` 必填** — 回复消息时必须携带，否则消息不会送达
2. **`get_updates_buf` 游标** — 每次必须更新，否则重复收消息
3. **`bot_type=3` 固定** — 获取二维码时必须带此参数
4. **`X-WECHAT-UIN` 随机** — 每次请求都生成新的随机值

### 13.2 容错处理

1. **发送消息失败** — 降级为控制台打印，不抛异常
2. **AI 调用失败** — 返回友好错误提示，不 crash
3. **重连失败/超时** — 重置计时器，等待下次触发
4. **长轮询异常** — 循环继续，不退出主循环

### 13.3 安全提醒

- 腾讯会收集 IP 地址、操作记录等日志
- 腾讯有权对内容进行过滤/拦截
- 腾讯可随时变更或终止服务
- 不应将核心业务完全依赖此 API

### 13.4 已知限制

1. `bot_type=3` 的具体含义未完全明确
2. 需要通过 OpenClaw 平台审核/注册
3. 群聊可能需要额外权限
4. 没有拉取历史消息的 API
5. 速率限制未公开，需实测

---

## 十四、参考资源

| 资源 | 链接 |
|------|------|
| Demo 仓库 | https://github.com/hao-ji-xing/openclaw-weixin |
| API 文档 | https://github.com/hao-ji-xing/openclaw-weixin/blob/main/weixin-bot-api.md |
| npm 插件包 | https://www.npmjs.com/package/@tencent-weixin/openclaw-weixin |
| npm CLI 包 | https://www.npmjs.com/package/@tencent-weixin/openclaw-weixin-cli |
| Python 示例代码 | [docs/weixin_bot.py](weixin_bot.py) |
