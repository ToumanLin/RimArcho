import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, writeFileSync, existsSync, rmSync, symlinkSync } from 'node:fs';
import { join } from 'node:path';
import { hostOutput, cleanupRun } from './storage.mjs';
test('host paths cannot escape the single output root',()=>{
 assert.throws(()=>hostOutput('D:/vm-crawler-new'));
 assert.throws(()=>hostOutput('D:/RimArcho-test/../vm-crawler-new'));
});
test('cleanup removes only case-owned content and keeps reproduction evidence',()=>{
 const parent=mkdtempSync('D:/RimArcho-test/storage-');
 const run=join(parent,'walk-W-test');mkdirSync(join(run,'case'),{recursive:true});
 writeFileSync(join(run,'case','payload'),'content');writeFileSync(join(run,'trace.json'),'{}');
 writeFileSync(join(parent,'sentinel'),'untouched');
 const result=cleanupRun(run);
 assert(!existsSync(join(run,'case')));assert(existsSync(join(run,'trace.json')));assert(existsSync(join(parent,'sentinel')));assert(result.retainedBytes<result.beforeBytes);
 rmSync(parent,{recursive:true});
});

test('cleanup never follows a content junction into a sibling directory',()=>{
 const parent=mkdtempSync('D:/RimArcho-test/storage-');const run=join(parent,'crawl-W-test');
 mkdirSync(join(run,'case'),{recursive:true});mkdirSync(join(parent,'outside'));
 writeFileSync(join(parent,'outside','sentinel'),'keep');symlinkSync(join(parent,'outside'),join(run,'case','link'),'junction');
 cleanupRun(run);assert(existsSync(join(parent,'outside','sentinel')));
 rmSync(parent,{recursive:true});
});
