import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules, Element } from "./dom.mjs";

test("replacing rows keeps visible identities valid before the filter answers", async () => {
  const key=it=>it.id||it.ref;
  const c={items:[{id:"a"},{id:"b"},{id:"c"}],view:[2,1],cursor:0,sel:new Set(["b","c"])};
  const S={cols:[c]};
  const lists=await loadModules("./lists.js", {
    "./state.js":{S,key,cur:k=>S.cols[k].items[S.cols[k].view[S.cols[k].cursor]],col:k=>S.cols[k],$:()=>null,$$:()=>[],el:tag=>new Element(tag),factText:()=>{},esc:String,api:{},T:s=>s,Tf:s=>s,AREAS:[],ROW_H:28,slideThumb:()=>{}},
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

test("the Disabled tags view groups rows under their first tag; a closed group keeps the cursor on its header", async () => {
  const key=it=>it.id||it.ref;
  const items=[{id:"a",tags:["UI"]},{id:"b",tags:["apparel","UI"]},{id:"c"},{id:"d",tags:["ui"]},{id:"e",tags:["new"]}];
  const c={items,view:[0,1,2,3,4],cursor:0,sel:new Set()};
  const S={cols:[c],board:{stamp:1}};
  const g=await loadModules("./disabledgroups.js", {"./state.js":{S,key,api:{SetSetting:async()=>{},TagList:async()=>[{tag:"apparel"},{tag:"UI"}]}}}, {localStorage:{getItem:()=>null,setItem:()=>{}}});
  assert.equal(await g.setView("tags"),true);
  // the Tag manager's order, a tag it does not list yet after it, no tag last; each row once
  assert.equal(JSON.stringify(c.groups.map(x=>x.key)),JSON.stringify(["apparel","ui","new",""]));
  assert.equal(JSON.stringify([...c.view]),JSON.stringify([-1,1,-2,0,3,-3,4,-4,2]));
  c.cursor=4; // d, under UI
  g.setGroupOpen("ui",false);
  assert.equal(JSON.stringify([...c.view]),JSON.stringify([-1,1,-2,-3,4,-4,2]));
  assert.equal(c.cursor,2); assert.equal(g.curKey(0),"\u0000ui");
  const geometry=await loadModules("./listgeometry.js",{});
  const layout=geometry.listGeometry(items,c.view,0);
  assert.equal(layout.total,7*28); assert.equal(layout.heads.some(Boolean),false);
  // filtered: the groups show only matching rows, and a group with none is left out
  c.view=[0,2]; g.shape(0);
  assert.equal(JSON.stringify(c.groups.map(x=>[x.key,x.rows.length,x.total])),JSON.stringify([["ui",1,2],["",1,1]]));
  assert.equal(await g.setView("individual"),true);
  assert.equal(JSON.stringify([...c.view]),JSON.stringify([0,2])); assert.equal(c.groups,null);
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
test("the bar's search asks again for another list's board even when the database stamp is the same", async () => {
  // Player report (1.0.9): searched in one list, switched to another without clearing the
  // search, and the lists showed the old list's rows. Opening a list changes no database
  // state, so the stamp stays; the answer's rows are positions in the first board's lists.
  const key=it=>it.id||it.ref;
  const calls=[];
  const S={cols:[{items:[],view:[],cursor:0,sel:new Set()},{items:[],view:[],cursor:0,sel:new Set()},{items:[],view:[],cursor:0,sel:new Set(),query:""}],
    bar:{search:"armor",filters:[],open:false,active:()=>true},envSeq:0};
  const api={BarFilter:async()=>{ calls.push(S.board.name); return S.board.name==="A"?{rows:[[0,1,2],[3]]}:{rows:[[0],[]]}; },Filter:async()=>({rows:[]})};
  const lists=await loadModules("./lists.js", {
    "./state.js":{S,key,cur:k=>S.cols[k].items[S.cols[k].view[S.cols[k].cursor]],col:k=>S.cols[k],$:()=>null,$$:()=>[],el:tag=>new Element(tag),factText:()=>{},esc:String,api,T:s=>s,Tf:s=>s,AREAS:[],ROW_H:28,slideThumb:()=>{}},
    "./icons.js":{icon:{},sourceIcon:()=>"",statusIcon:()=>""},
    "./badgepop.js":{wireBadges:()=>{},rowDrag:()=>{}},"./motion.js":{bump:()=>{}},
    "./screens.js":{beginPageRead:()=>{},pageLoading:()=>false},
  });
  const ids=n=>Array.from({length:n},(_,i)=>({id:"m"+i}));
  S.board={name:"A",stamp:"7.1"};
  lists.setItems(0,ids(5)); lists.setItems(1,ids(4));
  await Promise.all([0,1].map(k=>lists.refilter(k)));
  assert.deepEqual([...S.cols[0].view],[0,1,2]);
  // another list: a new board, the same stamp
  S.board={name:"B",stamp:"7.1"};
  lists.setItems(0,ids(1)); lists.setItems(1,[]);
  await Promise.all([0,1].map(k=>lists.refilter(k)));
  assert.deepEqual(calls,["A","B"]);
  assert.deepEqual([...S.cols[0].view],[0]);
  assert.deepEqual([...S.cols[1].view],[]);
});
test("opening another list forgets the filter bar's search with the lists' own searches", async () => {
  let forgot=0, reloads=0;
  const col=()=>({query:"x",cursor:3,sel:new Set(["a"]),searching:true});
  const S={shell:{plan:{id:1},plans:[{id:1},{id:2}]},cols:[col(),col(),col()],bar:{forget(){ forgot++; }}};
  const plans=await loadModules("./plans.js", {
    "./state.js":{S,$:()=>null,el:tag=>new Element(tag),api:{OpenPlan:async()=>({})},T:s=>s,Tf:s=>s,errText:String,say(){},keyCaps:()=>new Element("span")},
    "./popups.js":{},"./feedbacksemantics.js":{fieldError:()=>{}},
  });
  plans.initPlans({reload:async()=>{ reloads++; }});
  assert.equal(await plans.openPlan(1),true); // already open: nothing forgotten
  assert.equal(forgot,0);
  assert.equal(await plans.openPlan(2),true);
  assert.equal(forgot,1); assert.equal(reloads,1);
  assert.deepEqual(S.cols.map(c=>[c.query,c.cursor,c.sel.size,c.searching]),[["",0,0,false],["",0,0,false],["",0,0,false]]);
});

test("the history's bar says 1 step, 2 steps (VM 2026-11-23: \"1 steps\")", async () => {
  const els = {};
  const $ = (sel) => (els[sel] ||= { classList: { toggle() {} }, setAttribute() {}, dataset: {}, hidden: false, innerHTML: "", append() {} });
  const S = { cols: [{ items: [] }, { items: [] }, { items: [] }], history: [{ kind: "save" }, { kind: "undo" }] };
  const lists = await loadModules("./lists.js", {
    "./state.js": { S, key: (it) => it.id, cur: () => null, col: (k) => S.cols[k], $, $$: () => [], el: (tag) => new Element(tag), factText: () => {}, esc: String, api: {}, T: (s) => s, Tf: (s, ...a) => s.replace("%d", String(a[0])), AREAS: [], ROW_H: 28, slideThumb: () => {} },
    "./icons.js": { icon: {}, sourceIcon: () => "", statusIcon: () => "" },
    "./badgepop.js": { wireBadges: () => {}, rowDrag: () => {} }, "./motion.js": { bump: () => {} },
    "./screens.js": { beginPageRead: () => {}, pageLoading: () => false },
  });
  lists.renderHistory();
  assert.match($(".hbar").innerHTML, /1 step</);
  S.history.push({ kind: "undo" });
  lists.renderHistory();
  assert.match($(".hbar").innerHTML, /2 steps</);
});

test("the details' tags view: every tag of Enabled's Mods with how many carry it, no tag last; a row toggles its filter, the pencil opens the Tag manager", async () => {
  const doc = { documentElement: { lang: "en" } };
  const tv = await loadModules("./tagview.js", {
    "./state.js": { el: (tag, cls = "", text) => { const e = new Element(tag); e.className = cls; if (text != null) e.textContent = text; return e; }, T: (s) => s, Tf: (s, ...a) => s + " " + a.join(","), factText: () => {} },
    "./tagcolor.js": { tagCSS: (v) => (v ? "var(--tag-" + v + ")" : ""), tagColors: () => ({}), markOf: () => "", paintMark: () => {}, TAG_PRESETS: [] },
    "./icons.js": { icon: { edit: "<svg></svg>" } },
  }, { document: doc });
  const items = [{ tags: ["visuals"] }, { tags: ["QoL", "visuals"] }, { tags: ["qol"] }, {}, { tags: [] }, { tags: ["QoL", "qol"] }];
  const sums = tv.tally(items, { qol: "grass" });
  // a Mod counts once for each tag it carries anywhere, as tag:<tag> filters
  assert.deepEqual(JSON.parse(JSON.stringify(sums.tags.map((t) => [t.tag, t.n, t.color]))), [["QoL", 3, "var(--tag-grass)"], ["visuals", 2, ""]]);
  assert.equal(sums.none, 2);
  const sets = [], edits = [];
  const act = { filter: (f, on) => sets.push([f, on]), edit: (t) => edits.push(t) };
  const box = tv.tagView(items, { qol: "grass" }, "", act);
  const lines = box.children;
  assert.deepEqual(lines.map((l) => l.className), ["tv-line", "tv-line plain", "tv-line none"]);
  assert.equal(lines[2].children.length, 2, "no tag has no pencil, only its place");
  lines[0].children[0].onclick();
  lines[2].children[0].onclick();
  lines[1].children[1].onclick();
  assert.deepEqual(sets, [["tag:QoL", 1], ["has:notag", 1]]);
  assert.deepEqual(edits, ["visuals"]);
  assert.equal(tv.tagView(items, { qol: "grass" }, "", act), box, "unchanged numbers keep the element");
  // a filter in the query marks its row; a click takes it away
  const on = tv.tagView(items, { qol: "grass" }, "tags:qol has:notag -tags:visuals", act);
  assert.notEqual(on, box);
  assert.deepEqual(on.children.map((l) => l.children[0].className), ["tv-row on", "tv-row", "tv-row on"]);
  on.children[0].children[0].onclick();
  assert.deepEqual(sets.at(-1), ["tag:QoL", 0]);
  assert.deepEqual([...tv.filtersOn('tags:"Big Pack" -has:notag').tags], ["big pack"]);
});
