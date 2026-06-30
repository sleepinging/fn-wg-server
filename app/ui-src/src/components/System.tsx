import React from 'react'
import { getSystemInfo, getWGKernel } from '../api'

interface Props {
  systemInfo: any
  wgKernel: any
  refreshInterval: number
  onRefreshIntervalChange: (ms: number) => void
}

const INTERVAL_OPTIONS = [
  { label: '1 秒', value: 1000 },
  { label: '2 秒', value: 2000 },
  { label: '5 秒', value: 5000 },
  { label: '10 秒', value: 10000 },
  { label: '30 秒', value: 30000 },
  { label: '不自动刷新', value: 0 },
]

const System: React.FC<Props> = ({ systemInfo, wgKernel, refreshInterval, onRefreshIntervalChange }) => {
  const handleManualRefresh = async () => {
    try {
      const [info, kernel] = await Promise.all([
        getSystemInfo(),
        getWGKernel(),
      ])
      // 通过父组件传递的 props 无法直接更新，手动触发刷新通过改变 interval 实现
      window.location.reload()
    } catch (e) {
      // ignore
    }
  }

  return (
    <div className="system-page">
      <div className="system-controls-bar">
        <div className="refresh-control">
          <label>刷新间隔：</label>
          <select
            value={refreshInterval}
            onChange={e => onRefreshIntervalChange(parseInt(e.target.value))}
          >
            {INTERVAL_OPTIONS.map(opt => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
        <button className="btn btn-sm" onClick={handleManualRefresh}>立即刷新</button>
      </div>

      <div className="info-cards">
        <div className="info-card">
          <h4>程序信息</h4>
          <div className="info-row">
            <label>版本号</label>
            <span>{systemInfo?.version || 'N/A'}</span>
          </div>
          <div className="info-row">
            <label>CPU占用</label>
            <span>{systemInfo?.cpuUsage || 'N/A'}</span>
          </div>
          <div className="info-row">
            <label>内存占用</label>
            <span>{systemInfo?.processMemory || 'N/A'}</span>
          </div>
          <div className="info-row">
            <label>服务运行时间</label>
            <span>{systemInfo?.uptime || 'N/A'}</span>
          </div>
        </div>

        <div className="info-card">
          <h4>系统信息</h4>
          <div className="info-row">
            <label>总内存</label>
            <span>{systemInfo?.memory ? formatBytes(systemInfo.memory.total) : 'N/A'}</span>
          </div>
          <div className="info-row">
            <label>可用内存</label>
            <span>{systemInfo?.memory ? formatBytes(systemInfo.memory.available) : 'N/A'}</span>
          </div>
          <div className="info-row">
            <label>内存使用率</label>
            <span>{systemInfo?.memory?.usedPercent || 'N/A'}</span>
          </div>
        </div>

        <div className="info-card">
          <h4>WireGuard 内核状态</h4>
          <div className="info-row">
            <label>内核模块</label>
            <span>
              <span className={`status-dot ${wgKernel?.moduleLoaded ? 'online' : 'offline'}`} />
              {wgKernel?.moduleLoaded ? '已加载' : '未加载'}
            </span>
          </div>
          <div className="info-row">
            <label>内核版本</label>
            <span className="mono" style={{ fontSize: '0.85em' }}>
              {wgKernel?.kernelVersion ? wgKernel.kernelVersion.substring(0, 80) + '...' : 'N/A'}
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export default System
