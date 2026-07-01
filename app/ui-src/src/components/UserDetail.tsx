import React, { useState, useEffect, useCallback, useRef } from 'react'
import { getUser, getUserStats, getUserTraffic, getUserConfig, User } from '../api'
import HistoryTable from './HistoryTable'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'

interface Props {
  userId: number
  onBack: () => void
}

const UserDetail: React.FC<Props> = ({ userId, onBack }) => {
  const [user, setUser] = useState<User | null>(null)
  const [stats, setStats] = useState<any>(null)
  const [traffic, setTraffic] = useState<any>(null)
  const [timeRange, setTimeRange] = useState('1h')
  const [showExportModal, setShowExportModal] = useState(false)
  const [exportConfig, setExportConfig] = useState<any>(null)
  const chartBuf = useRef<any[]>([])
  const firstLoad = useRef(true)

  const loadData = useCallback(async () => {
    try {
      const [u, s] = await Promise.all([
        getUser(userId),
        getUserStats(userId),
      ])
      setUser(u)
      setStats(s)

      if (firstLoad.current) {
        firstLoad.current = false
        const t = await getUserTraffic(userId, getStartTime(timeRange), 0)
        setTraffic(t)
        if (t?.chart?.length > 0) {
          chartBuf.current = t.chart.map((p: any) => ({ ...p }))
          setRenderKey(v => v + 1)
        }
      } else {
        const latest = chartBuf.current.length > 0
          ? chartBuf.current[chartBuf.current.length - 1].ts
          : 0
        if (latest > 0) {
          const t = await getUserTraffic(userId, latest, 0)
          if (t?.chart?.length > 0) {
            const seen = new Set(chartBuf.current.map((p: any) => p.ts))
            for (const p of t.chart) {
              if (!seen.has(p.ts)) {
                chartBuf.current.push(p)
                seen.add(p.ts)
              }
            }
            if (chartBuf.current.length > 200) {
              chartBuf.current = chartBuf.current.slice(-200)
            }
            setRenderKey(v => v + 1)
          }
        }
      }
    } catch (e) {
      console.error('Failed to load user detail', e)
    }
  }, [userId, timeRange])

  useEffect(() => {
    firstLoad.current = true
    chartBuf.current = []
    loadData()
    const timer = setInterval(loadData, 1000)
    return () => clearInterval(timer)
  }, [loadData])

  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const formatSpeed = (bps: number) => {
    if (!bps) return '0 B/s'
    const k = 1024
    const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s']
    const i = Math.floor(Math.log(bps) / Math.log(k))
    return parseFloat((bps / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const [renderKey, setRenderKey] = useState(0)

  const chartData = chartBuf.current.map((p: any) => ({
    time: new Date(p.ts).toLocaleTimeString(),
    rxSpeed: p.rxSpeed || 0,
    txSpeed: p.txSpeed || 0,
    rxBytes: p.rxBytes || 0,
    txBytes: p.txBytes || 0,
  }))

  const handleExportConfig = async () => {
    try {
      const data = await getUserConfig(userId)
      setExportConfig(data)
      setShowExportModal(true)
    } catch (e) {
      alert('获取配置失败')
    }
  }

  const downloadConfig = () => {
    if (!exportConfig) return
    const blob = new Blob([exportConfig.config], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = exportConfig.filename || 'wg-client.conf'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  if (!user) {
    return <div className="loading">加载中...</div>
  }

  return (
    <div className="user-detail">
      <div className="page-header">
        <button className="btn" onClick={onBack}>← 返回</button>
        <h3>用户详情: {user.username}</h3>
        <span className={`status-dot ${stats?.online ? 'online' : 'offline'}`} />
        <span>{stats?.online ? '在线' : '离线'}</span>
        <button className="btn btn-primary btn-sm" onClick={handleExportConfig}>导出配置</button>
      </div>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-label">内部IP</div>
          <div className="stat-value ip">{user.internalIP}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">外部IP</div>
          <div className="stat-value ip">{stats?.endpoint || '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">下载速度</div>
          <div className="stat-value">{stats ? formatSpeed(stats.rxSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">上传速度</div>
          <div className="stat-value">{stats ? formatSpeed(stats.txSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">本次下载</div>
          <div className="stat-value">{stats ? formatBytes(stats.sessionRxBytes || 0) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">本次上传</div>
          <div className="stat-value">{stats ? formatBytes(stats.sessionTxBytes || 0) : '-'}</div>
        </div>
      </div>

      <div className="info-grid">
        <div className="info-item">
          <label>公钥</label>
          <code>{user.publicKey}</code>
        </div>
        <div className="info-item">
          <label>私钥</label>
          <code className="blur" onClick={e => (e.target as HTMLElement).classList.toggle('blur')}>
            {user.privateKey || '未显示'}
          </code>
        </div>
        <div className="info-item">
          <label>预共享密钥</label>
          <code>{user.presharedKey || '无'}</code>
        </div>
        <div className="info-item">
          <label>允许的IP</label>
          <code>{user.allowedIPs}</code>
        </div>
        <div className="info-item">
          <label>DNS</label>
          <code>{user.dns || '默认'}</code>
        </div>
        <div className="info-item">
          <label>MTU</label>
          <code>{user.mtu}</code>
        </div>
        <div className="info-item">
          <label>Keepalive</label>
          <code>{user.persistentKeepalive}s</code>
        </div>
        <div className="info-item">
          <label>上线时间</label>
          <code>{stats?.onlineSince || '-'}</code>
        </div>
      </div>

      <div className="chart-section">
        <div className="section-header">
          <h3>带宽使用</h3>
          <div className="total-traffic">
            总下载: {formatBytes(traffic?.totalRx || 0)} | 总上传: {formatBytes(traffic?.totalTx || 0)}
          </div>
          <select value={timeRange} onChange={e => setTimeRange(e.target.value)}>
            <option value="15m">15分钟</option>
            <option value="1h">1小时</option>
            <option value="6h">6小时</option>
            <option value="24h">24小时</option>
            <option value="7d">7天</option>
          </select>
        </div>
        <ResponsiveContainer width="100%" height={250} key={renderKey}>
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="time" fontSize={12} />
            <YAxis fontSize={12} tickFormatter={v => formatSpeed(v)} />
            <Tooltip formatter={(value: number) => [formatSpeed(value), '']} />
            <Line type="monotone" dataKey="rxSpeed" stroke="#2196F3" strokeWidth={2} name="下载" dot={false} isAnimationActive={false} />
            <Line type="monotone" dataKey="txSpeed" stroke="#FF9800" strokeWidth={2} name="上传" dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="history-section">
        <h3>历史记录</h3>
        <HistoryTable uid={userId} />
      </div>
      {showExportModal && exportConfig && (
        <div className="modal-overlay" onClick={() => setShowExportModal(false)}>
          <div className="modal config-modal" onClick={e => e.stopPropagation()}>
            <h4>客户端配置 - {user?.username}</h4>
            <p className="config-hint">将此配置导入 WireGuard 客户端即可连接</p>
            <div className="config-details">
              <div className="config-row">
                <label>服务端公钥</label>
                <code>{exportConfig.serverPublicKey}</code>
              </div>
              <div className="config-row">
                <label>服务端地址</label>
                <code>{exportConfig.serverEndpoint}</code>
              </div>
              <div className="config-row">
                <label>客户端IP</label>
                <code>{exportConfig.clientAddress}</code>
              </div>
              <div className="config-row">
                <label>预共享密钥</label>
                <code>{exportConfig.presharedKey ? exportConfig.presharedKey.substring(0, 20) + '...' : '无'}</code>
              </div>
              <div className="config-row">
                <label>DNS</label>
                <code>{exportConfig.clientDNS}</code>
              </div>
            </div>
            <textarea
              className="config-textarea"
              readOnly
              value={exportConfig.config}
              rows={12}
              onClick={e => (e.target as HTMLTextAreaElement).select()}
            />
            <div className="modal-actions">
              <button className="btn" onClick={() => setShowExportModal(false)}>关闭</button>
              <button className="btn btn-primary" onClick={downloadConfig}>
                下载 .conf 文件
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function getStartTime(range: string): number {
  const now = Date.now()
  switch (range) {
    case '15m': return now - 15 * 60000
    case '1h': return now - 60 * 60000
    case '6h': return now - 360 * 60000
    case '24h': return now - 1440 * 60000
    case '7d': return now - 7 * 86400000
    default: return now - 60 * 60000
  }
}

export default UserDetail
