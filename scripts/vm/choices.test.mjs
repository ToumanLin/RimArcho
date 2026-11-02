// Synthetic DOM checks in Chromium, using the repository's dependency-free CDP driver.
// Set CHROME when it is not installed at the standard Windows location.
import { existsSync, readFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { connect, sleep } from './cdp.mjs';

const chromePath = process.env.CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const source = readFileSync(new URL('../../internal/surface/gui/assets/js/choices.js', import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
const icons = readFileSync(new URL('../../internal/surface/gui/assets/js/icons.js', import.meta.url), 'utf8').replace(/\bexport /g, '');
const refnums = readFileSync(new URL('../../internal/surface/gui/assets/js/refnums.js', import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
const css = readFileSync(new URL('../../internal/surface/gui/assets/style.css', import.meta.url), 'utf8');
const fixture = `<!doctype html><meta charset="utf-8"><style>${css}
body {overflow:auto;padding:20px} #clip {overflow:hidden;width:320px;height:48px} .fixture {display:flex;gap:12px;align-items:center} #outside {height:30px;position:fixed;left:20px;bottom:20px}
</style><div id="clip"><div id="host" class="fixture"></div></div><button id="outside">outside</button><fieldset id="fieldset"></fieldset><div id="menu" hidden></div><script type="module">${icons}
${source}
const S={screen:"settings"}, api={Setting:async()=>"",SetSetting:async()=>{},SaveRefNums:async()=>{}};
${refnums}
Object.assign(window,{choiceControl,closeChoiceMenu,toggleRefNums});
window.reset = () => {
 closeChoiceMenu(); document.querySelector('#host').replaceChildren(); document.querySelector('#fieldset').replaceChildren();
 document.querySelector('#fieldset').disabled=false;
 document.documentElement.dataset.scheme='dark';
 window.events=[];
 window.b=choiceControl('Language',[{value:'auto',label:'System'},{value:'en',label:'English'},{value:'zh',label:'简体中文'},{value:'no',label:'Disabled',disabled:true}], 'en');
 window.s=choiceControl('Automatic checks',[{value:'on',label:'开'},{value:'off',label:'关'}], 'on');
 for(const x of [b,s]) { x.addEventListener('input',e=>events.push(['input',e.target.value,e.bubbles])); x.onchange=e=>events.push(['change',e.target.value,e.bubbles]); }
 document.querySelector('#host').append(b,s);
}; reset();window.ready=true;
</script>`;

test('shared choices: real DOM, keyboard, events and placement', { skip: !existsSync(chromePath) && 'Chromium unavailable; set CHROME', timeout: 90000 }, async t => {
  const profile = mkdtempSync(join(tmpdir(), 'archo-choices-'));
  const server = createServer((req, res) => { res.setHeader('content-type', 'text/html; charset=utf-8'); res.end(fixture); });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const browser = spawn(chromePath, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-sync', '--disable-background-networking', '--disable-component-update', '--disable-extensions', '--remote-debugging-port=0', '--user-data-dir=' + profile, 'about:blank'], { windowsHide: true, stdio: 'ignore' });
  let c;
  try {
    const portFile = join(profile, 'DevToolsActivePort');
    for (let i=0;i<100 && !existsSync(portFile);i++) await sleep(100);
    assert.ok(existsSync(portFile), 'Chromium started');
    const port = readFileSync(portFile,'utf8').split('\n')[0];
    c = await connect('127.0.0.1:' + port);
    await c.call('Emulation.setDeviceMetricsOverride', { width: 800, height: 600, deviceScaleFactor: 1, mobile: false });
    await c.call('Page.navigate', { url: 'http://127.0.0.1:' + server.address().port });
    for(let i=0;i<100 && !await c.evaluate('window.ready === true');i++) await sleep(30);
    const state = () => c.evaluate(`({value:b.value, expanded:b.getAttribute('aria-expanded'), active:document.getElementById(b.getAttribute('aria-activedescendant'))?.textContent, focus:document.activeElement===b, menus:document.querySelectorAll('.choice-menu').length, events})`);
    const reset = async () => { await c.evaluate('reset();b.focus()'); };

    await t.test('programmatic values stay silent; commits bubble input/change once', async () => {
      await reset(); await c.evaluate('b.value="zh"'); assert.deepEqual((await state()).events, []);
      await c.key('Enter'); await c.key('Home'); assert.equal((await state()).value, 'zh');
      await c.key('Enter'); assert.deepEqual((await state()).events, [['input','auto',true],['change','auto',true]]);
      assert.equal((await state()).menus,0); assert.equal((await state()).focus,true);
      await c.key('Enter'); await c.key('Enter'); assert.equal((await state()).events.length,2);
      await c.evaluate('b.value="missing"'); assert.equal((await state()).value,'');
    });
    await t.test('space/down open; arrows/home/end navigate without saving; Escape restores focus', async () => {
      await reset(); await c.key('Space'); assert.equal((await state()).expanded,'true');
      await c.key('ArrowDown'); assert.match((await state()).active,/简体中文/);
      await c.key('End'); assert.match((await state()).active,/简体中文/); // skip disabled last option
      await c.key('Home'); assert.match((await state()).active,/System/);
      await c.key('Escape'); assert.equal((await state()).value,'en'); assert.equal((await state()).focus,true); assert.deepEqual((await state()).events,[]);
      await c.key('ArrowDown'); assert.equal((await state()).expanded,'true'); await c.key('ArrowUp'); assert.match((await state()).active,/System/);
    });
    await t.test('typeahead jumps to English and accepts only on Enter', async () => {
      await reset(); await c.key('KeyS'); assert.match((await state()).active,/System/); assert.equal((await state()).value,'en');
      await c.key('Enter'); assert.equal((await state()).value,'auto');
    });
    await t.test('mouse selection saves once, and empty or fully disabled menus stay closed', async () => {
      await reset(); await c.key('Enter'); await c.click('.choice-option:first-child');
      assert.equal((await state()).value,'auto'); assert.equal((await state()).events.length,2);
      await c.evaluate('window.empty=choiceControl("Empty",[],"");document.querySelector("#host").append(empty);empty.click()');
      assert.equal((await state()).menus,0);
      await c.evaluate('window.none=choiceControl("Disabled",[{value:"x",label:"X",disabled:true}],"x");document.querySelector("#host").append(none);none.click()');
      assert.equal((await state()).menus,0);
    });
    await t.test('switch preserves translated off/on order and arrow/space values', async () => {
      await reset();
      assert.deepEqual(await c.evaluate('[...s.children].map(x=>x.textContent)'), ['关','开']);
      assert.deepEqual(await c.evaluate(`[...choiceControl('Checks',[{value:'on',label:'on'},{value:'off',label:'off'}],'on').children].map(x=>x.textContent)`), ['Off','On']);
      await c.evaluate('s.focus()'); await c.key('ArrowLeft'); assert.equal(await c.evaluate('s.value'),'off');
      await c.key('ArrowLeft'); assert.equal(await c.evaluate('events.length'),2);
      await c.key('ArrowRight'); await c.key('Space'); assert.equal(await c.evaluate('s.value'),'off');
      assert.equal(await c.evaluate('s.getAttribute("aria-checked")'),'false');
      await c.click('.choice-part[data-value="on"]'); assert.equal(await c.evaluate('s.value'),'on');
    });
    await t.test('disabled controls and fieldsets refuse input; disabling an open control closes it', async () => {
      await reset(); await c.evaluate('s.disabled=true;s.click();b.disabled=true;b.click()'); assert.equal((await state()).menus,0);
      await c.evaluate('b.disabled=false;document.querySelector("#fieldset").append(b);document.querySelector("#fieldset").disabled=true;b.click()'); assert.equal((await state()).menus,0);
      await c.evaluate('document.querySelector("#fieldset").disabled=false;b.click();b.disabled=true');
      assert.equal((await state()).menus,0);
    });
    await t.test('click outside closes; only one choice/context menu can be open', async () => {
      await reset(); await c.key('Enter'); await c.click('#outside'); assert.equal((await state()).menus,0);
      await c.evaluate('window.other=choiceControl("Scale",["100","50"],"100");document.querySelector("#host").append(other);b.click();other.click()');
      assert.equal((await state()).menus,1); assert.equal((await state()).expanded,'false');
      await c.evaluate('document.querySelector("#menu").hidden=false;b.click()');
      assert.equal(await c.evaluate('document.querySelector("#menu").hidden'),true);
    });
    await t.test('Tab closes and reaches the next control; page hotkeys do not consume local keys', async () => {
      await reset(); await c.evaluate('window.leaked=0;document.onkeydown=()=>leaked++');
      await c.key('Enter'); await c.key('Tab'); assert.equal((await state()).menus,0);
      assert.equal(await c.evaluate('document.activeElement===s'),true); assert.equal(await c.evaluate('leaked'),0);
      await c.evaluate('document.onkeydown=null');
    });
    await t.test('menu escapes clipping, opens upward at the bottom and stays inside right edge', async () => {
      await reset(); await c.evaluate('b.style.position="fixed";b.style.right="0";b.style.bottom="5px";b.click()');
      const r = await c.evaluate(`(()=>{const m=document.querySelector('.choice-menu').getBoundingClientRect(),a=b.getBoundingClientRect();return {top:m.top,bottom:m.bottom,right:m.right,triggerTop:a.top,hit:document.elementFromPoint(m.left+15,m.top+15)?.closest('.choice-menu')!==null}})()`);
      assert.ok(r.top>=8 && r.bottom<r.triggerTop && r.right<=792); assert.equal(r.hit,true);
    });
    await t.test('long menu scrolls internally; page scroll, resize, removal release the menu', async () => {
      await reset(); await c.evaluate('window.long=choiceControl("Many",Array.from({length:100},(_,i)=>"Option "+i),"Option 0");document.querySelector("#host").append(long);long.click()');
      assert.equal(await c.evaluate('(()=>{const e=document.querySelector(".choice-options");return e.scrollHeight>e.clientHeight})()'),true);
      await c.evaluate('long.focus()'); await c.key('End'); await sleep(40);
      assert.equal((await state()).menus,1);
      assert.match(await c.evaluate('document.getElementById(long.getAttribute("aria-activedescendant")).textContent'),/Option 99/);
      await c.evaluate('document.querySelector(".choice-options").scrollTop=200'); await sleep(40); assert.equal((await state()).menus,1);
      await c.evaluate('document.dispatchEvent(new Event("scroll"))'); assert.equal((await state()).menus,0);
      await c.evaluate('b.click();window.dispatchEvent(new Event("resize"))'); assert.equal((await state()).menus,0);
      await c.evaluate('b.click();b.remove()'); assert.equal((await state()).menus,0);
    });
    await t.test('375/800 px in light/dark: concentric corners, centered SVG and no overflow', async () => {
      for (const width of [375,800]) {
        await c.call('Emulation.setDeviceMetricsOverride', { width,height:650,deviceScaleFactor:1,mobile:false });
        await sleep(40);
        for (const theme of ['light','dark']) {
          await reset(); await c.evaluate(`document.documentElement.dataset.scheme='${theme}';document.querySelector('#clip').style.width='100%';b.click()`);
          const r=await c.evaluate(`(()=>{
            const menu=document.querySelector('.choice-menu'), option=menu.querySelector('[role=option]');
            const outer=getComputedStyle(s),inner=getComputedStyle(s.querySelector('.on')),panel=getComputedStyle(menu),item=getComputedStyle(option);
            const btn=b.getBoundingClientRect(),arrow=b.querySelector('.choice-arrow svg').getBoundingClientRect();
            return {overflow:document.documentElement.scrollWidth>innerWidth,role:b.getAttribute('role'),title:menu.querySelector('.choice-title').textContent,selected:menu.querySelector('[aria-selected=true]').textContent,
              outer:parseFloat(outer.borderRadius),inner:parseFloat(inner.borderRadius),inset:parseFloat(outer.paddingLeft),menu:parseFloat(panel.borderRadius),option:parseFloat(item.borderRadius),menuInset:parseFloat(panel.paddingLeft),
              arrowOffset:Math.abs((arrow.top+arrow.bottom-btn.top-btn.bottom)/2),svgCount:b.querySelectorAll('.choice-arrow svg').length,arrowText:b.querySelector('.choice-arrow').textContent};
          })()`);
          assert.equal(r.overflow,false,`${width}/${theme}`); assert.equal(r.role,'combobox'); assert.equal(r.title,'Language'); assert.match(r.selected,/✓English/);
          assert.equal(r.outer,12); assert.equal(r.inner,r.outer-r.inset); assert.equal(r.menu,12); assert.equal(r.option,r.menu-r.menuInset);
          assert.equal(r.svgCount,1); assert.equal(r.arrowText,''); assert.ok(r.arrowOffset<0.5,`SVG center offset ${r.arrowOffset}`);
          await c.key('Escape');
          const offset=await c.evaluate(`(()=>{const a=b.querySelector('.choice-arrow svg').getBoundingClientRect(),r=b.getBoundingClientRect();return Math.abs((a.top+a.bottom-r.top-r.bottom)/2)})()`);
          assert.ok(offset<0.5,'closed SVG remains centered');
        }
      }
      await c.call('Emulation.setDeviceMetricsOverride', { width:800,height:600,deviceScaleFactor:1,mobile:false });
      await sleep(80);
    });
    await t.test('reference numbers reach the switch, trigger and floating options', async () => {
      await reset(); await c.evaluate('b.click();toggleRefNums()'); await sleep(60);
      const refs=await c.evaluate(`(()=>{const ids=[...document.querySelectorAll('.refnum > span')].map(x=>x.textContent);const targets=ids.map(id=>window.refnum(id));return {switch:targets.includes(s),trigger:targets.includes(b),option:targets.some(x=>x?.getAttribute('role')==='option')}})()`);
      assert.deepEqual(refs,{switch:true,trigger:true,option:true});
      const before=await c.evaluate(`[...document.querySelectorAll('.refnum > span')].find(x=>window.refnum(x.textContent)===b).textContent`);
      await c.evaluate('reset();b.click()'); await sleep(60);
      const after=await c.evaluate(`[...document.querySelectorAll('.refnum > span')].find(x=>window.refnum(x.textContent)===b).textContent`);
      assert.equal(after,before,'redraw keeps the control reference number');
      await c.evaluate('toggleRefNums()');
    });
    await t.test('100 open/close cycles release floating DOM and do not multiply events', async () => {
      await reset(); await c.evaluate('for(let i=0;i<100;i++){b.click();closeChoiceMenu()}b.click()');
      assert.equal((await state()).menus,1); await c.key('Home'); await c.key('Enter');
      assert.equal((await state()).events.length,2); assert.equal((await state()).menus,0);
    });
  } finally {
    c?.close();
    const exited=once(browser,'exit'); browser.kill(); await Promise.race([exited,sleep(3000)]);
    server.closeAllConnections(); server.close();
    for(let i=0;i<10;i++) { try { rmSync(profile,{recursive:true,force:true}); break; } catch(e) { if(i===9) throw e; await sleep(100); } }
  }
});
