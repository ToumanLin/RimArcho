import {readFileSync} from 'node:fs';
import {createContext, runInContext} from 'node:vm';
import {test} from 'node:test';
import assert from 'node:assert/strict';

function fixture() {
  const document = {activeElement: null};
  class Element {
    constructor(tag, cls='', text='') { this.tag=tag; this.className=cls; this.textContent=text; this.children=[]; this.dataset={}; this.classList={add: c=>this.className+=' '+c}; this.scrollTop=0; this.clientHeight=100; }
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
  const ctx=createContext({S,document,el:(...args)=>new Element(...args),$:()=>box,T:s=>s,api:{ProblemDetail:()=>pending},say:(...x)=>calls.push(x),errText:String});
  const src=readFileSync(new URL('../../internal/surface/gui/assets/js/popups.js',import.meta.url),'utf8').replace(/^import .*;\r?$/gm,'').replace(/\bexport /g,'');
  runInContext(src,ctx);
  ctx.initPopups({renderWhere(){},renderHeads(){}});
  return {ctx,box,S,document,resolve,reject,calls};
}
const detail={title:'3 things cannot load',meaning:'1 health condition; 2 pawn kinds',notes:['16% (4.1% by chance)'],affected:['Illness needs Base'],evidence:['Defs/Child.xml:1'],actions:[{kind:'enable',label:'enable Provider',refs:['#1234']},{kind:'open-url',label:'check requirements'}]};

test('focused panel exposes fixes as keyboard accessible buttons and closes before acting',async()=>{
  const f=fixture(), actions=[];
  const done=f.ctx.problem('p:123456',x=>actions.push(x),a=>a.kind==='enable'?'enable':'');
  assert.equal(f.S.popup.kind,'problem');
  f.resolve(detail); await done;
  const buttons=f.box.querySelectorAll('button');
  assert.equal(buttons.length,3);
  assert.equal(buttons[0].dataset.envWrite,'enable');
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
