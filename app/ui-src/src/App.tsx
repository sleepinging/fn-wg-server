import React, { useState, useEffect } from 'react'
import { getSystemInfo, getWGKernel } from './api'
import Dashboard from './components/Dashboard'
import Users from './components/Users'
import UserDetail from './components/UserDetail'
import Config from './components/Config'
import Logs from './components/Logs'
import System from './components/System'

type Tab = 'dashboard' | 'users' | 'userDetail' | 'config' | 'logs' | 'system'

const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<Tab>('dashboard')
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [systemInfo, setSystemInfo] = useState<any>(null)
  const [wgKernel, setWgKernel] = useState<any>(null)

  useEffect(() => {
    loadSystemInfo()
    const timer = setInterval(loadSystemInfo, 30000)
    return () => clearInterval(timer)
  }, [])

  const loadSystemInfo = async () => {
    try {
      const [info, kernel] = await Promise.all([
        getSystemInfo(),
        getWGKernel(),
      ])
      setSystemInfo(info)
      setWgKernel(kernel)
    } catch (e) {
      // ignore
    }
  }

  const handleViewUser = (userId: number) => {
    setSelectedUserId(userId)
    setActiveTab('userDetail')
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: 'dashboard', label: '仪表盘' },
    { key: 'users', label: '用户管理' },
    { key: 'config', label: '服务配置' },
    { key: 'logs', label: '运行日志' },
    { key: 'system', label: '系统信息' },
  ]

  const renderContent = () => {
    switch (activeTab) {
      case 'dashboard':
        return <Dashboard onViewUser={handleViewUser} />
      case 'users':
        return <Users onViewUser={handleViewUser} />
      case 'userDetail':
        return selectedUserId ? (
          <UserDetail userId={selectedUserId} onBack={() => setActiveTab('users')} />
        ) : (
          <Users onViewUser={handleViewUser} />
        )
      case 'config':
        return <Config />
      case 'logs':
        return <Logs />
      case 'system':
        return <System systemInfo={systemInfo} wgKernel={wgKernel} />
      default:
        return <Dashboard onViewUser={handleViewUser} />
    }
  }

  return (
    <div className="app-container">
      <header className="app-header">
        <h1>
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
          </svg>
          WireGuard 服务端管理
        </h1>
        <div className="header-info">
          {systemInfo && (
            <span className="badge">v{systemInfo.version}</span>
          )}
          {wgKernel && wgKernel.moduleLoaded && (
            <span className="badge success">WireGuard 已加载</span>
          )}
          {wgKernel && !wgKernel.moduleLoaded && (
            <span className="badge error">WireGuard 未加载</span>
          )}
        </div>
      </header>

      <nav className="tab-nav">
        {tabs.map(tab => (
          <button
            key={tab.key}
            className={`tab-btn ${activeTab === tab.key ? 'active' : ''}`}
            onClick={() => setActiveTab(tab.key)}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      <main className="tab-content">
        {renderContent()}
      </main>

      <footer className="app-footer">
        <span>wg-server v{systemInfo?.version || '1.0.0'}</span>
        {systemInfo && (
          <span>
            CPU: {systemInfo.cpuUsage || 'N/A'} | 内存: {systemInfo.processMemory || 'N/A'} | 运行: {systemInfo.uptime || 'N/A'}
          </span>
        )}
      </footer>
    </div>
  )
}

export default App
