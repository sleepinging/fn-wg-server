#!/usr/bin/env python3
"""WireGuard Server API 集成测试
在飞牛服务器上直接执行，测试所有后端 API。
自动生成测试数据，并在最后清理。
"""

import json
import os
import sqlite3
import subprocess
import sys
import time
import random
import re

CGI = "/vol1/@appcenter/wg-server/ui/index.cgi"
DB = "/vol1/@appdata/wg-server/wg-server.db"

PASS = 0
FAIL = 0

def ok(msg):
    global PASS
    PASS += 1
    print(f"  [PASS] {msg}")

def fail(msg):
    global FAIL
    FAIL += 1
    print(f"  [FAIL] {msg}")

def call_api(method, path, body=None, query=""):
    """调用 CGI API 并返回 JSON 响应"""
    env = {
        "GATEWAY_INTERFACE": "CGI",
        "REQUEST_METHOD": method,
        "SCRIPT_NAME": "/cgi/ThirdParty/wg-server/index.cgi",
        "PATH_INFO": path,
        "QUERY_STRING": query,
        "SERVER_PROTOCOL": "HTTP/1.1",
    }
    if body is not None:
        body_str = json.dumps(body)
        env["CONTENT_TYPE"] = "application/json"
        env["CONTENT_LENGTH"] = str(len(body_str))
        input_data = body_str
    else:
        body_str = ""
        input_data = None

    # 构建 shell 命令
    env_parts = " ".join(f'{k}="{v}"' for k, v in env.items())
    if input_data:
        cmd = f'echo {repr(input_data)} | {env_parts} timeout 5 {CGI} 2>/dev/null'
    else:
        cmd = f'{env_parts} timeout 5 {CGI} 2>/dev/null'

    result = subprocess.run(["bash", "-c", cmd], capture_output=True, text=True, timeout=10)
    # 取最后一行（JSON 响应）
    lines = result.stdout.strip().split('\n')
    last_line = lines[-1].strip() if lines else ""
    try:
        return json.loads(last_line)
    except:
        return {"_raw": result.stdout[:500], "_error": last_line}


