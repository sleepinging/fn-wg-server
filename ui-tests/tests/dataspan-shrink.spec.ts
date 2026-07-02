import { test, expect } from './fixtures'

/**
 * 复现：dataSpan 从初始的894s 逐渐缩小到 ~99s
 * 原因：初始 fixture 数据广域采样 → 增量 1s 密度 → slice(-100) 替换后压缩
 */
test.describe('复现数据跨度缩小', () => {
  test('15m范围下dataSpan是否缩小', async ({ debugPage: page }) => {
    test.slow()
    await page.waitForTimeout(2000)

    const selects = page.locator('select')
    await selects.first().selectOption('1')     // 1s刷新
    await selects.nth(1).selectOption('15m')    // 15分钟范围
    await page.waitForTimeout(2000)

    const readSpan = async () => {
      return await page.evaluate(() => {
        const el = document.evaluate(
          "//span[contains(text(), '📊')]",
          document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null
        ).singleNodeValue as any
        const text = el?.parentElement?.textContent || ''
        const pts = (text.match(/数据:(\d+)pts/) || [])[1]
        const first = (text.match(/首:(\S+)/) || [])[1]
        const last = (text.match(/尾:(\S+)/) || [])[1]
        // 数据跨度在 ⏱ ... 跨度:Xs
        const dataSpan = (text.match(/⏱.*?跨度:(\d+)s/) || [])[1]
        const domainSpan = (text.match(/🎯.*?跨度:(\d+)s/) || [])[1]
        return { pts, first, last, dataSpan: parseInt(dataSpan) || 0, domainSpan: parseInt(domainSpan) || 0 }
      })
    }

    // 等初始加载完成
    await page.waitForTimeout(3000)
    const t0 = await readSpan()
    console.log(`T+0s: pts=${t0.pts} dataSpan=${t0.dataSpan}s domainSpan=${t0.domainSpan}s first=${t0.first} last=${t0.last}`)

    let minSpan = t0.dataSpan
    let maxSpan = t0.dataSpan

    // 监控60秒
    for (let s = 3; s <= 60; s += 3) {
      await page.waitForTimeout(3000)
      const t = await readSpan()
      console.log(`T+${s}s: pts=${t.pts} dataSpan=${t.dataSpan}s first=${t.first} last=${t.last}`)
      minSpan = Math.min(minSpan, t.dataSpan)
      maxSpan = Math.max(maxSpan, t.dataSpan)
    }

    const ratio = minSpan / maxSpan
    console.log(`Span range: ${minSpan}s ~ ${maxSpan}s, ratio=${ratio.toFixed(2)}`)

    // 如果 span 缩小超过 50%，说明数据窗口在压缩
    if (ratio < 0.5) {
      console.log('⚠️ 数据跨度缩小超过50%，窗口在压缩！')
    }
    expect(ratio).toBeGreaterThan(0.8)  // 期望稳定在 ±20% 内
  }, 120000)
})
