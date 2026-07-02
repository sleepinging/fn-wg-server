import React, { useState, useMemo, useRef, useCallback } from 'react'
import ReactECharts from 'echarts-for-react'

interface BandwidthChartProps {
  title: string
  chartData: any[]
  domain: [number, number]
  loading: boolean
  height: number
  intervalSec: number
  timeRange: string
  onIntervalChange: (v: number) => void
  onTimeRangeChange: (v: string) => void
  line1Key: string
  line2Key: string
  formatSpeed: (v: number) => string
  extraContent?: React.ReactNode
}

const BandwidthChart: React.FC<BandwidthChartProps> = ({
  title, chartData, domain, loading, height,
  intervalSec, timeRange, onIntervalChange, onTimeRangeChange,
  line1Key, line2Key, formatSpeed, extraContent,
}) => {
  const [hidden, setHidden] = useState<Record<string, boolean>>({})
  const chartRef = useRef<any>(null)

  const onChartReady = useCallback((instance: any) => {
    chartRef.current = instance
    instance.on('legendselectchanged', (params: any) => {
      setHidden(prev => ({ ...prev, ...params.selected }))
    })
  }, [])

  const option = useMemo(() => ({
    animation: false,
    grid: { top: 40, right: 20, bottom: 30, left: 60 },
    legend: {
      data: ['下载', '上传'],
      top: 8,
      left: 'center',
      textStyle: { fontSize: 13 },
      selectedMode: true,
      selected: {
        '下载': hidden['下载'] === undefined ? true : hidden['下载'],
        '上传': hidden['上传'] === undefined ? true : hidden['上传'],
      },
    },
    xAxis: {
      type: 'time',
      min: domain[0],
      max: domain[1],
      axisLabel: {
        formatter: (v: number) => new Date(v).toLocaleTimeString(),
        fontSize: 12,
      },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: (v: number) => formatSpeed(v),
        fontSize: 12,
      },
      splitLine: { lineStyle: { color: '#e0e0e0', type: 'dashed' } },
    },
    tooltip: {
      trigger: 'axis',
      formatter: (params: any) => {
        if (!params || params.length === 0) return ''
        const ts = params[0].data[0]
        const lines = params.map((p: any) =>
          `${p.marker} ${p.seriesName}: ${formatSpeed(p.data[1])}`
        )
        return `${new Date(ts).toLocaleTimeString()}<br/>${lines.join('<br/>')}`
      },
    },
    series: [
      {
        name: '下载',
        type: 'line',
        smooth: true,
        symbol: 'none',
        color: '#2196F3',
        lineStyle: { color: '#2196F3', width: 2 },
        itemStyle: { color: '#2196F3' },
        data: chartData.map(p => [p.ts, p[line1Key] ?? 0]),
      },
      {
        name: '上传',
        type: 'line',
        smooth: true,
        symbol: 'none',
        color: '#FF9800',
        lineStyle: { color: '#FF9800', width: 2 },
        itemStyle: { color: '#FF9800' },
        data: chartData.map(p => [p.ts, p[line2Key] ?? 0]),
      },
    ],
  }), [chartData, domain, line1Key, line2Key, formatSpeed, hidden])

  return (
    <div className="chart-section">
      <div className="section-header">
        <h3>{title}</h3>
        <span style={{ marginLeft: 8, fontSize: 13 }}>刷新</span>
        <select value={intervalSec} onChange={e => onIntervalChange(Number(e.target.value))} style={{ marginLeft: 4 }}>
          <option value={1}>1s</option>
          <option value={3}>3s</option>
          <option value={5}>5s</option>
          <option value={10}>10s</option>
        </select>
        <span style={{ marginLeft: 12, fontSize: 13 }}>范围</span>
        <select value={timeRange} onChange={e => onTimeRangeChange(e.target.value)}>
          <option value="15m">15分钟</option>
          <option value="1h">1小时</option>
          <option value="6h">6小时</option>
          <option value="24h">24小时</option>
          <option value="7d">7天</option>
        </select>
        {extraContent}
      </div>
      <div style={{ position: 'relative', height }} data-testid="chart-wrapper">
        {loading && (
          <div style={{
            position: 'absolute', inset: 0, zIndex: 10,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            background: 'rgba(255,255,255,0.7)', borderRadius: 8,
          }}>
            <span style={{ color: '#888', fontSize: 14 }}>⏳ 加载图表数据...</span>
          </div>
        )}
        <ReactECharts
          option={option}
          style={{ height: '100%', width: '100%' }}
          notMerge
          opts={{ renderer: 'svg' }}
          onChartReady={onChartReady}
        />
      </div>
    </div>
  )
}

export default BandwidthChart
