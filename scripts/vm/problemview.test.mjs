import {readFileSync} from 'node:fs';
import {createContext, runInContext} from 'node:vm';
import {test} from 'node:test';
import assert from 'node:assert/strict';

function fixture(overrides={}) {
  const document = {activeElement: null};
  class Element {
    constructor(tag, cls='', text='') { this.tag=tag; this.className=cls; this.textContent=text; this.children=[]; this.dataset={}; this.classList={add: c=>this.className+=' '+c,remove:c=>this.className=this.className.split(' ').filter(x=>x!==c).join(' ')}; this.scrollTop=0; this.clientHeight=100; }
    append(...children) { this.children.push(...children); }
    set innerHTML(value) { assert.equal(value,''); this.children=[]; }
    querySelectorAll(tag) { return this.children.flatMap(x=>typeof x==='string'?[]:[...(x.tag===tag?[x]:[]),...x.querySelectorAll(tag)]); }
    focus() { document.activeElement=this; }
    click() { this.onclick?.(); }
  }
  const box=new Element('div');
  const S={popup:null,envSeq:0};
  let resolve, reject;
  const pending=new Promise((a,b)=>{resolve=a;reject=b;});
  const calls=[];
  const ctx=createContext({S,document,el:(...args)=>new Element(...args),$:()=>box,T:s=>s,api:{ProblemDetail:()=>pending,...overrides},say:(...x)=>calls.push(x),errText:String});
  const src=readFileSync(new URL('../../internal/surface/gui/assets/js/popups.js',import.meta.url),'utf8').replace(/^import .*;\r?$/gm,'').replace(/\bexport /g,'');
  runInContext(src,ctx);
  ctx.initPopups({renderWhere(){},renderHeads(){}});
  return {ctx,box,S,document,resolve,reject,calls};
}
const detail={title:'3 things cannot load',meaning:'1 health condition; 2 pawn kinds',notes:['16% (4.1% by chance)'],affected:['Illness needs Base'],locations:['Defs/Child.xml:1'],evidence:[],primary:true,actions:[{kind:'enable',label:'enable Provider',refs:['#1234']},{kind:'open-url',label:'check requirements'}]};

test('focused panel exposes fixes as keyboard accessible buttons and closes before acting',async()=>{
  const f=fixture(), actions=[];
  const done=f.ctx.problem('p:123456',x=>actions.push(x),a=>a.kind==='enable'?'enable':'');
  assert.equal(f.S.popup.kind,'problem');
  f.resolve(detail); await done;
  const buttons=f.box.querySelectorAll('button');
  assert.equal(buttons.length,3);
  assert.equal(buttons[0].dataset.envWrite,'enable');
  assert.equal(buttons[0].className,'go');
  assert.equal(buttons[1].className,'');
  const locations=f.box.querySelectorAll('span');
  assert.equal(locations[0].className,'muted');
  assert.equal(locations[0].textContent,'  Defs/Child.xml:1');
  assert.equal(f.box.querySelectorAll('h3').length,1);
  assert.equal(f.document.activeElement,buttons[0]);
  const key={key:'Tab',preventDefault(){}};
  f.S.popup.keys(key);assert.equal(f.document.activeElement,buttons[1]);
  f.S.popup.keys({key:'Enter',preventDefault(){}});
  assert.equal(actions[0].kind,'open-url');assert.equal(f.S.popup,null);
});

test('late problem reads cannot reopen a closed or superseded panel',async()=>{
  for(const change of ['close','supersede','environment']) {
    const f=fixture();const done=f.ctx.problem('p:123456',()=>{},()=> '');
    if(change==='close')f.ctx.close();
    if(change==='supersede')f.ctx.open('help','help',()=>{});
    if(change==='environment')f.S.envSeq++;
    const before=f.S.popup;
    f.resolve(detail);await done;
    assert.equal(f.S.popup,before);
  }
});


test('installed provider search fills the open panel and cancels on close',async()=>{
  for(const mode of ['complete','close','supersede','environment']) {
    let finish;const pending=new Promise(resolve=>{finish=resolve;});
    const cancelled=[],lookups=[];
    const f=fixture({BeginProblemProviders:async()=>7,CancelProblemProviders:async id=>{cancelled.push(id);},ProblemProviders:(ref,id)=>{lookups.push([ref,id]);return pending;}});
    const done=f.ctx.problem('p:123456',()=>{},()=> '');
    f.resolve({...detail,searching:true,primary:false,actions:[detail.actions[1]]});
    await new Promise(setImmediate);
    assert.equal(lookups.length,1);
    assert.ok(f.box.querySelectorAll('p').some(p=>p.textContent==='searching installed mods…'));
    assert.equal(f.box.querySelectorAll('button')[0].className,'');
    if(mode==='close')f.ctx.close();
    if(mode==='supersede')f.ctx.open('help','help',()=>{});
    if(mode==='environment'){
      // Run the real switch entry point; cancel without relying on popup teardown.
      f.S.busy={};f.S.screenSeq=0;
      f.ctx.setEnvironmentGuard=()=>{};f.ctx.Tf=s=>s;
      f.ctx.MutationObserver=class {observe(){} disconnect(){}};
      f.document.body={querySelectorAll:()=>[],matches:()=>false};
      const src=readFileSync(new URL('../../internal/surface/gui/assets/js/environment.js',import.meta.url),'utf8').replace(/^import .*;\r?$/gm,'').replace(/\bexport /g,'');
      runInContext(src,f.ctx);
      await f.ctx.withEnvironmentSwitch('Other',async()=>{});
    }
    if(mode!=='complete')assert.deepEqual(cancelled,[7]);
    const before=f.S.popup;
    finish(detail);await done;
    if(mode==='complete') {
      assert.equal(f.box.querySelectorAll('button')[0].className,'go');
      assert.equal(f.box.querySelectorAll('button')[0].textContent,'enable Provider');
      assert.ok(!f.box.querySelectorAll('p').some(p=>p.textContent==='searching installed mods…'));
    } else assert.equal(f.S.popup,before);
  }
});

test('closing while cancellation registration is pending cancels before file IO',async()=>{
  let registered;const registration=new Promise(resolve=>{registered=resolve;});
  const cancelled=[];let searched=false;
  const f=fixture({BeginProblemProviders:()=>registration,CancelProblemProviders:async id=>{cancelled.push(id);},ProblemProviders:()=>{searched=true;}});
  const done=f.ctx.problem('p:123456',()=>{},()=> '');
  f.resolve({...detail,searching:true});
  await new Promise(setImmediate);
  f.ctx.close();registered(9);await done;
  assert.deepEqual(cancelled,[9]);assert.equal(searched,false);
});
