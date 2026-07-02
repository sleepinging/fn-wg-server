/**
 * WireGuard Server Mock API
 * 回放从飞牛抓取的 fixture 数据，时间戳自动偏移到当前时间。
 *
 * 启动: node server.js
 * 端口: 8080 (Vite dev proxy 默认目标)
 * 模式: REPLAY - 静态回放, LIVE - 时间戳持续前移
 */

const express = require('express')
const cors = require('cors')
const fs = require('fs')
const path = require('path')
const app = express()
app.use(cors())
app.use(express.json())

// 请求日志（在所有路由之前）
app.use((req, _res, next) => {
  console.log(`[mock] ${req.method} ${req.url}`)
  next()
})

const FIXTURES = path.join(__dirname, 'fixtures')
const PORT = 8080
const MODE = process.env.MOCK_MODE || 'LIVE' // REPLAY | LIVE

// ========== 加载 fixtures ==========
function load(name) {
  const p = path.join(FIXTURES, name)
  if (!fs.existsSync(p)) return null
  return JSON.parse(fs.readFileSync(p, 'utf-8'))
}

const users = load('users.json') || []
const stats = load('stats.json') || {}

// 预加载全局 history
const historyGlobal = {}
for (const label of ['15m', '1h', '6h']) {
  historyGlobal[label] = load(`history_global_${label}.json`) || []
}

// 用户 history: 如果 fixture 不存在，从 global 生成
function getUserHistory(uid) {
  const out = {}
  for (const label of ['15m', '1h', '6h']) {
    let data = load(`history_u${uid}_${label}.json`)
    if (!data) {
      // 从 global history 克隆，稍作偏移模拟不同用户
      const src = historyGlobal[label] || []
      data = src.map((p, i) => ({
        ts: p.ts + uid * 10,           // 微小偏移区分
        rxSpeed: Math.max(0, (p.rxSpeed || 0) * (0.5 + Math.random())),
        txSpeed: Math.max(0, (p.txSpeed || 0) * (0.5 + Math.random())),
        rxBytes: (p.rxBytes || 0) + uid * 1000,
        txBytes: (p.txBytes || 0) + uid * 500,
      }))
    }
    out[label] = data
  }
  return out
}

// ========== 时间偏移 ==========
// 找出所有 fixture 中最新的时间戳，计算偏移量
function findMaxTs(data) {
  if (!data) return 0
  let max = 0
  for (const arr of Object.values(data)) {
    if (Array.isArray(arr) && arr.length > 0) {
      max = Math.max(max, arr[arr.length - 1].ts || 0)
    }
  }
  return max
}

let tsOffset = 0
let fixtureMaxTs = 0  // fixture 中最新的原始时间戳
let startTime = Date.now()

function computeOffset() {
  fixtureMaxTs = findMaxTs(historyGlobal)
  const userHistories = users.map(u => getUserHistory(u.id))
  for (const h of userHistories) fixtureMaxTs = Math.max(fixtureMaxTs, findMaxTs(h))
  if (fixtureMaxTs > 0) {
    tsOffset = Date.now() - fixtureMaxTs
  }
  startTime = Date.now()
  console.log(`[mock] tsOffset=${tsOffset}ms, fixtureMaxTs=${fixtureMaxTs} (mode=${MODE})`)
}
computeOffset()

function now() {
  return Date.now()
}

// 偏移 fixture 中的 ts
// REPLAY: 固定偏移  |  LIVE: 动态偏移 = 初始偏移 + 已过时间
function shiftTs(arr) {
  if (!arr) return []
  const offset = MODE === 'REPLAY'
    ? tsOffset
    : tsOffset + (Date.now() - startTime)
  return arr.map(p => ({ ...p, ts: (p.ts || 0) + offset }))
}

