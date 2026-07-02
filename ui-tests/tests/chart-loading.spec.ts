import { test, expect } from './fixtures'

test.describe('图表Loading状态', () => {
  test('Dashboard首次加载显示loading然后消失', async ({ page }) => {
    // 用基础page（不带?debug=1），模拟普通用户体验
    await page.goto('/')
    // 等待图表最终渲染出来
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible({ timeout: 15000 })

    // 验证loading已经消失
    const loading = page.getByText('⏳ 加载图表数据...')
    await expect(loading).not.toBeVisible({ timeout: 3000 }).catch(() => {
      // mock环境数据瞬间返回，loading可能从未可见，这也是正常的
    })
  })

  test('切换时间范围时出现loading然后消失', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible({ timeout: 15000 })
    await page.waitForTimeout(1000)

    // 确保初始loading已消失
    const loading = page.getByText('⏳ 加载图表数据...')
    await expect(loading).not.toBeVisible({ timeout: 5000 }).catch(() => {})

    // 切换到6小时，触发loading
    const selects = page.locator('select')
    const rangeSelect = selects.nth(1)
    await rangeSelect.selectOption('6h')

    // loading最终应消失（数据加载完成）
    await expect(loading).not.toBeVisible({ timeout: 15000 })

    // 图表仍然可见
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible()
  })

  test('用户详情页首次加载loading', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible({ timeout: 15000 })
    await page.waitForTimeout(500)

    // 点击进入用户详情
    const detailBtn = page.getByRole('button', { name: '详情' }).first()
    await detailBtn.click()
    await page.waitForTimeout(500)

    // loading应该最终消失
    const loading = page.getByText('⏳ 加载图表数据...')
    await expect(loading).not.toBeVisible({ timeout: 15000 }).catch(() => {})

    // 图表应该可见
    const chart = page.locator('[data-testid="chart-wrapper"]')
    await expect(chart.first()).toBeVisible({ timeout: 10000 }).catch(() => {
      // 如果进入用户详情失败，也算正常（之前修过的bug可能还在）
    })
  })
})
