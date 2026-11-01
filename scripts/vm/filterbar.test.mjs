import {readFileSync} from 'node:fs';
import {createContext, runInContext} from 'node:vm';
import {test} from 'node:test';
import assert from 'node:assert/strict';

const source = name => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8')
  .replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');

test('list suggestions and filter toggles preserve quoted names and other query terms', () => {
  const ctx = createContext({});
  runInContext(source('searchsuggest') + '\nglobalThis.queryAPI={suggestions,setQueryFilter};', ctx);
  const {suggestions, setQueryFilter} = ctx.queryAPI;
  const groups = [{key:'lists',title:'lists',values:[
    {filter:'lists:Main colony',label:'Main colony'},
    {filter:'lists:Solo',label:'Solo'},
    {filter:'lists:Empty list',label:'Empty list',count:0},
  ]}];
  const text = 'tags:"Vanilla Expanded" -lists:"ma';
  assert.deepEqual(Array.from(suggestions(groups,text,text.length), x=>x.token), ['-lists:"Main colony"']);
  assert.equal(suggestions(groups,'lists:so',8)[0].token, 'lists:"Solo"');
  assert.equal(suggestions(groups,'lists:em',8)[0].token, 'lists:"Empty list"');
  assert.equal(suggestions(groups,'li',2)[0].token, 'lists:');
  const added = setQueryFilter('tags:"Vanilla Expanded" source:workshop','lists:Main colony',1);
  assert.equal(added, 'tags:"Vanilla Expanded" source:workshop lists:"Main colony"');
  const excluded = setQueryFilter(added,'lists:Main colony',-1);
  assert.equal(excluded, 'tags:"Vanilla Expanded" source:workshop -lists:"Main colony"');
  assert.equal(setQueryFilter(excluded,'lists:MAIN COLONY',0), 'tags:"Vanilla Expanded" source:workshop');
  assert.equal(setQueryFilter(added,'lists:Other colony',1), added+' lists:"Other colony"');
});

function barHarness(groups) {
  const document = {activeElement:null,addEventListener(){}};
  class Node {
    constructor(tag, cls='', text='') {
      Object.assign(this,{tag,cls,textContent:text,children:[],dataset:{},value:'',isConnected:true,hidden:false});
      this.style={setProperty(){},removeProperty(){}};
      this.classList={toggle(){}};
    }
    append(...nodes){this.children.push(...nodes);for(const n of nodes)n.parent=this;}
    replaceChildren(...nodes){this.children=[];this.append(...nodes);}
    set innerHTML(v){this.children=[];}
    get firstChild(){return this.children[0];}
    get lastChild(){return this.children.at(-1);}
    setAttribute(){} removeAttribute(){} addEventListener(){}
    focus(){document.activeElement=this;}
    blur(){document.activeElement=null;}
    contains(node){return node===this || this.children.some(n=>n.contains(node));}
    getBoundingClientRect(){return {height:600};}
    querySelectorAll(sel){return this.children.flatMap(n=>[...(n.cls.split(' ').includes(sel.slice(1))?[n]:[]),...n.querySelectorAll(sel)]);}
    querySelector(sel){return this.querySelectorAll(sel)[0] || null;}
  }
  const ctx=createContext({document,Node,setTimeout,clearTimeout,
    el:(tag,cls,text)=>new Node(tag,cls,text),T:x=>x,Tf:(x,n)=>x.replace('%d',n),keyCaps:()=>new Node('span'),
    wireSeparator:()=>({refresh(){}}),api:{},groups,
  });
  runInContext(source('searchsuggest')+'\n'+source('filterbar')+'\nvar bar=createBar({data:()=>({groups}),change(){}});bar.open=true;bar.draw();globalThis.bar=bar;',ctx);
  return {bar:ctx.bar,document};
}

test('Lists is the first group, hides lists with no matching mods, and supports click, exclusion and keys', () => {
  const groups=[{key:'tag',title:'tags',values:[{filter:'tag:Visual',label:'Visual',count:2}]},
    {key:'lists',title:'lists',values:[
      {filter:'lists:Main colony',label:'Main colony',count:1,cur:true},
      {filter:'lists:Tribal start',label:'Tribal start',count:3},
      {filter:'lists:Empty list',label:'Empty list',count:0},
    ]}];
  const {bar,document}=barHarness(groups);
  const buttons=bar.el.querySelectorAll('.fbar-v');
  assert.equal(buttons.length,3);
  assert.equal(buttons[0].children[0].textContent,'Main colony');
  assert.equal(buttons[0].children[1].textContent,'current list');
  assert.equal(buttons[1].children.at(-1).textContent,'3');
  assert.ok(![...buttons].some((x)=>x.children[0].textContent==='Empty list'));
  buttons[0].onclick();assert.equal(bar.search,'lists:"Main colony"');
  buttons[0].oncontextmenu({preventDefault(){}});assert.equal(bar.search,'-lists:"Main colony"');
  const key = (node,key)=>node.onkeydown({key,preventDefault(){},stopPropagation(){}});
  key(buttons[1],'Enter');assert.equal(bar.search,'-lists:"Main colony" lists:"Tribal start"');
  key(buttons[1],'x');assert.equal(bar.search,'-lists:"Main colony" -lists:"Tribal start"');
  key(buttons[0],'ArrowDown');assert.equal(document.activeElement,buttons[2]);
  groups[1].values[0].on=-1;bar.draw();
  const main=bar.el.querySelectorAll('.fbar-v')[0];
  main.oncontextmenu({preventDefault(){}});assert.equal(bar.search,'-lists:"Tribal start"');
});

test('task groups do not acquire list suggestions', () => {
  const ctx=createContext({});
  runInContext(source('searchsuggest')+'\nglobalThis.suggest=suggestions;',ctx);
  const groups=[{key:'state',title:'state',values:[{filter:'state:running',label:'running',count:1}]}];
  assert.deepEqual(Array.from(ctx.suggest(groups,'',0),x=>x.token),['state:']);
});
