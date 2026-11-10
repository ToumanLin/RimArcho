import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

async function harness(time = "200ms", root = {dataset:{}}) {
 const reduce={matches:false}, animations=[];
 const s={textContent:"list changed", get offsetWidth(){throw Error("forced layout")},
  animate(frames,options){const a={frames,options,cancelled:false,cancel(){this.cancelled=true}};animations.push(a);return a}};
 const q=await loadModules("./motion.js",{"./state.js":{S:{},$$:()=>[],key:x=>x.id,ROW_H:28}},
  {document:{documentElement:root},matchMedia:()=>reduce,getComputedStyle:()=>({getPropertyValue:key=>key==="--t-enter"?time:"cubic-bezier(.2, .8, .2, 1)"})});
 return {q,s,reduce,animations};
}

test("status fade keeps CSS timing and replaces its previous animation without geometry reads",async()=>{
 const {q,s,animations}=await harness();
 q.statusIn(s);q.statusIn(s);
 assert.equal(animations.length,2);assert.equal(animations[0].cancelled,true);
 assert.equal(animations[1].options.duration,200);
 assert.equal(animations[1].options.easing,"cubic-bezier(.2, .8, .2, 1)");
 assert.deepEqual(JSON.parse(JSON.stringify(animations[1].frames)),[{opacity:0},{opacity:1}],"a new sentence fades in place: it does not move the bar");
});

test("empty status and reduced motion cancel animation; seconds tokens retain their duration",async()=>{
 const {q,s,reduce,animations}=await harness(".2s");
 q.statusIn(s);assert.equal(animations[0].options.duration,200);
 s.textContent="";q.statusIn(s);assert.equal(animations[0].cancelled,true);assert.equal(animations.length,1);
 s.textContent="changed";q.statusIn(s);reduce.matches=true;q.statusIn(s);
 assert.equal(animations[1].cancelled,true);assert.equal(animations.length,2);
});

test("settings › motion: reduced stills the status line as the system setting does",async()=>{
 const root={dataset:{motion:"auto"}};
 const {q,s,animations}=await harness("200ms",root);
 q.statusIn(s);assert.equal(animations.length,1);assert.equal(q.still(),false);
 root.dataset.motion="reduced";
 assert.equal(q.still(),true);q.statusIn(s);assert.equal(animations[0].cancelled,true);assert.equal(animations.length,1);
});
