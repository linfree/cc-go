# WeChat iLink Bot API 参考

> **注意**: 非官方文档，全部来自实际测试和第三方代码。2026-05-13 整理。

## 概述

iLink Bot 是微信企业版的机器人接口。**没有公开的官方文档**，所有信息来自逆向工程。

Base URL: `https://ilinkai.weixin.qq.com`（扫码后可能返回动态 `baseurl`）

---

## API 端点

### 1. GET `/ilink/bot/get_bot_qrcode?bot_type=3`

**用途**: 获取登录二维码

**认证**: 不需要 token

**响应**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `qrcode` | string | 二维码标识符，用于轮询状态 |
| `qrcode_img_content` | string | 二维码内容。可能是: ① `https://liteapp.weixin.qq.com/q/...` 链接 ② `data:image/...;base64,...` ③ SVG字符串 ④ 裸 base64 |

**实际观察**:
- `qrcode_img_content` 返回的是 liteapp URL: `https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=<hash>&bot_type=3`
- 二维码有效期约 30 秒，过期后 status 返回 `"expired"`
- `bot_type=3` 含义不明，固定值

### 2. GET `/ilink/bot/get_qrcode_status?qrcode=<hash>`

**用途**: 轮询扫码状态

**认证**: 不需要 token

**响应**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | `"confirmed"` = 已扫码，其他 = 等待中，`"expired"` = 已过期 |
| `bot_token` | string | **仅在 status=confirmed 时返回**。格式: `<hash>@im.bot:<hex>` |
| `baseurl` | string | **仅在 status=confirmed 时返回**。动态 base URL，可能为空 |

### 3. POST `/ilink/bot/getupdates`

**用途**: 长轮询接收消息

**认证**: 需要 Bearer token

**请求体**:
```json
{
  "get_updates_buf": "<上次返回的 buf，首次为空字符串>",
  "base_info": { "channel_version": "1.0.2" }
}
```

**响应**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `msgs` | array | 消息数组，可能为空 |
| `get_updates_buf` | string | **必须保存**，下次轮询时原样传回。base64 编码的 protobuf，解码后含 bot_token 前缀 |
| `sync_buf` | string | 同步缓冲，base64 编码，较短 |

**消息对象** (`msgs[]`):

完整字段 (来自实际测试):
| 字段 | 类型 | 说明 |
|------|------|------|
| `seq` | int | 消息序号，递增 |
| `message_id` | int | 服务端消息 ID (如 `7460172331083428744`) |
| `from_user_id` | string | 发送者 ID，格式 `xxx@im.wechat` |
| `to_user_id` | string | 接收者 ID，格式 `xxx@im.bot` |
| `client_id` | string | 客户端消息 ID (如 `mmassistant_bypmsg_inbox_...`) |
| `create_time_ms` | int | 创建时间戳(ms) |
| `update_time_ms` | int | 更新时间戳(ms) |
| `delete_time_ms` | int | 删除时间戳，0=未删除 |
| `session_id` | string | 会话 ID (私聊为空) |
| `group_id` | string | 群 ID (私聊为空) |
| `message_type` | int | `1` = 用户消息，`2` = 机器人发出的消息 |
| `message_state` | int | 通常为 `2` |
| `context_token` | string | **必须保存**，回复时传回，维持会话上下文。⚠️ 注意这是 context_token 不是 bot_token |
| `item_list` | array | 内容项数组 |

**item_list 元素**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | int | `1` = 文本 |
| `create_time_ms` | int | 创建时间 |
| `update_time_ms` | int | 更新时间 |
| `is_completed` | bool | 是否完成 |
| `button_item_list` | array | 按钮列表 (文本消息为空) |
| `text_item.text` | string | 消息文本内容 |

**关键发现**:
- `get_updates_buf` 解码后是 protobuf 格式，包含 `bot_token` 前缀（如 `56c6bf802dc7@im.bot:0600...`）
- 响应中**没有 bot_token 字段**（已确认）
- 每条消息中**没有 token 字段**（已确认）
- 这是一个标准的长轮询——无新消息时会阻塞等待

### 4. POST `/ilink/bot/sendmessage`

**用途**: 发送消息

**认证**: 需要 Bearer token

**请求体**:
```json
{
  "msg": {
    "from_user_id": "",
    "to_user_id": "<对方 user_id>",
    "client_id": "<唯一 ID，格式: prefix-8位hex>",
    "message_type": 2,
    "message_state": 2,
    "context_token": "<从收到的消息中获取>",
    "item_list": [
      { "type": 1, "text_item": { "text": "<消息内容>" } }
    ]
  },
  "base_info": { "channel_version": "1.0.2" }
}
```

