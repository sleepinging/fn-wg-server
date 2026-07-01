import React, { useState, useEffect, useCallback, useRef } from 'react'
import { getStats, getStatsHistory, getUsers, GlobalStats, BandwidthPoint, User } from '../api'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'

interface Props {
  onViewUser: (userId: number) => void
}

const Dashboard: React.FC<Props> = ({ onViewUser }) => {
  const [stats, setStats] = useState<GlobalStats | null>(null)
  const [history, setHistory] = useState<BandwidthPoint[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [timeRange, setTimeRange] = useState('1h')
  const chartBuf = useRef<any[]>([])
  const lastTs = useRef<string>('')
  const firstLoad = useRef(true)

  const loadData = useCallback(async () => {
    try {
      const [s, u] = await Promise.all([
        getStats(),
        getUsers(),
      ])
      setStats(s)
      setUsers(u)

      if (firstLoad.current) {
        firstLoad.current = false
        const h = await getStatsHistory(0, getStartTime(timeRange), '')
        setHistory(h)
        chartBuf.current = (h || []).map(p => ({
          time: new Date(p.timestamp).toLocaleTimeString(),
          rx: p.rxSpeed || 0,
          tx: p.txSpeed || 0,
        }))
        if (h?.length > 0) {
          lastTs.current = h[h.length - 1].timestamp
        }
      } else {
        if (!lastTs.current) return
        const newPts = await getStatsHistory(0, '', '', lastTs.current)
        if (newPts?.length > 0) {
          const buf = chartBuf.current
          for (const p of newPts) {
            buf.push({
              time: new Date(p.timestamp).toLocaleTimeString(),
              rx: p.rxSpeed || 0,
              tx: p.txSpeed || 0,
            })
            lastTs.current = p.timestamp
          }
          if (buf.length > 200) buf.splice(0, buf.length - 200)
          setHistory([...buf])
        }
      }
    } catch (e) {
      console.error('Failed to load dashboard data', e)
    }
  }, [timeRange])

  useEffect(() => {
    firstLoad.current = true
    chartBuf.current = []
    lastTs.current = ''
    loadData()
    const timer = setInterval(loadData, 1000)
    return () => clearInterval(timer)
  }, [loadData])

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

  const chartData = (history || []).map(p => ({
    time: new Date(p.timestamp).toLocaleTimeString(),
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

      <div className="chart-section">
        <div className="section-header">
          <h3>总带宽使用</h3>
          <select value={timeRange} onChange={e => setTimeRange(e.target.value)}>
            <option value="15m">15分钟</option>
            <option value="1h">1小时</option>
            <option value="6h">6小时</option>
            <option value="24h">24小时</option>
            <option value="7d">7天</option>
          </select>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="time" fontSize={12} />
            <YAxis fontSize={12} tickFormatter={v => formatSpeed(v)} />
            <Tooltip formatter={(value: number) => [formatSpeed(value), '']} />
            <Line type="monotone" dataKey="rx" stroke="#2196F3" strokeWidth={2} name="下载" dot={false} isAnimationActive={false} />
            <Line type="monotone" dataKey="tx" stroke="#FF9800" strokeWidth={2} name="上传" dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>

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
                <td>{formatBytes(user.rxBytes || 0)}</td>
                <td>{formatBytes(user.txBytes || 0)}</td>
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
    </div>
  )
}

function getStartTime(range: string): string {
  const now = new Date()
  switch (range) {
    case '15m': return new Date(now.getTime() - 15 * 60000).toISOString()
    case '1h': return new Date(now.getTime() - 60 * 60000).toISOString()
    case '6h': return new Date(now.getTime() - 360 * 60000).toISOString()
    case '24h': return new Date(now.getTime() - 1440 * 60000).toISOString()
    case '7d': return new Date(now.getTime() - 7 * 86400000).toISOString()
    default: return new Date(now.getTime() - 60 * 60000).toISOString()
  }
}

export default Dashboard
