import {spawn,spawnSync} from 'node:child_process';
// Kill the complete child tree while its Node parent still exists. spawnSync's
// timeout kills only Node on Windows, leaving the app/profile alive behind it.
export function runChild(args,timeoutMs){
 return new Promise(resolve=>{
  const child=spawn(process.execPath,args,{windowsHide:true,stdio:['ignore','pipe','pipe']});
  let stdout='',stderr='',error;
  child.stdout.on('data',b=>{stdout=(stdout+b).slice(-8*1024*1024);});
  child.stderr.on('data',b=>{stderr=(stderr+b).slice(-256*1024);});
  const timer=setTimeout(()=>{
   error=Error('child replay timed out; owned process tree terminated');
   if(process.platform==='win32')spawnSync('taskkill',['/PID',String(child.pid),'/T','/F'],{windowsHide:true});
   else child.kill('SIGKILL');
  },timeoutMs);
  child.once('error',e=>{error=e;});
  child.once('close',status=>{clearTimeout(timer);resolve({status,stdout,stderr,error,pid:child.pid});});
 });
}
