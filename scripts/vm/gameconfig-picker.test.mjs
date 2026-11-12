import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

// Exercise the real picker with deliberately different legacy and displayed order.
test('Game Config picker selects and removes the displayed row', async () => {
 const source = readFileSync(new URL('../../internal/surface/gui/assets/js/main.js', import.meta.url), 'utf8');
 const start = source.indexOf('async function chooseEnv()');
 const end = source.indexOf('async function useEnv(', start);
 let chooser;
 const selected = [], removed = [], added = [];
 const rows = [{name:'second',installation:'RimWorld 1.6'}, {name:'first',installation:'RimWorld 1.6',current:true}];
 const choose = runInNewContext(`(${source.slice(start,end).trim()})`, {
  S:{board:{envs:['first','second'],env:'first',environmentRows:rows}},
  T:s=>s, say:()=>{},
  pop:{choose:(...args)=>{chooser=args;}},
  useEnv:name=>selected.push(name),
  scr:{removeEnv:name=>removed.push(name),addEnvironment:row=>added.push(row.name)}
 });
 await choose();
 const [title,labels,pick,current,other] = chooser;
 assert.equal(title,'switch Game Config');
 assert.equal(labels[0],'second  RimWorld 1.6');
 assert.equal(current,1);
 pick(0);pick(1);pick(2);
 other.run(0);other.run(1);other.run(2);
 assert.deepEqual(selected,['second','first']);
 assert.deepEqual(removed,['second','first']);
 assert.deepEqual(added,['first']);
});
