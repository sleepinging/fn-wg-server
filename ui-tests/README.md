# UI 自测环境

本地无头浏览器 + Mock API Server 的自测方案，避免反复 打包→上传→安装→验证 的循环。

## 架构

```
React UI (Vite :5173)  ──proxy──▶  Mock Server (Express :8080)  ──load──▶  fixtures/
        │                                      │
        ▼                                      ▼
  Playwright 测试                    从飞牛抓取的真实数据
  (无头 Chromium)                    (时间戳自动偏移到当前)
```

## 快速开始

### 一键启动（Windows）

双击 `run.bat`，会自动：
1. 启动 Mock Server（端口 8080）
2. 启动 Vite Dev Server（端口 5173）
3. 等待 5 秒后运行 Playwright 测试

### 手动启动

```bash
# 1. 启动 Mock Server
cd ui-tests/server && npm start

# 2. 启动前端（新终端）
cd app/ui-src && npx vite --port 5173

# 3. 运行测试
cd ui-tests/tests && npx playwright test

# 4. 运行指定测试文件
cd ui-tests/tests && npx playwright test dashboard.spec.ts

# 5. 显示浏览器运行（调试用）
cd ui-tests/tests && npx playwright test --headed
```

## 目录结构

```
ui-tests/
├── README.md           ← 本文件
├── plan.md             ← 详细测试计划
├── run.bat             ← Windows 一键启动
├── server/             ← Mock API Server
│   ├── package.json
│   ├── server.js       ← Express 服务器（端口 8080）
│   └── fixtures/       ← 从飞牛抓取的 JSON 数据
│       ├── users.json
│       ├── stats.json
│       ├── history_global_15m.json
│       ├── history_global_1h.json
│       └── history_global_6h.json
├── tests/              ← Playwright 测试
│   ├── package.json
│   ├── playwright.config.ts
│   ├── fixtures.ts     ← 测试辅助（DebugBar 读取等）
│   ├── dashboard.spec.ts
│   ├── chart-domain.spec.ts
│   └── user-detail.spec.ts
├── scripts/
│   └── capture-data.py ← SSH 抓取真实 fixture 数据
└── screenshots/        ← 自动截图输出
```

## 更新 Fixture 数据

当飞牛上的数据变化后，重新抓取：

```bash
# 需要 paramiko
pip install paramiko

# 抓取（通过 SSH 调用 CGI API）
python ui-tests/scripts/capture-data.py
```

抓取内容：
- `GET /api/users` → `users.json`
- `GET /api/stats` → `stats.json`
- `GET /api/stats/history?userId=0&since=...` → `history_global_{15m,1h,6h}.json`
- 每个用户 `GET /api/users/{id}/traffic?since=...` → `history_u{id}_{15m,1h,6h}.json`

## Mock Server API 覆盖

| 端点 | 方法 | 返回 |
|------|------|------|
| `/api/users` | GET/POST | 用户列表 / 创建 |
| `/api/users/:id` | GET/PUT/DELETE | CRUD |
| `/api/users/:id/config` | GET | 客户端配置导出 |
| `/api/users/:id/stats` | GET | 用户实时状态 |
| `/api/users/:id/history` | GET | 连接历史 |
| `/api/users/:id/traffic` | GET | 用户流量+图表 |
| `/api/stats` | GET | 全局统计 |
| `/api/stats/history` | GET | 带宽历史（含 userId 筛选） |
| `/api/db/stats` | GET | 数据库缓存统计 |
| `/api/config` | GET/PUT | 配置读/写 |
| `/api/config/backup` | GET | 备份导出 |
| `/api/config/restore` | POST | 备份恢复 |
| `/api/config/export-all` | GET | 一键导出全部 |
| `/api/service/status` | GET | 服务状态 |
| `/api/service/:action` | POST | 启动/停止/重启 |
| `/api/system` | GET | 系统信息 |
| `/api/wg/kernel` | GET | WireGuard 内核状态 |
| `/api/logs` | GET | 日志（分页/过滤/搜索） |
| `/api/logs/clean` | POST | 清理日志 |
| `/api/ip/hint` | GET | 最小可用 IP |

## 模式

通过 `MOCK_MODE` 环境变量切换：

- **`LIVE`**（默认）：时间戳持续前移，模拟实时数据
- **`REPLAY`**：时间戳固定偏移到当前，静态回放

```bash
# 使用 REPLAY 模式
MOCK_MODE=REPLAY node server.js
```

## Playwright 测试说明

### 测试覆盖

| 文件 | 覆盖 |
|------|------|
| `dashboard.spec.ts` | 页面加载、DebugBar 可见性、图表渲染、域跨度、刷新切换 |
| `chart-domain.spec.ts` | 15m/1h/6h 域精确验证、锚点、时间戳递增、截图回归 |
| `user-detail.spec.ts` | 进入详情、详情图表、返回按钮 |

### 截图

截图自动保存到 `ui-tests/screenshots/`。首次运行生成基线截图，后续运行会进行视觉 diff。

```bash
# 更新截图基线
npx playwright test --update-snapshots
```

### 调试

```bash
# 带浏览器窗口运行
npx playwright test --headed

# 调试模式（逐步执行）
npx playwright test --debug

# 只运行失败的测试
npx playwright test --last-failed
```