// ========== 时间范围匹配 ==========
// since=0 时从 fixture 返回采样数据
// since>0 时生成随机新数据（模拟每秒新增）
function filterHistory(fixtures, sinceMs, maxPoints) {
  const nowMs = now()
  let label = '1h'
  if (sinceMs >= nowMs - 900_000) label = '15m'
  else if (sinceMs >= nowMs - 3_600_000) label = '1h'
  else label = '6h'

  if (sinceMs > 0 && sinceMs < nowMs - 1000) {
    // 增量查询：生成 sinceMs+1s ~ nowMs 之间的随机数据点
    const pts = []
    let ts = sinceMs + 1000  // 从 since+1s 开始
    while (ts <= nowMs) {
      pts.push({
        ts: ts,
        rxSpeed: Math.random() < 0.2 ? Math.floor(Math.random() * 500000) : 0,
        txSpeed: Math.random() < 0.15 ? Math.floor(Math.random() * 200000) : 0,
        rxBytes: Math.floor(Math.random() * 10000000),
        txBytes: Math.floor(Math.random() * 5000000),
      })
      ts += 1000  // 1秒间隔
    }
    return pts.slice(-(maxPoints || 100))
  }

  // 初始查询：从 fixture 返回
  let data = shiftTs(fixtures[label] || fixtures['1h'] || [])
  if (sinceMs > 0) {
    data = data.filter(p => p.ts > sinceMs)
  }
  if (maxPoints > 0 && data.length > maxPoints) {
    const step = Math.floor(data.length / maxPoints) || 1
    data = data.filter((_, i) => i % step === 0 || i === data.length - 1).slice(-maxPoints)
  }
  return data
}

// ========== API 路由 ==========
app.get('/api/users', (_req, res) => {
  // 偏移用户数据中的时间相关字段 (如果有的话)
  res.json(users.map(u => ({
    ...u,
    // 保持 online 状态和其他字段不变
  })))
})

app.post('/api/users', (_req, res) => {
  res.json({ id: Math.max(...users.map(u => u.id), 0) + 1, success: true })
})

app.put('/api/users/:id', (req, res) => {
  res.json({ ...req.body, id: parseInt(req.params.id) })
})

app.delete('/api/users/:id', (_req, res) => {
  res.json({ success: true })
})

app.get('/api/stats', (_req, res) => {
  res.json({ ...stats })
})

app.get('/api/stats/history', (req, res) => {
  const userId = parseInt(req.query.userId) || 0
  const since = parseInt(req.query.since) || 0
  const max = parseInt(req.query.end) || 100
  if (userId === 0) {
    res.json(filterHistory(historyGlobal, since, max))
  } else {
    const h = getUserHistory(userId)
    res.json(filterHistory(h, since, max))
  }
})

app.get('/api/users/:id', (req, res) => {
  const uid = parseInt(req.params.id)
  const u = users.find(x => x.id === uid)
  if (u) res.json({ ...u })
  else res.status(404).json({ error: 'not found' })
})

app.get('/api/users/:id/config', (req, res) => {
  const uid = parseInt(req.params.id)
  const u = users.find(x => x.id === uid)
  if (!u) return res.status(404).json({ error: 'not found' })
  res.json({
    config: `[Interface]
PrivateKey = <hidden>
Address = ${u.internalIP || '192.168.5.10/32'}
DNS = 1.1.1.1

[Peer]
PublicKey = <server-pubkey>
PresharedKey = <preshared-key>
Endpoint = example.com:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`,
    serverPublicKey: '<server-public-key>',
    serverEndpoint: 'example.com:51820',
    clientAddress: u.internalIP || '192.168.5.10/32',
    presharedKey: '<preshared-key>',
    clientDNS: '1.1.1.1',
  })
})

app.get('/api/users/:id/traffic', (req, res) => {
  const uid = parseInt(req.params.id)
  const since = parseInt(req.query.since) || 0
  const max = parseInt(req.query.end) || 100
  const h = getUserHistory(uid)
  const chart = filterHistory(h, since, max)
  res.json({
    chart,
    totalRx: chart.reduce((s, p) => s + (p.rxBytes || 0), 0),
    totalTx: chart.reduce((s, p) => s + (p.txBytes || 0), 0),
  })
})

// ========== 系统/配置/服务 端点 ==========
app.get('/api/system', (_req, res) => {
  res.json({
    version: '1.0.87',
    cpuUsage: '2.5%',
    memory: { total: 8589934592, available: 4294967296, usedPercent: '50.0%' },
    uptime: '6d 15h 6m',
    processMemory: '45.2 MB',
  })
})

app.get('/api/wg/kernel', (_req, res) => {
  res.json({
    moduleLoaded: true,
    interfaceUp: true,
    daemonRunning: true,
  })
})

app.get('/api/config', (_req, res) => {
  res.json({
    wireguard: {
      interfaceName: 'wg0',
      privateKey: '<server-private-key>',
      publicKey: '<server-public-key>',
      address: '192.168.5.1/24',
      listenPort: 51820,
      dns: '1.1.1.1',
      mtu: 1420,
      postUp: 'iptables -A FORWARD -i wg0 -j ACCEPT',
      postDown: 'iptables -D FORWARD -i wg0 -j ACCEPT',
      serverDomain: 'example.com',
    },
    historyRetentionDays: '7',
    autoStart: 'false',
    detectedIP: '39.184.130.60',
  })
})

