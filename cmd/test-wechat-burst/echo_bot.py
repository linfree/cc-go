"""
Echo bot — poll messages and reply with "[echo] <text>".
Used to observe API behavior without AI involvement.
"""
import json, base64, random, time, urllib.request, os, sys

try:
    sys.stdout.reconfigure(encoding='utf-8')
except Exception:
    pass

CONFIG = os.path.expanduser("~/.cc-go/config.json")

def load_cfg():
    with open(CONFIG, encoding="utf-8") as f:
        return json.load(f)

def save_cfg(cfg):
    os.makedirs(os.path.dirname(CONFIG), exist_ok=True)
    with open(CONFIG, "w", encoding="utf-8") as f:
        json.dump(cfg, f, indent=2, ensure_ascii=False)

def echo(msg):
    t = time.strftime("%H:%M:%S")
    print(f"[{t}] {msg}", flush=True)

def headers(token):
    uin = str(random.randint(0, 0xFFFFFFFF))
    return {
        "Content-Type": "application/json",
        "AuthorizationType": "ilink_bot_token",
        "X-WECHAT-UIN": base64.b64encode(uin.encode()).decode(),
        "Authorization": f"Bearer {token}",
    }

def api_get(path, token, base_url):
    req = urllib.request.Request(f"{base_url}/{path}", headers=headers(token))
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())

def api_post(path, data, token, base_url):
    req = urllib.request.Request(
        f"{base_url}/{path}",
        data=json.dumps(data).encode(),
        headers=headers(token),
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read())

def inspect(label, resp):
    """Print all keys/values, highlight token-related fields."""
    token_keys = [k for k in resp if "token" in k.lower()]
    echo(f"  [{label}] keys={list(resp.keys())}")
    if token_keys:
        echo(f"  [{label}] *** TOKEN FIELDS: {[(k, str(resp[k])[:80]) for k in token_keys]}")
    for k, v in sorted(resp.items()):
        s = str(v)
        if k in token_keys:
            echo(f"  [{label}] >>> {k}: {s[:120]}")
        elif len(s) < 200:
            echo(f"  [{label}] {k}: {s}")
        else:
            echo(f"  [{label}] {k}: {s[:60]}... ({len(s)} chars)")

# Load config
cfg = load_cfg()
token = cfg["wechat"]["bot_token"]
base_url = cfg["wechat"].get("base_url", "https://ilinkai.weixin.qq.com") or "https://ilinkai.weixin.qq.com"

echo(f"Bot启动, token={token[:40]}..., base={base_url}")

# Check if we need to login
need_login = False
try:
    r = api_post("ilink/bot/getupdates", {
        "get_updates_buf": "",
        "base_info": {"channel_version": "1.0.2"},
    }, token, base_url)
    inspect("getupdates", r)
    buf = r.get("get_updates_buf", "")
    echo(f"已连接, buf={buf[:30]}...")
except Exception as e:
    echo(f"需要重新登录: {e}")
    need_login = True

