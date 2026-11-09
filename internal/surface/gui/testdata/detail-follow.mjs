import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, Element, deferred } from "./dom.mjs";

async function harness(details=null) {
 const box=new Element("div"), S={envSeq:0,screen:null,cols:[{items:[]},{items:[]},{items:[]}]};
 let next=0;const timers=new Map(),calls=[],detail=deferred();
 const q=await loadModules("./detail.js",{
  "./state.js":{S,$:()=>box,el:(tag,cls,text)=>{const e=new Element(tag,cls,text);return e},esc:String,api:{LibraryCopy:async()=>null,Detail:id=>{calls.push("Detail");return details?details(id):detail.promise}},T:s=>s,Tf:s=>s,errText:String,say:()=>{},factText:()=>{}},
  "./icons.js":{icon:{},sourceIcon:()=>"",statusIcon:()=>""},"./fields.js":{fieldList:()=>new Element("div")},"./ops.js":{},
 },{setTimeout:fn=>{timers.set(++next,fn);return next},clearTimeout:id=>timers.delete(id)});
 return {q,S,box,timers,calls,detail};
}
const settle=()=>new Promise(r=>setImmediate(r));

test("cursor burst retains one timer and makes the old details inert at once", async()=>{
 const {q,S,box,timers}=await harness();
 box.append(new Element("button","","old action"));S.detail={id:"old"};
 for(let i=0;i<100;i++)q.followDetail({id:String(i),name:"row "+i},false);
 assert.equal(timers.size,1);assert.equal(S.detail,null);
 // the old Mod stays on screen until the next is ready, but nothing in it can be used
 assert.equal(box.textContent,"old action");assert.equal(box.inert,true);assert.ok("stale" in box.dataset);
 q.followDetail(null,false);assert.equal(timers.size,0);assert.equal(box.textContent,"");assert.equal(box.inert,false);
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
 assert.equal(S.detail,null);assert.ok(!box.textContent.includes("stale result"));assert.equal(box.inert,true);
});


test("only the final row starts a backend read after a cursor burst", async()=>{
 const {q,S,timers,calls}=await harness();
 for(let i=0;i<100;i++)q.followDetail({id:String(i),name:"row "+i},false);
 assert.equal(calls.length,0);
 [...timers.values()][0]();
 assert.deepEqual(calls,["Detail"]);
 assert.equal(S.detail,null);
});

test("the next Mod's details replace the old ones in one swap, once its parts are in", async()=>{
 const {q,S,box,timers,detail}=await harness();
 box.append(new Element("h1","","old mod"));
 q.followDetail({id:"new",name:"new mod"},false);
 const shown=q.showDetail({id:"new",name:"new mod"},false);
 await settle();
 // still the old Mod, inert, while the detail is on its way: nothing drawn part by part
 assert.equal(box.textContent,"old mod");assert.equal(box.inert,true);
 detail.resolve({id:"new",name:"new mod",authors:"someone",header:"",show:"about",about:[]});await shown;
 assert.ok(box.textContent.includes("new mod")&&box.textContent.includes("someone")&&!box.textContent.includes("old mod"));
 assert.equal(box.inert,false);assert.ok(!("stale" in box.dataset));assert.equal(S.detail.id,"new");
 assert.equal(timers.size,0,"the debounce and the reveal cap are both cleared");
});

test("back to a Mod before the next one showed builds its own frame, not the next one's", async()=>{
 const pending=new Map();
 const {q,box,timers}=await harness(id=>{if(id==="a"&&!pending.has("a")){pending.set("a",null);return Promise.resolve({id:"a",name:"A mod",header:"",show:"about",about:[]})}const d=deferred();pending.set(id,d);return d.promise});
 await q.showDetail({id:"a",name:"A mod"},false);
 assert.equal(box.querySelector("h1").textContent,"A mod");
 // B is still on its way when the cursor returns to A, whose detail is slow
 q.showDetail({id:"b",name:"B mod"},false);await settle();
 const back=q.showDetail({id:"a",name:"A mod"},false);await settle();
 for(const fn of [...timers.values()])fn();await settle();void back;
 assert.equal(box.querySelector("h1").textContent,"A mod");
});
