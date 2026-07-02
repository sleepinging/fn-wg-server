"""
UI 自测脚本 - 三层: API 层 / 数据层 / 图表层
用法: python test_ui.py [--server http://host/cgi/.../index.cgi]
"""
import sys, json, time, urllib.request, urllib.error, os

SERVER = os.environ.get("TEST_SERVER", "http://127.0.0.1:8080")
PASS = 0; FAIL = 0

def ok(msg):
    global PASS; PASS += 1
    print(f"  ✅ {msg}")

def ng(msg):
    global FAIL; FAIL += 1
    print(f"  ❌ {msg}")

def api(method, path, body=None):
    url = f"{SERVER}/api{path}"
    try:
        data = None
        if body: data = json.dumps(body).encode()
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.getcode(), json.loads(r.read())
    except Exception as e:
        return 0, str(e)

# ==================== 第一层: API 响应验证 ====================
def test_api():
    print("\n=== 第一层: API 响应验证 ===")
    
    # 1. /api/users 返回数组
    code, data = api("GET", "/users")
    if code == 200 and isinstance(data, list):
        ok(f"GET /users → {len(data)} users")
    else:
        ng(f"GET /users failed: {data}")
        return
    
    # 2. 每个 user 有必填字段
    required = ["id","username","online","rxSpeed","txSpeed","rxBytes","txBytes"]
    for u in data:
        for f in required:
            if f not in u:
                ng(f"user {u.get('username','?')} missing '{f}'")
                return
        # 速度是数字
        for f in ["rxSpeed","txSpeed"]:
            if not isinstance(u.get(f), (int,float)):
                ng(f"user {u.get('username')} {f} not number: {u.get(f)}")
                return
    ok("all users have required fields + numeric speed")
    
    # 3. /api/stats 全局状态
    code, s = api("GET", "/stats")
    if code == 200 and isinstance(s, dict):
        for f in ["rxSpeed","txSpeed","rxBytes","txBytes"]:
            if f not in s:
                ng(f"stats missing '{f}'")
                return
        ok(f"GET /stats rxSpeed={s['rxSpeed']:.1f} txSpeed={s['txSpeed']:.1f}")
    
    # 4. /api/stats/history 图表数据 (1小时前)
    since = int((time.time() - 3600) * 1000)
    code, chart = api("GET", f"/stats/history?userId=0&since={since}&end=0")
    if code == 200 and isinstance(chart, list):
        ok(f"GET /stats/history (1h) → {len(chart)} points")
        if len(chart) > 0:
            span = (chart[-1]["ts"] - chart[0]["ts"]) / 1000
            ok(f"  span={span:.0f}s")
            if span > 3600 * 2:
                ng(f"  span {span:.0f}s >> 1h - timestamp bug?")
    else:
        ng(f"/stats/history failed: {chart}")
    
    # 5. 15m 和 1h 应返回不同数据量 (需有 >15min 数据)
    since15 = int((time.time() - 900) * 1000)
    _, c15 = api("GET", f"/stats/history?userId=0&since={since15}&end=0")
    _, c1h = api("GET", f"/stats/history?userId=0&since={since}&end=0")
    if isinstance(c15, list) and isinstance(c1h, list):
        ok(f"15m:{len(c15)}pts  1h:{len(c1h)}pts  (different={len(c15)!=len(c1h)})")

# ==================== 第二层: 数据一致性验证 ====================
def test_data():
    print("\n=== 第二层: 数据一致性验证 ===")

    _, users = api("GET", "/users")
    if not isinstance(users, list) or len(users) == 0:
        ng("no users to test")
        return

    # 1. 在线用户应有 rxBytes > 0 或明确为 0
    online = [u for u in users if u.get("online")]
    ok(f"{len(online)} online users")
    for u in online:
        if u["rxBytes"] == 0 and u["txBytes"] == 0:
            ng(f"{u['username']} online but 0 bytes transferred")

    # 2. 流量单调递增检查 (两次采样间隔 3s)
    def get_rx(u):
        _, d = api("GET", f"/users/{u['id']}")
        return d.get("rxBytes", 0) if isinstance(d, dict) else 0
    for u in online[:2]:  # 只测前两个
        r1 = get_rx(u)
        time.sleep(3)
        r2 = get_rx(u)
        if r2 >= r1:
            ok(f"{u['username']} rxBytes monotonic: {r1} → {r2}")
        else:
            ng(f"{u['username']} rxBytes DECREASED: {r1} → {r2}")

    # 3. 每个用户的历史查询
    for u in users[:2]:
        _, chart = api("GET", f"/users/{u['id']}/traffic?since=0&end=0")
        if isinstance(chart, dict) and "chart" in chart:
            pts = chart["chart"]
            ok(f"  {u['username']}: {len(pts)} history points")
            if len(pts) > 1:
                # 时间戳严格递增
                for i in range(1, len(pts)):
                    if pts[i]["ts"] <= pts[i-1]["ts"]:
                        ng(f"  {u['username']} ts not monotonic at idx {i}")
                        break
                else:
                    ok(f"  {u['username']} ts strictly increasing")

# ==================== 第三层: 图表域/时间范围验证 ====================
def test_chart_domain():
    print("\n=== 第三层: 图表域验证 ===")

    # 取第一个 user
    _, users = api("GET", "/users")
    if not isinstance(users, list) or len(users) == 0:
        ng("no users")
        return
    uid = users[0]["id"]

    for label, since_s in [("15m", 900), ("1h", 3600), ("6h", 21600)]:
        since = int((time.time() - since_s) * 1000)
        _, data = api("GET", f"/users/{uid}/traffic?since={since}&end=0")
        if not isinstance(data, dict) or "chart" not in data:
            ng(f"{label}: no chart data")
            continue
        pts = data["chart"]
        if len(pts) == 0:
            ok(f"{label}: 0 points (no data yet)")
            continue
        first = pts[0]["ts"]
        last = pts[-1]["ts"]
        span_s = (last - first) / 1000
        expected = since_s

        # 锚点存在 = 首点应接近 since
        gap_s = (first - since) / 1000
        if gap_s < 60:
            ok(f"{label}: anchor present (gap={gap_s:.0f}s)")
        else:
            ng(f"{label}: anchor MISSING (gap={gap_s:.0f}s, should add padTimeRange anchor)")

        # 跨度不应超过期望
        if span_s <= expected * 1.1:
            ok(f"{label}: span={span_s:.0f}s ≤ {expected}s")
        else:
            ng(f"{label}: span={span_s:.0f}s > {expected}s")

def main():
    print("=" * 50)
    print("WG-Server UI 自测")
    print(f"Server: {SERVER}")
    print(f"Time: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 50)

    test_api()
    test_data()
    test_chart_domain()

    print(f"\n{'='*50}")
    print(f"结果: {PASS} passed / {FAIL} failed")
    if FAIL > 0:
        print("❌ RE-FAILED - 需要修复")
        sys.exit(1)
    else:
        print("✅ ALL PASS - 通过")

if __name__ == "__main__":
    main()
