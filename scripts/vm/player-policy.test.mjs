import test from 'node:test';
import assert from 'node:assert/strict';
import { allowed, canonical, classify, classifyKey, coverage, difference, random, replayAction } from './player-policy.mjs';
test('external and unknown targets fail closed, including deceptive button labels',()=>{
  for(const label of ['unsubscribe','launch RimWorld','choose a folder','save as a file…','open Steam browser'])assert.equal(classify({label}).tier,'external',label);
  assert.equal(classify({tag:'BUTTON',label:'do something new'}).tier,'unknown');
  assert.equal(classify({owner:'nav',label:'launch',identity:{'data-env-write':'launch'}}).tier,'external');
  assert.equal(classify({owner:'nav',label:'delete',identity:{'data-env-write':'remove-plan'}}).tier,'local-destructive');
  assert.equal(classify({owner:'nav',label:'my list',identity:{'data-id':'plan:1'}}).tier,'safe');
  assert.equal(classify({owner:'nav',label:'new operation'}).tier,'unknown');
  assert.equal(classify({row:true,label:'RimWorld Workshop downloads'}).tier,'safe');
  assert.equal(classify({row:true,label:'RimWorld',identity:{'data-env-write':'launch'}}).tier,'external');
  assert.equal(classify({tag:'BUTTON',label:'cancel'},{popup:'confirm'}).tier,'safe');
  assert.equal(classify({identity:{'data-env-write':'new-operation'},label:'go'}).tier,'local-destructive');
  assert.equal(classify({identity:{'data-env-write':'subscribe'},label:'add'}).tier,'external');
  assert.equal(classify({identity:{'data-id':'game|1.6'},label:'1.6'}).tier,'local-destructive');
  assert.equal(classifyKey('Control+Backslash').tier,'safe');
  assert.equal(classifyKey('Shift+KeyL').tier,'external');assert.equal(classifyKey('Enter',{popup:'confirm'}).tier,'unknown');
  assert.equal(classifyKey('Space',{popup:'palette'}).tier,'unknown');assert.equal(classifyKey('KeyT').tier,'safe');
  assert.equal(classifyKey('Space',{page:'home'}).tier,'unknown');assert.equal(classifyKey('Space',{page:null}).tier,'safe');
  for(const tier of ['unknown','local-destructive','external'])assert.equal(allowed(tier),false);
  assert.equal(allowed('external','local'),false);assert.equal(allowed('local-destructive','local'),true);assert.equal(allowed('unknown','external'),false);
});
test('canonical equality detects staging corruption despite identical enabled entries',()=>{
  const start={plans:[{name:'a',workspace:[{entries:'a'}],staging:[],undo:[],redo:[],undoDepth:0,redoDepth:0}],environments:[{name:'game',current_plan:'a'}]};
  const undone=structuredClone(start);undone.plans[0].redo=[{entries:'b'}];undone.plans[0].redoDepth=1;
  assert.equal(difference(canonical(start),canonical(undone)),null);
  undone.plans[0].staging=[{value:'leftover'}];assert.match(difference(canonical(start),canonical(undone)).path,/staging/);
});
test('refresh timestamps do not mask collection source changes',()=>{
  const a={plans:[],environments:[],plan_collection:[{source:'https://example.invalid/list',checked_at:1}]};
  const b=structuredClone(a);b.plan_collection[0].checked_at=2;assert.equal(difference(canonical(a),canonical(b)),null);
  b.plan_collection[0].source='https://example.invalid/other';assert.match(difference(canonical(a),canonical(b)).path,/source/);
});
test('seeded sequences repeat and coverage retains unreachable eligible edges',()=>{
  const a=random(12),b=random(12);assert.deepEqual(Array.from({length:100},a),Array.from({length:100},b));
  const pages=coverage([{page:'plan',eligible:true,asserted:true,outcome:'pressed-ok',tier:'safe'},{page:'plan',eligible:true,outcome:'unreachable',tier:'safe'},{page:'plan',eligible:false,outcome:'skipped',tier:'unknown'}]);
  assert.equal(pages.plan.coveragePercent,50);assert.equal(pages.plan.discovered,3);
});

test('shared baseline blocks file/deployment writes while permitting private plan records',()=>{
  const state={sharedContent:true};
  for(const identity of [{'data-cmd':'pin'},{'data-env-write':'move-directory'},{'data-env-write':'import-apply'}]){const p=classify({identity,label:'apply'},state);assert.equal(p.tier,'local-destructive');assert.match(p.blocked,/shared baseline/);assert.equal(allowed(p,'external'),false);}
  assert.equal(classify({identity:{'data-env-write':'create-plan'},label:'create and edit'},state).tier,'local-destructive');
  assert.equal(classify({identity:{'data-env-write':'toggle'},label:'enable'},state).tier,'safe');
  assert.equal(allowed(classify({identity:{'data-env-write':'plan-unfollow'},label:'stop following'},state),'local'),true);
  const steam=classify({identity:{'data-env-write':'subscription-update'},label:'download'},state);assert.equal(steam.tier,'external');assert.equal(allowed(steam,'external'),false);
});

test('opening subscription UI and choosing export formats do not submit external operations',()=>{
 assert.equal(classify({owner:'nav',identity:{'data-id':'subscribe'},label:'add a subscription'}).tier,'safe');
 assert.equal(classify({semantics:['export-format'],label:'Workshop links',identity:{}}).tier,'safe');
 assert.equal(classify({label:'look for it in the Workshop',identity:{}}).tier,'external');
 assert.equal(classify({identity:{'data-env-write':'subscribe'},label:'subscribe'}).tier,'external');
});

test('replay sends the actual click count after a moving target cancels repeats',()=>{
 const entry={action:{kind:'click',locator:'#navtoggle',count:3},issuedClicks:1,cancelledRepeats:{from:2}};
 assert.equal(replayAction(entry).count,1);
 assert.equal(entry.action.count,3);
 assert.equal(replayAction({action:{kind:'key',combo:'Escape'}}).combo,'Escape');
});
