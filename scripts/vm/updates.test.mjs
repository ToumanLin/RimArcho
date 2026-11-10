// Synthetic Updates / add-subscription pages and exact-copy library navigation in Chromium.
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
const jsRoot = new URL('../../internal/surface/gui/assets/js/', import.meta.url);
const css = readFileSync(new URL('../../internal/surface/gui/assets/style.css', import.meta.url), 'utf8');
const stubs = {
 'state.js': `export const S={screen:'changes',screenSeq:0,envSeq:0,busy:{}};
 export const $=s=>document.querySelector(s);
 export const el=(tag,cls='',text='')=>{const e=document.createElement(tag);e.className=cls;e.textContent=text;return e;};
 export const keyCaps=k=>el('kbd','',k);
 export const T=s=>s, Tf=(s,...a)=>{let i=0;return s.replace(/%[sd]/g,()=>a[i++]);};
 export const errText=e=>e.message, say=(...a)=>window.messages.push(a);
 export const api=new Proxy({},{get:(_,name)=>(...a)=>window.api[name](...a)});`,
 'api.js': `export const on=()=>{};`,
 'fields.js': `export const pathContent=()=>{},editablePath=()=>{};`,
 'popups.js': `export const menuOpener=n=>n; export const confirm=(...a)=>window.confirmations.push(a);`,
 'separator.js': `export const wireSeparator=()=>{};`,
 'detail.js': `export function detailFrame(box,sc,o={}){return {box,head(){},inst(i){box.textContent=i.label;},fields(){},detail(){},error(){},fetching(){},settle:()=>Promise.resolve(),show(){o.mount?.();}};} export function noteEl(){return document.createElement('div');} export function stale(){}`,
 'filterbar.js': `import {el} from './state.js';export function createBar(){const input=el('input'),root=el('div','fbar'),row=el('div','fbar-row');row.append(input);root.append(row);return {el:root,input,search:'',filters:[],draw(){},focus(){input.focus();}};}`,
 'ops.js': `export const setTable=()=>{}, sorted=x=>x;`,
 'screens.js': `import {S,el,$} from './state.js';
 export function openScreen(name){S.screen=name;const s=$('#screen');s.replaceChildren();return s;}
 export function beginPageRead(name){if(S.screen!==name){openScreen(name);S.screenSeq++;}return {current:()=>S.screen===name,finish(){},fail(e){throw e;}};}
 export const page=(cls,lead='')=>{const w=el('div','page '+cls);w.append(el('div','lead',lead));return w;};
 export const area=()=>{},full=()=>{},pageLoading=()=>false;
 export function button(label,key,run,cls=''){const b=el('button','btn '+cls,label);if(key)b.append(el('kbd','',key));b.onclick=e=>{e.stopPropagation();run();};return b;}
 export const act=(label,ic,run,key)=>button(label,key,run),rescanButton=()=>button('rescan','R',()=>{});`,
};
const html = `<!doctype html><meta charset="utf-8"><style>${css}
body{overflow:auto}#screen{height:720px;width:100%}.page{min-height:0}.lead:empty{display:none}</style><main id="screen"></main>
<script type="module">
import {S,el,api} from '/js/state.js';
import * as updates from '/js/subscriptions.js';
import * as library from '/js/library.js';
import * as helpers from '/js/screens.js';
window.calls=[];window.messages=[];window.opened=[];window.confirmations=[];
window.data={summary:{line:'checked 4 sources, the last at 10:42, 1 check failed'},sources:[
{key:'workshop',name:'Steam Workshop',count:2,checked:'10:42',result:'1 check failed',failures:[{label:'connection timed out',items:['Network Mod: connection timed out']}]},
{key:'steamcmd',name:'SteamCMD',count:1,checked:'10:42',result:'everything is up to date',account:'not logged in'},
{key:'git',name:'Git',count:1,checked:'10:42',result:'everything is up to date'},
{key:'page',name:'LoversLab',count:1,checked:'10:42',result:'everything is up to date',account:'logged in'},
{key:'local',name:'Local',count:0,result:'not checked yet'}],
pending:[{id:'copy-b',ref:'#second',name:'Same name',kind:'git',source:'Git',installed:'old',available:'new',inPlan:true}],
gone:[{id:'copy-a',ref:'#first',name:'Same name',inPlan:true}],recent:[
{id:'copy-a',name:'Same name',source:'Git',added:'today'},{id:'copy-b',name:'Same name',source:'Git',added:'yesterday'}],
history:{rows:[],arrivals:{text:'1 new mod: Same name',rows:[{id:'copy-b',mod:'Same name',text:'found in the Mods folder',present:true}]},older:[]}};
const copies=[{id:'copy-a',ref:'#first',label:'Workshop',name:'Workshop',source:'Workshop'},
{id:'copy-b',ref:'#second',label:'locked version',name:'locked version',source:'locked version'}];
window.api={
SubscriptionsPage:async()=>{calls.push(['SubscriptionsPage']);return data;}, AddSubscriptionInfo:async()=>data,
CheckSubscriptionSource:async key=>{calls.push(['check',key]);return 'queued';}, Setting:async()=>'',SetSetting:async()=>{},
Library:async(query,filters)=>{calls.push(['Library',query,filters]);return {lead:'',total:1,shown:query?0:1,size:'1 MB',opTable:[],columns:[{key:'name',title:'Name',shown:true},{key:'source',title:'Source',shown:true}],rows:query?[]:[{id:'family',name:'Same name',copies:2,insts:copies}]};},
LibraryFields:async()=>[],Detail:async id=>{calls.push(['Detail',id]);return {};},
LibraryPreview:async(code,refs)=>{calls.push(['preview',code,refs]);return {apply:true,lines:['dry run']};},
};
const ui={renderHints(){},renderWhere(){},go:async name=>{if(name==='changes')await updates.changes();},reload:async()=>{}};
updates.initSubscriptions(ui,{...helpers,seg:()=>el('div')});library.initLibrary(ui);
document.addEventListener('library-instance',async e=>{opened.push(e.detail);await library.openInstance(e.detail);});
document.addEventListener('keydown',e=>{if(S.screen==='changes')updates.changesKey(e);});
window.reset=async()=>{updates.changesPlan();S.screen='other';calls.length=0;opened.length=0;confirmations.length=0;await updates.changes();};
window.add=async()=>{S.screen='subscribe';await updates.openAdd();};
window.filteredLibrary=()=>library.openSearch('hidden');
await reset();window.ready=true;
</script>`;

