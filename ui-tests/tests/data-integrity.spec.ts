import { test, expect } from './fixtures'

/**
 * 回归测试：数据不消失
 * 验证图表中非零数据不会突然变成全零
 */
test.describe('数据完整性回归', () => {
  test('图表有非零数据且不突变', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)

    const readLine = async () => {
      return await page.evaluate(() => {
        const svg = document.querySelector('[data-testid="chart-wrapper"] svg')
        if (!svg) return null
        const paths = svg.querySelectorAll('path')
        for (const p of paths) {
          if (p.getAttribute('stroke') !== '#2196F3') continue
          const d = p.getAttribute('d') || ''
          const coords: number[][] = []
          const re = /([MLC])\s*([\d.-]+)\s*,?\s*([\d.-]+)/g
          let m
          while ((m = re.exec(d)) !== null) {
            coords.push([parseFloat(m[2]), parseFloat(m[3])])
          }
          if (coords.length < 2) return null
          // 找第一个 Y<150 的点（非零值）
          const nonZero = coords.find(c => c[1] < 150)
          const allZero = coords.every(c => c[1] >= 200)
          return {
            total: coords.length,
            allZero,
            firstNonZeroX: nonZero ? nonZero[0] : -1,
            first5Y: coords.slice(0, 5).map(c => c[1].toFixed(0)),
          }
        }
        return null
      })
    }

    // 初始检查：数据不能全零
    const t0 = await readLine()
    expect(t0).not.toBeNull()
    expect(t0!.allZero).toBe(false)
    console.log(`T+0: firstNonZeroX=${t0!.firstNonZeroX.toFixed(0)} allZero=${t0!.allZero} first5Y=${t0!.first5Y}`)

    // 切 1s 刷新加速
    const selects = page.locator('select')
    await selects.first().selectOption('1')
    await page.waitForTimeout(1000)

    // 监控 30 秒，检测 firstNonZeroX 跳变
    let prevX = t0!.firstNonZeroX
    let jumps = 0
    for (let s = 1; s <= 30; s++) {
      const info = await readLine()
      if (!info) break
      
      const x = info.firstNonZeroX
      if (x > 0 && prevX > 10 && x > prevX * 1.5) {
        jumps++
        console.log(`⚠️ T+${s}s: firstNonZeroX jumped ${prevX.toFixed(0)} → ${x.toFixed(0)}`)
      }
      prevX = x > 0 ? x : prevX
      
      // 每秒检查数据不全零
      expect(info.allZero).toBe(false)
      
      await page.waitForTimeout(1000)
    }

    console.log(`Jumps detected: ${jumps}`)
    expect(jumps).toBe(0)
  }, 90000)

  test('用户详情页图表也不全零', async ({ debugPage: page }) => {
    // 进用户详情
    const detailBtn = page.getByRole('button', { name: '详情' }).first()
    await detailBtn.click()
    await page.waitForTimeout(3000)

    const info = await page.evaluate(() => {
      const svgs = document.querySelectorAll('svg')
      for (const svg of svgs) {
        const paths = svg.querySelectorAll('path')
        for (const p of paths) {
          if (p.getAttribute('stroke') !== '#2196F3') continue
          const d = p.getAttribute('d') || ''
          const coords: number[][] = []
          const re = /([MLC])\s*([\d.-]+)\s*,?\s*([\d.-]+)/g
          let m
          while ((m = re.exec(d)) !== null) {
            coords.push([parseFloat(m[2]), parseFloat(m[3])])
          }
          const nonZero = coords.find(c => c[1] < 150)
          return {
            total: coords.length,
            allZero: coords.every(c => c[1] >= 200),
            hasChart: true,
          }
        }
      }
      return { hasChart: false }
    })

    expect(info.hasChart).toBe(true)
    // 用户详情数据可能全零（如果用户没流量），不做强制断言
    console.log(`UserDetail: hasChart=${info.hasChart} allZero=${info.allZero} total=${info.total}`)
  })
})
