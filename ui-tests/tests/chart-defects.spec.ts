import { test, expect } from './fixtures'

test.describe('图表渲染缺陷复现', () => {
  test('Tooltip悬浮显示格式化时间', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)

    const chart = page.locator('[data-testid="chart-wrapper"]').first()
    const box = await chart.boundingBox()
    expect(box).not.toBeNull()

    if (box) {
      await page.mouse.move(box.x + box.width * 0.7, box.y + box.height * 0.5)
      await page.waitForTimeout(800)

      // ECharts tooltip
      const tooltip = page.locator('.echarts-tooltip, div[style*="absolute"][style*="pointer-events: none"]')
      const count = await tooltip.count()
      console.log('Tooltip count:', count)

      if (count > 0) {
        const text = await tooltip.first().textContent()
        console.log('Tooltip text:', text)
        // 不含长数字时间戳
        expect(text).not.toMatch(/\d{10,}/)
        // 含格式化时间
        expect(text).toMatch(/\d{1,2}:\d{2}/)
      }
    }
  })

  test('图表线无消失动画：连续帧路径一致', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)
    const selects = page.locator('select')
    await selects.first().selectOption('1')
    await page.waitForTimeout(1000)

    const readPathLen = async () => {
      return await page.evaluate(() => {
        const svg = document.querySelector('[data-testid="chart-wrapper"] svg')
        if (!svg) return []
        const result: any[] = []
        for (const p of svg.querySelectorAll('path')) {
          const s = p.getAttribute('stroke') || ''
          if (s === '#2196F3' || s === '#FF9800') {
            const d = p.getAttribute('d') || ''
            result.push({ stroke: s, dLen: d.length })
          }
        }
        return result
      })
    }

    const f0 = await readPathLen()
    console.log('Frame 0:', JSON.stringify(f0))
    await page.waitForTimeout(300)
    const f1 = await readPathLen()
    console.log('Frame 1:', JSON.stringify(f1))
    await page.waitForTimeout(300)
    const f2 = await readPathLen()
    console.log('Frame 2:', JSON.stringify(f2))

    // 线不应消失（dLength不应变为0或大幅减小）
    for (let i = 0; i < f0.length; i++) {
      expect(f1[i]?.dLen).toBeGreaterThan(0)
      expect(f2[i]?.dLen).toBeGreaterThan(0)
    }
  })

  test('Tooltip显示中文标签', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)

    const chart = page.locator('[data-testid="chart-wrapper"]').first()
    const box = await chart.boundingBox()
    if (!box) return

    await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.3)
    await page.waitForTimeout(800)

    const hasChinese = await page.evaluate(() => {
      const tooltips = document.querySelectorAll('.echarts-tooltip, div[style*="absolute"]')
      for (const tt of tooltips) {
        const text = tt.textContent || ''
        if (text.includes('下载') || text.includes('上传')) return true
      }
      return false
    })
    console.log('Has Chinese labels:', hasChinese)
  })
})
