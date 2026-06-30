import React, { useState, useEffect } from 'react'
import { getLogs, cleanLogs, LogEntry } from '../api'

const LOG_LEVELS = ['', 'INFO', 'WARN', 'ERROR']

const Logs: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [size, setSize] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [cleanDays, setCleanDays] = useState(7)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    loadLogs()
  }, [page, level])

  const loadLogs = async () => {
    setLoading(true)
    try {
      const data = await getLogs(page, 50, level)
      setLogs(data.data || [])
      setTotal(data.total || 0)
      setSize(data.size || 0)
    } catch (e) {
      console.error('Failed to load logs', e)
    } finally {
      setLoading(false)
    }
  }

  const handleClean = async () => {
    if (!confirm(`确定清理 ${cleanDays} 天前的日志？`)) return
    try {
      await cleanLogs(cleanDays)
      alert('日志已清理')
      loadLogs()
    } catch (e: any) {
      alert('清理失败: ' + (e.response?.data?.error || e.message))
    }
  }

  const formatSize = (bytes: number) => {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const getLevelClass = (lvl: string) => {
    switch (lvl) {
      case 'ERROR': return 'log-error'
      case 'WARN': return 'log-warn'
      default: return 'log-info'
    }
  }

  return (
    <div className="logs-page">
      <div className="page-header">
        <h3>运行日志</h3>
        <div className="log-tools">
          <span className="log-size">日志占用: {formatSize(size)}</span>
          <select value={level} onChange={e => { setLevel(e.target.value); setPage(1) }}>
            {LOG_LEVELS.map(l => (
              <option key={l} value={l}>{l || '全部'}</option>
            ))}
          </select>
          <button className="btn btn-sm" onClick={loadLogs}>刷新</button>
        </div>
      </div>

      <div className="log-clean">
        <label>清理</label>
        <input
          type="number"
          value={cleanDays}
          onChange={e => setCleanDays(parseInt(e.target.value) || 7)}
          style={{ width: 60 }}
        />
        <span>天前的日志</span>
        <button className="btn btn-sm btn-danger" onClick={handleClean}>清理</button>
      </div>

      <div className="log-list" style={{ maxHeight: '500px', overflowY: 'auto' }}>
        {loading && <div className="loading">加载中...</div>}
        {!loading && logs.length === 0 && <div className="empty">暂无日志</div>}
        {(logs || []).map(log => (
          <div key={log.id} className={`log-entry ${getLevelClass(log.level)}`}>
            <span className="log-time">[{log.createdAt}]</span>
            <span className={`log-level log-level-${log.level.toLowerCase()}`}>[{log.level}]</span>
            <span className="log-msg">{log.message}</span>
          </div>
        ))}
      </div>

      <div className="pagination">
        <button className="btn btn-sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>上一页</button>
        <span>第 {page} 页 / 共 {Math.ceil(total / 50)} 页 (共{total}条)</span>
        <button className="btn btn-sm" disabled={page >= Math.ceil(total / 50)} onClick={() => setPage(p => p + 1)}>下一页</button>
      </div>
    </div>
  )
}

export default Logs
