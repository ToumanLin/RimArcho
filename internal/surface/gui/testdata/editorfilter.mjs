import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, Element } from "./dom.mjs";

test("replacing rows keeps visible identities valid before the filter answers", async () => {
  const key=it=>it.id||it.ref;
  const c={items:[{id:"a"},{id:"b"},{id:"c"}],view:[2,1],cursor:0,sel:new Set(["b","c"])};
  const S={cols:[c]};
  const lists=await loadModules("./lists.js", {
    "./state.js":{S,key,cur:k=>S.cols[k].items[S.cols[k].view[S.cols[k].cursor]],col:k=>S.cols[k],$:()=>null,$$:()=>[],el:tag=>new Element(tag),factText:()=>{},esc:String,api:{},T:s=>s,Tf:s=>s,AREAS:[],ROW_H:28},
    "./icons.js":{icon:{},sourceIcon:()=>"",statusIcon:()=>""},
    "./badgepop.js":{wireBadges:()=>{},rowDrag:()=>{}},"./motion.js":{bump:()=>{}},
    "./screens.js":{beginPageRead:()=>{},pageLoading:()=>false},
  });
  const keep=lists.setItems(0,[{id:"b"},{id:"a"}]);
  assert.equal(keep,"c");
  assert.deepEqual([...c.view],[0]);
  assert.deepEqual([...c.sel],["b"]);
  const geometry=await loadModules("./listgeometry.js",{});
  assert.doesNotThrow(()=>geometry.listGeometry(c.items,c.view,0));
});

test("typed conditions preserve quoted paths, replace one condition and share the tag alias", async () => {
  const q = await loadModules("./searchsuggest.js", {});
  const text = 'name:"Combat Extended" source:local author:"A B"';
  assert.equal(q.setQueryFilter(text, "source:local", 0), 'name:"Combat Extended" author:"A B"');
  assert.equal(q.setQueryFilter('tags:"Facial Hair" location:"D:\\Old Mods"', "tag:Facial Hair", -1), 'location:"D:\\Old Mods" -tags:"Facial Hair"');
  assert.equal(q.setQueryFilter('source:', 'source:local', 1), 'source:local');
  const parts=q.queryParts(text); assert.equal(parts[2].value,"A B");
  assert.equal(q.queryContext(text, text.indexOf("source")+6).text, "source");
});
test("suggestions omit locations and result counts, and quote the full value", async () => {
  const q = await loadModules("./searchsuggest.js", {});
  const groups=[{key:"tag",title:"标签",values:[{filter:"tag:Facial Hair",label:"Facial Hair",count:99}]},{key:"source",title:"来源",values:[{filter:"source:local",label:"本地",count:3}]},{key:"location",title:"位置",values:[]}];
  assert.equal(q.suggestions(groups,"",0).map(c=>c.token).join(","),"tags:,source:");
  const candidate=q.suggestions(groups,"tags:Fa",7)[0];
  assert.equal(candidate.token,'tags:"Facial Hair"'); assert.equal(candidate.label,""); assert.equal("count" in candidate,false);
  assert.equal(q.suggestions(groups,"source:l",8)[0].label,"本地");
  groups.unshift({key:"lists",title:"lists",values:[{filter:"lists:Main colony",label:"Main colony",count:1,cur:true}]});
  const list=q.suggestions(groups,'-lists:"Ma',10)[0];
  assert.equal(list.token,'-lists:"Main colony"');assert.equal(list.label,"");
  assert.equal(q.setQueryFilter('tags:"Facial Hair"','lists:Main colony',1),'tags:"Facial Hair" lists:"Main colony"');
});
test("virtual row boundaries account for group headings; an enabled copy keeps one line", async () => {
  const q = await loadModules("./listgeometry.js", {});
  const items=[{id:"a",lineage:"ce",copyCount:3},{id:"b",lineage:"ce",copyCount:3},{id:"c",lineage:"hud",copyCount:1}];
  const inactive=q.listGeometry(items,[0,1,2],0);
  assert.equal(JSON.stringify(inactive.tops),"[0,56,84]");assert.equal(inactive.total,112); // a lone copy has no heading
  assert.equal(inactive.heads[2],false);
  assert.equal(q.rowAt(inactive,55),0);assert.equal(q.rowAt(inactive,56),1);assert.equal(q.rowAt(inactive,112),3);
  const active=q.listGeometry(items,[0,2],1);assert.equal(JSON.stringify(active.tops),"[0,28]");assert.equal(active.total,56);
  const filtered=q.listGeometry(items,[1,2],0);assert.equal(filtered.heads[0],true);assert.equal(filtered.heights[0],56);
});
test("a shell read that failed never replaces the sidebar's lists", async () => {
  const S={shell:null};
  const shell=await loadModules("./shell.js", {
    "./state.js":{S,factText:()=>{},$:()=>null,el:tag=>new Element(tag),api:{},T:s=>s,Tf:s=>s},
    "./icons.js":{icon:{}},"./screens.js":{liveCount:()=>0,seg:()=>{},setSeg:()=>{},syncPageAccessibility:()=>{}},
    "./motion.js":{bump:()=>{},slideNav:()=>{},places:()=>{},slideFrom:()=>{}},
    "./popups.js":{},"./separator.js":{wireSeparator:()=>{}},
  });
  assert.equal(shell.shellFailed({error:"busy"}),false); // nothing shown yet: take it
  S.shell={envs:2,plans:[{id:1}]};
  assert.equal(shell.shellFailed({error:"database is locked",envs:2}),true);
  assert.equal(shell.shellFailed({envs:0}),true);
  assert.equal(shell.shellFailed(undefined),true);
  assert.equal(shell.shellFailed({envs:2,plans:[]}),false);
  S.shell={error:"busy"};
  assert.equal(shell.shellFailed({error:"still busy"}),false); // a failed shell is replaced
});
