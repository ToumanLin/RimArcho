// Actual search/filter/viewer modules with synthetic streamed API data in Chromium.
import {existsSync,readFileSync,mkdtempSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {connect,sleep} from './cdp.mjs';

const chromePath=process.env.CHROME||'C:/Program Files/Google/Chrome/Application/chrome.exe';
const jsRoot=new URL('../../internal/surface/gui/assets/js/',import.meta.url);
const css=readFileSync(new URL('../../internal/surface/gui/assets/style.css',import.meta.url),'utf8');
const stubs={
 'state.js':`export const S={screen:'search',screenSeq:0,envSeq:0,busy:{}};
 export const $=s=>document.querySelector(s),el=(tag,cls='',text='')=>{const e=document.createElement(tag);e.className=cls;e.textContent=text;return e;};
 export const keyCaps=k=>el('kbd','',k),T=s=>s,Tf=(s,...a)=>{let i=0;return s.replace(/%[sd]/g,()=>a[i++]);};
 export const errText=e=>e.message,say=(...a)=>window.messages.push(a);
 export const api=new Proxy({},{get:(_,n)=>(...a)=>window.api[n](...a)});`,
 'fields.js':`import {el} from './state.js';export const copy=path=>window.calls.push(['copy',path]);export function copyablePath(node,path){const b=el('button','','copy');b.onclick=()=>copy(path);node.append(b);}`,
 'screens.js':`import {el} from './state.js';export const beginPageRead=()=>({current:()=>true,finish(){},fail(e){throw e;}}),page=cls=>el('div','page '+cls),area=()=>{},hintsChanged=()=>{};
 export function button(label,key,run,cls=''){const b=el('button','btn '+cls,label);if(key)b.append(el('kbd','',key));b.onclick=e=>{e.stopPropagation();run();};return b;}`
};
const html=`<!doctype html><meta charset="utf-8"><style>${css}
body{overflow:auto}#screen{height:720px;width:100%}</style><main id="screen"></main>
<script>window.errors=[];window.addEventListener('error',e=>errors.push(e.message));window.addEventListener('unhandledrejection',e=>errors.push(String(e.reason)));</script>
<script type="module">
import * as fs from '/js/filesearch.js';window.fs=fs;window.calls=[];window.messages=[];let request=0,stopped=false;
const groups=[{key:'lists',title:'lists',values:[{filter:'lists:"Current"',label:'Current',count:2,cur:true,on:1}]},{key:'tag',title:'tags',values:[{filter:'tag:test',label:'test',count:1}]},{key:'source',title:'library source',values:[{filter:'source:workshop',label:'Workshop',count:2}]},{key:'content',title:'content',values:[{filter:'content:xml',label:'XML',count:2}]}];
const file=(name,count=2)=>({rel:'Defs/'+name,path:'C:/synthetic/'+name,count,lines:[{line:1,text:'needle one',parts:[{text:'needle',hit:true},{text:' one'}]},{line:3,text:'needle two',parts:[{text:'needle',hit:true},{text:' two'}]}].slice(0,count)});
window.api={Setting:async()=>'',SetSetting:async(...a)=>calls.push(['setting',...a]),FileSearchInfo:async()=>({files:'Defs patches About',mods:'lists:"Current"',groups}),FileSearchFilters:async q=>({groups}),
 BeginFileSearch:async()=>{stopped=false;calls.push(['begin']);return ++request;},StartFileSearch:async(q,n)=>calls.push(['start',q,n]),StopFileSearch:async n=>{stopped=true;calls.push(['stop',n]);},
 FileSearchUpdate:async(n,since)=>{calls.push(['poll',since]);const deltas=since===0?[{id:'b',name:'Beta',copy:'Workshop',file:file('Beta.xml')}]:since===1?[{id:'a',name:'Alpha',copy:'local',file:file('Alpha.xml',1)},{id:'b',name:'Beta',copy:'Workshop',file:file('More.xml',1)}]:[];return {next:since+deltas.length,deltas,done:stopped||since>0,summary:since===0?'searching 1 of 2 mods · 2 matches so far':'4 matches in 3 files of 2 mods · 0.3 s',notes:[]};},
 ReadSearchFile:async(id,path)=>({path,type:'UTF-8 · XML',hits:[1,3],lines:[{number:1,parts:[{text:'<x>'},{text:'needle',hit:true},{text:'</x>'}]},{number:2,parts:[{text:'plain'}]},{number:3,parts:[{text:'needle',hit:true}]}]}),
 OpenSearchFile:async(...a)=>calls.push(['open',...a]),OpenSearchFolder:async(...a)=>calls.push(['folder',...a])};
window.set=(id,value)=>{const e=document.getElementById(id);e.value=value;e.dispatchEvent(new Event('input',{bubbles:true}));};
document.addEventListener('keydown',e=>fs.key(e));await fs.open();window.ready=true;
</script>`;

test('Search files: shared panel, streaming tree, viewer and keys',{skip:!existsSync(chromePath)&&'Chromium unavailable; set CHROME',timeout:90000},async t=>{
 const profile=mkdtempSync(join(tmpdir(),'archo-filesearch-'));
 const server=createServer((req,res)=>{try{if(req.url.startsWith('/js/')){const name=req.url.slice(4);res.setHeader('content-type','text/javascript; charset=utf-8');res.end(stubs[name]??readFileSync(new URL(name,jsRoot),'utf8'));}else{res.setHeader('content-type','text/html; charset=utf-8');res.end(html);}}catch(e){res.statusCode=500;res.end(String(e));}});
 server.listen(0,'127.0.0.1');await once(server,'listening');
 const browser=spawn(chromePath,['--headless=new','--no-first-run','--no-default-browser-check','--disable-sync','--disable-background-networking','--disable-component-update','--disable-extensions','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{windowsHide:true,stdio:'ignore'});
 let c;
 try{
  const portFile=join(profile,'DevToolsActivePort');for(let i=0;i<100&&!existsSync(portFile);i++)await sleep(100);
  assert.ok(existsSync(portFile));c=await connect('127.0.0.1:'+readFileSync(portFile,'utf8').split('\n')[0]);
  await c.call('Emulation.setDeviceMetricsOverride',{width:1440,height:800,deviceScaleFactor:1,mobile:false});
  await c.call('Page.navigate',{url:'http://127.0.0.1:'+server.address().port});
  const activate=async()=>{await c.call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'});await c.call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});};
  for(let i=0;i<100&&!await c.evaluate('window.ready===true');i++)await sleep(30);
  assert.equal(await c.evaluate('window.ready'),true,await c.evaluate('errors'));
  await t.test('three fields; unknown kind blocks start; one panel targets both inputs',async()=>{
   assert.deepEqual(await c.evaluate('[...document.querySelectorAll(".fs-query input")].map(x=>x.id)'),['fs-search','fs-files','fs-mods']);
   await c.evaluate(`set('fs-search','needle');set('fs-files','typo');fs.run()`);
   assert.equal(await c.evaluate('calls.some(x=>x[0]==="start")'),false);
   assert.match(await c.evaluate('document.querySelector(".fs-validation").textContent'),/typo/);
   await c.evaluate(`set('fs-search','');set('fs-files','Defs patches About')`);await sleep(130);
   await c.click('.fs-query .fbar-tog:not([hidden])');
   assert.deepEqual(await c.evaluate('[...document.querySelectorAll(".fbar-panel .fbar-gt")].map(x=>x.textContent)'),['files','lists','tags','library source','content']);
   await c.evaluate(`document.querySelectorAll('.fbar-panel .fbar-line')[0].querySelector('button').click()`);
   assert.match(await c.evaluate('document.querySelector("#fs-files").value'),/C#/);
   assert.equal(await c.evaluate('document.querySelector("#fs-mods").value'),'lists:"Current"');
   await c.evaluate(`document.querySelectorAll('.fbar-panel .fbar-line')[2].querySelector('button').click()`);await sleep(130);
   assert.match(await c.evaluate('document.querySelector("#fs-mods").value'),/tags:test/);
   await c.evaluate(`document.querySelectorAll('.fbar-panel .fbar-line')[0].querySelector('button').dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true}))`);
   assert.match(await c.evaluate('document.querySelector("#fs-files").value'),/-C#/);
  });
  await t.test('deltas preserve first-hit order and grow existing Mod; file expands and selects',async()=>{
   await c.evaluate(`set('fs-search','needle');fs.run()`);await sleep(150);
   assert.equal(await c.evaluate('document.querySelectorAll(".fs-mod").length'),1);
   assert.match(await c.evaluate('document.querySelector(".fs-summary").textContent'),/searching/);
   await sleep(350);
   assert.deepEqual(await c.evaluate('[...document.querySelectorAll(".fs-mod-name")].map(x=>x.textContent)'),['Beta','Alpha']);
   assert.equal(await c.evaluate('document.querySelectorAll(".fs-file").length'),3);
   assert.deepEqual(await c.evaluate('calls.filter(x=>x[0]==="poll").map(x=>x[1])'),[0,1]);
   await c.click('.fs-file');await sleep(50);
   assert.equal(await c.evaluate('document.querySelectorAll(".fs-line").length'),2,await c.evaluate('document.querySelector(".fs-tree").innerHTML'));
   assert.equal(await c.evaluate('document.querySelectorAll(".fs-code mark").length'),2);
   await c.click('.fs-file');await sleep(50);
   assert.equal(await c.evaluate('document.querySelectorAll(".fs-line").length'),0);
   await c.key('ArrowRight');await sleep(50);assert.equal(await c.evaluate('document.querySelectorAll(".fs-line").length'),2);
   await c.key('ArrowLeft');await sleep(50);assert.equal(await c.evaluate('document.querySelectorAll(".fs-line").length'),0);
  });
  await t.test('wrap persists; match navigation and open actions use selected copy',async()=>{
   await c.key('Alt+KeyZ');assert.equal(await c.evaluate('document.querySelector(".fs-code").classList.contains("wrap")'),true);
   assert.ok(await c.evaluate('calls.some(x=>x[0]==="setting"&&x[1]==="ui.gui.search.wrap"&&x[2]==="true")'));
   await c.key('KeyN');assert.equal(await c.evaluate('document.querySelector(".fs-match-position").textContent'),'match 2 of 2');
   await c.evaluate(`document.querySelector('.fs-view-foot button').focus()`);await activate();
   assert.equal(await c.evaluate('document.querySelector(".fs-match-position").textContent'),'match 1 of 2');
   assert.equal(await c.evaluate('calls.some(x=>x[0]==="open")'),false);
   await c.evaluate(`document.querySelector('.fs-view-actions button:last-child').focus()`);await activate();
   assert.ok(await c.evaluate('calls.some(x=>x[0]==="folder"&&x[1]==="b")'));
   assert.equal(await c.evaluate('calls.some(x=>x[0]==="open")'),false);
   await c.evaluate(`document.querySelector('.fs-tree').focus()`);
   await c.key('Enter');await c.key('Control+KeyO');await c.key('Control+KeyC');
   assert.ok(await c.evaluate('calls.some(x=>x[0]==="open"&&x[1]==="b")'));
   assert.ok(await c.evaluate('calls.some(x=>x[0]==="folder"&&x[1]==="b")'));
   if(process.env.ARCHO_FILESEARCH_SHOT)await c.shot(process.env.ARCHO_FILESEARCH_SHOT);
  });
  await t.test('large result trees and bounded viewers retain only viewport DOM',async()=>{
   await c.call('Emulation.setDeviceMetricsOverride',{width:1440,height:800,deviceScaleFactor:1,mobile:false});
   await c.evaluate(`api.FileSearchUpdate=async()=>({next:1354,done:true,summary:'1354 matches in 1354 files of 1354 mods · 1.0 s',notes:[],deltas:Array.from({length:1354},(_,i)=>({id:'large'+i,name:'Mod '+i,copy:'Workshop',file:{rel:'Defs/Test.xml',path:'C:/synthetic/'+i+'.xml',count:1,lines:[{line:1,text:'needle',parts:[{text:'needle',hit:true}]}]}}))});api.ReadSearchFile=async()=>({type:'UTF-8 · XML',hits:[1],lines:Array.from({length:10000},(_,i)=>({number:i+1,parts:[{text:i?'plain':'needle',hit:!i}]}))});set('fs-search','needle');fs.run()`);
   await sleep(100);
   assert.ok(await c.evaluate('document.querySelectorAll(".fs-tree-row").length<100'));
   assert.ok(await c.evaluate('document.querySelectorAll(".fs-code-line").length<120'));
   await c.evaluate('document.querySelector(".fs-tree").focus()');await c.key('End');await sleep(40);
   assert.ok(await c.evaluate('document.querySelector(".fs-tree").textContent.includes("Mod 1353")'));
   assert.ok(await c.evaluate('document.querySelectorAll(".fs-tree-row").length<100'));
  });
  await t.test('responsive split and input layout stay inside cards',async()=>{
   for(const width of [1440,1000,899,375]){
    await c.call('Emulation.setDeviceMetricsOverride',{width,height:800,deviceScaleFactor:1,mobile:false});await sleep(60);
    const layout=await c.evaluate(`(()=>{const q=document.querySelector('.fs-query').getBoundingClientRect(),v=document.querySelector('.fs-viewer').getBoundingClientRect(),l=document.querySelector('.fs-left').getBoundingClientRect();return {over:[...document.querySelectorAll('.fs-query input,.fs-toggle')].some(e=>e.getBoundingClientRect().right>q.right+1),stack:v.top>=l.bottom-1};})()`);
    assert.equal(layout.over,false,'query overflow at '+width);assert.equal(layout.stack,width<900);
   }
  });
  assert.deepEqual(await c.evaluate('errors'),[]);assert.deepEqual(await c.evaluate('messages'),[]);
 }finally{c?.close();browser.kill();await once(browser,'exit').catch(()=>{});server.close();rmSync(profile,{recursive:true,force:true,maxRetries:20,retryDelay:100});}
});
