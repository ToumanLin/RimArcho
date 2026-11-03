// Run after guidev on :18765 with -steam-api http://127.0.0.1:18766 and disposable data.
// npm install --prefix tmp/dialog-check --no-package-lock playwright-core@1.58.2
// node spikes/guidev/dialogcheck.cjs
// All browser state and screenshots stay under this worktree's ignored tmp directory.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || require('path').resolve('tmp/dialog-check/node_modules/playwright-core'));
const fs = require('fs'), path = require('path'), http = require('http');
const assert = (ok, message) => { if (!ok) throw new Error(message); };
const checkRoot = process.env.RIMARCHO_CHECK_ROOT || 'tmp/dialog-check';
const results = [];
const server = http.createServer((req, res) => {
  req.resume(); res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(req.url === '/collection' ? {response:{collectiondetails:[{publishedfileid:'3000000001',result:1,children:[{publishedfileid:'1001001001',sortorder:0,filetype:0}]}]}} : {response:{publishedfiledetails:[{publishedfileid:'3000000001',result:1,title:'Fixture collection'},{publishedfileid:'1001001001',result:1,title:'Alpha'}]}}));
});
(async () => {
  await new Promise(resolve => server.listen(18766, '127.0.0.1', resolve));
  let context;
  try {
    context = await chromium.launchPersistentContext(path.resolve(path.join(checkRoot, 'chrome-profile')), {executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe', headless:true, viewport:{width:1440,height:1000}});
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', e => { errors.push(e.message); console.error('PAGEERROR', e.message); });
    await page.goto('http://127.0.0.1:18765');
    await page.locator('#side [data-id=plan]').waitFor({state:'visible'});
    const state = fn => page.evaluate(fn);
    const original = await state(async () => (await import('/js/state.js')).S.shell.plan.id);
    const gamePath = path.resolve(path.join(checkRoot, 'fixture/UserData/Config/ModsConfig.xml'));
    const game = fs.readFileSync(gamePath, 'utf8');
    const open = id => page.evaluate(async id => (await import('/js/navigation.js')).go(id), id);
    const editor = async () => { await open('plan:' + original); await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen); };
    await open('plans');
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('n');
    const name = page.locator('.pm-new input.pm-name'); await name.waitFor();
    assert(await name.evaluate(e => e === document.activeElement), 'new name is not focused');
    assert(await page.locator('.pm-new .pm-modes button').count() === 4, 'not four starting points');
    const newCrumbs = (await page.locator('#crumbs').innerText()).replace(/\s/g,'');
    assert(/library\/lists\/alllists(\(\d+\))?\/newlist$/i.test(newCrumbs), 'new-list breadcrumb: ' + newCrumbs);
    const existing = await state(async () => (await import('/js/state.js')).S.shell.plan.name);
    await name.fill(existing);
    await page.waitForFunction(() => document.querySelector('.pm-new .plan-why')?.textContent.length > 0);
    assert(await page.locator('.pm-new .primary').isDisabled(), 'duplicate enabled create');
    const newName = 'Browser new list ' + Date.now();
    await name.fill(newName); await page.keyboard.press('Enter');
    await page.waitForFunction(async value => { const {S} = await import('/js/state.js'); return !S.screen && S.shell?.plan?.name === value; }, newName);
    await page.waitForFunction(value => document.querySelector('#status')?.textContent.includes(value), newName);
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'creation opened a popup');
    results.push('PASS new list: breadcrumb, focus, four sources, duplicate check, Enter creates and opens editor');
    await open('plans'); await page.locator('.pm-row.on').focus(); await page.keyboard.press('n');
    await name.waitFor(); await name.focus(); await page.keyboard.press('Escape');
    await page.waitForFunction(() => !document.querySelector('.pm-new'));
    await open('plans');
    results.push('PASS Esc closes the new-list form');
    // Management import stays inline; the editor uses the shared import subpage.
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('i');
    await page.locator('.pm-inline-import textarea').waitFor();
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'management import opened a popup');
    await page.keyboard.press('Escape'); await page.waitForFunction(() => !document.querySelector('.pm-new'));
    await editor(); await page.keyboard.press('i'); await page.locator('.pm-import textarea').waitFor();
    const code = await state(async () => (await (await import('/js/state.js')).api.ExportCopy('rimcrow')).text);
    await page.locator('.pm-import textarea').fill(code);
    await page.locator('.xc-body button').filter({hasText:'preview'}).click();
    await page.locator('.xc-preview input.xc-input').waitFor();
    assert(await page.locator('.xc-preview input[type=radio]').count() === 3, 'not three destinations');
    const shareName = 'Browser share import ' + Date.now();
    await page.locator('.xc-preview input.xc-input').fill(shareName); await page.keyboard.press('Enter');
    await page.waitForFunction(async value => { const {S} = await import('/js/state.js'); return !S.screen && S.shell?.plan?.name === value; }, shareName);
    await editor(); await page.keyboard.press('i'); await page.locator('.pm-import textarea').fill('https://steamcommunity.com/sharedfiles/filedetails/?id=3000000001');
    await page.locator('.xc-body button').filter({hasText:'preview'}).click();
    const follow = page.locator('.xc-preview input[type=checkbox]'); await follow.waitFor();
    assert(await follow.isChecked(), 'collection follow is not on by default');
    await page.locator('.xc-preview .btn.primary[data-env-write="import-apply"]').click();
    await page.waitForFunction(async () => { const {S} = await import('/js/state.js'); return !S.screen && S.shell?.plan?.name === 'Fixture collection'; });
    await open('plans'); await page.locator('.pm-row').filter({hasText:'Fixture collection'}).first().click();
    await page.waitForFunction(() => document.querySelector('.pm-detail')?.innerText.includes('Fixture collection'));
    results.push('PASS share code and Steam fixture collection: import creates the list and opens it; the followed collection shows in All lists');
    await editor();
    await page.keyboard.press('e'); await page.locator('.pm-export').waitFor();
    await page.keyboard.press('Escape'); await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen);
    results.push('PASS inline management import; editor import/export; share-code destinations; fixture collection follow; Esc returns');
    await page.evaluate(async id => {
      const {api}=await import('/js/state.js');
      await api.OpenPlan(id); await api.ImportPreviewFor(id, 'text', 'tester.b'); await api.ImportApplyTo(id, false);
    }, original);
    await page.reload(); await page.locator('#side [data-id=plan]').waitFor({state:'visible'}); await editor();
    const before = await state(async () => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(','));
    await page.keyboard.press('H'); await page.locator('#popup .opt').first().waitFor();
    const versionIndex = await page.locator('#popup .opt').evaluateAll(rows => rows.findIndex(r => r.textContent.includes('◆')));
    assert(versionIndex >= 0, 'history has no version');
    for (let i = 0; i < versionIndex; i++) await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(async value => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(',') !== value, before);
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'history asked for confirmation');
    assert(await page.locator('#status a').count() === 1, 'history has no undo');
    await page.keyboard.press('u');
    await page.waitForFunction(async value => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(',') === value, before);
    assert(fs.readFileSync(gamePath,'utf8') === game, 'forms or checkout wrote game files');
    results.push('PASS history version: Enter checks out directly, status undo and u restore; game file unchanged');
    await page.evaluate(async () => { (await import('/js/guide.js')).resume(); await (await import('/js/exchange.js')).exchange(); });
    await page.locator('.pm-import textarea').waitFor(); await page.keyboard.press('Escape');
    await page.waitForFunction(async () => (await import('/js/state.js')).S.screen === 'guide');
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'guide import opened a popup');
    await open('plans');
    await page.evaluate(() => {
      const files = new DataTransfer(); files.items.add(new File(['tester.b'], 'dropped.txt', {type:'text/plain'}));
      document.dispatchEvent(new DragEvent('drop', {bubbles:true, cancelable:true, dataTransfer:files}));
    });
    await page.locator('.xc-preview input[type=radio]').first().waitFor();
    assert((await page.locator('.xc-preview').innerText()).includes('dropped.txt'), 'dropped file not previewed');
    await page.keyboard.press('Escape'); await page.locator('.pm-list').waitFor();
    results.push('PASS guide uses the shared import page without a popup and resumes on Esc; file drop opens preview');
    // A1: secondary pages own the only exposed main, and the editor returns intact.
    for (const destination of ["home", "library", "needs", "plans", "env", "settings", "tasks", "plan"]) {
      await page.evaluate(async id => { await (await import('/js/navigation.js')).go(id); }, destination);
      assert(await page.locator('#board').evaluate(e => e.inert) === (destination !== "plan"), destination + ': editor boundary');
      const mains = page.locator('main:not([inert]), [role=main]');
      assert(await mains.count() === 1, destination + ': one main');
      assert(await mains.first().evaluate(e => !!document.getElementById(e.getAttribute('aria-labelledby'))?.textContent.trim()), destination + ': named main');
      assert(await page.locator('#side [aria-current=page]').count() === 1, destination + ': current sidebar entry');
      assert(await page.locator('#crumbs [aria-current=page]').count() === 1, destination + ': current breadcrumb');
    }
    results.push('PASS A1: one named main, inert editor, current sidebar and breadcrumb');
    await page.keyboard.press('Control+b');
    for (const destination of ["home", "library", "changes", "plans", "plan", "settings"]) {
      await open(destination);
      assert(await page.locator('#side [aria-current]:visible').count() === 1, destination + ': current top navigation');
      assert(await page.locator('#crumbs [aria-current=page]').count() === 1, destination + ': current top breadcrumb');
      assert(await page.locator('main:not([inert]), [role=main]').first().evaluate(e => !!document.getElementById(e.getAttribute('aria-labelledby'))?.textContent.trim()), destination + ': named top main');
    }
    await page.keyboard.press('Control+b'); await editor();
    await page.locator('#board .area[data-area="1"] .rows').focus();
    await open('settings'); await editor();
    assert(await page.locator('#board .area[data-area="1"] .rows').evaluate(e => e === document.activeElement), 'editor focus not restored');
    results.push('PASS A1 top navigation current state, main names, layout switching and editor focus restoration');

    // A6/A15: shared dialogs trap both Tab directions and restore their opener.
    for (const kind of ["confirm", "path", "memory", "url", "rename", "ask", "uninstall"]) {
      await page.locator('#nav-settings').focus();
      await page.evaluate(async kind => {
        const p = await import('/js/popups.js');
        if (kind === "confirm") p.confirm('delete the list', ['Synthetic deletion warning'], 'delete', () => { window.a11yAccepted = true; });
        if (kind === "path") p.confirm('remove the game', ['C:\\Fixture\\Game'], 'remove', () => {});
        if (kind === "memory") p.memoryConfirm({needed:'8 GB', available:'4 GB'}, () => {});
        if (kind === "url") (await import('/js/screens.js')).urlPrompt('Workshop URL', 'https://example.test/', () => {});
        if (kind === "ask") p.ask("note", "", () => {}, "");
        if (kind === "uninstall") { await (await import("/js/navigation.js")).go("uninstall"); document.querySelector("#screen .un-foot .primary").click(); }
        if (kind === "rename") (await import('/js/plans.js')).renamePlan((await import('/js/state.js')).S.shell.plan.id);
      }, kind);
      const dialog = page.locator('#popup [role=dialog]');
      await page.waitForFunction(() => document.querySelector('#popup [role=dialog]')?.contains(document.activeElement));
      // A confirmation starts on the dialog itself (Enter confirms, as its key cap says); the RAM warning and uninstall on Cancel.
      if (['confirm', 'path'].includes(kind)) assert(await dialog.evaluate(e => e === document.activeElement), kind + ': initial focus is not the dialog');
      if (['memory', 'uninstall'].includes(kind)) assert(await dialog.locator('.btns button').first().evaluate(e => e === document.activeElement), kind + ': initial focus is not Cancel');
      assert(await dialog.getAttribute('aria-modal') === 'true', kind + ': modal');
      assert(await dialog.evaluate(e => !!document.getElementById(e.getAttribute('aria-labelledby'))?.textContent), kind + ': existing title');
      assert(await page.locator('#main').evaluate(e => e.inert), kind + ': inert background');
      for (const key of ['Tab', 'Shift+Tab', ...Array(12).fill('Tab'), ...Array(12).fill('Shift+Tab')]) {
        await page.keyboard.press(key);
        assert(await dialog.evaluate(e => e.contains(document.activeElement)), kind + ': focus escaped');
      }
      if (kind === 'rename') { await dialog.locator('.btns button').first().focus(); await page.keyboard.press('Enter'); }
      else await page.keyboard.press('Escape');
      assert(!await page.locator('#main').evaluate(e => e.inert), kind + ': background restored');
      assert(await page.locator('#nav-settings').evaluate(e => e === document.activeElement), kind + ': opener restored');
    }
    await editor();
    const deletion = () => page.evaluate(async () => { window.a11yAccepted = false; (await import('/js/popups.js')).confirm('delete the list', ['Synthetic deletion warning'], 'delete', () => { window.a11yAccepted = true; }); });
    // Tab reaches Cancel first; Enter there cancels. Enter on the dialog confirms (the enter key cap).
    await deletion(); await page.keyboard.press('Tab');
    assert(await page.locator('#popup .btns button').first().evaluate(e => e === document.activeElement), 'Tab did not reach Cancel first');
    await page.keyboard.press('Enter');
    assert(!await page.evaluate(() => window.a11yAccepted), 'Enter on Cancel accepted deletion');
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'Enter on Cancel did not close');
    await deletion(); await page.keyboard.press('Enter');
    assert(await page.evaluate(() => window.a11yAccepted), 'Enter on the dialog did not confirm');
    // ctrl+\ (reference numbers) still works over a dialog.
    await deletion(); await page.keyboard.press('Control+Backslash');
    await page.locator('.refnums').waitFor({state:'attached'});
    await page.keyboard.press('Control+Backslash'); await page.keyboard.press('Escape');
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'Esc after reference numbers did not close');
    await page.evaluate(async () => {
      const opener = document.createElement('button'); opener.textContent = 'Synthetic opener'; document.querySelector('#board').append(opener); opener.focus();
      (await import('/js/popups.js')).confirm('delete the list', ['Synthetic deletion warning'], 'delete', () => {});
      opener.remove();
    });
    await page.keyboard.press('Escape');
    assert(await page.locator('#board').evaluate(e => document.activeElement === e), 'missing opener did not return to named main');
    await open('settings');
    const confirmation = () => page.evaluate(async () => (await import('/js/popups.js')).confirm('delete the list', ['Synthetic deletion warning'], 'delete', () => {}));
    await confirmation(); await page.keyboard.press('Alt+ArrowLeft');
    assert(!await state(async () => (await import('/js/state.js')).S.popup) && await state(async () => (await import('/js/state.js')).S.screen) === 'settings', 'Back did not close just the modal');
    await confirmation(); await page.keyboard.press('Control+b');
    await page.waitForFunction(() => document.querySelector('#app').classList.contains('nav-top'));
    assert(await dialogVisible(), 'layout shortcut closed the modal');
    await page.keyboard.press('Control+b'); await page.keyboard.press('Control+k');
    assert(await page.locator('#popup .command-panel').count() === 1 && await page.locator('#popup [aria-modal=true]').count() === 0 && !await page.locator('#main').evaluate(e => e.inert), 'command panel became modal');
    await page.keyboard.press('Escape');
    results.push('PASS A6/A15: confirmations, path warning, RAM, URL, rename, ask and uninstall; initial focus, Tab/Shift+Tab, inert, Esc, restore; Enter confirms, Enter on Cancel cancels, ctrl+backslash passes; Back, layout shortcut and nonmodal command panel');
    async function dialogVisible() { return await page.locator('#popup [role=dialog]').isVisible(); }
    assert(errors.length === 0, 'browser errors: ' + errors.join('; '));
    await page.screenshot({path:path.join(checkRoot, 'editor.png')});
    console.log(results.join('\n'));
    fs.writeFileSync(path.join(checkRoot, 'results.txt'), results.join('\n'));
  } finally {
    if (context) { if (results.length < 6) console.error('LAST PAGE', await context.pages().at(-1).evaluate(() => document.body.innerText.slice(-4500))); await context.close(); }
    await new Promise(resolve => server.close(resolve));
  }
})().catch(e => { console.error(results.join('\n')); console.error(e); process.exitCode=1; });
