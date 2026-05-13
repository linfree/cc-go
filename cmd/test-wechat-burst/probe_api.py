"""
Probe getupdates response structure in detail.
"""
import json, base64, random, urllib.request, os, sys

try:
    sys.stdout.reconfigure(encoding='utf-8')
except Exception:
    pass

BASE = "https://ilinkai.weixin.qq.com"
CONFIG = os.path.expanduser("~/.cc-go/config.json")

with open(CONFIG, encoding="utf-8") as f:
    cfg = json.load(f)

token = cfg["wechat"]["bot_token"]
base_url = cfg["wechat"].get("base_url", BASE) or BASE
last_id = cfg["wechat"].get("last_from_id", "")
last_ctx = cfg["wechat"].get("last_context_token", "")
last_buf = cfg["wechat"].get("last_updates_buf", "")

print(f"token: {token[:50]}...")
print(f"last_from_id: {last_id}")
print()

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
        f"{base_url}/{path}",
        data=json.dumps(data).encode(),
        headers=headers(),
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read())

# 1. getconfig → typing_ticket
r = api_post("ilink/bot/getconfig", {
    "ilink_user_id": last_id,
    "context_token": last_ctx,
    "base_info": {"channel_version": "1.0.2"},
})
typing_ticket = r.get("typing_ticket", "")
print(f"typing_ticket: {typing_ticket[:60]}...")
print()

# 2. sendtyping
r = api_post("ilink/bot/sendtyping", {
    "ilink_user_id": last_id,
    "typing_ticket": typing_ticket,
    "status": 1,
})
print(f"sendtyping(status=1): {r}")
r = api_post("ilink/bot/sendtyping", {
    "ilink_user_id": last_id,
    "typing_ticket": typing_ticket,
    "status": 2,
})
print(f"sendtyping(status=2): {r}")
print()

# 3. getupdates — check response structure and buf format
print("=== getupdates (empty buf) ===")
r = api_post("ilink/bot/getupdates", {
    "get_updates_buf": "",
    "base_info": {"channel_version": "1.0.2"},
})
for k, v in sorted(r.items()):
    s = str(v)
    if len(s) < 300:
        print(f"  {k}: {s}")
    else:
        print(f"  {k}: {s[:80]}... ({len(s)} chars)")

buf = r.get("get_updates_buf", "")
sync_buf = r.get("sync_buf", "")
print(f"\nget_updates_buf length: {len(buf)}")
print(f"sync_buf length: {len(sync_buf)}")

# Try decoding the buf
try:
    decoded = base64.b64decode(buf)
    print(f"get_updates_buf decoded ({len(decoded)} bytes): {decoded[:100]}")
except Exception as e:
    print(f"get_updates_buf decode error: {e}")

# 4. Poll getupdates with buf to see if response differs
print()
print("=== getupdates (with buf) ===")
r2 = api_post("ilink/bot/getupdates", {
    "get_updates_buf": buf,
    "base_info": {"channel_version": "1.0.2"},
})
for k, v in sorted(r2.items()):
    s = str(v)
    if len(s) < 300:
        print(f"  {k}: {s}")
    else:
        print(f"  {k}: {s[:80]}... ({len(s)} chars)")

# 5. Try unknown endpoints that might return token info
print()
print("=== Trying other endpoints ===")
for path in ["ilink/bot/getinfo", "ilink/bot/status", "ilink/bot/gettoken", "ilink/bot/refresh"]:
    try:
        r = api_post(path, {})
        print(f"  {path}: {r}")
    except Exception as e:
        print(f"  {path}: {e}")