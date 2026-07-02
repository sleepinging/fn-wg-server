"""
从飞牛 fnOS 抓取 WireGuard API 真实数据，保存为 JSON fixtures。
用法: python capture-data.py
需要 SSH 连接到 your-server.example.com:22
"""
import paramiko, json, time, re, os, sys

HOST = "your-server.example.com"
PORT = 22
USER = "your-user"
PASS = "REMOVED"
CGI = "/vol1/@appcenter/wg-server/ui/index.cgi"
DATA_DIR = "/vol1/@appdata/wg-server"
OUT = os.path.join(os.path.dirname(__file__), "..", "server", "fixtures")

def ssh():
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    return c

def cgi(path, query=""):
    """调用 CGI 并返回 JSON"""
    env = f"TRIM_PKGVAR={DATA_DIR} GATEWAY_INTERFACE=CGI"
    url = f"PATH_INFO=/cgi/ThirdParty/wg-server/index.cgi{path}"
    if query:
        url += f" QUERY_STRING='{query}'"
    cmd = f"{env} {url} REQUEST_METHOD=GET timeout 10 {CGI}"
    _, out, err = c.exec_command(cmd, timeout=15)
    text = out.read().decode()
    # 提取 JSON: CGI 输出可能包含 HTTP 头
    m = re.search(r'\{.*\}|\[.*\]', text, re.DOTALL)
    if m:
        return json.loads(m.group())
    print(f"  WARN: no JSON in response: {text[:200]}")
    return None

os.makedirs(OUT, exist_ok=True)
c = ssh()

print("=== 1. 抓取用户列表 ===")
users = cgi("/api/users")
if users:
    with open(os.path.join(OUT, "users.json"), "w") as f:
        json.dump(users, f, indent=2)
    print(f"  {len(users)} users → users.json")
    for u in users:
        print(f"    {u['username']:12s} online={u.get('online')} rx={u.get('rxBytes',0)}")
else:
    print("  FAILED")
    sys.exit(1)

print("\n=== 2. 抓取全局状态 ===")
stats = cgi("/api/stats")
if stats:
    with open(os.path.join(OUT, "stats.json"), "w") as f:
        json.dump(stats, f, indent=2)
    print(f"  rxSpeed={stats.get('rxSpeed',0):.1f} txSpeed={stats.get('txSpeed',0):.1f} → stats.json")

print("\n=== 3. 抓取全局图表历史 ===")
now_ms = int(time.time() * 1000)
ranges = [
    ("15m", now_ms - 900_000),
    ("1h",  now_ms - 3_600_000),
    ("6h",  now_ms - 21_600_000),
]
for label, since in ranges:
    data = cgi("/api/stats/history", f"userId=0&since={since}&end=0")
    if isinstance(data, list):
        fname = f"history_global_{label}.json"
        with open(os.path.join(OUT, fname), "w") as f:
            json.dump(data, f, indent=2)
        span = (data[-1]['ts'] - data[0]['ts']) / 1000 if len(data) > 1 else 0
        print(f"  {label}: {len(data)}pts span={span:.0f}s → {fname}")

print("\n=== 4. 抓取每个用户图表历史 ===")
for u in (users or []):
    uid = u['id']
    for label, since in ranges:
        data = cgi(f"/users/{uid}/traffic", f"since={since}&end=0")
        if isinstance(data, dict) and "chart" in data:
            fname = f"history_u{uid}_{label}.json"
            with open(os.path.join(OUT, fname), "w") as f:
                json.dump(data["chart"], f, indent=2)
            span = (data["chart"][-1]['ts'] - data["chart"][0]['ts']) / 1000 if len(data["chart"]) > 1 else 0
            print(f"  {u['username']} {label}: {len(data['chart'])}pts → {fname}")

print(f"\n=== 完成: fixtures 已保存到 {OUT} ===")
c.close()
