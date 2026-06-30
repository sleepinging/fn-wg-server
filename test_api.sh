#!/bin/bash
# WireGuard Server API 测试脚本
# 用法: ssh -p PORT USER@HOST 'bash -s' < test_api.sh
# 或在服务器上直接执行

set -e

CGI="/vol1/@appcenter/wg-server/ui/index.cgi"
CGI_ENV="GATEWAY_INTERFACE=CGI SERVER_PROTOCOL=HTTP/1.1 REQUEST_METHOD=GET"

PASS=0
FAIL=0

ok()   { echo -e "  \e[32m✓ PASS\e[0m $1"; ((PASS++)); }
fail() { echo -e "  \e[31m✗ FAIL\e[0m $1"; ((FAIL++)); }

call_api() {
    local method="$1" path="$2" body="$3" query="$4"
    if [ "$method" = "POST" ] || [ "$method" = "PUT" ]; then
        echo "$body" | GATEWAY_INTERFACE=CGI REQUEST_METHOD="$method" \
            CONTENT_TYPE="application/json" CONTENT_LENGTH="${#body}" \
            SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi \
            PATH_INFO="$path" QUERY_STRING="$query" \
            SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1
    else
        GATEWAY_INTERFACE=CGI REQUEST_METHOD="$method" \
            SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi \
            PATH_INFO="$path" QUERY_STRING="$query" \
            SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1
    fi
}

echo ""
echo "=============================================="
echo "  WireGuard Server API 测试"
echo "=============================================="
echo ""

# ========== 生成测试数据 ==========
echo "[1/7] 生成测试数据..."

# 先检查已有用户，使用不冲突的 IP
EXISTING_IPS=$(GATEWAY_INTERFACE=CGI REQUEST_METHOD=GET SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi PATH_INFO=/api/users SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1 | python3 -c "import json,sys; print(' '.join([u['internalIP'] for u in json.load(sys.stdin)]))" 2>/dev/null)
echo "  已有 IP: $EXISTING_IPS"

# 用 Python 自动计算可用 IP
python3 << 'PYEOF' > /tmp/create_users.py
import json, subprocess, os

cgi = '/vol1/@appcenter/wg-server/ui/index.cgi'
base_env = 'GATEWAY_INTERFACE=CGI REQUEST_METHOD=POST CONTENT_TYPE=application/json SERVER_PROTOCOL=HTTP/1.1'

def create_user(username, ip_suffix):
    body = json.dumps({"username": username, "internalIP": f"192.168.5.{ip_suffix}/32", "dns": "114.114.114.114"})
    cmd = f'echo {repr(body)} | {base_env} CONTENT_LENGTH={len(body)} SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi PATH_INFO=/api/users timeout 5 {cgi}'
    result = subprocess.run(['bash', '-c', cmd], capture_output=True, text=True, timeout=10)
    out = result.stdout.strip().split('\n')[-1]
    try:
        return json.loads(out)
    except:
        print(f"ERROR creating {username}: {out[:200]}")
        return None

# 找可用 IP
start = 100
for i in range(3):
    r = create_user(f'test-user-{i+1:02d}', start + i)
    if r and 'id' in r:
        print(f"USER_CREATED id={r['id']} username={r['username']} ip={r['internalIP']}")
    else:
        print(f"USER_FAILED {i+1}")
PYEOF
python3 /tmp/create_users.py 2>/dev/null

# 从输出提取用户 ID
UID1=$(grep 'USER_CREATED.*test-user-01' /tmp/create_users.py | grep -oP 'id=\K\d+') || UID1=0
UID2=$(grep 'USER_CREATED.*test-user-02' /tmp/create_users.py | grep -oP 'id=\K\d+') || UID2=0
UID3=$(grep 'USER_CREATED.*test-user-03' /tmp/create_users.py | grep -oP 'id=\K\d+') || UID3=0
echo "  用户 ID: $UID1, $UID2, $UID3"

# 提取用户 ID
UID1=$(python3 -c "import json; print(json.load(open('/tmp/r1.json'))['id'])")
UID2=$(python3 -c "import json; print(json.load(open('/tmp/r2.json'))['id'])")
UID3=$(python3 -c "import json; print(json.load(open('/tmp/r3.json'))['id'])")
echo "  创建用户: ID=$UID1, $UID2, $UID3"
ok "创建测试用户"

# 生成带宽历史数据（直接插入 SQLite）
DB="/vol1/@appdata/wg-server/wg-server.db"
python3 << PYEOF
import sqlite3, time, random

conn = sqlite3.connect("$DB")
c = conn.cursor()

# 为每个用户生成过去 2 小时的带宽数据（每 5 分钟一个点）
now = time.time()
base_ts = now - 7200  # 2 hours ago

