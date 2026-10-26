import assert from "node:assert/strict";
import { test } from "node:test";
import { loadModules } from "./dom.mjs";

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
});
test("virtual row boundaries account for group headings and current-copy subtitles", async () => {
  const q = await loadModules("./listgeometry.js", {});
  const items=[{id:"a",lineage:"ce",copyCount:3},{id:"b",lineage:"ce",copyCount:3},{id:"c",lineage:"hud",copyCount:1}];
  const inactive=q.listGeometry(items,[0,1,2],0);
  assert.equal(JSON.stringify(inactive.tops),"[0,56,84]");assert.equal(inactive.total,112); // a lone copy has no heading
  assert.equal(inactive.heads[2],false);
  assert.equal(q.rowAt(inactive,55),0);assert.equal(q.rowAt(inactive,56),1);assert.equal(q.rowAt(inactive,112),3);
  const active=q.listGeometry(items,[0,2],1);assert.equal(JSON.stringify(active.tops),"[0,48]");assert.equal(active.total,76);
  const filtered=q.listGeometry(items,[1,2],0);assert.equal(filtered.heads[0],true);assert.equal(filtered.heights[0],56);
});
