"""
Test: does /ilink/bot/getconfig return a fresh bot_token during normal use?
"""
import json, base64, random, urllib.request, os

BASE = "https://ilinkai.weixin.qq.com"
CONFIG = os.path.expanduser("~/.cc-go/config.json")

with open(CONFIG, encoding="utf-8") as f:
    cfg = json.load(f)

token = cfg["wechat"]["bot_token"]
last_id = cfg["wechat"].get("last_from_id", "")
last_ctx = cfg["wechat"].get("last_context_token", "")

def headers():
    uin = str(random.randint(0, 0xFFFFFFFF))
    return {
        "Content-Type": "application/json",
        "AuthorizationType": "ilink_bot_token",
        "X-WECHAT-UIN": base64.b64encode(uin.encode()).decode(),
        "Authorization": f"Bearer {token}",
    }

def api_post(path, data):
    req = urllib.request.Request(
        f"{BASE}/{path}",
        data=json.dumps(data).encode(),
        headers=headers(),
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())

print(f"当前 token: {token[:50]}...")
print(f"last_from_id: {last_id}")
print()

if last_id and last_ctx:
    print("=== 调用 /ilink/bot/getconfig ===")
    result = api_post("ilink/bot/getconfig", {
        "ilink_user_id": last_id,
        "context_token": last_ctx,
        "base_info": {"channel_version": "1.0.2"},
    })

    ret = result.get("ret", "N/A")
    print(f"ret: {ret}")

    # Check if there's a new bot_token
    token_keys = [k for k in result if "token" in k.lower()]
    print(f"包含 token 的字段: {token_keys}")
    for k in token_keys:
        v = str(result[k])
        print(f"  {k}: {v[:60]}...")

    # Show all keys and short values
    for k in sorted(result.keys()):
        v = str(result[k])
        if len(v) < 200:
            print(f"  {k}: {v}")
        else:
            print(f"  {k}: {v[:60]}... ({len(v)} chars)")

    # Compare tokens
    new_token = result.get("bot_token", "")
    if new_token and new_token != token:
        print(f"\n*** 发现新 token! 旧: {token[:30]}... -> 新: {new_token[:30]}...")
    elif new_token == token:
        print(f"\ntoken 未变化（与当前相同）")
    else:
        print(f"\n响应中没有 bot_token 字段")
else:
    print("缺少 last_from_id 或 last_context_token，无法测试")
    print("请先在微信给机器人发一条消息")