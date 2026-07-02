import { test, expect, readDebugBar } from './fixtures'

test.describe('图表长时间稳定性 (1分钟)', () => {
  test('持续切换时间范围 + 验证数据不越界', async ({ debugPage: page }) => {
    test.setTimeout(120_000)

    const ranges = ['15m', '1h', '6h', '1h', '15m', '6h']
    const domainExpected: Record<string, [number, number]> = {
      '15m': [800, 1000],
      '1h': [3500, 4200],
      '6h': [21000, 22000],
    }

    for (let cycle = 0; cycle < 6; cycle++) {
      for (const range of ranges) {
        const selects = page.locator('select')
        await selects.nth(1).selectOption(range)

        const [min, max] = domainExpected[range]
        await expect(async () => {
          const db = await readDebugBar(page)
          expect(db.domainSpan).toBeGreaterThan(min)
          expect(db.domainSpan).toBeLessThan(max)
        }).toPass({ timeout: 15000 })

        const db = await readDebugBar(page)
        expect(db.points).toBeLessThanOrEqual(100)
        if (db.points > 1) {
          expect(db.dataSpan).toBeGreaterThan(0)
        }
        console.log(`  cycle${cycle+1} ${range}: pts=${db.points} domain=${db.domainSpan}s dataSpan=${db.dataSpan}s first=${db.firstTime} last=${db.lastTime}`)
        await page.waitForTimeout(1500)
      }
    }
  })

  test('固定1h持续观察1分钟：域/锚点/时序不退化', async ({ debugPage: page }) => {
    test.setTimeout(120_000)

    const selects = page.locator('select')
    await selects.nth(1).selectOption('1h')
    await selects.first().selectOption('1')
    await page.waitForTimeout(3000)

    const start = Date.now()
    let lastDetail = ''

    while (Date.now() - start < 60_000) {
      await page.waitForTimeout(2000)

      // 从DOM直接读取完整的DebugBar信息
      const detail = await page.evaluate(() => {
        const el = document.evaluate(
          "//span[contains(text(), '📊')]",
          document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null
        ).singleNodeValue as HTMLSpanElement | null;
        if (!el?.parentElement) return 'NO_DEBUGBAR';
        const spans = el.parentElement.querySelectorAll('span');
        return Array.from(spans).map(s => s.textContent).join(' | ');
      });

      const db = await readDebugBar(page)

      // 1. 数据点不超100
      expect(db.points).toBeLessThanOrEqual(100)

      // 2. 域跨度保持1h
      expect(db.domainSpan).toBeGreaterThan(3500)
      expect(db.domainSpan).toBeLessThan(4200)

      // 3. 有数据时首尾时间合法
      if (db.points > 0) {
        expect(db.firstTime).not.toBe('-')
        expect(db.lastTime).not.toBe('-')
      }

      // 4. dataSpan ≤ domainSpan (数据不能超出域范围)
      if (db.dataSpan > 0) {
        expect(db.dataSpan).toBeLessThanOrEqual(db.domainSpan + 10)
      }

      const elapsed = Math.round((Date.now() - start) / 1000)
      if (elapsed % 10 === 0 || detail !== lastDetail) {
        console.log(`  ${elapsed}s: ${detail}`)
        lastDetail = detail
      }
    }
  })

  test('完整DOM快照：确认图表X轴时间标签正常', async ({ debugPage: page }) => {
    // 分别切到 15m / 1h / 6h，抓取X轴标签
    const ranges = [
      { val: '15m', expectedHours: 0.25 },
      { val: '1h', expectedHours: 1 },
      { val: '6h', expectedHours: 6 },
    ]

    for (const r of ranges) {
      const selects = page.locator('select')
      await selects.nth(1).selectOption(r.val)
      await page.waitForTimeout(3000)

      // 读取X轴上的时间标签
      const xLabels = await page.evaluate(() => {
        const svg = document.querySelector('[data-testid="chart-wrapper"] svg')
        if (!svg) return []
        const texts = svg.querySelectorAll('text')
        return Array.from(texts)
          .map(t => t.textContent || '')
          .filter(t => /^\d{1,2}:\d{2}/.test(t))
      });

      console.log(`  [${r.val}] X labels: ${xLabels.join(', ')}`)

      // 时间标签不应为空，数量≥2
      expect(xLabels.length).toBeGreaterThanOrEqual(2)

      // 每个标签应该是合法时间格式 (HH:MM:SS 或类似)
      for (const label of xLabels) {
        expect(label).toMatch(/^\d{1,2}:/)
      }
    }
  })

  test('切换范围后图表区域始终可见无白屏', async ({ debugPage: page }) => {
    test.setTimeout(120_000)

    const ranges = ['15m', '1h', '6h', '24h', '7d', '1h', '15m']

    for (let i = 0; i < ranges.length; i++) {
      const selects = page.locator('select')
      await selects.nth(1).selectOption(ranges[i])

      // 等待loading消失 + 数据就绪
      await page.waitForTimeout(2000)

      // 图表容器必须始终可见
      const chart = page.locator('[data-testid="chart-wrapper"]')
      await expect(chart).toBeVisible({ timeout: 5000 })

      // 不能有白屏 (root内应该有内容)
      const rootHasContent = await page.evaluate(() => {
        const root = document.getElementById('root');
        return root ? root.innerHTML.length > 100 : false;
      });
      expect(rootHasContent).toBe(true)

      // Loading overlay应该已消失
      const loading = page.getByText('⏳ 加载图表数据...')
      const loadingVisible = await loading.isVisible().catch(() => false)
      expect(loadingVisible).toBe(false)

      console.log(`  [${ranges[i]}] OK: chart visible, no loading, no blank`)
    }
  })
})
