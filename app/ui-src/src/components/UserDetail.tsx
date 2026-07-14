import React, { useState, useEffect, useCallback, useRef } from 'react'
import { getUser, getUserStats, getUserTraffic, getUserConfig, User } from '../api'
import HistoryTable from './HistoryTable'
import BandwidthChart from './BandwidthChart'
import DebugBar from './DebugBar'
import { downsampleMax } from './downsample'

interface Props {
  userId: number
  onBack: () => void
}

const UserDetail: React.FC<Props> = ({ userId, onBack }) => {
  const [user, setUser] = useState<User | null>(null)
  const [stats, setStats] = useState<any>(null)
  const [traffic, setTraffic] = useState<any>(null)
  const [timeRange, setTimeRange] = useState('1h')
  const [intervalSec, setIntervalSec] = useState(3)
  const [showExportModal, setShowExportModal] = useState(false)
  const [exportConfig, setExportConfig] = useState<any>(null)
  const [chartLoading, setChartLoading] = useState(false)
  const chartBuf = useRef<any[]>([])
  const [domain, setDomain] = useState<[number,number]>([Date.now() - 3600000, Date.now()])
  const firstLoad = useRef(true)

  const loadData = useCallback(async () => {
    try {
      const [u, s] = await Promise.all([
        getUser(userId),
        getUserStats(userId),
      ])

      const rangeMs = getRangeMs(timeRange)
      const now = Date.now()

      if (firstLoad.current) {
        firstLoad.current = false
        const startMs = now - rangeMs
        setChartLoading(true)
        try {
          // 首次全量：后端聚合到 100 点
          const t = await getUserTraffic(userId, startMs, 0)
          setTraffic(t)
          if (t?.chart?.length > 0) {
            chartBuf.current = padTimeRange(t.chart, startMs)
          }
          if (chartBuf.current.length === 0) {
            chartBuf.current = [{ ts: startMs, rxSpeed: 0, txSpeed: 0, rxBytes: 0, txBytes: 0 }]
          }
          setDomain([startMs, now])
        } finally {
          setChartLoading(false)
        }
      } else {
        // 增量拉取：只取最新点之后的少量原始点
        const latest = chartBuf.current.length > 0
          ? chartBuf.current[chartBuf.current.length - 1].ts
          : 0
        if (latest > 0) {
          const t = await getUserTraffic(userId, latest, 0)
          setTraffic(t)
          if (t?.chart?.length > 0) {
            for (const p of t.chart) {
              if (p.ts > latest) {
                chartBuf.current.push(p)
              }
            }
          }
        }
        // 按时间范围裁剪：丢弃超出范围的旧点，保持跨度稳定
        const cutoff = now - rangeMs
        chartBuf.current = chartBuf.current.filter(p => p.ts >= cutoff)
        // 点数累积够后本地合并到 100 点（max 算法，与后端一致）
        if (chartBuf.current.length > 120) {
          chartBuf.current = downsampleMax(chartBuf.current, 100)
        }
        setDomain([now - rangeMs, now])
      }
      // state 更新放在 chartBuf 之后，确保渲染时数据已最新
      setUser(u)
      setStats(s)
    } catch (e) {
      console.error('Failed to load user detail', e)
    }
  }, [userId, timeRange])

  useEffect(() => {
    firstLoad.current = true
    chartBuf.current = []
    setChartLoading(true)
    loadData().finally(() => setChartLoading(false))
    const timer = setInterval(loadData, intervalSec * 1000)
    return () => clearInterval(timer)
  }, [loadData, intervalSec])

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

  const chartData = chartBuf.current.map((p: any) => ({
    ts: p.ts,
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
          <div className="stat-label">下载速度</div>
          <div className="stat-value">{stats ? formatSpeed(stats.txSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">上传速度</div>
          <div className="stat-value">{stats ? formatSpeed(stats.rxSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">本次下载</div>
          <div className="stat-value">{stats ? formatBytes(stats.sessionTxBytes || 0) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">本次上传</div>
          <div className="stat-value">{stats ? formatBytes(stats.sessionRxBytes || 0) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">内部IP</div>
          <div className="stat-value ip">{user.internalIP}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">外部IP</div>
          <div className="stat-value ip">{stats?.endpoint || '-'}</div>
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

      <BandwidthChart
        title="带宽使用"
        chartData={chartData}
        domain={domain}
        loading={chartLoading}
        height={250}
        intervalSec={intervalSec}
        timeRange={timeRange}
        onIntervalChange={setIntervalSec}
        onTimeRangeChange={setTimeRange}
        line1Key="txSpeed"
        line2Key="rxSpeed"
        formatSpeed={formatSpeed}
        extraContent={
          <div className="total-traffic">
            总下载: {formatBytes(traffic?.totalTx || 0)} | 总上传: {formatBytes(traffic?.totalRx || 0)}
          </div>
        }
      />

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
      <DebugBar
        dataPoints={chartBuf.current.length}
        firstTs={chartBuf.current[0]?.ts || 0}
        lastTs={chartBuf.current[chartBuf.current.length - 1]?.ts || 0}
        domainStart={domain[0]}
        domainEnd={domain[1]}
      />
    </div>
  )
}

function padTimeRange(pts: any[], startMs: number): any[] {
  if (pts.length === 0) return pts
  const gap = pts[0].ts - startMs
  if (gap > 30000) {
    return [{ ts: startMs, rxSpeed: 0, txSpeed: 0, rxBytes: 0, txBytes: 0 }, ...pts].slice(-100)
  }
  return pts.slice(-100)
}

function getRangeMs(range: string): number {
  switch (range) {
    case '15m': return 15 * 60000
    case '1h': return 60 * 60000
    case '6h': return 360 * 60000
    case '24h': return 1440 * 60000
    case '7d': return 7 * 86400000
    default: return 60 * 60000
  }
}

function getStartTime(range: string): number {
  const now = Date.now()
  return now - getRangeMs(range)
}

export default UserDetail
