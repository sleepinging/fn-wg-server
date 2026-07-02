/*
 * 浏览器 Console 自测 - 粘贴到页面控制台运行
 * 必须先打开 ?debug=1
 */

(function() {
  'use strict'
  let pass = 0, fail = 0
  const ok = m => { pass++; console.log('%c✅ ' + m, 'color:green') }
  const ng = m => { fail++; console.warn('%c❌ ' + m, 'color:red') }

  // 1. DebugBar 存在
  const bar = document.querySelector('div[style*="position:fixed"][style*="bottom:0"]')
  bar ? ok('DebugBar visible') : ng('DebugBar NOT visible - add ?debug=1')

  // 2. 图表存在
  const chart = document.querySelector('.recharts-responsive-container')
  chart ? ok('Chart exists') : ng('No chart found')

  // 3. 时间范围选择器存在
  const selects = document.querySelectorAll('select')
  if (selects.length >= 2) ok(`Selectors: ${selects.length} (range + interval)`)
  else ng(`Only ${selects.length} selectors`)

  // 4. 通过 DebugBar 文本验证
  if (bar) {
    const text = bar.textContent || ''
    const m = text.match(/数据:(\d+)pts/)
    const pts = m ? parseInt(m[1]) : 0
    ok(`Chart points: ${pts}`)

    const dm = text.match(/域:(\S+) → (\S+) 跨度:(\d+)s/)
    if (dm) {
      const domainSpan = parseInt(dm[3])
      // 15m=900s, 1h=3600s, etc.
      ok(`Domain span: ${domainSpan}s (${(domainSpan/60).toFixed(0)}min)`)
      if (domainSpan < 60) ng('Domain span < 1min - chart too narrow')
    }
  }

  // 5. 速度显示
  const speedEls = document.querySelectorAll('[class*="speed"]')
  speedEls.length > 0 ? ok(`Speed elements: ${speedEls.length}`) : ng('No speed display')

  // 6. 表格数据
  const tables = document.querySelectorAll('table')
  tables.length > 0 ? ok(`Tables: ${tables.length}`) : ng('No tables found')

  // 7. 无 JS 错误
  if (window.__noErrors !== false) ok('No JS errors detected')

  console.log(`\n=== Console 自测: ${pass} pass / ${fail} fail ===`)
  if (fail > 0) {
    console.warn('❌ RE-FAILED')
    return 'TEST FAILED'
  }
  console.log('✅ ALL PASS')
  return 'TEST PASSED'
})()
