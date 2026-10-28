import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, Element, deferred } from "./dom.mjs";

async function harness() {
 const box=new Element("div"), S={envSeq:0,screen:null,cols:[{items:[]},{items:[]},{items:[]}]};
 let next=0;const timers=new Map(),calls=[],detail=deferred();
 const q=await loadModules("./detail.js",{
  "./state.js":{S,$:()=>box,el:(tag,cls,text)=>{const e=new Element(tag,cls,text);return e},esc:String,api:{LibraryCopy:async()=>null,Detail:()=>{calls.push("Detail");return detail.promise}},T:s=>s,Tf:s=>s,errText:String,say:()=>{},factText:()=>{}},
  "./icons.js":{icon:{},sourceIcon:()=>"",statusIcon:()=>""},"./fields.js":{fieldList:()=>{}},"./ops.js":{},
 },{setTimeout:fn=>{timers.set(++next,fn);return next},clearTimeout:id=>timers.delete(id)});
 return {q,S,box,timers,calls,detail};
}

test("cursor burst retains one timer and clears old actionable details immediately", async()=>{
 const {q,S,box,timers}=await harness();
 box.append(new Element("button","","old action"));S.detail={id:"old"};
 for(let i=0;i<100;i++)q.followDetail({id:String(i),name:"row "+i},false);
 assert.equal(timers.size,1);assert.equal(S.detail,null);
 assert.equal(box.textContent,"row 99");assert.equal(box.querySelector("button"),null);
 q.followDetail(null,false);assert.equal(timers.size,0);assert.equal(box.textContent,"");
});

test("a pending cursor read cannot start in another page or environment", async()=>{
 const {q,S,timers}=await harness();
 q.followDetail({id:"a",name:"a"},false);const callback=[...timers.values()][0];
 S.envSeq++;assert.doesNotThrow(callback);
 q.followDetail({id:"b",name:"b"},false);S.screen="home";assert.doesNotThrow([...timers.values()].at(-1));
});


test("a response already in flight cannot refill the pane after the cursor changes", async()=>{
 const {q,S,box,detail,calls}=await harness();
 const old=q.showDetail({id:"old",name:"old"},false);
 assert.deepEqual(calls,["Detail"]);
 q.followDetail({id:"new",name:"new"},false);
 detail.resolve({id:"old",name:"stale result"});await old;
 assert.equal(S.detail,null);assert.equal(box.textContent,"new");
});


test("only the final row starts a backend read after a cursor burst", async()=>{
 const {q,S,timers,calls}=await harness();
 for(let i=0;i<100;i++)q.followDetail({id:String(i),name:"row "+i},false);
 assert.equal(calls.length,0);
 [...timers.values()][0]();
 assert.deepEqual(calls,["Detail"]);
 assert.equal(S.detail,null);
});
