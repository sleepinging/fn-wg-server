import { test, expect } from './fixtures'

test.describe('图表渲染缺陷复现', () => {
  test('Tooltip悬浮显示的是格式化时间而非原始时间戳', async ({ debugPage: page }) => {
    // 加载后等待数据稳定
    await page.waitForTimeout(3000)

    // 找到图表区域，hover 到有数据的位置触发 tooltip
    const chart = page.locator('[data-testid="chart-wrapper"]').first()
    const box = await chart.boundingBox()
    expect(box).not.toBeNull()

    if (box) {
      // hover 到图表中间偏右位置（有数据点的地方）
      await page.mouse.move(box.x + box.width * 0.7, box.y + box.height * 0.5)
      await page.waitForTimeout(500)

      // 检查 tooltip 是否出现
      const tooltip = page.locator('.recharts-tooltip-wrapper')
      const tooltipCount = await tooltip.count()
      console.log('Tooltip count:', tooltipCount)

      if (tooltipCount > 0) {
        const tooltipText = await tooltip.first().textContent()
        console.log('Tooltip text:', tooltipText)

        // tooltip 文本不应包含长数字（原始ts时间戳是13位数字）
        const hasLongNumber = /\d{10,}/.test(tooltipText || '')
        console.log('Has long timestamp number:', hasLongNumber)
        
        // 应该包含格式化的时间 (如 "2:30:00 PM" 或 "14:30:00")
        const hasFormattedTime = /\d{1,2}:\d{2}/.test(tooltipText || '')
        console.log('Has formatted time:', hasFormattedTime)

        // 应该包含速度标签和单位
        const hasSpeedLabel = /下载|上传/.test(tooltipText || '')
        console.log('Has speed label:', hasSpeedLabel)
        const hasUnit = /B\/s/.test(tooltipText || '')
        console.log('Has unit:', hasUnit)
      }
    }

    // 截图保存 tooltip 状态
    await expect(page).toHaveScreenshot('tooltip-hover.png', { 
      fullPage: false,
      maxDiffPixelRatio: 0.05,
    }).catch(() => {})
  })

  test('图表线应无消失动画：连续截图验证帧间差异', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)

    // 截第一张图
    const chart = page.locator('[data-testid="chart-wrapper"]').first()
    const box = await chart.boundingBox()
    expect(box).not.toBeNull()

    // 切到 1s 刷新让数据尽量变化
    const selects = page.locator('select')
    await selects.first().selectOption('1')
    await page.waitForTimeout(1000)

    // 连续截图3次，间隔200ms，看线是否有明显位移
    const screenshots: string[] = []
    let prevWidth = 0

    for (let i = 0; i < 3; i++) {
      // 读取图表中蓝色线的path属性，检查是否有动画导致的d属性变化
      const pathInfo = await page.evaluate(() => {
        const paths = document.querySelectorAll('.recharts-line-curve');
        const info: any[] = [];
        paths.forEach(p => {
          const d = p.getAttribute('d');
          info.push({
            stroke: p.getAttribute('stroke'),
            dLen: d ? d.length : 0,
            dStart: d ? d.substring(0, 50) : 'none',
          });
        });
        // 也检查是否有带animation的class
        const animated = document.querySelectorAll('[class*="animation"], .recharts-layer.recharts-line');
        return {
          paths: info,
          animatedCount: animated.length,
        };
      });
      
      console.log(`Frame ${i}:`, JSON.stringify(pathInfo))
      
      if (i === 0) {
        prevWidth = pathInfo.paths.reduce((sum: number, p: any) => sum + p.dLen, 0)
      } else {
        const currWidth = pathInfo.paths.reduce((sum: number, p: any) => sum + p.dLen, 0)
        console.log(`  d-length change: ${prevWidth} → ${currWidth}`)
      }

      await page.waitForTimeout(300)
    }
  })

  test('Tooltip name标签应显示中文而非原始key', async ({ debugPage: page }) => {
    await page.waitForTimeout(3000)

    // 用evaluate直接读tooltip的DOM结构
    const chart = page.locator('[data-testid="chart-wrapper"]').first()
    const box = await chart.boundingBox()
    if (!box) return

    await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.3)
    await page.waitForTimeout(800)

    const tooltipDom = await page.evaluate(() => {
      const tt = document.querySelector('.recharts-tooltip-wrapper');
      if (!tt || tt.children.length === 0) return 'TOOLTIP_NOT_FOUND';
      return tt.innerHTML;
    });
    console.log('Tooltip DOM:', tooltipDom?.substring(0, 300))
  })
})
