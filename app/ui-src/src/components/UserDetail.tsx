import React, { useState, useEffect } from 'react'
import { getUser, getUserStats, getUserTraffic, User } from '../api'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Area, AreaChart } from 'recharts'

interface Props {
  userId: number
  onBack: () => void
}

const UserDetail: React.FC<Props> = ({ userId, onBack }) => {
  const [user, setUser] = useState<User | null>(null)
  const [stats, setStats] = useState<any>(null)
  const [traffic, setTraffic] = useState<any>(null)
  const [timeRange, setTimeRange] = useState('1h')

  useEffect(() => {
    loadData()
    const timer = setInterval(loadData, 10000)
    return () => clearInterval(timer)
  }, [userId, timeRange])

  const loadData = async () => {
    try {
      const [u, s, t] = await Promise.all([
        getUser(userId),
        getUserStats(userId),
        getUserTraffic(userId, getStartTime(timeRange), ''),
      ])
      setUser(u)
      setStats(s)
      setTraffic(t)
    } catch (e) {
      console.error('Failed to load user detail', e)
    }
  }

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

  const chartData = (traffic?.chart || []).map((p: any) => ({
    time: new Date(p.timestamp).toLocaleTimeString(),
    rxSpeed: p.rxSpeed || 0,
    txSpeed: p.txSpeed || 0,
    rxBytes: p.rxBytes || 0,
    txBytes: p.txBytes || 0,
  }))

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
        <ResponsiveContainer width="100%" height={250}>
          <AreaChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e0e0e0" />
            <XAxis dataKey="time" fontSize={12} />
            <YAxis fontSize={12} tickFormatter={v => formatSpeed(v)} />
            <Tooltip formatter={(value: number) => [formatSpeed(value), '']} />
            <Area type="monotone" dataKey="rxSpeed" stroke="#2196F3" fill="#2196F3" fillOpacity={0.1} name="下载" />
            <Area type="monotone" dataKey="txSpeed" stroke="#FF9800" fill="#FF9800" fillOpacity={0.1} name="上传" />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      <div className="history-section">
        <h3>历史记录</h3>
        <HistoryTable userId={userId} />
      </div>
    </div>
  )
}

const HistoryTable: React.FC<{ userId: number }> = ({ userId }) => {
  const [history, setHistory] = useState<any[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)

  useEffect(() => {
    loadHistory()
  }, [userId, page])

  const loadHistory = async () => {
    try {
      const { default: api } = await import('../api')
      const data = await api.getUserHistory(userId, page, 20)
      setHistory(data.data)
      setTotal(data.total)
    } catch (e) {
      console.error('Failed to load history', e)
    }
  }

  return (
    <div>
      <table className="data-table">
        <thead>
          <tr>
            <th>内部IP</th>
            <th>外部IP</th>
            <th>上线时间</th>
            <th>下线时间</th>
            <th>下载流量</th>
            <th>上传流量</th>
          </tr>
        </thead>
        <tbody>
          {(history || []).map((h: any) => (
            <tr key={h.id}>
              <td>{h.internalIP}</td>
              <td className="ip">{h.externalIP}</td>
              <td>{h.connectedAt}</td>
              <td>{h.disconnectedAt || '在线中'}</td>
              <td>{formatBytesSimple(h.rxBytes)}</td>
              <td>{formatBytesSimple(h.txBytes)}</td>
            </tr>
          ))}
          {history.length === 0 && (
            <tr><td colSpan={6} className="empty">暂无历史记录</td></tr>
          )}
        </tbody>
      </table>
      {total > 20 && (
        <div className="pagination">
          <button className="btn btn-sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>上一页</button>
          <span>第 {page} 页 / 共 {Math.ceil(total / 20)} 页</span>
          <button className="btn btn-sm" disabled={page >= Math.ceil(total / 20)} onClick={() => setPage(p => p + 1)}>下一页</button>
        </div>
      )}
    </div>
  )
}

function formatBytesSimple(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
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

export default UserDetail
