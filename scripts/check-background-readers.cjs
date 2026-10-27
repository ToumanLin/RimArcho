// Real guidev pages, measured from headless Chrome. Keep the manifest, profiles,
// output and database copies outside git. Only --execute performs a removal;
// the caller must back up and restore every explicitly listed mod.
// node scripts/check-background-readers.cjs <origin> <manifest.json> <output.json> [--execute]
const {spawn} = require('node:child_process');
const fs = require('node:fs'), path = require('node:path'), net = require('node:net');
const [origin, manifest, output] = process.argv.slice(2);
if (!origin || !manifest || !output) throw Error('Expected origin, manifest and output');
const mods = JSON.parse(fs.readFileSync(manifest, 'utf8'));
if (!mods.length || mods.some(m => !m.id || !/^\d+$/.test(m.workshop))) throw Error('Invalid manifest');
const execute = process.argv.includes('--execute');
const sleep = ms => new Promise(r => setTimeout(r, ms));
async function port() {
  const server = net.createServer();
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const p = server.address().port;
  await new Promise(r => server.close(r));
  return p;
}
(async () => {
  let chrome, ws;
  try {
    const debug = await port();
    chrome = spawn(process.env.CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe',
      ['--headless=new', '--no-first-run', '--disable-sync', '--disable-background-networking',
       '--disable-component-update', '--remote-debugging-port=' + debug,
       '--user-data-dir=' + path.resolve(path.dirname(output), 'chrome-' + path.basename(output, '.json')), 'about:blank'],
      {windowsHide: true, stdio: 'ignore'});
    let targets;
    for (let i = 0; i < 100; i++) {
      try { targets = await (await fetch('http://127.0.0.1:' + debug + '/json')).json(); break; } catch {}
      await sleep(100);
    }
    if (!targets) throw Error('Chrome did not start');
    ws = new WebSocket(targets.find(t => t.type === 'page').webSocketDebuggerUrl);
    await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
    let serial = 0;
    const pending = new Map();
    ws.onmessage = e => {
      const m = JSON.parse(e.data);
      if (!m.id) return;
      const p = pending.get(m.id); pending.delete(m.id);
      m.error ? p.j(Error(m.error.message)) : p.r(m.result);
    };
    const call = (method, params = {}) => new Promise((r, j) => {
      const id = ++serial; pending.set(id, {r, j}); ws.send(JSON.stringify({id, method, params}));
    });
    await call('Page.navigate', {url: origin});
    await sleep(15000);
    const evaluated = await call('Runtime.evaluate', {awaitPromise: true, returnByValue: true, expression: `(async () => {
      const {api} = await import('/js/api.js'), {S} = await import('/js/state.js');
      const {go} = await import('/js/navigation.js'), pop = await import('/js/popups.js');
      const mods = ${JSON.stringify(mods)}, refs = mods.map(m => m.id);
      const out = window.__readerCheck = {measures: [], errors: [], spaces: []};
      const sleep = ms => new Promise(r => setTimeout(r, ms));
      const frame = () => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      window.addEventListener('error', e => out.errors.push(e.message));
      window.addEventListener('unhandledrejection', e => out.errors.push(String(e.reason)));
      pop.close();
      const measured = async (phase, name, fn) => {
        const active = await api.Tasks(), t = performance.now();
        try {
          const result = await fn();
          const ms = Math.round(performance.now() - t);
          out.measures.push({phase, name, ms, active: active.running, error: result?.error || '', stale: !!result?.stale,
            rows: result?.rows?.length, envs: result?.envs});
          return result;
        } catch (e) {
          out.measures.push({phase, name, ms: Math.round(performance.now() - t), active: active.running, error: String(e)});
        }
      };
      const workspace = async () => {
        for (let i = 0; i < 5; i++) {
          const result = await api.Workspace();
          if (!result.stale) return result;
        }
        throw Error('Workspace remained stale after five reads');
      };
      const reads = phase => Promise.all([
        measured(phase, 'Home', () => api.Home()), measured(phase, 'Shell', () => api.Shell()),
        measured(phase, 'Workspace', workspace), measured(phase, 'PlanManage', () => api.PlanManage(0)),
        measured(phase, 'Library', () => api.Library('', [], 'name'))]);
      const pages = async phase => {
        for (const id of ['home', 'plans', 'library', 'plan']) {
          await measured(phase, 'page:' + id, async () => {
            await go(id); await frame();
            const error = document.querySelector('#screen:not([hidden]) .page-read-error');
            return {error: error?.textContent || (S.screen !== (id === 'plan' ? null : id) ? 'Destination did not open' : '')};
          });
        }
      };
      // Warm every path equally; all measured repetitions use the same server/cache.
      await pages('warm'); await reads('warm');
      for (let i = 0; i < 3; i++) { await reads('idle'); await pages('idle'); }
      if (${execute}) {
        const preview = await api.LibraryPreview('subscription-remove', refs);
        if (!preview.apply) throw Error('Removal preview refused');
        await go('plan'); pop.close();
        const original = window.fetch;
        window.fetch = async (...args) => {
          const req = JSON.parse(args[1]?.body || '{}'), t = performance.now();
          const response = await original(...args);
          if (req.m === 'Toggle') {
            const result = await response.clone().json();
            out.spaces.push({ms: Math.round(performance.now() - t), error: result.error || ''});
          }
          return response;
        };
        const t = performance.now();
        await api.LibraryDo('subscription-remove', refs);
        for (let i = 0; i < 100; i++) { if ((await api.Tasks()).running) break; await sleep(5); }
        const concurrent = reads('removing');
        S.focus = 1; S.cols[1].sel.clear(); S.cols[1].cursor = 1; document.activeElement?.blur();
        for (let i = 0; i < 10; i++) document.body.dispatchEvent(new KeyboardEvent('keydown', {key: ' ', code: 'Space', bubbles: true}));
        for (let i = 0; i < 2400 && (S.editing || out.spaces.length < 10); i++) await sleep(25);
        await concurrent;
        await pages('after-space');
        for (let i = 0; i < 2400; i++) {
          const tasks = await api.Tasks();
          if (!tasks.running && !tasks.queued) {
            out.tasks = tasks.rows.filter(r => r.kind === 'subscription-remove').map(r => ({state:r.state, items:r.items?.map(i => ({state:i.state, why:i.why}))}));
            break;
          }
          await sleep(50);
        }
        out.elapsedMs = Math.round(performance.now() - t);
        out.editorRows = document.querySelectorAll('#board .row').length;
        out.sidebarLists = S.shell?.plans?.length;
        window.fetch = original;
        await reads('after'); await pages('after');
      }
      return out;
    })()`});
    if (evaluated.exceptionDetails) throw Error(JSON.stringify(evaluated.exceptionDetails));
    const result = evaluated.result.value;
    fs.writeFileSync(output, JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
    if (execute && (result.spaces.length !== 10 || result.spaces.some(s => s.error) || result.errors.length ||
        result.measures.some(m => m.error || m.stale) ||
        !result.editorRows || !result.sidebarLists ||
        !result.tasks?.length || result.tasks.some(t => t.state !== 'done'))) {
      throw Error('Deletion, keyboard or browser check failed; inspect output and restore the manifest mods');
    }
  } finally { ws?.close(); chrome?.kill(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
