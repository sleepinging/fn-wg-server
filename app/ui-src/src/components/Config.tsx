import React, { useState, useEffect } from 'react'
import { getConfig, updateConfig, exportBackup, restoreBackup, startService, stopService, getServiceStatus } from '../api'

const Config: React.FC = () => {
  const [config, setConfig] = useState<any>(null)
  const [wg, setWg] = useState<any>({})
  const [serviceStatus, setServiceStatus] = useState<any>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    loadConfig()
    loadServiceStatus()
  }, [])

  const loadConfig = async () => {
    try {
      const data = await getConfig()
      setConfig(data)
      setWg(data.wireguard || {})
    } catch (e) {
      console.error('Failed to load config', e)
    }
  }

  const loadServiceStatus = async () => {
    try {
      const data = await getServiceStatus()
      setServiceStatus(data)
    } catch (e) {
      // ignore
    }
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      await updateConfig({
        wireguard: wg,
        historyRetentionDays: config?.historyRetentionDays || '7',
        autoStart: config?.autoStart || 'false',
      })
      alert('配置已保存')
    } catch (e: any) {
      alert('保存失败: ' + (e.response?.data?.error || e.message))
    } finally {
      setSaving(false)
    }
  }

  const handleExport = async () => {
    try {
      const blob = await exportBackup()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `wg-server-backup-${new Date().toISOString().slice(0, 10)}.json`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      alert('导出失败')
    }
  }

  const handleRestore = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.json'
    input.onchange = async (e: any) => {
      const file = e.target.files[0]
      if (!file) return
      try {
        const text = await file.text()
        const data = JSON.parse(text)
        await restoreBackup(data)
        alert('配置已恢复')
        loadConfig()
      } catch (err) {
        alert('恢复失败: 无效的备份文件')
      }
    }
    input.click()
  }

  const handleServiceAction = async (action: 'start' | 'stop' | 'restart') => {
    try {
      if (action === 'start') await startService()
      else if (action === 'stop') await stopService()
      else {
        await stopService()
        await new Promise(r => setTimeout(r, 1000))
        await startService()
      }
      await loadServiceStatus()
    } catch (e: any) {
      alert('操作失败: ' + (e.response?.data?.error || e.message))
    }
  }

  return (
    <div className="config-page">
      <div className="config-section">
        <h3>服务控制</h3>
        <div className="service-controls">
          <div className="service-status">
            <span className={`status-dot ${serviceStatus.wgRunning ? 'online' : 'offline'}`} />
            WireGuard: {serviceStatus.wgRunning ? '运行中' : '已停止'}
            <span className="sep">|</span>
            <span className={`status-dot ${serviceStatus.monitorRunning ? 'online' : 'offline'}`} />
            监控: {serviceStatus.monitorRunning ? '运行中' : '已停止'}
          </div>
          <div className="service-actions">
            <button className="btn btn-success" onClick={() => handleServiceAction('start')} disabled={serviceStatus.wgRunning}>
              启动服务
            </button>
            <button className="btn btn-danger" onClick={() => handleServiceAction('stop')} disabled={!serviceStatus.wgRunning}>
              停止服务
            </button>
            <button className="btn" onClick={() => handleServiceAction('restart')}>
              重启服务
            </button>
          </div>
        </div>
      </div>

      <div className="config-section">
        <h3>WireGuard 配置</h3>
        <div className="form-group">
          <label>接口名称</label>
          <input type="text" value={wg.interfaceName || 'wg0'} onChange={e => setWg({...wg, interfaceName: e.target.value})} />
        </div>
        <div className="form-group">
          <label>监听端口</label>
          <input type="number" value={wg.listenPort || 51820} onChange={e => setWg({...wg, listenPort: parseInt(e.target.value) || 51820})} />
        </div>
        <div className="form-group">
          <label>IP网段</label>
          <input type="text" value={wg.address || '192.168.5.1/24'} onChange={e => setWg({...wg, address: e.target.value})} />
          <small>格式: 192.168.5.1/24，用户IP将从192.168.5.10开始分配</small>
        </div>
        <div className="form-group">
          <label>私钥</label>
          <textarea rows={2} value={wg.privateKey || ''} onChange={e => setWg({...wg, privateKey: e.target.value})} />
        </div>
        <div className="form-group">
          <label>公钥</label>
          <textarea rows={2} value={wg.publicKey || ''} onChange={e => setWg({...wg, publicKey: e.target.value})} readOnly />
        </div>
        <div className="form-group">
          <label>DNS</label>
          <input type="text" value={wg.dns || ''} onChange={e => setWg({...wg, dns: e.target.value})} placeholder="8.8.8.8, 114.114.114.114" />
        </div>
        <div className="form-group">
          <label>MTU</label>
          <input type="number" value={wg.mtu || 1420} onChange={e => setWg({...wg, mtu: parseInt(e.target.value) || 1420})} />
        </div>
        <div className="form-group">
          <label>PostUp</label>
          <textarea rows={2} value={wg.postUp || ''} onChange={e => setWg({...wg, postUp: e.target.value})} />
        </div>
        <div className="form-group">
          <label>PostDown</label>
          <textarea rows={2} value={wg.postDown || ''} onChange={e => setWg({...wg, postDown: e.target.value})} />
        </div>
      </div>

      <div className="config-section">
        <h3>其他设置</h3>
        <div className="form-group">
          <label>历史记录保留天数</label>
          <input
            type="number"
            value={config?.historyRetentionDays || '7'}
            onChange={e => setConfig({...config, historyRetentionDays: e.target.value})}
          />
          <small>默认7天，带宽数据每秒采样一次</small>
        </div>
        <div className="form-group">
          <label>开机自启</label>
          <select
            value={config?.autoStart || 'false'}
            onChange={e => setConfig({...config, autoStart: e.target.value})}
          >
            <option value="true">启用</option>
            <option value="false">禁用</option>
          </select>
        </div>
      </div>

      <div className="config-actions">
        <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
          {saving ? '保存中...' : '保存配置'}
        </button>
        <button className="btn" onClick={handleExport}>导出备份</button>
        <button className="btn" onClick={handleRestore}>恢复备份</button>
      </div>
    </div>
  )
}

export default Config
