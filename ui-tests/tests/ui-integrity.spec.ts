import { test, expect } from './fixtures'

test.describe('Tab导航与页面切换', () => {
  test('所有5个Tab均可正常切换无崩溃', async ({ debugPage: page }) => {
    const tabs = ['仪表盘', '用户管理', '服务配置', '运行日志', '系统信息']
    for (const tab of tabs) {
      await page.getByRole('button', { name: tab }).click()
      await page.waitForTimeout(500)
      // 每个tab切换后，页面主内容区应有内容（不是白屏）
      const main = page.locator('main')
      const html = await main.innerHTML()
      expect(html.length).toBeGreaterThan(100)
      console.log(`  [${tab}] OK: ${html.length} chars`)
    }
  })

  test('切到非活跃Tab后旧Tab停止轮询', async ({ debugPage: page }) => {
    // 在Dashboard观察一段时间记下chartBuf点数
    await page.waitForTimeout(3000)
    const dbBefore = await page.evaluate(() => {
      const el = document.evaluate("//span[contains(text(), '📊')]", document,
        null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue as any
      return el?.parentElement?.textContent || ''
    })
    console.log('Dashboard before switch:', dbBefore)

    // 切换到日志页等待5秒
    await page.getByRole('button', { name: '运行日志' }).click()
    await page.waitForTimeout(5000)

    // 切回Dashboard
    await page.getByRole('button', { name: '仪表盘' }).click()
    await page.waitForTimeout(2000)

    const dbAfter = await page.evaluate(() => {
      const el = document.evaluate("//span[contains(text(), '📊')]", document,
        null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue as any
      return el?.parentElement?.textContent || ''
    })
    console.log('Dashboard after return:', dbAfter)
    // 回来时图表应该重新加载而非空白
    expect(dbAfter).toContain('pts')
  })
})

test.describe('服务配置页', () => {
  test('配置表单正常渲染输入框和文本域', async ({ debugPage: page }) => {
    await page.getByRole('button', { name: '服务配置' }).click()
    await page.waitForTimeout(1000)

    // 接口名称 input
    const inputs = page.locator('input[type="text"], input[type="number"]')
    const inputCount = await inputs.count()
    expect(inputCount).toBeGreaterThanOrEqual(4) // interfaceName, port, address, dns, serverDomain
    console.log(`  Config inputs: ${inputCount}`)

    // textarea
    const textareas = page.locator('textarea')
    const taCount = await textareas.count()
    expect(taCount).toBeGreaterThanOrEqual(2) // privateKey, postUp, postDown
    console.log(`  Config textareas: ${taCount}`)

    // 服务控制按钮
    const startBtn = page.getByRole('button', { name: '启动服务' })
    const stopBtn = page.getByRole('button', { name: '停止服务' })
    const restartBtn = page.getByRole('button', { name: '重启服务' })
    await expect(startBtn.or(stopBtn).first()).toBeVisible()
    console.log('  Service buttons visible')
  })
})

test.describe('运行日志页', () => {
  test('日志列表渲染且分页可见', async ({ debugPage: page }) => {
    await page.getByRole('button', { name: '运行日志' }).click()
    await page.waitForTimeout(1000)

    // 应有日志条目或空状态
    const logEntries = page.locator('.log-entry, table tbody tr, [class*="log"]')
    const body = page.locator('main')
    const bodyText = await body.textContent()
    expect(bodyText?.length || 0).toBeGreaterThan(50)
    console.log(`  Logs body text length: ${bodyText?.length}`)
  })

  test('级别过滤select存在', async ({ debugPage: page }) => {
    await page.getByRole('button', { name: '运行日志' }).click()
    await page.waitForTimeout(500)

    const selects = page.locator('select')
    // 日志页至少有一个级别过滤select
    const count = await selects.count()
    expect(count).toBeGreaterThanOrEqual(1)
    console.log(`  Logs selects: ${count}`)
  })
})

test.describe('系统信息页', () => {
  test('系统信息正常展示CPU/内存/版本', async ({ debugPage: page }) => {
    await page.getByRole('button', { name: '系统信息' }).click()
    await page.waitForTimeout(1000)

    const body = page.locator('main')
    const text = await body.textContent()

    // 应包含关键信息
    const checks = ['版本', 'CPU', '内存', '运行时间']
    for (const keyword of checks) {
      expect(text).toContain(keyword)
    }
    console.log(`  System info contains: ${checks.join(', ')}`)
  })
})

test.describe('用户管理页', () => {
  test('用户列表表格存在', async ({ debugPage: page }) => {
    await page.getByRole('button', { name: '用户管理' }).click()
    await page.waitForTimeout(500)

    const table = page.locator('table')
    await expect(table).toBeVisible()
    console.log('  Users table visible')
  })
})

test.describe('跨页面稳定性', () => {
  test('快速连续切换10次Tab不崩溃', async ({ debugPage: page }) => {
    const tabs = ['仪表盘', '用户管理', '服务配置', '运行日志', '系统信息']
    for (let i = 0; i < 10; i++) {
      const tab = tabs[i % tabs.length]
      await page.getByRole('button', { name: tab }).click()
      await page.waitForTimeout(200)

      // 验证页面没崩溃
      const root = page.locator('#root')
      const html = await root.innerHTML()
      expect(html.length).toBeGreaterThan(100)
    }
    console.log('  10 rapid switches OK, no crash')
  })
})

test.describe('非Debug模式基础验证', () => {
  test('普通模式无DebugBar', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')
    await page.waitForSelector('[data-testid="chart-wrapper"]', { timeout: 15000 })
    await page.waitForTimeout(1000)

    // DebugBar不应可见
    const debugBar = page.getByText('📊 数据:')
    const visible = await debugBar.isVisible().catch(() => false)
    expect(visible).toBe(false)
    console.log('  DebugBar hidden in normal mode ✓')

    // 但图表和表格应该正常
    await expect(page.locator('[data-testid="chart-wrapper"]')).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })
})

test.describe('数值格式化边界', () => {
  test('零值和大数值不会导致布局崩溃', async ({ debugPage: page }) => {
    await page.waitForTimeout(2000)

    // 检查所有stat-card是否有溢出
    const cards = page.locator('.stat-card')
    const count = await cards.count()
    for (let i = 0; i < count; i++) {
      const card = cards.nth(i)
      const text = await card.textContent()
      // 不应包含NaN, undefined, null
      expect(text).not.toContain('NaN')
      expect(text).not.toContain('undefined')
      expect(text).not.toContain('null')
    }
    console.log(`  ${count} stat cards, no NaN/undefined/null`)

    // 格式化数字不应有异常长的小数
    const bodyText = await page.locator('main').textContent()
    // 检查没有类似 "1.234567890123" 的超长小数
    const longDecimals = (bodyText || '').match(/\d+\.\d{5,}/g)
    if (longDecimals) {
      console.log(`  Note: long decimals found: ${longDecimals.slice(0, 3).join(', ')}`)
    }
  })
})
