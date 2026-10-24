// Run after guidev on :18765 with -steam-api http://127.0.0.1:18766 and disposable data.
// npm install --prefix tmp/dialog-check --no-package-lock playwright-core@1.58.2
// node spikes/guidev/dialogcheck.cjs
// All browser state and screenshots stay under this worktree's ignored tmp directory.
const { chromium } = require(require('path').resolve('tmp/dialog-check/node_modules/playwright-core'));
const fs = require('fs'), path = require('path'), http = require('http');
const assert = (ok, message) => { if (!ok) throw new Error(message); };
const results = [];
const server = http.createServer((req, res) => {
  req.resume(); res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(req.url === '/collection' ? {response:{collectiondetails:[{publishedfileid:'3000000001',result:1,children:[{publishedfileid:'1001001001',sortorder:0,filetype:0}]}]}} : {response:{publishedfiledetails:[{publishedfileid:'3000000001',result:1,title:'Fixture collection'},{publishedfileid:'1001001001',result:1,title:'Alpha'}]}}));
});
(async () => {
  await new Promise(resolve => server.listen(18766, '127.0.0.1', resolve));
  let context;
  try {
    context = await chromium.launchPersistentContext(path.resolve('tmp/dialog-check/chrome-profile'), {executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe', headless:true, viewport:{width:1440,height:1000}});
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', e => { errors.push(e.message); console.error('PAGEERROR', e.message); });
    await page.goto('http://127.0.0.1:18765');
    await page.waitForFunction(async () => !!(await import('/js/state.js')).S.shell?.plan);
    const state = fn => page.evaluate(fn);
    const shareName = 'Browser share import ' + Date.now();
    const original = await state(async () => (await import('/js/state.js')).S.shell.plan.id);
    const gamePath = path.resolve('tmp/dialog-check/fixture/UserData/Config/ModsConfig.xml');
    const game = fs.readFileSync(gamePath, 'utf8');
    await state(async () => (await import('/js/planmanage.js')).open());
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('n');
    await page.locator('.pm-new input[type=text], .pm-new input:not([type])').waitFor();
    const name = page.locator('.pm-new input:not([type])');
    assert(await name.evaluate(e => e === document.activeElement), 'new name is not focused');
    assert(await page.locator('.pm-new input[type=radio]').count() === 4, 'not four starting points');
    await page.screenshot({path:'tmp/dialog-check/new-list.png'});
    const existing = await state(async () => (await import('/js/state.js')).S.shell.plan.name);
    await name.fill(existing);
    await page.waitForFunction(() => document.querySelector('.pm-new .plan-why')?.textContent.length > 0);
    assert(await page.locator('.pm-new .primary').isDisabled(), 'duplicate enabled create');
    await name.fill('Browser new list'); await page.keyboard.press('Enter');
    await page.locator('.pm-row.on').filter({hasText:'Browser new list'}).waitFor();
    assert((await page.locator('.pm-row').first().innerText()).includes('Browser new list'), 'draft row did not become the created list');
    assert(await page.locator('#status a').count() === 1, 'create has no undo');
    await page.keyboard.press('u');
    await page.waitForFunction(() => !document.querySelector('.pm-new') && document.querySelector('.pm-list') && !document.querySelector('.pm-list').textContent.includes('Browser new list'));
    results.push('PASS new list: focus, four sources, duplicate check, enter, u undo');
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('n'); await name.waitFor();
    await page.keyboard.press('Escape'); await page.waitForFunction(() => !document.querySelector('.pm-new'));
    results.push('PASS esc abandons the new-list form');
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('i');
    await page.locator('.pm-import textarea').waitFor();
    assert((await page.locator('#crumbs').innerText()).replace(/\s/g,'').endsWith('库/列表/导入'), 'import breadcrumb');
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'import opened a popup');
    await page.keyboard.press('Escape'); await page.locator('.pm-list').waitFor();
    results.push('PASS import breadcrumb, management selection and esc return');
    const code = await state(async () => (await (await import('/js/state.js')).api.ExportCopy('rimcrow')).text);
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('i');
    await page.locator('.pm-import textarea').fill(code);
    await page.locator('.xc-body button').filter({hasText:'预览'}).click();
    await page.locator('.xc-preview input.xc-input').waitFor();
    assert(await page.locator('.xc-preview input[type=radio]').count() === 3, 'not three destinations');
    await page.locator('.xc-preview input.xc-input').fill(shareName);
    await page.keyboard.press('Enter');
    await page.locator('.pm-row.on').filter({hasText:shareName}).waitFor();
    results.push('PASS share code: preview, three destinations, import, selected result');
    await page.locator('.pm-row.on').focus(); await page.keyboard.press('i');
    await page.locator('.pm-import textarea').fill('https://steamcommunity.com/sharedfiles/filedetails/?id=3000000001');
    await page.locator('.xc-body button').filter({hasText:'预览'}).click();
    const follow = page.locator('.xc-preview input[type=checkbox]'); await follow.waitFor();
    await page.screenshot({path:'tmp/dialog-check/collection-import.png'});
    assert(await follow.isChecked(), 'collection follow is not on by default');
    await page.locator('.xc-preview .btn.primary').click();
    await page.locator('.pm-row.on').filter({hasText:'Fixture collection'}).waitFor();
    assert((await page.locator('.pm-detail').innerText()).includes('Fixture collection'), 'followed collection not shown');
    results.push('PASS Steam fixture collection: preview, default follow, import and persisted association');
    // Open the original editor via its actual sidebar command.
    await page.evaluate(async id => {
      const {api}=await import('/js/state.js');
      await api.UsePlan(id); await api.ImportPreviewFor(id, 'text', 'tester.b'); await api.ImportApplyTo(id, false);
    }, original);
    await page.reload();
    await page.waitForFunction(async () => !!(await import('/js/state.js')).S.shell?.plan);
    await page.locator('#side [data-id=plan]').click();
    await page.locator(`#side [data-id=\"plan:${original}\"]`).click();
    await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen);
    const before = await state(async () => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(','));
    await page.keyboard.press('H');
    await page.locator('#popup .opt').first().waitFor();
    const versionIndex = await page.locator('#popup .opt').evaluateAll(rows => rows.findIndex(r => r.textContent.includes('◆')));
    assert(versionIndex >= 0, 'history has no version');
    for (let i = 0; i < versionIndex; i++) await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    await page.waitForFunction(async value => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(',') !== value, before);
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'history asked for confirmation');
    assert(await page.locator('#status a').count() === 1, 'history has no undo');
    await page.keyboard.press('u');
    await page.waitForFunction(async value => (await import('/js/state.js')).S.board.cols[1].map(r => r.name).join(',') === value, before);
    await page.keyboard.press('i'); await page.locator('.pm-import').waitFor();
    await page.keyboard.press('Escape'); await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen);
    await page.keyboard.press('e'); await page.locator('.pm-export').waitFor();
    await page.keyboard.press('Escape'); await page.waitForFunction(async () => !(await import('/js/state.js')).S.screen);
    assert(fs.readFileSync(gamePath,'utf8') === game, 'forms or checkout wrote game files');
    await page.evaluate(async () => {
      (await import('/js/guide.js')).resume();
      await (await import('/js/exchange.js')).exchange();
    });
    await page.locator('.pm-import textarea').waitFor();
    await page.keyboard.press('Escape');
    await page.waitForFunction(async () => (await import('/js/state.js')).S.screen === 'guide');
    assert(!await state(async () => (await import('/js/state.js')).S.popup), 'guide import opened a popup');
    await page.evaluate(async () => (await import('/js/planmanage.js')).open());
    await page.evaluate(() => {
      const files = new DataTransfer(); files.items.add(new File(['tester.b'], 'dropped.txt', {type:'text/plain'}));
      document.dispatchEvent(new DragEvent('drop', {bubbles:true, cancelable:true, dataTransfer:files}));
    });
    await page.locator('.xc-preview input[type=radio]').first().waitFor();
    assert((await page.locator('.xc-preview').innerText()).includes('dropped.txt'), 'dropped file not previewed');
    await page.keyboard.press('Escape'); await page.locator('.pm-list').waitFor();
    assert(errors.length === 0, 'browser errors: ' + errors.join('; '));
    await page.screenshot({path:'tmp/dialog-check/editor.png'});
    results.push('PASS history version: enter checks out directly, status undo and u restore; editor i/e open subpages; game file unchanged');
    results.push('PASS guide uses the shared import page; esc resumes guide; window file drop opens preview');
    console.log(results.join('\n'));
    fs.writeFileSync('tmp/dialog-check/results.txt', results.join('\n'));
  } finally {
    if (context) { if (results.length < 6) console.error('LAST PAGE', await context.pages().at(-1).evaluate(() => document.body.innerText.slice(-4500))); await context.close(); }
    await new Promise(resolve => server.close(resolve));
  }
})().catch(e => { console.error(results.join('\n')); console.error(e); process.exitCode=1; });
