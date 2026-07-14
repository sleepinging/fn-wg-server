import React, { useState, useEffect, useCallback, useRef } from 'react'
import { getStats, getStatsHistory, getUsers, getDBStats, GlobalStats, BandwidthPoint, User } from '../api'
import DebugBar from './DebugBar'
import BandwidthChart from './BandwidthChart'
import { downsampleMax } from './downsample'

interface Props {
  onViewUser: (userId: number) => void
}

const Dashboard: React.FC<Props> = ({ onViewUser }) => {
  const [stats, setStats] = useState<GlobalStats | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [timeRange, setTimeRange] = useState('1h')
  const [intervalSec, setIntervalSec] = useState(3)
  const [dbStats, setDbStats] = useState<any>(null)
  const [chartLoading, setChartLoading] = useState(false)
  const chartBuf = useRef<any[]>([])
  const [domain, setDomain] = useState<[number,number]>([Date.now() - 3600000, Date.now()])
  const firstLoad = useRef(true)
  const dbLastFetch = useRef(0)

  const loadData = useCallback(async () => {
    try {
      const [s, u] = await Promise.all([
        getStats(),
        getUsers(),
      ])
      // DB stats every 30s
      if (Date.now() - dbLastFetch.current > 30000) {
        dbLastFetch.current = Date.now()
        getDBStats().then(setDbStats).catch(() => {})
      }

      const rangeMs = getRangeMs(timeRange)
      const now = Date.now()

      if (firstLoad.current) {
        firstLoad.current = false
        const startMs = now - rangeMs
        setChartLoading(true)
        try {
          // 首次全量：后端聚合到 100 点
          const pts = await getStatsHistory(0, startMs, 0)
          if (pts?.length > 0) {
            chartBuf.current = padTimeRange(pts, startMs)
          }
          // 无数据时至少放一个起始锚点
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
          const pts = await getStatsHistory(0, latest, 0)
          if (pts?.length > 0) {
            for (const p of pts) {
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
      // state 更新放在 chartBuf 之后
      setStats(s)
      setUsers(u)
    } catch (e) {
      console.error('Failed to load dashboard data', e)
    }
  }, [timeRange])

  useEffect(() => {
    firstLoad.current = true
    chartBuf.current = []
    setChartLoading(true)
    loadData().finally(() => setChartLoading(false))
    const timer = setInterval(loadData, intervalSec * 1000)
    return () => clearInterval(timer)
  }, [loadData, intervalSec])

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const formatSpeed = (bps: number) => {
    if (bps === 0) return '0 B/s'
    const k = 1024
    const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s']
    const i = Math.floor(Math.log(bps) / Math.log(k))
    return parseFloat((bps / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const chartData = chartBuf.current.map(p => ({
    ts: p.ts,
    time: new Date(p.ts).toLocaleTimeString(),
    rx: p.rxSpeed || 0,
    tx: p.txSpeed || 0,
  }))

  return (
    <div className="dashboard">
      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-label">总下载</div>
          <div className="stat-value">{stats ? formatBytes(stats.rxBytes) : '-'}</div>
          <div className="stat-speed">{stats ? formatSpeed(stats.rxSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">总上传</div>
          <div className="stat-value">{stats ? formatBytes(stats.txBytes) : '-'}</div>
          <div className="stat-speed">{stats ? formatSpeed(stats.txSpeed) : '-'}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">在线用户</div>
          <div className="stat-value">{stats?.onlineCount || 0}</div>
          <div className="stat-speed">总用户: {stats?.totalPeers || 0}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">外部IP</div>
          <div className="stat-value ip">{stats?.externalIP || '-'}</div>
          <div className="stat-speed">内部: {stats?.internalIP || '-'}</div>
        </div>
      </div>

      <BandwidthChart
        title="总带宽使用"
        chartData={chartData}
        domain={domain}
        loading={chartLoading}
        height={300}
        intervalSec={intervalSec}
        timeRange={timeRange}
        onIntervalChange={setIntervalSec}
        onTimeRangeChange={setTimeRange}
        line1Key="rx"
        line2Key="tx"
        formatSpeed={formatSpeed}
      />

      <div className="online-users-section">
        <h3>当前在线用户</h3>
        <table className="data-table">
          <thead>
            <tr>
              <th>用户名</th>
              <th>内部IP</th>
              <th>外部IP</th>
              <th>下载</th>
              <th>上传</th>
              <th>在线时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {(users || []).filter(u => u.online).map(user => (
              <tr key={user.id}>
                <td>{user.username}</td>
                <td>{user.internalIP}</td>
                <td className="ip">{user.endpoint || '-'}</td>
                <td>{formatBytes(user.txBytes || 0)}</td>
                <td>{formatBytes(user.rxBytes || 0)}</td>
                <td>{user.onlineSince || '-'}</td>
                <td>
                  <button className="btn btn-sm" onClick={() => onViewUser(user.id)}>
                    详情
                  </button>
                </td>
              </tr>
            ))}
            {users.filter(u => u.online).length === 0 && (
              <tr><td colSpan={7} className="empty">暂无在线用户</td></tr>
            )}
          </tbody>
        </table>
      </div>

      {dbStats && (
        <div className="db-stats" style={{ marginTop: 16, padding: 12, background: '#f5f5f5', borderRadius: 8, fontSize: 13, color: '#666' }}>
          <strong>数据缓存:</strong> 带宽 {dbStats.bandwidthRows?.toLocaleString()} 条 | 日志 {dbStats.logRows?.toLocaleString()} 条 | 数据库 {formatBytes(dbStats.dbSize || 0)}
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
  // 若第一个点离起始时间超过 30 秒，补一个锚点让 X 轴从正确位置开始
  const gap = pts[0].ts - startMs
  if (gap > 30000) {
    return [{ ts: startMs, rxSpeed: 0, txSpeed: 0, rxBytes: 0, txBytes: 0 }, ...pts].slice(-100)
  }
  return pts.slice(-100)
}

function getStartTime(range: string): number {
  const now = Date.now()
  return now - getRangeMs(range)
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

export default Dashboard