def main():
    global PASS, FAIL
    
    print("=" * 60)
    print("  WireGuard Server API 集成测试")
    print("=" * 60)
    print()
    
    created_users = []
    test_ips = [150, 151, 152]  # 用大号 IP 避免冲突
    
    # ========== 第 1 步：准备测试数据 ==========
    print("[1/7] 生成测试数据...")
    
    for i, ip_suffix in enumerate(test_ips):
        uname = f"test-user-{i+1:02d}"
        ip = f"192.168.5.{ip_suffix}/32"
        r = call_api("POST", "/api/users", {
            "username": uname,
            "internalIP": ip,
            "dns": "114.114.114.114"
        })
        if "id" in r:
            created_users.append(r["id"])
            ok(f"创建用户 {uname} (ID={r['id']}, IP={ip})")
        else:
            fail(f"创建用户 {uname} 失败: {r.get('error', str(r)[:100])}")
    
    if not created_users:
        fail("无法创建测试用户，终止")
        sys.exit(1)
    
    print(f"  共 {len(created_users)} 个测试用户\n")
    
    # 生成带宽和连接历史
    conn = sqlite3.connect(DB)
    c = conn.cursor()
    now = time.time()
    
    for uid in created_users:
        # 带宽数据：每 5 分钟一个点，覆盖 2 小时
        rx, tx = random.randint(10**6, 10**7), random.randint(5*10**5, 5*10**6)
        for j in range(25):
            ts = time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(now - 7200 + j * 300))
            rx += random.randint(1000, 50000)
            tx += random.randint(500, 25000)
            c.execute("""INSERT INTO bandwidth_history 
                (user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp)
                VALUES (?,?,?,?,?,?)""",
                (uid, rx, tx, random.uniform(1000, 50000), random.uniform(500, 25000), ts))
        # 全局带宽
        ts = time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(now - 7200))
        c.execute("""INSERT INTO bandwidth_history 
            (user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp)
            VALUES (0,?,?,?,?,?)""",
            (rx, tx, random.uniform(5000, 100000), random.uniform(3000, 50000), ts))
        
        # 连接历史：5 天
        for day in range(5):
            base = now - day * 86400 - random.randint(0, 43200)
            conn_t = time.localtime(base)
            disc_t = time.localtime(base + random.randint(3600, 28800))
            rx_c = random.randint(10**6, 5*10**8)
            tx_c = random.randint(5*10**5, 25*10**7)
            ext_ip = f"{random.randint(1,255)}.{random.randint(0,255)}.{random.randint(0,255)}.{random.randint(1,255)}"
            c.execute("""INSERT INTO connection_log 
                (user_id, username, internal_ip, external_ip, connected_at, disconnected_at, rx_bytes, tx_bytes)
                VALUES (?,?,?,?,?,?,?,?)""",
                (uid, f"test-user-{uid:02d}", f"192.168.5.{150+created_users.index(uid)}/32",
                 ext_ip, time.strftime('%Y-%m-%d %H:%M:%S', conn_t),
                 time.strftime('%Y-%m-%d %H:%M:%S', disc_t), rx_c, tx_c))
    
    conn.commit()
    conn.close()
    ok(f"生成带宽数据 (25点/用户 + 全局) + 连接历史 (5条/用户)")
    print()
    
    # ========== 第 2 步：系统信息 ==========
    print("[2/7] 系统信息...")
    
    r = call_api("GET", "/api/system")
    for key in ["version", "cpuUsage", "memory", "uptime"]:
        if key in r:
            pass  # ok
        else:
            fail(f"GET /api/system 缺少字段: {key}")
    ok(f"GET /api/system (version={r.get('version','?')}, cpu={r.get('cpuUsage','?')}, mem={r.get('memory',{}).get('usedPercent','?')})")
    print()
    
    # ========== 第 3 步：WireGuard 内核 ==========
    print("[3/7] WireGuard 内核状态...")
    
    r = call_api("GET", "/api/wg/kernel")
    if "moduleLoaded" in r and "kernelVersion" in r:
        ok(f"GET /api/wg/kernel (moduleLoaded={r['moduleLoaded']})")
    else:
        fail("GET /api/wg/kernel 字段缺失")
    print()
    
    # ========== 第 4 步：用户管理 ==========
    print("[4/7] 用户管理...")
    
    # 列表
    r = call_api("GET", "/api/users")
    if isinstance(r, list) and len(r) >= 3:
        ok(f"GET /api/users 返回 {len(r)} 个用户")
    else:
        fail(f"GET /api/users 返回 {type(r).__name__}")

    # 单个用户
    r = call_api("GET", f"/api/users/{created_users[0]}")
    if r.get("username") == "test-user-01":
        ok("GET /api/users/{id} 返回正确用户")
    else:
        fail(f"GET /api/users/{created_users[0]} 用户名不匹配: {r.get('username')}")
    
    # 更新用户
    r = call_api("PUT", f"/api/users/{created_users[0]}", {"username": "test-user-01-updated"})
    r2 = call_api("GET", f"/api/users/{created_users[0]}")
    if r2.get("username") == "test-user-01-updated":
        ok("PUT /api/users/{id} 更新用户名成功")
    else:
        fail("PUT /api/users/{id} 更新失败")
    # 恢复
    call_api("PUT", f"/api/users/{created_users[0]}", {"username": "test-user-01"})
    
    # IP 提示
    r = call_api("GET", "/api/ip/hint")
    if "ip" in r and "/32" in r["ip"]:
        ok(f"GET /api/ip/hint => {r['ip']}")
    else:
        fail(f"GET /api/ip/hint 返回无效: {r}")
    print()
    
    # ========== 第 5 步：用户详情与统计 ==========
    print("[5/7] 用户详情与统计...")
    
    # 用户统计
    r = call_api("GET", f"/api/users/{created_users[0]}/stats")
    if r.get("username") and r.get("internalIP"):
        ok(f"GET /api/users/{created_users[0]}/stats (user={r['username']})")
    else:
        fail(f"GET /api/users/{created_users[0]}/stats 字段缺失")
    
    # 连接历史
    r = call_api("GET", f"/api/users/{created_users[0]}/history", query="page=1&pageSize=10")
    if "data" in r and "total" in r:
        ok(f"GET /api/users/{created_users[0]}/history ({r['total']} 条)")
    else:
        fail("GET /api/users/{id}/history 字段缺失")
    
    # 流量+图表
    r = call_api("GET", f"/api/users/{created_users[0]}/traffic",
                 query="start=2026-01-01T00:00:00Z&end=2027-01-01T00:00:00Z")
    if "totalRx" in r and "chart" in r:
        chart_count = len(r["chart"])
        ok(f"GET /api/users/{created_users[0]}/traffic ({chart_count} 个数据点)")
    else:
        fail("GET /api/users/{id}/traffic 字段缺失")
    print()
    
    # ========== 第 6 步：全局统计与图表 ==========
    print("[6/7] 全局统计与图表...")
    
    r = call_api("GET", "/api/stats")
    for key in ["rxBytes", "txBytes", "onlineCount", "externalIP", "uptime"]:
        if key not in r:
            fail(f"GET /api/stats 缺少字段: {key}")
    ok(f"GET /api/stats (online={r.get('onlineCount',0)}, extIP={r.get('externalIP','?')})")
    
    r = call_api("GET", "/api/stats/history", query="userId=0&start=2026-01-01T00:00:00Z&end=2027-01-01T00:00:00Z")
    if isinstance(r, list):
        ok(f"GET /api/stats/history ({len(r)} 个数据点)")
    else:
        fail("GET /api/stats/history 不是数组")
    print()
    
    # ========== 第 7 步：配置 ==========
    print("[7/7] 服务配置...")
    
    r = call_api("GET", "/api/config")
    if "wireguard" in r:
        wg = r["wireguard"]
        ok(f"GET /api/config (port={wg.get('listenPort','?')}, subnet={wg.get('address','?')})")
    else:
        fail("GET /api/config 缺少 wireguard 字段")
    
    r = call_api("GET", "/api/config/backup")
    if "version" in r and "users" in r:
        ok(f"GET /api/config/backup ({len(r['users'])} 个用户)")
    else:
        fail("GET /api/config/backup 字段缺失")
    print()
    
    # ========== 第 8 步：运行日志 ==========
    print("[8/7] 运行日志...")
    
    r = call_api("GET", "/api/logs", query="page=1&pageSize=5")
    if "data" in r and "total" in r:
        ok(f"GET /api/logs ({r['total']} 条)")
    else:
        fail("GET /api/logs 字段缺失")
    print()
    
    # ========== 清理 ==========
    print("=" * 60)
    print("  清理测试数据...")
    print("=" * 60)
    
    # 删除用户
    for uid in created_users:
        r = call_api("DELETE", f"/api/users/{uid}")
        if "message" in r:
            print(f"  删除用户 ID={uid}: {r['message']}")
    
    # 清理数据库残留
    conn = sqlite3.connect(DB)
    c = conn.cursor()
    c.execute("DELETE FROM bandwidth_history WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'test-user-%')")
    c.execute("DELETE FROM bandwidth_history WHERE user_id=0 AND timestamp < datetime('now', '-1 hour')")
    c.execute("DELETE FROM connection_log WHERE username LIKE 'test-user-%'")
    c.execute("DELETE FROM users WHERE username LIKE 'test-user-%'")
    conn.commit()
    conn.close()
    print("  数据库清理完成")
    print()
    
    # ========== 结果 ==========
    total = PASS + FAIL
    print("=" * 60)
    print(f"  测试结果: {PASS}/{total} 通过, {FAIL} 失败")
    if FAIL == 0:
        print("  ALL TESTS PASSED!")
    print("=" * 60)
    
    # 最终验证
    r = call_api("GET", "/api/users")
    final_count = len(r) if isinstance(r, list) else -1
    print(f"  最终用户数: {final_count} (应为 0)")
    
    return FAIL


if __name__ == "__main__":
    sys.exit(main())
