// 06 §3.1 / G24: exercise every card gap in guidev with disposable game data.
// CGO_ENABLED=0 go build -o tmp/gap-check/archo.exe ./cmd/archo
// CGO_ENABLED=0 go build -o tmp/gap-check/guidev.exe ./spikes/guidev
// npm install --prefix tmp/gap-check --no-package-lock playwright-core@1.58.2
// node spikes/guidev/gapcheck.cjs
const fs = require('fs'), path = require('path'), { spawn, execFileSync } = require('child_process');
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || path.resolve('tmp/gap-check/node_modules/playwright-core'));
const root = path.resolve(process.env.RIMARCHO_CHECK_ROOT || 'tmp/gap-check'), fixture = path.join(root, 'fixture');
const game = path.join(fixture, 'Game'), data = path.join(fixture, 'UserData'), home = path.join(fixture, 'Home');
const write = (file, value) => { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, value); };
const assert = (ok, message) => { if (!ok) throw new Error(message); };
const near = (a, b, message) => assert(Math.abs(a - b) <= 2, `${message}: ${a} != ${b}`);
const results = [], errors = [];
const cli = (...args) => execFileSync(path.join(root, 'archo.exe'), args, { env: { ...process.env, RIMARCHO_HOME: home }, windowsHide: true, encoding: 'utf8' });
write(path.join(game, 'RimWorldWin64.exe'), ''); write(path.join(game, 'Version.txt'), '1.6.4633 rev1261');
for (const [dir, id, name] of [['Data/Core', 'ludeon.rimworld', 'Core'], ['Mods/Alpha', 'tester.a', 'Alpha']]) {
  write(path.join(game, dir, 'About/About.xml'), `<ModMetaData><name>${name}</name><packageId>${id}</packageId><supportedVersions><li>1.6</li></supportedVersions></ModMetaData>`);
}
write(path.join(data, 'Config/ModsConfig.xml'), '<ModsConfigData><version>1.6.4633</version><activeMods><li>ludeon.rimworld</li></activeMods></ModsConfigData>');
if (!fs.existsSync(home)) cli('env', 'add', 'gaps', '--game', game, '--user-data', data);
cli('scan');

