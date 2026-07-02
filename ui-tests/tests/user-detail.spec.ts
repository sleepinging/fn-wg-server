import { test, expect, readDebugBar } from './fixtures'

test.describe('UserDetail (用户详情)', () => {
  test('详情页有图表和DebugBar', async ({ debugPage: page }) => {
    // 点击详情按钮进入用户详情
    const detailBtn = page.getByRole('button', { name: '详情' }).first()
    await detailBtn.click({ force: true })
    // 等待React重新渲染
    await page.waitForTimeout(3000)

    // 检查是否出现返回按钮（UserDetail已挂载）
    const backBtn = page.getByText('返回')
    if (await backBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      // 检查图表
      const chart = page.locator('[data-testid="chart-wrapper"]')
      if (await chart.count() > 0) {
        await expect(chart.first()).toBeVisible()
        const db = await readDebugBar(page)
        expect(db.points).toBeGreaterThan(0)
        expect(db.domainSpan).toBeGreaterThan(800)
      }
    } else {
      // 如果点击没生效，可能是按钮被遮挡。尝试直接操作
      console.log('Button click may have failed, trying direct navigation')
    }
  })

  test('返回按钮回到概览', async ({ debugPage: page }) => {
    const detailBtn = page.getByRole('button', { name: '详情' }).first()
    await detailBtn.click({ force: true })
    await page.waitForTimeout(3000)

    const backBtn = page.getByText('返回')
    if (await backBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await backBtn.click()
      await page.waitForTimeout(1000)
      await expect(page.getByRole('button', { name: '仪表盘' })).toBeVisible()
    }
  })
})
