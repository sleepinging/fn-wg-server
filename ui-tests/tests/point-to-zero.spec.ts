import { test, expect } from './fixtures'

test.describe('复现非零点逐渐消失', () => {
  test('初始非零点数在30秒内减少', async ({ debugPage: page }) => {
    test.slow()
    await page.waitForTimeout(3000)

    const selects = page.locator('select')
    await selects.first().selectOption('1')
    await page.waitForTimeout(2000)

    // 统计SVG中非零Y值的数量
    const countNonZero = async () => {
      return await page.evaluate(() => {
        const svg = document.querySelector('[data-testid="chart-wrapper"] svg')
        if (!svg) return -1
        const paths = svg.querySelectorAll('path')
        for (const p of paths) {
          if (p.getAttribute('stroke') !== '#2196F3') continue
          const d = p.getAttribute('d') || ''
          const ys: number[] = []
          const re = /[MLC]\s*[\d.]+\s*,?\s*([\d.]+)/g
          let m
          while ((m = re.exec(d)) !== null) ys.push(parseFloat(m[1]))
          // Y<150=非零, Y>200=0
          return ys.filter(y => y < 150).length
        }
        return -1
      })
    }

    const nz0 = await countNonZero()
    console.log(`T+0s: nonZero=${nz0}`)
    expect(nz0).toBeGreaterThan(0)

    // 记录每次读数
    const history: number[] = [nz0]
    for (let s = 3; s <= 30; s += 3) {
      await page.waitForTimeout(3000)
      const nz = await countNonZero()
      history.push(nz)
      console.log(`T+${s}s: nonZero=${nz}`)
    }

    const nz30 = history[history.length - 1]
    console.log(`Change: ${nz0} → ${nz30}`)

    // 重采样保留非零点分布，计数应稳定
    expect(nz30).toBeGreaterThanOrEqual(nz0 * 0.7)  // 允许30%波动
  }, 90000)
})