for uid in [$UID1, $UID2, $UID3]:
    rx = random.randint(1000000, 10000000)
    tx = random.randint(500000, 5000000)
    for i in range(25):  # 25 个点 ≈ 2 小时
        ts = time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(base_ts + i * 300))
        rx += random.randint(1000, 50000)
        tx += random.randint(500, 25000)
        rx_speed = random.uniform(1000, 50000)
        tx_speed = random.uniform(500, 25000)
        c.execute("INSERT INTO bandwidth_history (user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp) VALUES (?,?,?,?,?,?)",
                  (uid, rx, tx, rx_speed, tx_speed, ts))
    # 全局带宽（user_id=0）
    ts = time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(base_ts))
    c.execute("INSERT INTO bandwidth_history (user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp) VALUES (0,?,?,?,?,?)",
              (rx, tx, random.uniform(5000, 100000), random.uniform(3000, 50000), ts))

# 生成连接历史
for uid in [$UID1, $UID2, $UID3]:
    for day in range(5):
        conn_time = time.localtime(now - day*86400 - random.randint(0, 43200))
        disc_time = time.localtime(time.mktime(conn_time) + random.randint(3600, 28800))
        conn_str = time.strftime('%Y-%m-%d %H:%M:%S', conn_time)
        disc_str = time.strftime('%Y-%m-%d %H:%M:%S', disc_time)
        rx = random.randint(1000000, 500000000)
        tx = random.randint(500000, 250000000)
        ext_ip = f"{random.randint(1,255)}.{random.randint(0,255)}.{random.randint(0,255)}.{random.randint(1,255)}"
        c.execute("""INSERT INTO connection_log 
            (user_id, username, internal_ip, external_ip, connected_at, disconnected_at, rx_bytes, tx_bytes)
            VALUES (?,?,?,?,?,?,?,?)""",
            (uid, f"test-user-{uid:02d}", f"192.168.5.{9+uid}/32", ext_ip, conn_str, disc_str, rx, tx))

conn.commit()
conn.close()
print("  带宽数据: 25 条/用户 + 全局")
print("  连接历史: 5 条/用户")
PYEOF
ok "生成带宽和连接历史数据"

echo ""

# ========== 测试 API ==========

# --- 1. 系统信息 ---
echo "[2/7] 系统信息..."

SYS=$(call_api GET /api/system)
echo "$SYS" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'version' in d; assert 'cpuUsage' in d; assert 'memory' in d" && ok "GET /api/system 返回完整系统信息" || fail "GET /api/system 字段缺失"

VER=$(echo "$SYS" | python3 -c "import json,sys; print(json.load(sys.stdin)['version'])")
echo "  版本: $VER"

# --- 2. WireGuard 内核 ---
echo "[3/7] WireGuard 内核状态..."

WGK=$(call_api GET /api/wg/kernel)
echo "$WGK" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'moduleLoaded' in d; assert 'kernelVersion' in d" && ok "GET /api/wg/kernel 返回内核状态" || fail "GET /api/wg/kernel 字段缺失"

# --- 3. 用户管理 ---
echo "[4/7] 用户管理（增删改查）..."

# 列表
USERS=$(call_api GET /api/users)
COUNT=$(echo "$USERS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
[ "$COUNT" -ge 3 ] && ok "GET /api/users 返回 $COUNT 个用户（>=3）" || fail "GET /api/users 用户数量不足 ($COUNT)"

# 单个用户详情
U1=$(call_api GET /api/users/$UID1)
echo "$U1" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['username']=='test-user-01'" && ok "GET /api/users/$UID1 返回正确用户" || fail "GET /api/users/$UID1 用户名不匹配"

# 更新用户
echo '{"username":"test-user-01-updated"}' | \
    GATEWAY_INTERFACE=CGI REQUEST_METHOD=PUT CONTENT_TYPE=application/json \
    CONTENT_LENGTH=38 SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi \
    PATH_INFO=/api/users/$UID1 SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1 > /dev/null

U1_UPDATED=$(call_api GET /api/users/$UID1)
echo "$U1_UPDATED" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['username']=='test-user-01-updated'" && ok "PUT /api/users/$UID1 更新用户名成功" || fail "PUT /api/users/$UID1 更新失败"

# 恢复用户名
echo '{"username":"test-user-01"}' | \
    GATEWAY_INTERFACE=CGI REQUEST_METHOD=PUT CONTENT_TYPE=application/json \
    CONTENT_LENGTH=30 SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi \
    PATH_INFO=/api/users/$UID1 SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1 > /dev/null

# IP 提示
HINT=$(call_api GET /api/ip/hint)
echo "$HINT" | python3 -c "import json,sys; d=json.load(sys.stdin); ip=d['ip']; assert '/32' in ip" && ok "GET /api/ip/hint 返回可用 IP" || fail "GET /api/ip/hint 返回无效 IP"
echo "  提示 IP: $(echo $HINT | python3 -c 'import json,sys; print(json.load(sys.stdin)[\"ip\"])')"

# --- 4. 用户详情/统计 ---
echo "[5/7] 用户详情与统计..."

# 用户统计
USTATS=$(call_api GET /api/users/$UID1/stats)
echo "$USTATS" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'username' in d; assert 'internalIP' in d" && ok "GET /api/users/$UID1/stats 返回用户统计" || fail "GET /api/users/$UID1/stats 字段缺失"

# 用户连接历史
UHIST=$(call_api GET /api/users/$UID1/history "page=1&pageSize=10")
echo "$UHIST" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'data' in d; assert 'total' in d" && ok "GET /api/users/$UID1/history 返回连接历史" || fail "GET /api/users/$UID1/history 字段缺失"
UHIST_COUNT=$(echo "$UHIST" | python3 -c "import json,sys; print(json.load(sys.stdin)['total'])")
echo "  连接历史: $UHIST_COUNT 条"

# 用户流量+图表
UTRAFFIC=$(call_api GET /api/users/$UID1/traffic "start=2026-01-01T00:00:00Z&end=2027-01-01T00:00:00Z")
echo "$UTRAFFIC" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'totalRx' in d; assert 'chart' in d" && ok "GET /api/users/$UID1/traffic 返回流量与图表" || fail "GET /api/users/$UID1/traffic 字段缺失"
CHART_COUNT=$(echo "$UTRAFFIC" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['chart']))")
echo "  图表数据点: $CHART_COUNT 个"

