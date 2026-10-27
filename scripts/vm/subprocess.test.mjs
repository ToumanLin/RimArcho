import test from 'node:test';
import assert from 'node:assert/strict';
import {runChild} from './subprocess.mjs';
test('a timed-out replay terminates its owned descendant process on Windows',{skip:process.platform!=='win32'},async()=>{
 const script="const {spawn}=require('node:child_process');const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{windowsHide:true,stdio:'ignore'});console.log(child.pid);setInterval(()=>{},1000);";
 const result=await runChild(['-e',script],1200);
 assert.match(result.error?.message,/timed out/);
 const descendant=Number(result.stdout.trim());assert(descendant>0);
 assert.throws(()=>process.kill(descendant,0));assert.throws(()=>process.kill(result.pid,0));
});
