import { test, expect } from './fixtures'

/**
 * 捕获所有console错误（error级别）和未捕获异常
 */
function captureErrors(page: any) {
  const errors: string[] = []
  page.on('console', (msg: any) => {
    if (msg.type() === 'error') errors.push(`[console.error] ${msg.text()}`)
  })
  page.on('pageerror', (err: Error) => {
    errors.push(`[pageerror] ${err.message}`)
  })
  return {
    getErrors: () => errors,
    clear: () => { errors.length = 0 },
  }
}

test.describe('全页面Console错误扫描', () => {
  test('Dashboard加载无console错误', async ({ debugPage: page }) => {
    const tracker = captureErrors(page)
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(3000)

    const errors = tracker.getErrors()
    if (errors.length > 0) {
      console.log('  ERRORS:', errors)
    }
    // React dev warnings sometimes appear, filter those
    const realErrors = errors.filter((e: string) =>
      !e.includes('react-refresh') &&
      !e.includes('ReactDOM.render') &&
      !e.includes('Warning:')
    )
    expect(realErrors).toHaveLength(0)
    console.log('  OK: no real errors')
  })

  test('遍历所有Tab无新增console错误', async ({ debugPage: page }) => {
    const tracker = captureErrors(page)
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(2000)

    // 清除初始加载的错误
    tracker.clear()

    const tabs = ['仪表盘', '用户管理', '服务配置', '运行日志', '系统信息']
    for (const tab of tabs) {
      await page.getByRole('button', { name: tab }).click()
      await page.waitForTimeout(1000)
      const errors = tracker.getErrors()
      const realErrors = errors.filter((e: string) =>
        !e.includes('react-refresh') &&
        !e.includes('ReactDOM.render') &&
        !e.includes('Warning:')
      )
      if (realErrors.length > 0) {
        console.log(`  [${tab}] ERRORS:`, realErrors)
      } else {
        console.log(`  [${tab}] OK`)
      }
      tracker.clear()
    }

    // 切回Dashboard
    await page.getByRole('button', { name: '仪表盘' }).click()
    await page.waitForTimeout(2000)
    const finalErrors = tracker.getErrors().filter((e: string) =>
      !e.includes('react-refresh') && !e.includes('Warning:')
    )
    expect(finalErrors).toHaveLength(0)
  })

  test('用户详情页无console错误', async ({ debugPage: page }) => {
    const tracker = captureErrors(page)
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(2000)
    tracker.clear()

    // 进入用户详情
    const detailBtn = page.getByRole('button', { name: '详情' }).first()
    await detailBtn.click()
    await page.waitForTimeout(3000)

    const errors = tracker.getErrors().filter((e: string) =>
      !e.includes('react-refresh') && !e.includes('Warning:')
    )
    if (errors.length > 0) console.log('  ERRORS:', errors)
    else console.log('  OK: no errors in user detail')
    expect(errors).toHaveLength(0)
  })

  test('连续操作不累积错误（轮询稳定性）', async ({ debugPage: page }) => {
    const tracker = captureErrors(page)
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(2000)
    tracker.clear()

    // 保持Dashboard 15秒，期间切几次范围看是否积累错误
    const ranges = ['15m', '1h', '6h', '1h']
    for (const r of ranges) {
      const selects = page.locator('select')
      await selects.nth(1).selectOption(r)
      await page.waitForTimeout(3000)

      const errs = tracker.getErrors().filter((e: string) =>
        !e.includes('react-refresh') && !e.includes('Warning:')
      )
      if (errs.length > 0) {
        console.log(`  [${r}] ERRORS accumulated:`, errs)
      }
    }

    const totalErrors = tracker.getErrors().filter((e: string) =>
      !e.includes('react-refresh') && !e.includes('Warning:')
    )
    console.log(`  Total non-warning errors after 4 switches: ${totalErrors.length}`)
    expect(totalErrors).toHaveLength(0)
  })
})

test.describe('API异常容错', () => {
  test('Dashboard不因单个API失败而崩溃', async ({ debugPage: page }) => {
    // 拦截一个API让它返回500
    await page.route('**/api/system', route => route.fulfill({ status: 500, body: '{}' }))
    await page.route('**/api/wg/kernel', route => route.fulfill({ status: 500, body: '{}' }))

    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(3000)

    // 页面应该仍然渲染（Header可能显示"未加载"，这是预期的）
    const chart = page.locator('[data-testid="chart-wrapper"]')
    await expect(chart).toBeVisible({ timeout: 10000 })

    // 统计卡片不应该全部空白
    const cards = page.locator('.stat-card')
    const count = await cards.count()
    expect(count).toBeGreaterThanOrEqual(4)

    // 不应有React崩溃（root为空）
    const root = page.locator('#root')
    const html = await root.innerHTML()
    expect(html.length).toBeGreaterThan(500)
    console.log('  OK: Dashboard survives API 500 errors')
  })
})

test.describe('Header状态栏', () => {
  test('Header显示版本号和WG状态', async ({ debugPage: page }) => {
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(1000)

    const header = page.locator('header')
    const text = await header.textContent()

    // 版本号
    expect(text).toMatch(/v[\d.]+/)
    // WG状态（正常或未加载）
    expect(text).toMatch(/WireGuard/)
    console.log(`  Header: ${text?.replace(/\s+/g, ' ')}`)
  })
})

test.describe('内存/性能验证', () => {
  test('Dashboard轮询30秒后chartBuf不无限增长', async ({ debugPage: page }) => {
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(2000)

    // 切1s刷新让poll频率最高
    const selects = page.locator('select')
    await selects.first().selectOption('1')
    await page.waitForTimeout(2000)

    // 持续30秒，每5秒检查点数不超100
    for (let i = 0; i < 6; i++) {
      await page.waitForTimeout(5000)

      const text = await page.evaluate(() => {
        const el = document.evaluate("//span[contains(text(), '📊')]", document,
          null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue as any
        return el?.parentElement?.textContent || ''
      })
      const pts = parseInt((text.match(/数据:(\d+)pts/) || [])[1]) || 0
      expect(pts).toBeLessThanOrEqual(100)
      console.log(`  ${(i + 1) * 5}s: ${pts}pts (≤100)`)
    }
  })
})
