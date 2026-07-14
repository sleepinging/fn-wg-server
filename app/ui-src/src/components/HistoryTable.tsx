import React, { useState, useEffect, useCallback } from 'react'
import * as api from '../api'

const PAGE_SIZE = 10

const HistoryTable: React.FC<{ uid: number }> = ({ uid }) => {
  const [history, setHistory] = useState<any[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)

  const loadHistory = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.getUserHistory(uid, page, PAGE_SIZE)
      setHistory(data.history || data.data || [])
      setTotal(data.total || 0)
    } catch (e) {
      console.error('Failed to load history', e)
    } finally {
      setLoading(false)
    }
  }, [uid, page])

  useEffect(() => {
    loadHistory()
  }, [loadHistory])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

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
          {loading && (
            <tr><td colSpan={6} className="empty">加载中...</td></tr>
          )}
          {!loading && (history || []).map((h: any) => (
            <tr key={h.id}>
              <td>{h.internalIP}</td>
              <td className="ip">{h.externalIP}</td>
              <td>{h.connectedAt ? new Date(h.connectedAt).toLocaleString() : '-'}</td>
              <td>{h.disconnectedAt ? new Date(h.disconnectedAt).toLocaleString() : '在线中'}</td>
              <td>{formatBytesSimple(h.txBytes)}</td>
              <td>{formatBytesSimple(h.rxBytes)}</td>
            </tr>
          ))}
          {!loading && (history || []).length === 0 && (
            <tr><td colSpan={6} className="empty">暂无历史记录</td></tr>
          )}
        </tbody>
      </table>
      <div className="pagination">
        <button className="btn btn-sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>上一页</button>
        <span>第 {page} 页 / 共 {totalPages} 页 (共{total}条)</span>
        <button className="btn btn-sm" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</button>
      </div>
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

export default HistoryTable
