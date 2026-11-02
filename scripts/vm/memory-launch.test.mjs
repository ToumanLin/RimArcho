import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';
const source = name => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
function harness() {
 const document = { activeElement:null };
 function el(tag, cls='', text='') {
  const n={tag,className:cls,textContent:text,children:[],dataset:{},setAttribute(){},append(...a){this.children.push(...a)},prepend(...a){this.children.unshift(...a)},focus(){document.activeElement=this},click(){return this.onclick?.()}};
  n.classList={add(name){n.className+=' '+name},remove(name){n.className=n.className.split(' ').filter(x=>x!==name).join(' ')},toggle(name,on){(on??!n.className.split(' ').includes(name))?n.classList.add(name):n.classList.remove(name)},contains(name){return n.className.split(' ').includes(name)}};
  Object.defineProperty(n,'innerHTML',{set(){n.children=[]}});
  return n;
 }
 const box=el('div'), S={screen:'home',busy:{},envSeq:1}, calls=[], dialogs=[];
 const row={kind:'plan',current:true,plan:1,envName:'fixture',memory:{token:'one',assessment:{ramSeverity:'severe'}}};
 let memory={env:'fixture',plan:1,token:'one',assessment:{estimated:true,ramSeverity:'severe'},needed:'6–8 GiB',available:'2 GiB'};
 const ctx=createContext({S,document,el,calls,$:()=>box,T:x=>x,Tf:x=>x,clearTimeout(){},setTimeout(){return 1},say:(...a)=>calls.push(a),errText:String,closeChoiceMenu(){},api:{HomeMemory:async()=>memory},launchPack:async()=>calls.push('pack'),console});
 runInContext('(function(){'+source('popups')+'\nglobalThis.pop={memoryConfirm,popupKey,initPopups};})()',ctx);
 ctx.pop.initPopups({renderWhere(){},renderHeads(){}});
 runInContext('(function(){ const memoryConfirm=(m,go)=>dialogs.push({m,go});'+source('home')+'\nglobalThis.homeTest={launch,refreshHomeMemory,launchStep,setRow(r){data={rows:[r],featured:0};return data}}; draw=()=>{}; home=async()=>{}; ui={run:async()=>{calls.push("launch")}}; })()',Object.assign(ctx,{dialogs}));
 const data=ctx.homeTest.setRow(row);
 return {ctx,S,row,data,calls,dialogs,box,el,setMemory:m=>memory=m};
}
const all=n=>[n,...n.children.flatMap(all)];
test('red needs confirmation; default Enter and Escape cancel, Tab then Enter approves once',()=>{
 const h=harness();let launches=0;
 h.ctx.pop.memoryConfirm({needed:'6–8 GiB',available:'2 GiB'},()=>launches++);
 let bs=all(h.box).filter(x=>x.tag==='button');
 assert.equal(h.ctx.document.activeElement,bs[0]);
 h.ctx.pop.popupKey({key:'Enter',preventDefault(){}});
 assert.equal(launches,0);assert.equal(h.S.popup,null);
 h.ctx.pop.memoryConfirm({needed:'6–8 GiB',available:'2 GiB'},()=>launches++);
 h.ctx.pop.popupKey({key:'Escape'});assert.equal(launches,0);
 h.ctx.pop.memoryConfirm({needed:'6–8 GiB',available:'2 GiB'},()=>launches++);
 bs=all(h.box).filter(x=>x.tag==='button');
 h.ctx.pop.popupKey({key:'Tab',preventDefault(){}});
 assert.equal(h.ctx.document.activeElement,bs[1]);
 h.ctx.pop.popupKey({key:'Enter',preventDefault(){}});bs[1].click();assert.equal(launches,1);
});
test('yellow, normal and unknown launch directly; only RAM severe asks',async()=>{
 for(const severity of ['warning','ok','unknown','severe']) {
  const h=harness();h.setMemory({env:'fixture',plan:1,token:'one',assessment:{ramSeverity:severity,vramSeverity:'warning'}});
  await h.ctx.homeTest.launch(h.row);
  assert.equal(h.dialogs.length,severity==='severe'?1:0,severity);
  assert.equal(h.calls.filter(x=>x==='launch').length,severity==='severe'?0:1,severity);
 }
});
test('confirmation cannot launch a changed list or a different environment',async()=>{
 for(const change of ['list','env']) {
  const h=harness();await h.ctx.homeTest.launch(h.row);assert.equal(h.dialogs.length,1);
  if(change==='list')h.setMemory({env:'fixture',plan:1,token:'two',assessment:{ramSeverity:'severe'}});else h.S.envSeq++;
  await h.dialogs[0].go();assert.ok(!h.calls.includes('launch'));
 }
});
test('late memory response cannot color a superseding page',async()=>{
 const h=harness();let resolve;
 h.ctx.api.HomeMemory=()=>new Promise(r=>resolve=r);
 const pending=h.ctx.homeTest.refreshHomeMemory(h.data);h.S.envSeq++;
 resolve({env:'fixture',plan:1,token:'late',assessment:{ramSeverity:'severe',estimated:true}});
 await pending;assert.equal(h.row.memory.token,'one');
});
