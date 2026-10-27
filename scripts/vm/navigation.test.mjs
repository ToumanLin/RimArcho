import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

function harness() {
  const state = { shell: { plan: { id: 1 } }, busy: {}, cols: [] };
  const calls = [], resolvers = [];
  let read = 0;
  const context = createContext({ S: state, api: { OpenPlan(id) { calls.push(id); return new Promise((resolve, reject) => resolvers.push({ resolve: () => { state.backendPlan=id;resolve(); }, reject })); } }, openEditor: async load => { const ticket=++read;return load({current:()=>ticket===read}); } });
  const source = readFileSync(new URL('../../internal/surface/gui/assets/js/navigation.js',import.meta.url),'utf8').replace(/^import .*;\r?$/gm,'').replace(/\bexport /g,'');
  runInContext(source + '\ninitNavigation({reload:async()=>{ S.shell.plan.id=S.backendPlan;return true; },render(){},detailFollow(){}});globalThis.openPageForTest=openPage;',context);
  return { state, calls, resolvers, open: id => context.openPageForTest('plan',id) };
}
const flush = () => new Promise(resolve => setImmediate(resolve));
test('returning to the displayed plan waits for and supersedes an unfinished open',async()=>{
  const h=harness();const first=h.open(2);await flush();const last=h.open(1);await flush();
  assert.deepEqual(h.calls,[2]);h.resolvers[0].resolve();await flush();
  assert.deepEqual(h.calls,[2,1]);h.resolvers[1].resolve();await Promise.all([first,last]);
  assert.equal(h.state.backendPlan,1);assert.equal(h.state.shell.plan.id,1);
});
test('a rejected plan request does not strand the next click',async()=>{
  const h=harness();const first=h.open(2).catch(()=>{});await flush();const last=h.open(1);h.resolvers[0].reject(Error('test failure'));await flush();
  assert.deepEqual(h.calls,[2,1]);h.resolvers[1].resolve();await Promise.all([first,last]);assert.equal(h.state.shell.plan.id,1);
});