**注意**:
- `from_user_id` 固定为空字符串，服务端自动填充
- `client_id` 用于去重/幂等，前缀可以是任意标识（参考实现用 `openclaw-weixin-` 或 `cc-go-`）
- **10 条消息限制**: 用户发一条消息后，机器人最多回复 10 条。超过后服务端拒绝

**响应**: 空对象 `{}`

### 5. POST `/ilink/bot/getconfig`

**用途**: 获取 typing_ticket（"正在输入..."状态所需的票据）

**认证**: 需要 Bearer token

**请求体**:
```json
{
  "ilink_user_id": "<用户 ID>",
  "context_token": "<上下文 token>",
  "base_info": { "channel_version": "1.0.2" }
}
```

**响应**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `ret` | int | `0` = 成功, `-4` = token 过期 |
| `typing_ticket` | string | 用于 sendtyping，约 448 字符 |
| `errmsg` | string | 错误时返回，如 "GetTypingTicket rpc failed" |

**发现**:
- ret=-4 时 token 已过期，需要重新扫码
- **不返回新 bot_token**（已确认）

### 6. POST `/ilink/bot/sendtyping`

**用途**: 控制"正在输入..."状态

**认证**: 需要 Bearer token

**请求体**:
```json
{
  "ilink_user_id": "<用户 ID>",
  "typing_ticket": "<从 getconfig 获取>",
  "status": 1
}
```

**status 值**: `1` = 开始显示, `2` = 停止显示

**响应**: `{"ret": 0}`

**注意**: typing 状态有超时，需要心跳维持（每 5 秒重发 status=1）

### ❌ 不存在的端点

以下端点返回 404:
- `/ilink/bot/getinfo`
- `/ilink/bot/status`
- `/ilink/bot/gettoken`
- `/ilink/bot/refresh`

---

## 认证机制

### 请求头
```
Content-Type: application/json
AuthorizationType: ilink_bot_token
X-WECHAT-UIN: <base64(随机 uint32)>
Authorization: Bearer <bot_token>
```

### Token 格式
```
<hash>@im.bot:<hex>
例: 56c6bf802dc7@im.bot:060000d4b286c7d47ea4b48291bc1e87fbddbe
```

### Token 生命周期
- **唯一获取方式**: 扫码登录时 `get_qrcode_status` 返回
- **有效期**: 约 24 小时
- **续期机制**: ❌ **通信过程中不会自动续期**（已通过检查所有 API 响应确认无 token 字段）
- **过期表现**: `getconfig` 返回 `ret: -4`
- **过期后**: 必须重新扫码

---

## 消息格式

### 接收 (message_type=1)
```json
{
  "from_user_id": "o9cq807dPGGB94AbyAn-jj7J69kg@im.wechat",
  "to_user_id": "xxx@im.bot",
  "message_type": 1,
  "message_state": 2,
  "context_token": "AARzJWAFAAABAAAAAADeZGM7m2oSBQ...",
  "item_list": [
    { "type": 1, "text_item": { "text": "你好" } }
  ]
}
```

### 发送 (message_type=2)
```json
{
  "msg": {
    "from_user_id": "",
    "to_user_id": "xxx@im.wechat",
    "client_id": "cc-go-xxxxxxxx",
    "message_type": 2,
    "message_state": 2,
    "context_token": "<从接收消息中获取>",
    "item_list": [
      { "type": 1, "text_item": { "text": "回复内容" } }
    ]
  },
  "base_info": { "channel_version": "1.0.2" }
}
```

---

## 限制与边界

| 限制 | 说明 |
|------|------|
| 10 条消息/轮 | 用户发一条消息后，机器人最多连续回复 10 条 |
| Token 24h 过期 | 不支持自动续期，必须重新扫码 |
| 二维码 30s 过期 | 获取二维码后需在约 30 秒内扫码 |
| 消息长度 | 参考实现限制 3500 字符 |
| typing 超时 | 需要 5 秒心跳维持 |

---

## 待确认

- [ ] `get_updates_buf` 的 protobuf 完整结构
- [ ] `sync_buf` 的作用
- [ ] 是否支持 `item_list` 中 text 以外的类型（图片、卡片等）
- [ ] 24h 是否精确，是否有宽限期
- [ ] 并发连接同一 token 的行为
- [ ] `bot_type` 其他可能的值

---

## 参考文件

- `docs/weixin_bot.py` — 第三方 Python 参考实现（非官方）
- `internal/wechat/client.go` — Go 客户端实现
- `cmd/test-wechat-burst/relogin.py` — 扫码登录脚本
- `cmd/test-wechat-burst/echo_bot.py` — 测试用 echo 机器人
- `cmd/test-wechat-burst/probe_api.py` — API 探测脚本