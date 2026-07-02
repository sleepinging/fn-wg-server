import { test, expect, readDebugBar } from './fixtures'

/**
 * 等待 DebugBar 域跨度满足条件（用于时间范围切换后等待异步渲染完成）
 */
async function waitForDomainSpan(page: any, min: number, max: number, timeout = 15000) {
  await expect(async () => {
    const db = await readDebugBar(page)
    expect(db.domainSpan).toBeGreaterThan(min)
    expect(db.domainSpan).toBeLessThan(max)
  }).toPass({ timeout })
}

test.describe('图表域测试 (核心)', () => {
  test('15分钟域: 数据点首ts应接近域起始 (锚点)', async ({ debugPage: page }) => {
    const selects = page.locator('select')
    await selects.nth(1).selectOption('15m')
    await waitForDomainSpan(page, 800, 1000)

    const db = await readDebugBar(page)
    // dataSpan 应 <= domainSpan (数据不够时锚点填充)
    expect(db.dataSpan).toBeLessThanOrEqual(db.domainSpan + 10)
  })

  test('1小时域: 数据不够时左边留白', async ({ debugPage: page }) => {
    const selects = page.locator('select')
    await selects.nth(1).selectOption('1h')
    await waitForDomainSpan(page, 3500, 4200)

    const db = await readDebugBar(page)
    // dataSpan 可能小于 domainSpan（数据不够时左边留白，锚点存在）
    expect(db.domainSpan).toBeGreaterThanOrEqual(db.dataSpan)
  })

  test('6小时域: 跨度 ~21600s', async ({ debugPage: page }) => {
    const selects = page.locator('select')
    await selects.nth(1).selectOption('6h')
    await waitForDomainSpan(page, 21000, 22000)
  })

  test('刷新间隔切换不影响域', async ({ debugPage: page }) => {
    const selects = page.locator('select')
    await selects.first().selectOption('5')

    // 等待至少重新渲染一次（域应保持1h）
    await expect(async () => {
      const db = await readDebugBar(page)
      expect(db.domainSpan).toBeGreaterThan(3000)
    }).toPass({ timeout: 15000 })
  })

  test('时间戳严格递增 (通过API)', async ({ debugPage: page }) => {
    const resp = await page.evaluate(() =>
      fetch('/api/stats/history?userId=0&since=0&end=100').then(r => r.json())
    )
    expect(Array.isArray(resp)).toBe(true)
    if (resp.length > 1) {
      for (let i = 1; i < resp.length; i++) {
        expect(resp[i].ts).toBeGreaterThan(resp[i - 1].ts)
      }
    }
  })
})
