// 16 §2.13, 18 §2: real 450-Mod texture page measurements (no conversion).
// npm install --prefix tmp/tex-perf --no-package-lock playwright-core@1.58.2
// node spikes/guidev/textureperf.cjs <guidev.exe> <home> <output.json>
const fs = require('fs'), path = require('path'), { spawn } = require('child_process');
const { chromium } = require(path.resolve('tmp/tex-perf/node_modules/playwright-core'));
const [exe, home, output] = process.argv.slice(2);
(async () => {
  let browser, service;
  const port = 18793, origin = `http://127.0.0.1:${port}`, errors = [];
  try {
    service = spawn(path.resolve(exe), ['-home', path.resolve(home), '-addr', `127.0.0.1:${port}`], { windowsHide: true });
    service.stderr.on('data', d => process.stderr.write(d));
    browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true });
    const page = await browser.newPage({ viewport: { width: 1600, height: 1100 } });
    page.on('pageerror', e => errors.push(e.message));
    await page.addInitScript(() => {
      window.textureEvents = 0;
      const Source = window.EventSource;
      window.EventSource = class extends Source {
        constructor(...args) { super(...args); this.addEventListener('message', e => { if (JSON.parse(e.data).name === 'textures') window.textureEvents++; }); }
      };
    });
    for (let i = 0; i < 100; i++) { try { if ((await fetch(origin)).ok) break; } catch {} await new Promise(r => setTimeout(r, 100)); }
    const ready = page.waitForResponse(r => r.url().endsWith('/call') && r.request().postDataJSON()?.m === 'Ready', {timeout:120000}); await page.goto(origin); await ready;
    await page.waitForFunction(async () => { const { S } = await import('/js/state.js'); return S.board && !S.busy.env; }, null, { timeout: 120000 });
    const cdp = await page.context().newCDPSession(page); await cdp.send('Performance.enable');
    const runs = [];
    for (let i = 0; i < 3; i++) {
      await page.evaluate(async () => { const s = await import('/js/screens.js'); s.openScreen('plans'); const {api}=await import('/js/state.js'); await api.Leave('textures'); });
      const before = await cdp.send('Performance.getMetrics');
      await page.evaluate(async () => {
        const tex = await import('/js/textures.js');
        const p = window.texPerf = { start: performance.now(), calls: [], longTasks: [], created: 0, events: 0, eventStart: window.textureEvents };
        const origCreate = document.createElement.bind(document);
        document.createElement = (...args) => { p.created++; return origCreate(...args); };
        const origFetch = window.fetch;
        window.fetch = async (...args) => {
          const start = performance.now(), res = await origFetch(...args);
          if (String(args[0]) === '/call') {
            const req = JSON.parse(args[1].body);
            if (['Textures', 'TextureMeasure'].includes(req.m)) {
              const j = await res.clone().json(), v = j.result;
              p.calls.push({ method: req.m, start: start - p.start, ms: performance.now() - start, rows: v?.rows?.length || 0, pending: v?.rows?.filter(r => r.pending).length || 0, loading: v?.loading });
            }
          }
          return res;
        };
        const long = new PerformanceObserver(ls => p.longTasks.push(...ls.getEntries().map(e => e.duration))); long.observe({ type: 'longtask' });
        const observer = new MutationObserver(() => {
          const rows = document.querySelectorAll('.tex-rows .data-row');
          if (rows.length && !p.listMs) p.listMs = performance.now() - p.start;
          if (rows.length && document.querySelector('.textures')?.getAttribute('aria-busy') === 'false' && !p.doneMs) {
            p.doneMs = performance.now() - p.start;
            requestAnimationFrame(() => requestAnimationFrame(() => { p.paintedMs = performance.now() - p.start; }));
          }
        }); observer.observe(document.querySelector('#screen'), { childList: true, subtree: true, attributes: true });
        p.restore = () => { observer.disconnect(); long.disconnect(); document.createElement = origCreate; window.fetch = origFetch; };
        await tex.open();
        // the page opens without reading textures: count the textures starts the count
        [...document.querySelectorAll('#screen .tex-foot button')].find(b => !b.disabled && b !== document.querySelector('#screen .tex-foot button:last-child'))?.click();
      });
      await page.waitForFunction(() => window.texPerf.paintedMs, null, { timeout: 600000 }).catch(async e => { fs.writeFileSync(output, JSON.stringify(await page.evaluate(() => ({screen:document.querySelector('#screen').textContent.slice(0,200),p:{...window.texPerf,restore:undefined}})),null,2)); throw e; });
      const after = await cdp.send('Performance.getMetrics');
      const result = await page.evaluate(() => { const p = window.texPerf; p.events = window.textureEvents - p.eventStart; p.restore(); const { restore, ...data } = p; return data; });
      const metrics = obj => Object.fromEntries(obj.metrics.map(x => [x.name, x.value]));
      const a = metrics(after), b = metrics(before);
      result.metrics = Object.fromEntries(['ScriptDuration', 'LayoutDuration', 'RecalcStyleDuration', 'TaskDuration'].map(k => [k, (a[k] - b[k]) * 1000]));
      runs.push(result); console.log(JSON.stringify({ run: i, listMs: result.listMs, paintedMs: result.paintedMs,
        calls: result.calls.length, events: result.events, created: result.created, longTasks: result.longTasks.length,
        maxTask: Math.max(0, ...result.longTasks), metrics: result.metrics }));
    }
    // Exercise the actual table and stale request guards after timing is complete.
    const scale = page.locator('[data-f="scale"]'), scope = page.locator('[data-f="scope"]');
    await scale.selectOption('50'); await scale.selectOption('100');
    await scope.selectOption('all'); await scope.selectOption('plan');
    await page.waitForFunction(() => document.querySelector('.textures')?.getAttribute('aria-busy') === 'false', null, {timeout:120000});
    const name = page.locator('.tex-rows .th[data-k="name"]'); await name.click();
    if (await name.getAttribute('aria-sort') === 'none') throw Error('name sort failed');
    await page.locator('.tex-rows .data-row').last().scrollIntoViewIfNeeded();
    const oldColumns = await page.locator('.tex-rows').evaluate(e => e.style.getPropertyValue('--lib-cols'));
    await page.locator('.tex-rows').focus(); await page.keyboard.press('>');
    const newColumns = await page.locator('.tex-rows').evaluate(e => e.style.getPropertyValue('--lib-cols'));
    if (oldColumns === newColumns) throw Error('column resize failed');
    await page.keyboard.press('=');
    fs.writeFileSync(output, JSON.stringify({ runs, errors, interactions: 'scope, scale, sort, scroll, column width passed' }, null, 2));
    if (errors.length) throw Error(errors.join('\n'));
  } finally {
    if (browser) await browser.close();
    if (service) { service.kill(); await new Promise(r => service.once('exit', r)); }
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