app.get('/api/config/export-all', (_req, res) => {
  res.json({ config: '{}', users: users, version: '1.0.87' })
})

app.get('/api/service/status', (_req, res) => {
  res.json({
    wgRunning: true,
    monitorRunning: true,
  })
})

app.post('/api/service/start', (_req, res) => res.json({ success: true }))
app.post('/api/service/stop', (_req, res) => res.json({ success: true }))
app.post('/api/service/restart', (_req, res) => res.json({ success: true }))
app.put('/api/config', (_req, res) => res.json({ success: true }))
app.get('/api/config/backup', (_req, res) => {
  res.json({ config: '{}', users: users, version: '1.0.87' })
})
app.post('/api/config/restore', (_req, res) => res.json({ success: true }))

app.get('/api/logs', (req, res) => {
  const page = parseInt(req.query.page) || 1
  const pageSize = parseInt(req.query.pageSize) || 50
  const level = req.query.level || ''
  const search = req.query.search || ''
  const logs = []
  const levels = ['INFO', 'WARN', 'ERROR', 'INFO', 'DEBUG']
  for (let i = 0; i < pageSize; i++) {
    const lv = level || levels[i % levels.length]
    let msg = `[${lv}] mock log entry #${(page - 1) * pageSize + i}`
    if (search) msg = msg.replace('mock', `[匹配:${search}]`)
    logs.push({
      id: (page - 1) * pageSize + i,
      level: lv,
      message: msg,
      createdAt: String(Date.now() - i * 60000),
    })
  }
  res.json({ logs, total: 500, page, pageSize })
})

app.post('/api/logs/clean', (_req, res) => {
  res.json({ deleted: 0, remaining: 500 })
})

app.get('/api/ip/hint', (_req, res) => {
  res.json({ nextIP: '192.168.5.13/32' })
})

app.get('/api/users/:id/stats', (req, res) => {
  const uid = parseInt(req.params.id)
  const u = users.find(x => x.id === uid)
  if (!u) return res.status(404).json({ error: 'not found' })
  res.json({
    rxSpeed: u.rxSpeed || 0,
    txSpeed: u.txSpeed || 0,
    rxBytes: u.rxBytes || 0,
    txBytes: u.txBytes || 0,
    endpoint: u.endpoint || '',
    onlineSince: u.onlineSince || '',
    online: u.online || false,
  })
})

app.get('/api/users/:id/history', (req, res) => {
  const uid = parseInt(req.params.id)
  const page = parseInt(req.query.page) || 1
  const pageSize = parseInt(req.query.pageSize) || 20
  const u = users.find(x => x.id === uid) || {}
  const history = []
  const baseTime = Date.now()
  for (let i = 0; i < pageSize; i++) {
    const delta = ((page - 1) * pageSize + i) * 300_000
    history.push({
      id: (page - 1) * pageSize + i,
      userId: uid,
      connectedAt: String(baseTime - delta - 60000),
      disconnectedAt: String(baseTime - delta),
      rxBytes: Math.floor(Math.random() * 10000),
      txBytes: Math.floor(Math.random() * 5000),
      endpoint: u.endpoint || '1.2.3.4:12345',
    })
  }
  res.json({ history, total: 100, page, pageSize })
})

app.get('/api/db/stats', (_req, res) => {
  res.json({
    bandwidthRows: historyGlobal['1h']?.length * 10 || 1000,
    logRows: 150,
    dbSize: 1024 * 1024 * 2.5,
  })
})

// 兼容旧路径
app.get('/api/db-stats', (_req, res) => {
  res.json({
    bandwidthRows: historyGlobal['1h']?.length * 10 || 1000,
    logRows: 150,
    dbSize: 1024 * 1024 * 2.5,
  })
})

// 未匹配路由（在所有路由之后）
app.use((req, res) => {
  console.log(`[mock] 404 ${req.method} ${req.url}`)
  res.status(404).json({ error: 'not found' })
})

app.listen(PORT, () => {
  console.log(`[mock] WG-Server Mock API on http://localhost:${PORT}`)
  console.log(`[mock] mode=${MODE}, users=${users.length}, fixtures ready`)
})
