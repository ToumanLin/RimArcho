// Compare only shared domain checkpoints. Window input/DOM coverage stays W-only.
import { readFileSync, writeFileSync, mkdtempSync } from 'node:fs';
import { resolve, relative, isAbsolute, join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
const args=process.argv.slice(2);
assert(args.length===2,'usage: node scripts/vm/compare.mjs <left-result-directory> <right-result-directory>');
const read=(root,file)=>JSON.parse(readFileSync(join(root,file),'utf8'));
const [left,right]=args.map(p=>resolve(p));
const l=read(left,'result.json'),r=read(right,'result.json');
assert(l.scenario===r.scenario&&l.fixture===r.fixture&&l.plan===r.plan,'compare the same scenario, fixture and plan');
const projection=d=>Object.fromEntries(['environments','plans','sources','instances','lineage_data','lineage_tag','instance_note','user_rule','modsConfigHashes','links'].map(k=>[k,d[k]]));
function difference(a,b,path='') {
  if(Object.is(a,b))return null;
  if(typeof a!==typeof b || a===null || b===null || typeof a!=='object')return {path,left:a,right:b};
  if(Array.isArray(a)!==Array.isArray(b))return {path,left:a,right:b};
  for(const k of [...new Set([...Object.keys(a),...Object.keys(b)])].sort()){
    const d=difference(a[k],b[k],path+'/'+k.replaceAll('~','~0').replaceAll('/','~1'));if(d)return d;
  }
  return null;
}
const checks=[];
for(const lc of l.checkpoints){
  const rc=r.checkpoints.find(c=>c.label===lc.label);if(!rc)continue;
  const d=difference(projection(read(left,lc.artifact).domain),projection(read(right,rc.artifact).domain));
  checks.push({checkpoint:lc.label,equal:!d,firstDifference:d});if(d)break;
}
const status=!checks.length?'unsupported':checks.some(c=>!c.equal)?'different':'equal';
const out=mkdtempSync(join(tmpdir(),'RimArcho-vm-compare-'));
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../..'),rel=relative(repo,out);
assert(rel.startsWith('..')||isAbsolute(rel),'comparison output must be outside repository');
const report={schema:1,status,left,right,scenario:l.scenario,drivers:[l.driver,r.driver],checks,scope:'shared canonical domain checkpoints; does not imply equivalent input/DOM coverage'};
writeFileSync(join(out,'comparison.json'),JSON.stringify(report,null,2));
console.log(JSON.stringify({...report,output:out},null,2));process.exitCode=status==='equal'?0:status==='unsupported'?2:1;