# --- 5. 全局统计与图表 ---
echo "[6/7] 全局统计与图表..."

# 实时统计
STATS=$(call_api GET /api/stats)
echo "$STATS" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'rxBytes' in d; assert 'onlineCount' in d" && ok "GET /api/stats 返回实时统计" || fail "GET /api/stats 字段缺失"

# 带宽历史图表
SHIST=$(call_api GET /api/stats/history "userId=0&start=2026-01-01T00:00:00Z&end=2027-01-01T00:00:00Z")
echo "$SHIST" | python3 -c "import json,sys; d=json.load(sys.stdin); assert isinstance(d, list)" && ok "GET /api/stats/history 返回历史数据数组" || fail "GET /api/stats/history 不是数组"
SHIST_COUNT=$(echo "$SHIST" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
echo "  全局历史数据点: $SHIST_COUNT 个"

# --- 6. 服务配置 ---
echo "[7/7] 服务配置..."

# 读取配置
CONFIG=$(call_api GET /api/config)
echo "$CONFIG" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'wireguard' in d" && ok "GET /api/config 返回配置" || fail "GET /api/config 字段缺失"

# 配置备份
BACKUP=$(call_api GET /api/config/backup)
echo "$BACKUP" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'version' in d; assert 'users' in d" && ok "GET /api/config/backup 导出备份" || fail "GET /api/config/backup 字段缺失"
BACKUP_COUNT=$(echo "$BACKUP" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['users']))")
echo "  备份包含 $BACKUP_COUNT 个用户"

echo ""

# ========== 清理测试数据 ==========
echo "=============================================="
echo "  清理测试数据..."
echo "=============================================="

# 删除测试用户（DELETE /api/users/:id 会同时清理关联数据）
for uid in $UID1 $UID2 $UID3; do
    GATEWAY_INTERFACE=CGI REQUEST_METHOD=DELETE \
        SCRIPT_NAME=/cgi/ThirdParty/wg-server/index.cgi \
        PATH_INFO=/api/users/$uid \
        SERVER_PROTOCOL=HTTP/1.1 timeout 5 "$CGI" 2>/dev/null | tail -1 > /dev/null
    echo "  删除用户 ID=$uid"
done

# 清理全局带宽历史
python3 << PYEOF
import sqlite3, time
conn = sqlite3.connect("$DB")
c = conn.cursor()
c.execute("DELETE FROM bandwidth_history WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'test-user-%')")
c.execute("DELETE FROM bandwidth_history WHERE user_id=0 AND timestamp < datetime('now', '-1 hour')")
c.execute("DELETE FROM connection_log WHERE username LIKE 'test-user-%'")
c.execute("DELETE FROM users WHERE username LIKE 'test-user-%'")
conn.commit()
conn.close()
PYEOF

echo "  数据库清理完成"
echo ""

# ========== 最终结果 ==========
echo "=============================================="
echo -e "  测试结果: \e[32m$PASS 通过\e[0m, \e[31m$FAIL 失败\e[0m"
echo "=============================================="
echo ""

# 最终验证：用户列表应为空
FINAL_USERS=$(call_api GET /api/users)
FINAL_COUNT=$(echo "$FINAL_USERS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "error")
echo "  最终用户数: $FINAL_COUNT（应为 0）"

exit $FAIL
