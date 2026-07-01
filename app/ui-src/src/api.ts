import axios from 'axios'

// 在 CGI 模式下，页面 URL 形如 /cgi/ThirdParty/wg-server/index.cgi
// API 请求必须走相同的 CGI 路径：/cgi/ThirdParty/wg-server/index.cgi/api/xxx
// 不能直接请求 /api/xxx（会 404，因为不走 CGI 路由）
const cgiBasePath = (() => {
  const path = window.location.pathname;
  const match = path.match(/^(.+\/index\.cgi)/);
  if (match) {
    return match[1];
  }
  return '';
})();

export const api = axios.create({
  baseURL: cgiBasePath + '/api',
  timeout: 10000,
  headers: { 'Content-Type': 'application/json' },
})

export interface User {
  id: number
  username: string
  publicKey: string
  privateKey?: string
  presharedKey?: string
  allowedIPs: string
  internalIP: string
  dns: string
  mtu: number
  persistentKeepalive: number
  enabled: boolean
  createdAt: string
  updatedAt: string
  rxBytes?: number
  txBytes?: number
  online?: boolean
  endpoint?: string
  latestHandshake?: number
  onlineSince?: string
}

export interface GlobalStats {
  rxBytes: number
  txBytes: number
  rxSpeed: number
  txSpeed: number
  onlineCount: number
  totalPeers: number
  externalIP: string
  internalIP: string
  uptime: string
}

export interface BandwidthPoint {
  timestamp: string
  rxBytes: number
  txBytes: number
  rxSpeed: number
  txSpeed: number
}

export interface WGConfig {
  interfaceName: string
  privateKey: string
  publicKey: string
  address: string
  listenPort: number
  dns: string
  mtu: number
  postUp: string
  postDown: string
}

export interface SystemInfo {
  version: string
  cpuUsage: string
  memory: { total: number; available: number; usedPercent: string }
  uptime: string
  processMemory: string
}

export interface LogEntry {
  id: number
  level: string
  message: string
  createdAt: string
}

// Users
export const getUsers = () => api.get<User[]>('/users').then(r => r.data)
export const getUser = (id: number) => api.get<User>(`/users/${id}`).then(r => r.data)
export const createUser = (data: Partial<User>) => api.post('/users', data).then(r => r.data)
export const updateUser = (id: number, data: Partial<User>) => api.put(`/users/${id}`, data).then(r => r.data)
export const deleteUser = (id: number) => api.delete(`/users/${id}`).then(r => r.data)
export const getUserStats = (id: number) => api.get(`/users/${id}/stats`).then(r => r.data)
export const getUserHistory = (id: number, page = 1, pageSize = 20) =>
  api.get(`/users/${id}/history?page=${page}&pageSize=${pageSize}`).then(r => r.data)
export const getUserTraffic = (id: number, start = '', end = '', raw = '') => {
  // startTs 用数值时间戳（毫秒），避免 ISO 字符串被 shell/环境变量截断
  let params = `end=${encodeURIComponent(end)}&raw=${raw}`
  if (start) {
    const ts = new Date(start).getTime()
    if (!isNaN(ts)) {
      params = `startTs=${ts}&` + params
    } else {
      params = `start=${encodeURIComponent(start)}&` + params
    }
  }
  return api.get(`/users/${id}/traffic?${params}`).then(r => r.data)
}

// Stats
export const getStats = () => api.get<GlobalStats>('/stats').then(r => r.data)
export const getStatsHistory = (userId = 0, start = '', end = '', raw = '') => {
  let params = `userId=${userId}&end=${encodeURIComponent(end)}&raw=${raw}`
  if (start) {
    const ts = new Date(start).getTime()
    if (!isNaN(ts)) {
      params = `userId=${userId}&startTs=${ts}&` + params.split('&').slice(1).join('&')
    } else {
      params = `userId=${userId}&start=${encodeURIComponent(start)}&` + params.split('&').slice(1).join('&')
    }
  }
  return api.get<BandwidthPoint[]>(`/stats/history?${params}`).then(r => r.data)
}

// Config
export const getConfig = () => api.get('/config').then(r => r.data)
export const updateConfig = (data: any) => api.put('/config', data).then(r => r.data)
export const exportBackup = () => api.get('/config/backup', { responseType: 'blob' }).then(r => r.data)
export const restoreBackup = (data: any) => api.post('/config/restore', data).then(r => r.data)

// Service
export const getServiceStatus = () => api.get('/service/status').then(r => r.data)
export const startService = () => api.post('/service/start').then(r => r.data)
export const stopService = () => api.post('/service/stop').then(r => r.data)
export const restartService = () => api.post('/service/restart').then(r => r.data)

// System
export const getSystemInfo = () => api.get<SystemInfo>('/system').then(r => r.data)
export const getWGKernel = () => api.get('/wg/kernel').then(r => r.data)

// Logs
export const getLogs = (page = 1, pageSize = 50, level = '', search = '') =>
  api.get(`/logs?page=${page}&pageSize=${pageSize}&level=${level}&search=${encodeURIComponent(search)}`).then(r => r.data)
export const cleanLogs = (days: number) => api.post('/logs/clean', { days }).then(r => r.data)

// IP Hint
export const getIPHint = () => api.get('/ip/hint').then(r => r.data)
export const getUserConfig = (id: number) => api.get(`/users/${id}/config?format=json`).then(r => r.data)

export default api
