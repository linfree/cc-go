"""
Re-login to WeChat iLink Bot. Renders QR code in terminal.
"""
import json, base64, random, time, urllib.request, os, sys
import qrcode

try:
    sys.stdout.reconfigure(encoding='utf-8')
except Exception:
    pass

BASE = "https://ilinkai.weixin.qq.com"
CONFIG = os.path.expanduser("~/.cc-go/config.json")

def echo(msg):
    print(msg, flush=True)

def headers(token=None):
    uin = str(random.randint(0, 0xFFFFFFFF))
    h = {
        "Content-Type": "application/json",
        "AuthorizationType": "ilink_bot_token",
        "X-WECHAT-UIN": base64.b64encode(uin.encode()).decode(),
    }
    if token:
        h["Authorization"] = f"Bearer {token}"
    return h

def api_get(path, token=None):
    req = urllib.request.Request(f"{BASE}/{path}", headers=headers(token))
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read())

echo("获取登录二维码...")
r = api_get("ilink/bot/get_bot_qrcode?bot_type=3")
qrcode_url = r.get("qrcode_img_content", "")
qrcode_hash = r["qrcode"]
echo(f"qrcode: {qrcode_hash}")
echo("")

# Render QR code in terminal (encoding the liteapp URL)
qr = qrcode.QRCode()
qr.add_data(qrcode_url)
qr.make()
qr.print_ascii()

echo("请用微信扫描上方二维码完成登录")
echo("等待扫码... (有效期约30秒)")

success = False
for i in range(180):
    try:
        s = api_get(f"ilink/bot/get_qrcode_status?qrcode={qrcode_hash}")
    except Exception:
        time.sleep(1)
        continue

    status = s.get("status", "")
    if status == "confirmed":
        new_token = s.get("bot_token", "")
        new_base = s.get("baseurl", "")
        echo("")
        echo("登录成功!")
        echo(f"token: {new_token[:40]}...")

        os.makedirs(os.path.dirname(CONFIG), exist_ok=True)
        if os.path.exists(CONFIG):
            with open(CONFIG, encoding="utf-8") as f:
                cfg = json.load(f)
        else:
            cfg = {"wechat": {}}
        cfg["wechat"]["bot_token"] = new_token
        if new_base:
            cfg["wechat"]["base_url"] = new_base
        cfg["wechat"]["login_time"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        with open(CONFIG, "w", encoding="utf-8") as f:
            json.dump(cfg, f, indent=2, ensure_ascii=False)
        echo(f"配置已更新: {CONFIG}")
        success = True
        break

    if status == "expired":
        echo("\n二维码已过期，请重新运行。")
        break

    if i % 10 == 0:
        echo(f"  状态: {status} ({i}s)")
    time.sleep(1)

if not success:
    echo("登录未完成。")