if need_login:
    echo("获取二维码...")
    r = api_get("ilink/bot/get_bot_qrcode?bot_type=3", None, base_url)
    qrcode_url = r.get("qrcode_img_content", "")
    qrcode_hash = r["qrcode"]
    echo(f"qrcode_url: {qrcode_url}")
    echo("请复制链接到微信打开，30秒内扫码")

    for i in range(180):
        try:
            s = api_get(f"ilink/bot/get_qrcode_status?qrcode={qrcode_hash}", None, base_url)
        except Exception:
            time.sleep(1)
            continue
        st = s.get("status", "")
        if st == "confirmed":
            token = s["bot_token"]
            base_url = s.get("baseurl", base_url) or base_url
            echo(f"登录成功! token={token[:40]}...")
            cfg["wechat"]["bot_token"] = token
            cfg["wechat"]["base_url"] = base_url
            cfg["wechat"]["login_time"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
            save_cfg(cfg)
            buf = ""
            break
        if st == "expired":
            echo("二维码过期，请重试")
            sys.exit(1)
        if i % 10 == 0:
            echo(f"  等待中... ({i}s)")
        time.sleep(1)
    else:
        echo("超时未扫码")
        sys.exit(1)

# Typing ticket cache
typing_ticket = None

# Main poll loop
echo("开始轮询消息... (Ctrl+C 停止)")
echo("=" * 50)

while True:
    try:
        r = api_post("ilink/bot/getupdates", {
            "get_updates_buf": buf,
            "base_info": {"channel_version": "1.0.2"},
        }, token, base_url)
    except Exception as e:
        echo(f"getupdates 错误: {e}")
        time.sleep(2)
        continue

    # Check for token in response
    token_keys = [k for k in r if "token" in k.lower()]
    if token_keys:
        echo(f"!!! getupdates 有 TOKEN 字段: {[(k, r[k][:60]) for k in token_keys]}")

    # Update buf
    new_buf = r.get("get_updates_buf", "")
    if new_buf and new_buf != buf:
        buf = new_buf
        # Save to config
        cfg["wechat"]["last_updates_buf"] = buf
        save_cfg(cfg)

    msgs = r.get("msgs", [])
    for msg in msgs:
        # Check msg for token fields
        mtk = [k for k in msg if "token" in k.lower()]
        if mtk:
            echo(f"!!! 消息中有 TOKEN: {[(k, str(msg[k])[:80]) for k in mtk]}")

        echo(f"收到消息: keys={list(msg.keys())}")
        for k, v in sorted(msg.items()):
            s = str(v)
            if len(s) < 200:
                echo(f"  {k}: {s}")
            else:
                echo(f"  {k}: {s[:80]}...")

        mt = msg.get("message_type", 0)
        if mt == 1:  # user message
            from_id = msg.get("from_user_id", "")
            ctx = msg.get("context_token", "")
            text = ""
            for item in msg.get("item_list", []):
                if item.get("type") == 1:
                    text = item.get("text_item", {}).get("text", "")
                    break

            echo(f"用户消息: from={from_id}, text={text}")

            # Save contact info
            cfg["wechat"]["last_from_id"] = from_id
            cfg["wechat"]["last_context_token"] = ctx
            save_cfg(cfg)

            # Get typing ticket if not cached
            if not typing_ticket:
                try:
                    tr = api_post("ilink/bot/getconfig", {
                        "ilink_user_id": from_id,
                        "context_token": ctx,
                        "base_info": {"channel_version": "1.0.2"},
                    }, token, base_url)
                    inspect("getconfig", tr)
                    typing_ticket = tr.get("typing_ticket", "")
                    echo(f"typing_ticket={typing_ticket[:40]}...")
                except Exception as e:
                    echo(f"getconfig 错误: {e}")

            # Send typing indicator
            if typing_ticket:
                try:
                    tr = api_post("ilink/bot/sendtyping", {
                        "ilink_user_id": from_id,
                        "typing_ticket": typing_ticket,
                        "status": 1,
                    }, token, base_url)
                except Exception:
                    pass

            time.sleep(1)

            # Echo reply
            reply = f"[echo] {text}"
            client_id = f"echo-bot-{random.randint(0, 0xFFFFFFFF):08x}"
            try:
                sr = api_post("ilink/bot/sendmessage", {
                    "msg": {
                        "from_user_id": "",
                        "to_user_id": from_id,
                        "client_id": client_id,
                        "message_type": 2,
                        "message_state": 2,
                        "context_token": ctx,
                        "item_list": [{"type": 1, "text_item": {"text": reply}}],
                    },
                    "base_info": {"channel_version": "1.0.2"},
                }, token, base_url)
                inspect("sendmessage", sr)
                echo(f"回复: {reply}")
            except Exception as e:
                echo(f"sendmessage 错误: {e}")

            # Stop typing
            if typing_ticket:
                try:
                    tr = api_post("ilink/bot/sendtyping", {
                        "ilink_user_id": from_id,
                        "typing_ticket": typing_ticket,
                        "status": 2,
                    }, token, base_url)
                except Exception:
                    pass

    # If no messages, just print a dot every ~10 cycles
    # (getupdates is long-poll, so this naturally paces)

    # Check for response-level token
    if "ret" in r and r["ret"] != 0:
        echo(f"getupdates ret={r['ret']} errmsg={r.get('errmsg','')}")