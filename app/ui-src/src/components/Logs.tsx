import React, { useState, useEffect, useRef } from 'react'
import { getLogs, cleanLogs, LogEntry } from '../api'

const LOG_LEVELS = ['', 'INFO', 'WARN', 'ERROR']

const Logs: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [size, setSize] = useState(0)
  const [page, setPage] = useState(1)
  const [level, setLevel] = useState('')
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [cleanDays, setCleanDays] = useState(7)
  const [loading, setLoading] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  const searchTimer = useRef<number | null>(null)
  const logListRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    loadLogs()
  }, [page, level, search])

  // 每次日志加载后自动滚动到底部
  useEffect(() => {
    if (autoScroll && logListRef.current) {
      logListRef.current.scrollTop = logListRef.current.scrollHeight
    }
  }, [logs, autoScroll])

  const loadLogs = async () => {
    setLoading(true)
    try {
      const data = await getLogs(page, 50, level, search)
      setLogs(data.data || [])
      setTotal(data.total || 0)
      setSize(data.size || 0)
    } catch (e) {
      console.error('Failed to load logs', e)
    } finally {
      setLoading(false)
    }
  }

  const handleSearchInput = (value: string) => {
    setSearchInput(value)
    if (searchTimer.current) clearTimeout(searchTimer.current)
    searchTimer.current = window.setTimeout(() => {
      setSearch(value)
      setPage(1)
    }, 400)
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

  /** 导出原始日志文件（plain text） */
  const handleExportRaw = async () => {
    try {
      // 拉取全部日志（不分页）以纯文本格式导出
      const allData = await getLogs(1, total || 10000, '', '')
      const entries = allData.data || []
      const lines = entries.map((log: LogEntry) =>
        `[${log.createdAt}] [${log.level}] ${log.message}`
      ).join('\n')
      const blob = new Blob([lines + '\n'], { type: 'text/plain;charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `wg-server-logs-${new Date().toISOString().slice(0, 10)}.txt`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (e) {
      alert('导出失败')
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
          <button className="btn btn-sm btn-primary" onClick={handleExportRaw}>导出原始日志</button>
          <button className="btn btn-sm" onClick={loadLogs}>刷新</button>
        </div>
      </div>

      <div className="log-search-bar">
        <input
          type="text"
          value={searchInput}
          onChange={e => handleSearchInput(e.target.value)}
          placeholder="搜索日志关键词..."
          className="search-input"
        />
        {search && (
          <span className="search-hint">
            搜索 &ldquo;{search}&rdquo; 共 {total} 条结果
            <button className="btn btn-sm" onClick={() => { setSearch(''); setSearchInput(''); setPage(1) }}>清除</button>
          </span>
        )}
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
        <label style={{ marginLeft: 16, display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 12 }}>
          <input type="checkbox" checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} />
          自动滚动到底部
        </label>
      </div>

      <div className="log-list" ref={logListRef} style={{ maxHeight: '500px', overflowY: 'auto' }}>
        {loading && <div className="loading">加载中...</div>}
        {!loading && logs.length === 0 && <div className="empty">暂无日志</div>}
        {(logs || []).map(log => (
          <div key={log.id} className="log-entry">
            <span className="log-time">[{log.createdAt}]</span>
            <span className={`log-level log-level-${log.level.toLowerCase()}`}>[{log.level}]</span>
            <span className={`log-msg ${getLevelClass(log.level)}`}>
              {search ? highlightText(log.message, search) : log.message}
            </span>
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

/** 高亮搜索关键词 */
function highlightText(text: string, keyword: string): React.ReactNode {
  if (!keyword) return text
  const parts = text.split(new RegExp(`(${escapeRegex(keyword)})`, 'gi'))
  return parts.map((part, i) =>
    part.toLowerCase() === keyword.toLowerCase()
      ? <mark key={i} className="log-highlight">{part}</mark>
      : part
  )
}

function escapeRegex(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export default Logs
