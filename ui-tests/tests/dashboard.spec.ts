import { test, expect } from './fixtures'

test.describe('Dashboard (概览)', () => {
  test('页面加载正常', async ({ debugPage: page }) => {
    await expect(page.locator('h1')).toContainText('WireGuard 服务端管理')
    await expect(page.getByRole('button', { name: '仪表盘' })).toBeVisible()
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })

  test('DebugBar 可见且数据正确', async ({ debugPage: page }) => {
    const bar = page.getByText('📊 数据:')
    await expect(bar).toBeVisible({ timeout: 5000 })
    const ptsText = await bar.textContent()
    expect(ptsText).toMatch(/数据:(\d+)pts/)
  })

  test('默认 1h 域跨度 ~3600s', async ({ debugPage: page }) => {
    await expect(async () => {
      const text = await page.locator('span').filter({ hasText: '🎯 域:' }).textContent()
      const m = text?.match(/跨度:(\d+)s/)
      const span = m ? parseInt(m[1]) : 0
      expect(span).toBeGreaterThan(3000)
      expect(span).toBeLessThan(4200)
    }).toPass({ timeout: 10000 })
  })

  test('切换到 15分钟 域跨度 ~900s', async ({ debugPage: page }) => {
    const rangeSelect = page.locator('select').nth(1)
    await rangeSelect.selectOption('15m')

    // 等待域跨度更新为 900s 左右
    await expect(async () => {
      const text = await page.locator('span').filter({ hasText: '🎯 域:' }).textContent()
      const m = text?.match(/跨度:(\d+)s/)
      const span = m ? parseInt(m[1]) : 0
      expect(span).toBeGreaterThan(800)
      expect(span).toBeLessThan(1000)
    }).toPass({ timeout: 15000 })
  })

  test('速度信息可见', async ({ debugPage: page }) => {
    await expect(page.getByText('总下载')).toBeVisible()
    await expect(page.getByText('总上传')).toBeVisible()
  })
})
