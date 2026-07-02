# UI 测试计划

## 目标

在我（开发者）本地电脑上，用无头浏览器 + 真实数据桩，跑通 UI 全部场景，确认无误后再部署到飞牛。
避免反复打包 → 上传 → 安装 → 发现问题 → 再打包 的循环。

## 架构

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│  React UI    │────▶│ Mock Server  │◀────│   Fixtures   │
│ vite dev     │ API │ (Express)    │     │ users.json   │
│ localhost    │     │ localhost:3001│    │ stats.json   │
└─────────────┘     └──────────────┘     │ history.json │
                                          └──────────────┘
       │
       ▼
┌─────────────┐
│  Playwright │  无头 Chromium
│  tests/     │  截图 + DOM 断言
└─────────────┘
```

## 实施步骤

### Step 1: 捕获真实数据 → fixtures

**脚本**: `ui-tests/scripts/capture-data.py`

通过 SSH 调用飞牛上的 CGI API，抓取一份真实数据快照：
- `GET /api/users` → `users.json`
- `GET /api/stats` → `stats.json`
- `GET /api/stats/history?userId=0&since=...` → `history_global.json`
- 每个在线用户 `GET /api/users/{id}/traffic?since=...` → `history_u{id}.json`

数据全部是 Unix 毫秒时间戳的 JSON，可直接回放。

### Step 2: Mock API Server

**路径**: `ui-tests/server/`

Express 服务器，端口 3001，提供与原版 CGI 完全相同的 API：

| 端点 | 返回 |
|------|------|
| `GET /api/users` | `users.json` |
| `GET /api/stats` | `stats.json` |
| `GET /api/stats/history?userId=0&since=X&end=0` | `history_global.json` 中过滤 `ts > since`，上限 100 点 |
| `GET /api/users/:id` | 从 `users.json` 匹配 |
| `GET /api/users/:id/config` | 返回固定 `.conf` 文本 |
| `GET /api/users/:id/traffic?since=X&end=0` | 同上过滤逻辑 |
| `GET /api/db-stats` | 固定 JSON |
| 其他 | 404 |

**特性：**
- 启动时把 fixtures 中所有时间戳偏移到"当前时间附近"，确保 `since` 过滤正确
- 可切换模式：`REPLAY`（静态回放）/ `LIVE`（时间戳持续前移模拟实时数据）
- 日志输出每次请求

### Step 3: Playwright 测试

**路径**: `ui-tests/tests/`

用 Playwright + Chromium headless：

#### 3a. 基础渲染测试
- 页面加载完成，DebugBar 存在
- 图表 `.recharts-responsive-container` 渲染
- 用户列表表格渲染
- 无 console 错误

#### 3b. 图表域测试（核心）
- 默认 1h：通过 DebugBar 文本读取域跨度，应 ≈ 3600s
- 切换到 15m：点击 select option，域跨度应变为 ≈ 900s
- 切换到 6h：域跨度应变为 ≈ 21600s
- 首个数据点 ts 应接近域起始（锚点存在）

#### 3c. 速度与数据
- 在线用户速度显示非空、非负
- 图表数据点的 `ts` 严格单调递增

#### 3d. 截图回归
- 每个测试步骤自动截图保存到 `ui-tests/screenshots/`
- 历史截图可做视觉 diff（Playwright `expect(page).toHaveScreenshot()`）

#### 3e. 用户详情
- 点击用户进入详情
- 图表域检查同上
- 返回按钮可用

### Step 4: 集成到开发流程

```bash
# 1. 抓取最新真实数据
python ui-tests/scripts/capture-data.py

# 2. 启动 mock 服务器
cd ui-tests/server && npm start &

# 3. 启动前端 dev server（指向 mock）
cd app/ui-src && VITE_API_BASE=http://localhost:3001 npm run dev &

# 4. 运行测试
cd ui-tests/tests && npx playwright test

# 5. 查看截图
ls ui-tests/screenshots/
```

## 目录结构

```
ui-tests/
├── plan.md              ← 本文件
├── README.md            ← 使用说明
├── server/
│   ├── package.json
│   ├── server.js        ← Express mock
│   └── fixtures/        ← 从飞牛抓取的数据
│       ├── users.json
│       ├── stats.json
│       ├── history_global.json
│       ├── history_u19.json
│       └── ...
├── tests/
│   ├── package.json
│   ├── playwright.config.ts
│   ├── fixtures.ts      ← 测试辅助函数
│   ├── dashboard.spec.ts
│   ├── user-detail.spec.ts
│   └── chart-domain.spec.ts
├── scripts/
│   └── capture-data.py  ← SSH 抓取真实数据
└── screenshots/         ← 自动截图输出
```

## 时间估算

| 步骤 | 内容 | 预估 |
|------|------|------|
| Step 1 | capture-data.py | 20 分钟 |
| Step 2 | mock server | 30 分钟 |
| Step 3 | playwright tests | 40 分钟 |
| Step 4 | 集成脚本 | 10 分钟 |
| **合计** | | **~2 小时** |

## 后续扩展

- 模拟数据抖动（模拟真实的 rxSpeed 波动）
- 模拟用户上下线
- CI 集成（GitHub Actions 跑 playwright）
- 性能测试（大量历史数据下的图表渲染）
