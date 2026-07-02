import { useRef, useEffect, useState } from 'react'

interface DebugBarProps {
  dataPoints: number
  firstTs: number
  lastTs: number
  domainStart: number
  domainEnd: number
}

export default function DebugBar({ dataPoints, firstTs, lastTs, domainStart, domainEnd }: DebugBarProps) {
  const [visible, setVisible] = useState(false)
  useEffect(() => {
    const p = new URLSearchParams(window.location.search)
    setVisible(p.get('debug') === '1')
  }, [])

  if (!visible) return null

  const fmt = (ts: number) => {
    if (ts <= 0) return '-'
    const d = new Date(ts)
    return `${d.getHours().toString().padStart(2,'0')}:${d.getMinutes().toString().padStart(2,'0')}:${d.getSeconds().toString().padStart(2,'0')}`
  }
  const spanSec = lastTs > firstTs ? ((lastTs - firstTs) / 1000).toFixed(0) : '-'
  const domainSpan = domainEnd > domainStart ? ((domainEnd - domainStart) / 1000).toFixed(0) : '-'

  return (
    <div style={{
      position:'fixed', bottom:0, left:0, right:0,
      background:'#1a1a2e', color:'#00ff88', fontSize:11,
      padding:'4px 12px', fontFamily:'monospace',
      display:'flex', gap:20, flexWrap:'wrap', zIndex:9999,
      borderTop:'1px solid #333',
    }}>
      <span>📊 数据:{dataPoints}pts</span>
      <span>⏱ 首:{fmt(firstTs)} 尾:{fmt(lastTs)} 跨度:{spanSec}s</span>
      <span>🎯 域:{fmt(domainStart)} → {fmt(domainEnd)} 跨度:{domainSpan}s</span>
    </div>
  )
}
