import {readFileSync} from 'node:fs';
import {createContext,runInContext} from 'node:vm';
import {test} from 'node:test';
import assert from 'node:assert/strict';

test('log resize measurements defer layout writes and coalesce scroll anchoring',()=>{
  const frames=new Map(),observers=[],nodes=[];let next=0,delivering=false,top=100;
  function element(){const e={dataset:{},children:[],classList:{toggle(){}},append(c){this.children.push(c)},remove(){},addEventListener(){},removeEventListener(){},getBoundingClientRect(){return {top:0,height:this.height||80}},height:80};e.style=new Proxy({},{set(o,k,v){assert.equal(delivering,false,'resize delivery must not write layout');o[k]=v;return true}});return e;}
  const scroll=element();scroll.clientTop=0;scroll.clientHeight=150;
  Object.defineProperty(scroll,'scrollTop',{get:()=>top,set:v=>{assert.equal(delivering,false,'resize delivery must not change scroll');top=v}});
  const context=createContext({document:{createElement:element},Float64Array,ResizeObserver:class{constructor(fn){observers.push(fn)}observe(){}unobserve(){}disconnect(){}},requestAnimationFrame:fn=>{frames.set(++next,fn);return next},cancelAnimationFrame:id=>frames.delete(id),scroll,renderRow:i=>nodes[i]=element()});
  const source=readFileSync(new URL('../../internal/surface/gui/assets/js/virtualrows.js',import.meta.url),'utf8').replace(/\bexport /g,'');
  runInContext(source+'\nvar rows=virtualRows(scroll,3,renderRow);',context);
  const spacer=scroll.children[0];spacer.getBoundingClientRect=()=>({top:-top,height:240});
  const flush=()=>{const work=[...frames.values()];frames.clear();work.forEach(fn=>fn())};flush();
  assert.equal(spacer.style.height,'240px');
  delivering=true;nodes[0].height=100;observers[0]([{target:nodes[0]}]);nodes[1].height=90;observers[0]([{target:nodes[1]}]);delivering=false;
  assert.equal(spacer.style.height,'240px');assert.equal(top,100);assert.equal(frames.size,1);
  // A player scroll between notification and frame must also survive correction.
  top=110;flush();assert.equal(spacer.style.height,'270px');assert.equal(top,130);assert.equal(nodes[1].style.top,'100px');
  delivering=true;nodes[0].height=120;observers[0]([{target:nodes[0]}]);delivering=false;
  runInContext('rows.destroy()',context);assert.equal(frames.size,0);flush();assert.equal(top,130);
});