test('Updates: native keyboard, detail placement and exact-copy library links', {skip:!existsSync(chromePath)&&'Chromium unavailable; set CHROME',timeout:90000},async t=>{
 const profile=mkdtempSync(join(tmpdir(),'archo-updates-'));
 const server=createServer((req,res)=>{
  try{
   if(req.url.startsWith('/js/')){const name=req.url.slice(4);res.setHeader('content-type','text/javascript; charset=utf-8');res.end(stubs[name]??readFileSync(new URL(name,jsRoot),'utf8'));}
   else{res.setHeader('content-type','text/html; charset=utf-8');res.end(html);}
  }catch(e){res.statusCode=500;res.end(String(e));}
 });
 server.listen(0,'127.0.0.1');await once(server,'listening');
 const browser=spawn(chromePath,['--headless=new','--no-first-run','--no-default-browser-check','--disable-sync','--disable-background-networking','--disable-component-update','--disable-extensions','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{windowsHide:true,stdio:'ignore'});
 let c;
 try{
  const portFile=join(profile,'DevToolsActivePort');for(let i=0;i<100&&!existsSync(portFile);i++)await sleep(100);
  assert.ok(existsSync(portFile),'Chromium started');c=await connect('127.0.0.1:'+readFileSync(portFile,'utf8').split('\n')[0]);
  await c.call('Emulation.setDeviceMetricsOverride',{width:1200,height:800,deviceScaleFactor:1,mobile:false});
  await c.call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port});
  for(let i=0;i<100&&!await c.evaluate('window.ready===true');i++)await sleep(30);
  assert.equal(await c.evaluate('window.ready'),true);
  // Native button activation needs the translated keypress; the shared driver's raw keys only send keydown/up.
  const enter=async()=>{
   await c.call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r',unmodifiedText:'\r'});
   await c.call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
  };
  await t.test('arrows select checks without a read; Enter focuses a check, then activates it',async()=>{
   await c.evaluate('reset()');await c.evaluate('document.querySelector(".subs-row").focus()');await c.key('End');
   assert.equal(await c.evaluate('document.querySelector(".subs-detail h2").textContent'),'Update checks');
   assert.equal(await c.evaluate('document.querySelectorAll(".subs-list .subs-sources,.subs-failed").length'),0);
   assert.equal(await c.evaluate('document.querySelectorAll(".subs-detail .subs-src").length'),5);
   assert.equal(await c.evaluate('document.querySelectorAll(".acct").length'),2);
   assert.match(await c.evaluate('document.querySelector(".subs-detail").textContent'),/Network Mod: connection timed out/);
   assert.deepEqual(await c.evaluate('calls'),[['SubscriptionsPage']]);
   await enter();assert.equal(await c.evaluate('document.activeElement.closest(".subs-src").querySelector("b").textContent'),'Steam Workshop');
   await enter();assert.deepEqual(await c.evaluate('calls.at(-1)'),['check','workshop']);
  });
  await t.test('gone row previews locking and uses the exact instance for its library action',async()=>{
   await c.evaluate('reset()');await c.click('[data-key="gone:copy-a"]');
   assert.equal(await c.evaluate('document.querySelectorAll(".subs-gone p").length'),2);
   await c.key('KeyP');assert.deepEqual(await c.evaluate('calls.at(-1)'),['preview','pin',['#first']]);
   assert.equal(await c.evaluate('confirmations.length'),1);
   await c.key('KeyL');
   assert.equal(await c.evaluate('document.querySelector(".lib-row.cur").textContent.includes("Workshop")'),true);
   assert.deepEqual(await c.evaluate('opened'),['copy-a']);
  });
  await t.test('recent names are reached by Tab and Enter selects the second same-name version',async()=>{
   await c.evaluate('filteredLibrary()');await c.evaluate('add()');
   await c.evaluate('document.querySelector(".subscribe-recent button").focus()');await c.key('Tab');
   assert.equal(await c.evaluate('document.activeElement===document.querySelectorAll(".subscribe-recent button")[1]'),true);
   await enter();
   assert.deepEqual(await c.evaluate('opened'),['copy-a','copy-b']);
   assert.equal(await c.evaluate('document.querySelector(".lib-row.cur").textContent.includes("locked version")'),true);
   assert.equal(await c.evaluate('document.querySelectorAll(".lib-row.sel").length'),1);
   assert.deepEqual(await c.evaluate('calls.filter(x=>x[0]==="Library").at(-1)'),['Library','',[]]);
  });
  await t.test('arrival detail link and pending library shortcut select the same exact copy',async()=>{
   await c.evaluate('reset()');await c.click('[data-key="g:now"]');
   assert.equal(await c.evaluate(`document.querySelector('[data-key="g:now"] .ri').textContent`),'');
   await enter();await enter();
   assert.equal(await c.evaluate('document.querySelector(".lib-row.cur").textContent.includes("locked version")'),true);
   await c.evaluate('reset()');await c.click('[data-key="p:#second"]');await c.key('KeyL');
   assert.deepEqual(await c.evaluate('opened'),['copy-b']);
  });
  await t.test('summary and source rows stay inside their cards at 375 and 1200 pixels',async()=>{
   for(const width of [375,1200]){
    await c.call('Emulation.setDeviceMetricsOverride',{width,height:800,deviceScaleFactor:1,mobile:false});
    await c.evaluate('reset()');await c.click('.subs-srcsum');
    const layout=await c.evaluate(`(()=>{const d=document.querySelector('.subs-detail').getBoundingClientRect();return {inline:document.querySelector('.subs-list .subs-sources')!==null,over:[...document.querySelectorAll('.subs-src button')].some(b=>b.getBoundingClientRect().right>d.right+1),summary:document.querySelector('.subs-srcsum').scrollWidth>document.querySelector('.subs-srcsum').clientWidth+1};})()`);
    assert.deepEqual(layout,{inline:false,over:false,summary:false});
   }
  });
 }finally{c?.close();browser.kill();await once(browser,'exit').catch(()=>{});server.close();rmSync(profile,{recursive:true,force:true,maxRetries:20,retryDelay:100});}
});
