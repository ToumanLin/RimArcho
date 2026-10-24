// Run after building tmp/accounts-check/guidev.exe. All profiles and local data
// stay in this worktree; /dev/pagecheck supplies its own offline API fixtures.
const {spawn} = require('node:child_process');
const fs = require('node:fs'), path = require('node:path'), net = require('node:net');
const root = path.resolve('tmp/accounts-check');
fs.mkdirSync(root, {recursive:true});
const delay = ms => new Promise(resolve => setTimeout(resolve,ms));
async function freePort(){const s=net.createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const p=s.address().port;await new Promise(r=>s.close(r));return p;}
(async()=>{
 const port=await freePort(), debug=await freePort();
 const env={...process.env,LOCALAPPDATA:path.join(root,'Local'),APPDATA:path.join(root,'Roaming')};
 const server=spawn(path.join(root,'guidev.exe'),['-home',path.join(root,'Home'),'-addr','127.0.0.1:'+port],{env,windowsHide:true,stdio:'ignore'});
 let chrome,ws;
 try{
  const origin='http://127.0.0.1:'+port;
  for(let i=0;i<100;i++){try{if((await fetch(origin)).ok)break}catch{}await delay(100);}
  chrome=spawn('C:/Program Files/Google/Chrome/Application/chrome.exe',['--headless=new','--no-first-run','--disable-sync','--disable-background-networking','--disable-component-update','--remote-debugging-port='+debug,'--user-data-dir='+path.join(root,'chrome-profile'),'about:blank'],{env,windowsHide:true,stdio:'ignore'});
  let targets;for(let i=0;i<100;i++){try{targets=await(await fetch('http://127.0.0.1:'+debug+'/json')).json();break}catch{}await delay(100);}
  if(!targets)throw Error('Chrome did not start');
  ws=new WebSocket(targets.find(t=>t.type==='page').webSocketDebuggerUrl);
  await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject});
  let serial=0;const pending=new Map();
  ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(Error(m.error.message)):p.resolve(m.result)}};
  const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}))});
  await call('Page.navigate',{url:origin+'/dev/pagecheck'});
  for(let i=0;i<1200;i++){
   const r=await call('Runtime.evaluate',{expression:'JSON.stringify({state:document.querySelector("#results")?.dataset.result,text:document.querySelector("#results")?.textContent})',returnByValue:true});
   let result;try{result=JSON.parse(r.result.value)}catch{}
   if(result?.state){fs.writeFileSync(path.join(root,'pagecheck.txt'),result.text);console.log(result.text);if(result.state!=='pass')throw Error('pagecheck failed');return}
   await delay(100);
  }
  throw Error('pagecheck timed out');
 }finally{ws?.close();chrome?.kill();server.kill()}
})().catch(e=>{console.error(e);process.exitCode=1});
