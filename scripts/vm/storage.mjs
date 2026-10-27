import { lstatSync, readdirSync, rmSync, readFileSync, writeFileSync } from 'node:fs';
import { resolve, relative, join, isAbsolute } from 'node:path';
export const HOST_ROOT='D:/RimArcho-test';
export function treeBytes(root){
  let bytes=0;
  for(const e of readdirSync(root,{withFileTypes:true})){
    const p=join(root,e.name);let s;try{s=lstatSync(p);}catch(err){if(err.code==='ENOENT')continue;throw err;}
    if(s.isSymbolicLink())continue;
    bytes+=s.isDirectory()?treeBytes(p):s.size;
  }
  return bytes;
}
export function hostOutput(path=HOST_ROOT){
  const root=resolve(HOST_ROOT),target=resolve(path),r=relative(root,target);
  if(r.startsWith('..')||isAbsolute(r))throw Error('host output must be under D:/RimArcho-test');
  try{if(treeBytes(root)>1800*1024*1024)throw Error('host test output exceeds 1800 MiB; free space before another run');}catch(e){if(e.code!=='ENOENT')throw e;}
  return target;
}
const keep=/^(?:result|ledger|trace|replay|minimal-trace|shrink|operation-seeds|branch-[a-f0-9]+|replay-branch-\d+|shrink-candidate-\d+)\.json$|^ledger\.md$|\.log$/;
export function cleanupRun(run){
  const root=resolve(run);
  if(!/^(?:crawl|walk|s[1-4])-[WAC]-[\w-]+$/.test(root.split(/[\\/]/).at(-1)))throw Error('cleanup requires a runner-created case directory');
  const beforeBytes=treeBytes(root);
  for(const entry of readdirSync(root,{withFileTypes:true})){
    const p=resolve(root,entry.name),r=relative(root,p);
    if(!r||r.startsWith('..')||isAbsolute(r))throw Error('cleanup escaped run root');
    if(entry.isDirectory()&&entry.name==='retries'){for(const child of readdirSync(p,{withFileTypes:true})){if(child.isDirectory())cleanupRun(join(p,child.name));}continue;}
    if(entry.isFile()&&keep.test(entry.name)){
      if(entry.name.endsWith('.log')&&lstatSync(p).size>256*1024){const b=readFileSync(p);writeFileSync(p,b.subarray(-256*1024));}
      continue;
    }
    // Never traverse links, especially content junctions in a fixture case.
    if(lstatSync(p).isSymbolicLink())rmSync(p,{force:true});
    else rmSync(p,{recursive:entry.isDirectory(),force:true,maxRetries:10,retryDelay:250});
  }
  return {beforeBytes,retainedBytes:treeBytes(root),retention:'result, ledger, trace/replay/shrink, operation seeds and bounded logs; owned homes/profiles/content removed'};
}
