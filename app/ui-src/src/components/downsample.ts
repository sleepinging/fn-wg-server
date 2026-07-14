// 前端本地降采样：把点数压缩到 targetPoints，使用 max 算法
// 算法与后端 db/stats.go getBandwidthHistoryAgg 的 max 聚合保持一致
// points 须按 ts 升序排列
export function downsampleMax<T extends { ts: number; rxSpeed?: number; txSpeed?: number; rxBytes?: number; txBytes?: number }>(
  points: T[],
  targetPoints: number,
): T[] {
  if (targetPoints <= 0 || points.length <= targetPoints) {
    return points
  }
  const step = points.length / targetPoints
  const result: T[] = []
  for (let i = 0; i < targetPoints; i++) {
    const startIdx = Math.floor(i * step)
    const endIdx = Math.min(Math.floor((i + 1) * step), points.length)
    if (startIdx >= endIdx) continue
    const window = points.slice(startIdx, endIdx)
    if (window.length === 0) continue
    const p = { ...window[0] } // 保留首个点的 ts 及其它字段
    let maxRx = 0, maxTx = 0
    for (const wp of window) {
      if ((wp.rxSpeed || 0) > maxRx) maxRx = wp.rxSpeed || 0
      if ((wp.txSpeed || 0) > maxTx) maxTx = wp.txSpeed || 0
    }
    p.rxSpeed = maxRx
    p.txSpeed = maxTx
    // 累计字节取窗口最后一个点
    const last = window[window.length - 1]
    p.rxBytes = last.rxBytes
    p.txBytes = last.txBytes
    result.push(p)
  }
  return result
}
