import { test as base, Page } from '@playwright/test'

// 自定义 fixtures
export const test = base.extend<{
  debugPage: Page
}>({
  debugPage: async ({ page }, use) => {
    // 所有测试页面都带 ?debug=1
    await page.goto('/?debug=1')
    await page.waitForLoadState('networkidle')
    // 等待图表渲染
    await page.waitForSelector('canvas, svg', { timeout: 15000 })
    await page.waitForTimeout(2000) // 等待动画/数据加载
    await use(page)
  },
})

/**
 * 从 DebugBar 读取实时数据
 * DebugBar 渲染 3 个 span:
 *   <span>📊 数据:84pts</span>
 *   <span>⏱ 首:13:04:47 尾:13:37:25 跨度:1958s</span>
 *   <span>🎯 域:13:04:26 → 14:04:26 跨度:3600s</span>
 */
export async function readDebugBar(page: Page) {
  const raw = await page.evaluate(() => {
    // 找到包含 📊 的 span，上溯到父 div
    const el = document.evaluate(
      "//span[contains(text(), '📊')]",
      document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null
    ).singleNodeValue as HTMLSpanElement | null;
    if (!el || !el.parentElement) return null;
    const spans = el.parentElement.querySelectorAll('span');
    return Array.from(spans).map(s => s.textContent || '');
  });

  if (!raw || raw.length < 3) {
    return { points: 0, firstTime: '', lastTime: '', dataSpan: 0, domainSpan: 0 };
  }

  // span[0]: "📊 数据:84pts"
  const pts = (raw[0].match(/(\d+)pts/) || [])[1];

  // span[1]: "⏱ 首:13:04:47 尾:13:37:25 跨度:1958s"
  const firstTime = (raw[1].match(/首:(\S+)/) || [])[1];
  const lastTime = (raw[1].match(/尾:(\S+)/) || [])[1];
  const dataSpan = (raw[1].match(/跨度:(\d+)s/) || [])[1];

  // span[2]: "🎯 域:13:04:26 → 14:04:26 跨度:3600s"
  const domainSpan = (raw[2].match(/跨度:(\d+)s/) || [])[1];

  return {
    points: pts ? parseInt(pts) : 0,
    firstTime: firstTime || '',
    lastTime: lastTime || '',
    dataSpan: dataSpan ? parseInt(dataSpan) : 0,
    domainSpan: domainSpan ? parseInt(domainSpan) : 0,
  };
}

export { expect } from '@playwright/test'
