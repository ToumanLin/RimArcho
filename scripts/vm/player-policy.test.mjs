import test from 'node:test';
import assert from 'node:assert/strict';
import { allowed, canonical, classify, coverage, difference, random } from './player-policy.mjs';
test('external and unknown targets fail closed, including deceptive button labels',()=>{
  for(const label of ['unsubscribe','launch RimWorld','choose a folder','save as a file…','open Steam browser'])assert.equal(classify({label}).tier,'external',label);
  assert.equal(classify({tag:'BUTTON',label:'do something new'}).tier,'unknown');
  assert.equal(classify({tag:'BUTTON',label:'cancel'},{popup:'confirm'}).tier,'safe');
  assert.equal(classify({identity:{'data-env-write':'new-operation'},label:'go'}).tier,'local-destructive');
  assert.equal(classify({identity:{'data-env-write':'subscribe'},label:'add'}).tier,'external');
  for(const tier of ['unknown','local-destructive','external'])assert.equal(allowed(tier),false);
  assert.equal(allowed('external','local'),false);assert.equal(allowed('local-destructive','local'),true);assert.equal(allowed('unknown','external'),false);
});
test('canonical equality detects staging corruption despite identical enabled entries',()=>{
  const start={plans:[{name:'a',workspace:[{entries:'a'}],staging:[],undo:[],redo:[],undoDepth:0,redoDepth:0}],environments:[{name:'game',current_plan:'a'}]};
  const undone=structuredClone(start);undone.plans[0].redo=[{entries:'b'}];undone.plans[0].redoDepth=1;
  assert.equal(difference(canonical(start),canonical(undone)),null);
  undone.plans[0].staging=[{value:'leftover'}];assert.match(difference(canonical(start),canonical(undone)).path,/staging/);
});
test('seeded sequences repeat and coverage retains unreachable eligible edges',()=>{
  const a=random(12),b=random(12);assert.deepEqual(Array.from({length:100},a),Array.from({length:100},b));
  const pages=coverage([{page:'plan',eligible:true,asserted:true,outcome:'pressed-ok',tier:'safe'},{page:'plan',eligible:true,outcome:'unreachable',tier:'safe'},{page:'plan',eligible:false,outcome:'skipped',tier:'unknown'}]);
  assert.equal(pages.plan.coveragePercent,50);assert.equal(pages.plan.discovered,3);
});