(async () => {
  let browser;
  const probe = require('net').createServer();
  await new Promise((resolve) => probe.listen(0, '127.0.0.1', resolve));
  const port = probe.address().port;
  await new Promise((resolve) => probe.close(resolve));
  const origin = 'http://127.0.0.1:' + port;
  const service = spawn(path.join(root, 'guidev.exe'), ['-home', home, '-addr', '127.0.0.1:' + port, '-assets', path.resolve('internal/surface/gui/assets')], { windowsHide: true });
  let log = ''; service.stderr.on('data', (d) => { log += d; });
  try {
    browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true });
    const context = await browser.newContext({ viewport: { width: 1600, height: 1100 } });
    const page = await context.newPage(); page.on('pageerror', (e) => errors.push(e.message));
    // Log analysis and the empty task list use fixtures; input and settings use the product.
    await page.route('**/call', async (route) => {
      const { m } = route.request().postDataJSON();
      let result;
      if (m === 'LogPage') result = { path: 'fixture', files: [], rows: [{ id: 'E1', text: 'Fixture error', class: 'Exception', count: 1, status: 'bad' }] };
      else if (m === 'LogPattern') result = { level: 'medium', levelText: 'Fixture analysis', summary: 'A deterministic analysis card', next: 'Check the fixture' };
      else if (m === 'LogCluster') result = ['Fixture error details'];
      else if (m === 'Tasks') result = { rows: [] };
      else return route.continue();
      await route.fulfill({ json: { result } });
    });
    for (let i = 0; i < 100; i++) {
      try { if ((await fetch(origin)).ok) break; } catch {}
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    const settle = () => page.evaluate(async () => { for (let i = 0; i < 4; i++) await new Promise(requestAnimationFrame); });
    const setting = (key) => page.evaluate(async (key) => (await import('/js/state.js')).api.Setting(key), key);
    const seed = (key, value) => page.evaluate(async ([key, value]) => (await import('/js/state.js')).api.SetSetting(key, value), [key, value]);
    const open = (module, method = 'open', args = []) => page.evaluate(async ([module, method, args]) => (await import('/js/' + module + '.js'))[method](...args), [module, method, args]);
    const value = (handle) => handle.evaluate((e) => +e.getAttribute('aria-valuenow'));
    await page.goto(origin);
    await page.waitForFunction(async () => !!(await import('/js/state.js')).S.shell?.plan);
    async function check(selector, key, reopen, reverse = false) {
      let handle = page.locator(selector); await handle.waitFor({ state: 'visible' }); await settle();
      assert(await handle.getAttribute('role') === 'separator', selector + ' role');
      assert(await handle.getAttribute('tabindex') === '0', selector + ' tab stop');
      await handle.focus(); await page.keyboard.press('Shift+Tab'); await page.keyboard.press('Tab');
      assert(await handle.evaluate((e) => e === document.activeElement), selector + ' native tab reachability');
      const axis = await handle.getAttribute('aria-orientation'), vertical = axis === 'vertical';
      const before = await value(handle), max = +(await handle.getAttribute('aria-valuemax'));
      const delta = max - before >= 40 ? 40 : -40;
      await page.keyboard.press(vertical ? delta > 0 ? 'ArrowRight' : 'ArrowLeft' : (delta > 0) !== reverse ? 'ArrowDown' : 'ArrowUp');
      await settle(); near(await value(handle), before + delta, selector + ' keyboard 40px');
      await page.waitForFunction(async (key) => !!(await import('/js/state.js')).api && (await (await import('/js/state.js')).api.Setting(key)) !== '', key);
      const saved = await setting(key), moved = await value(handle);
      if (reopen) { await reopen(); await settle(); handle = page.locator(selector); near(await value(handle), moved, selector + ' restored size'); assert(await setting(key) === saved, selector + ' saved format'); }
      const rect = await handle.boundingBox(), dragBefore = await value(handle);
      const space = +(await handle.getAttribute('aria-valuemax')) - dragBefore;
      const dragDelta = selector.includes('fbar-gap') && space < 80 ? -63 : space >= 23 ? 23 : -23;
      // Start in the extended hit area, three pixels over the card edge (06 §7.3).
      const x = rect.x + rect.width / 2 + (vertical && selector !== '#side-split' ? 4 : 0), y = rect.y + rect.height / 2;
      await page.mouse.move(x, y); await page.mouse.down();
      await page.mouse.move(x + (vertical ? dragDelta : 0), y + (vertical ? 0 : reverse ? -dragDelta : dragDelta), { steps: 4 });
      await page.mouse.up(); await settle();
      near(await value(handle), dragBefore + dragDelta, selector + ' drag');
      await handle.focus(); await page.keyboard.press('Enter');
      await page.waitForFunction(async (key) => (await (await import('/js/state.js')).api.Setting(key)) === '', key);
      await handle.press(vertical ? 'ArrowLeft' : 'ArrowUp'); await handle.dblclick({ force: true });
      await page.waitForFunction(async (key) => (await (await import('/js/state.js')).api.Setting(key)) === '', key);
      results.push('PASS ' + selector + ': tab, 40px arrows, drag/hit area, enter, double click, persistence');
    }
    await check('#side-split', 'ui.gui.side', async () => { await page.reload(); await page.waitForFunction(async () => !!(await import('/js/state.js')).S.shell?.plan); });
    const editor = async () => { await page.locator('#side [data-id="plan"]').click(); const id = await page.evaluate(async () => (await import('/js/state.js')).S.shell.plan.page); await page.locator(`#side [data-id="${id}"]`).click(); await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen && !(await import('/js/screens.js')).pageLoading() && !document.querySelector('#board').inert); };
    await editor();
    const columns = 'ui.gui.columns:' + await page.evaluate(async () => (await import('/js/state.js')).S.board.env);
    const reloadEditor = async () => { await page.reload(); await page.waitForFunction(async () => !!(await import('/js/state.js')).S.shell?.plan); await editor(); };
    for (const i of [0, 1]) await check(`#board > [data-split="${i}"]`, columns, reloadEditor);
    async function filter(selector, key, reopen) {
      await page.locator(selector + ' .fbar-tog').click(); await settle();
      const h = page.locator(selector + ' .fbar-gap');
      await check(selector + ' .fbar-gap', key, async () => { await reopen(); if (!(await page.locator(selector).evaluate((e) => e.classList.contains('open')))) await page.locator(selector + ' .fbar-tog').click(); });
      const first = page.locator(selector + ' .fbar-v').first();
      if (await first.count()) {
        await h.focus(); await h.press('Enter'); const start = await value(h);
        await first.focus(); await page.keyboard.press('Control+ArrowUp'); await settle(); near(await value(h), start - 40, key + ' legacy ctrl+up');
      }
      await page.locator(selector + ' .fbar-tog').click();
    }
    await filter('#board .fbar', 'ui.gui.filter.split:plan', editor);
    const expandStack = () => page.evaluate(async () => {
      const { S } = await import('/js/state.js');
      S.history = [{ kind: 'undo', text: 'Fixture edit' }, { kind: 'now', text: 'now' }];
      (await import('/js/lists.js')).renderHistory();
      (await import('/js/lists.js')).setStagingOpen(true);
      const history = document.querySelector('#board .histarea');
      if (!history.classList.contains('open')) history.querySelector('.hbar').click();
    });
    await expandStack(); await settle();
    for (const [target, name] of [['[data-area="2"]', 'stage'], ['.histarea', 'history']]) await check(`#board .stack-split:has(+ ${target})`, 'ui.gui.plan.' + name, async () => { await reloadEditor(); await expandStack(); }, true);
    await open('library'); await check('.lib-split', 'ui.gui.library.side', () => open('library'));
    await filter('.lib .fbar', 'ui.gui.filter.split:library', () => open('library'));
    await open('tasks');
    await seed('ui.gui.tasks.split', '200'); await open('tasks');
    await check('.tk-split', 'ui.gui.tasks.side', () => open('tasks'));
    await check('.tk-hsplit', 'ui.gui.tasks.split', () => open('tasks'));
    await filter('.tk .fbar', 'ui.gui.filter.split:tasks', () => open('tasks'));
    const logs = () => open('screens', 'gameLog', ['fixture', true]); await logs();
    await page.locator('.ana-card').waitFor({ state: 'visible' });
    await check('.logsplit', 'ui.gui.log.left', logs);
    await check('.logside > .hsplit', 'ui.gui.log.ana', logs);
    await open('planmanage'); await seed('ui.gui.plans.side', '300'); await open('planmanage');
    await check('.pm-split', 'ui.gui.plans.side', () => open('planmanage'));
    await page.screenshot({ path: path.join(root, 'plans.png') });
    // Stacked layouts must reject synthetic input as well as disappearing from tab order.
    for (const [load, selector, key, width] of [[logs, '.logside > .hsplit', 'ui.gui.log.ana', 800], [() => open('planmanage'), '.pm-split', 'ui.gui.plans.side', 700], [() => open('tasks'), '.tk-hsplit', 'ui.gui.tasks.split', 800], [() => open('library'), '.lib-split', 'ui.gui.library.side', 800]]) {
      await page.setViewportSize({ width: 1600, height: 1100 }); await load(); await page.setViewportSize({ width, height: 1000 }); await settle();
      const old = await setting(key), handle = page.locator(selector); assert(await handle.getAttribute('tabindex') === '-1', selector + ' narrow tab order');
      await handle.evaluate((e) => { e.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })); e.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })); });
      assert(await setting(key) === old, selector + ' changed while stacked'); results.push('PASS ' + selector + ': narrow layout disabled');
    }
    assert(errors.length === 0, errors.join('\n'));
    fs.writeFileSync(path.join(root, 'results.txt'), results.join('\n') + '\n'); console.log(results.join('\n'));
  } catch (error) {
    if (browser?.contexts()[0]?.pages()[0]) {
      const p = browser.contexts()[0].pages()[0];
      await p.screenshot({ path: path.join(root, 'failure.png') });
    }
    console.error(log); throw error;
  }
  finally { await browser?.close(); service.kill(); }
})().catch((error) => { console.error(error); process.exitCode = 1; });
