import React, { useState, useEffect, useCallback } from 'react'
import { getUsers, deleteUser, createUser, getIPHint, User } from '../api'

interface Props {
  onViewUser: (userId: number) => void
}

const Users: React.FC<Props> = ({ onViewUser }) => {
  const [users, setUsers] = useState<User[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ username: '', internalIP: '', dns: '', mtu: 1420, persistentKeepalive: 25 })
  const [ipHint, setIpHint] = useState('')
  const [loading, setLoading] = useState(false)

  const loadUsers = useCallback(async () => {
    try {
      const data = await getUsers()
      setUsers(data)
    } catch (e) {
      console.error('Failed to load users', e)
    }
  }, [])

  useEffect(() => {
    loadUsers()
    const timer = setInterval(loadUsers, 10000)
    return () => clearInterval(timer)
  }, [loadUsers])

  const handleDelete = async (user: User) => {
    if (!confirm(`确定删除用户 "${user.username}"？\n删除后将强制该用户下线。`)) return
    try {
      await deleteUser(user.id)
      loadUsers()
    } catch (e: any) {
      alert('删除失败: ' + (e.response?.data?.error || e.message))
    }
  }

  const handleAdd = async () => {
    if (!form.username || !form.internalIP) {
      alert('用户名和内部IP不能为空')
      return
    }
    setLoading(true)
    try {
      await createUser(form)
      setShowAdd(false)
      setForm({ username: '', internalIP: '', dns: '', mtu: 1420, persistentKeepalive: 25 })
      loadUsers()
    } catch (e: any) {
      alert('创建失败: ' + (e.response?.data?.error || e.message))
    } finally {
      setLoading(false)
    }
  }

  const getHint = async () => {
    try {
      const data = await getIPHint()
      setIpHint(data.ip)
      setForm(prev => ({ ...prev, internalIP: data.ip }))
    } catch (e) {
      console.error('Failed to get IP hint', e)
    }
  }

  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  return (
    <div className="users-page">
      <div className="page-header">
        <h3>用户管理 ({users.length})</h3>
        <button className="btn btn-primary" onClick={() => { setShowAdd(true); getHint() }}>
          + 添加用户
        </button>
      </div>

      {showAdd && (
        <div className="modal-overlay" onClick={() => setShowAdd(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h4>添加用户</h4>
            <div className="form-group">
              <label>用户名 *</label>
              <input
                type="text"
                value={form.username}
                onChange={e => setForm(prev => ({ ...prev, username: e.target.value }))}
                placeholder="输入用户名"
              />
            </div>
            <div className="form-group">
              <label>内部IP *</label>
              <div className="input-with-hint">
                <input
                  type="text"
                  value={form.internalIP}
                  onChange={e => setForm(prev => ({ ...prev, internalIP: e.target.value }))}
                  placeholder="例如: 192.168.5.10/32"
                />
                <button className="btn btn-sm" onClick={getHint} title="获取最小可用IP">提示</button>
              </div>
              {ipHint && <small className="hint-text">建议: {ipHint}</small>}
            </div>
            <div className="form-group">
              <label>DNS</label>
              <input
                type="text"
                value={form.dns}
                onChange={e => setForm(prev => ({ ...prev, dns: e.target.value }))}
                placeholder="例如: 8.8.8.8, 114.114.114.114"
              />
            </div>
            <div className="form-row">
              <div className="form-group">
                <label>MTU</label>
                <input
                  type="number"
                  value={form.mtu}
                  onChange={e => setForm(prev => ({ ...prev, mtu: parseInt(e.target.value) || 1420 }))}
                />
              </div>
              <div className="form-group">
                <label>Keepalive (秒)</label>
                <input
                  type="number"
                  value={form.persistentKeepalive}
                  onChange={e => setForm(prev => ({ ...prev, persistentKeepalive: parseInt(e.target.value) || 25 }))}
                />
              </div>
            </div>
            <div className="modal-actions">
              <button className="btn" onClick={() => setShowAdd(false)}>取消</button>
              <button className="btn btn-primary" onClick={handleAdd} disabled={loading}>
                {loading ? '添加中...' : '添加'}
              </button>
            </div>
          </div>
        </div>
      )}

      <table className="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>用户名</th>
            <th>内部IP</th>
            <th>公钥</th>
            <th>状态</th>
            <th>下载</th>
            <th>上传</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {(users || []).map(user => (
            <tr key={user.id} className={user.online ? 'row-online' : ''}>
              <td>{user.id}</td>
              <td>{user.username}</td>
              <td className="ip">{user.internalIP}</td>
              <td className="mono" title={user.publicKey}>{user.publicKey.substring(0, 16)}...</td>
              <td>
                <span className={`status-dot ${user.online ? 'online' : 'offline'}`} />
                {user.online ? '在线' : '离线'}
              </td>
              <td>{formatBytes(user.rxBytes || 0)}</td>
              <td>{formatBytes(user.txBytes || 0)}</td>
              <td>{user.createdAt}</td>
              <td className="actions">
                <button className="btn btn-sm" onClick={() => onViewUser(user.id)}>详情</button>
                <button className="btn btn-sm btn-danger" onClick={() => handleDelete(user)}>删除</button>
              </td>
            </tr>
          ))}
          {users.length === 0 && (
            <tr><td colSpan={9} className="empty">暂无用户，点击"添加用户"开始</td></tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

export default Users
