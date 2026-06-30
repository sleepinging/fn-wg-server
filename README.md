# WireGuard 服务端 (wg-server)

飞牛 fnOS 原生应用，WireGuard 异地组网服务端管理系统。

## 功能特性

- **用户管理**：增删改查，删除用户后强制下线
- **实时监控**：总带宽(上下行)、在线用户数、内外网IP、在线时间、实时速度
- **总带宽图表**：从数据库加载历史数据，可视化展示
- **用户详情**：所有简略信息、上线时间、历史记录、总流量、带宽图表
- **数据库**：SQLite3，带宽历史默认保存7天(可配置)，每秒采样一次
- **历史记录**：用户、内外网IP、上下线时间、流量使用
- **服务控制**：启动/停止/重启 WireGuard 服务
- **IP网段设置**：默认 192.168.5.1/24，添加用户时自动提示最小可用IP
- **配置备份**：导出/恢复所有配置
- **开机自启**：可配置
- **系统信息**：版本号、CPU/内存占用、服务运行时间
- **WireGuard 内核状态**：模块加载情况
- **运行日志**：查看、按时间清理

## 技术架构

- **后端**：Go 语言，CGI 方式运行（无独立端口）
- **前端**：React + TypeScript，Vite 构建
- **数据库**：SQLite3
- **WireGuard 控制**：wg 命令行工具 + wgctrl-go 库
- **打包格式**：fpk（飞牛原生应用包）

## 目录结构

```
wg-server/
├── app/
│   ├── backend/              # Go 后端源码
│   │   ├── main.go           # 入口(CGI/守护进程模式)
│   │   ├── go.mod / go.sum
│   │   ├── api/              # CGI API 处理器
│   │   ├── db/               # SQLite 数据库层
│   │   ├── wg/               # WireGuard 管理
│   │   └── daemon/           # 带宽监控守护进程
│   ├── ui/                   # 构建后的前端
│   │   ├── index.cgi         # CGI 入口
│   │   ├── index.html        # React 应用
│   │   └── assets/           # 编译后的 JS/CSS
│   └── ui-src/               # React 前端源码
│       └── src/
│           ├── api.ts        # API 客户端
│           ├── App.tsx       # 主应用(标签页导航)
│           ├── components/   # 组件
│           └── styles.css    # 样式
├── pkg/
│   ├── manifest              # 应用清单
│   ├── cmd/                  # 生命周期脚本
│   ├── config/               # 权限/资源配置
│   ├── ui/                   # 桌面入口配置
│   ├── ICON.PNG / ICON_256.PNG
│   └── wg-server.service    # systemd 服务文件
├── .skel/                    # fnpack 骨架脚本
├── build.sh                  # 打包脚本
├── make_icons.sh             # 图标生成
└── README.md
```

## 本地开发

### 环境要求
- Go 1.21+
- Node.js 18+ (构建前端)
- Git Bash 或 Linux (运行 build.sh)

### 构建步骤

```bash
# 1. 构建 Go 后端
cd app/backend
export GOOS=linux GOARCH=amd64
go build -ldflags="-s -w" -o wg-server .

# 2. 构建 React 前端
cd ../ui-src
npm install --registry=https://registry.npmmirror.com
npm run build

# 3. 打包 fpk
cd ../..
bash build.sh
```

### 开发模式

后端直接启动 HTTP 服务（方便调试）：
```bash
cd app/backend
go run .   # 默认 :8080 端口
```

## 安装

1. 飞牛 Web 桌面 → **应用中心 → 手动安装**，选择 `wg-server_<version>_x86.fpk`
2. 安装后 `install_callback` 会重启 `trim_http_cgi` 服务使 CGI 路由生效
3. 桌面出现「WireGuard 服务端」图标，点开即为管理界面

## API 接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/users` | GET/POST | 用户列表/创建 |
| `/api/users/:id` | GET/PUT/DELETE | 用户详情/更新/删除 |
| `/api/users/:id/stats` | GET | 用户实时状态 |
| `/api/users/:id/history` | GET | 用户连接历史 |
| `/api/users/:id/traffic` | GET | 用户流量+图表 |
| `/api/stats` | GET | 全局实时统计 |
| `/api/stats/history` | GET | 带宽历史数据 |
| `/api/config` | GET/PUT | 配置读取/更新 |
| `/api/config/backup` | GET | 导出配置备份 |
| `/api/config/restore` | POST | 恢复配置备份 |
| `/api/service/:action` | POST | 启动/停止/重启 |
| `/api/system` | GET | 系统信息 |
| `/api/wg/kernel` | GET | WireGuard 内核状态 |
| `/api/logs` | GET | 运行日志 |
| `/api/logs/clean` | POST | 清理日志 |
| `/api/ip/hint` | GET | 获取最小可用IP |
| `/healthz` | GET | 健康检查 |

## 版本历史

- **1.0.0** - 初始版本，完整功能实现
