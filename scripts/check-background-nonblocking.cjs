// Real-data guidev check. Pass an explicitly prepared manifest of backed-up,
// disposable mods: [{id,name,path,workshop}]. Without --execute only the modal is tested.
// node scripts/check-background-nonblocking.cjs <origin> <manifest> <output> [--execute] [--fast]
const {spawn}=require('node:child_process');
const fs=require('node:fs'),path=require('node:path'),net=require('node:net');
const [origin,manifest,output]=process.argv.slice(2);
if(!origin||!manifest||!output)throw Error('Expected origin, manifest and output file');
const execute=process.argv.includes('--execute'),fast=process.argv.includes('--fast');
const mods=JSON.parse(fs.readFileSync(manifest,'utf8'));
if(!mods.length||mods.some(m=>!m.id||!m.name||!m.path||!/^\d+$/.test(m.workshop)))throw Error('Invalid mod manifest');
const delay=ms=>new Promise(r=>setTimeout(r,ms));
async function freePort(){const s=net.createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const p=s.address().port;await new Promise(r=>s.close(r));return p;}
(async()=>{let chrome,ws;try{
 const debug=await freePort(),profile=path.resolve(path.dirname(output),'chrome-'+path.basename(output,'.json'));
 chrome=spawn(process.env.CHROME||'C:/Program Files/Google/Chrome/Application/chrome.exe',['--headless=new','--no-first-run','--disable-sync','--disable-background-networking','--remote-debugging-port='+debug,'--user-data-dir='+profile,'about:blank'],{windowsHide:true,stdio:'ignore'});
 let targets;for(let i=0;i<100;i++){try{targets=await(await fetch('http://127.0.0.1:'+debug+'/json')).json();break}catch{}await delay(100);}
 if(!targets)throw Error('Chrome did not start');
 ws=new WebSocket(targets.find(t=>t.type==='page').webSocketDebuggerUrl);
 await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});let serial=0;const pending=new Map();
 ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.j(Error(m.error.message)):p.r(m.result)}};
 const call=(method,params={})=>new Promise((r,j)=>{const id=++serial;pending.set(id,{r,j});ws.send(JSON.stringify({id,method,params}))});
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 await call('Page.navigate',{url:origin});await delay(15000);
 await evaluate('(async()=>{(await import("/js/popups.js")).close()})()');
 const result=await evaluate(`(async()=>{
 const {api}=await import('/js/api.js'),{S}=await import('/js/state.js'),{go}=await import('/js/navigation.js'),pop=await import('/js/popups.js');
 const mods=${JSON.stringify(mods)},refs=mods.map(m=>m.id),out=window.__backgroundCheck={measures:[],spaceCalls:[],errors:[]};
 window.addEventListener('error',e=>out.errors.push({message:e.message,file:e.filename,line:e.lineno,stack:e.error?.stack}));window.addEventListener('unhandledrejection',e=>out.errors.push({message:String(e.reason),stack:e.reason?.stack}));
 const sleep=ms=>new Promise(r=>setTimeout(r,ms));
 const timed=async(name,fn)=>{const active=await api.Tasks(),t=performance.now();try{const r=await fn();out.measures.push({name,ms:Math.round(performance.now()-t),active:active.running,error:r?.error||'',empty:r?.envs===0});return r}catch(e){out.measures.push({name,ms:Math.round(performance.now()-t),active:active.running,error:String(e)});throw e}};
 // Exercise the actual library selection action. Never accept this popup here.
 await (await import('/js/library.js')).openSearch(mods[0].name);
 const row=[...document.querySelectorAll('.lib-row')].find(r=>r.querySelector('.c.name')?.textContent.includes(mods[0].name));
 if(!row)throw Error('Test mod is not visible');row.dispatchEvent(new MouseEvent('click',{bubbles:true,ctrlKey:true}));
 const button=document.querySelector('button[data-env-write="subscription-remove"]');if(!button)throw Error('Remove subscription action unavailable');
 let requested;const originalFetch=window.fetch;
 window.fetch=async(...args)=>{const req=JSON.parse(args[1]?.body||'{}');if(req.m==='LibraryPreview'){requested=req.a[1];if(requested.length!==1)throw Error('Unexpected preview selection')};if(req.m==='LibraryDo')throw Error('Unexpected mutation during modal check');return originalFetch(...args)};
 const click=performance.now();button.click();for(let i=0;i<2000&&!S.popup;i++)await sleep(5);
 out.modalMs=Math.round(performance.now()-click);out.modalLoading=document.querySelector('#popup .body')?.textContent;
 for(let i=0;i<2000&&document.querySelector('#popup .go')?.disabled;i++)await sleep(5);
 pop.close();window.fetch=originalFetch;
 if(!${execute})return out;
 const preview=await timed('preview',()=>api.LibraryPreview('subscription-remove',refs));
 if(!preview.apply)throw Error('Test deletion preview refused');
 await go('plan');pop.close();await sleep(200);
 // All mutations below must be exactly the manifest, including the fast dialog.
 window.fetch=async(...args)=>{const req=JSON.parse(args[1]?.body||'{}');if(req.m==='LibraryDo'&&(JSON.stringify(req.a[1])!==JSON.stringify(refs)||!['subscription-remove','subscription-remove-fast'].includes(req.a[0])))throw Error('Unexpected deletion target');const t=performance.now();const response=await originalFetch(...args);if(req.m==='Toggle'){const j=await response.clone().json();out.spaceCalls.push({ms:Math.round(performance.now()-t),error:j.error||''})}return response};
 if(${fast}){
   let completed;const done=new Promise(r=>completed=r);
   const p=pop.libraryConfirm('subscription-remove',refs,'remove subscription',promise=>promise.then(completed));await p;
   document.querySelector('#popup input[type=checkbox]').checked=true;
   document.querySelector('#popup .go').click();out.secondConfirmation=!!S.popup&&document.querySelector('#popup .body').textContent.includes(refs[0]);
   if(!out.secondConfirmation)throw Error('Missing irreversible confirmation');document.querySelector('#popup .go').click();await done;
 }else await timed('submit',()=>api.LibraryDo('subscription-remove',refs));
 const began=performance.now();out.tasksAtStart=await api.Tasks();
 const reads=Promise.all([timed('Home',()=>api.Home()),timed('Shell',()=>api.Shell()),timed('Workspace',()=>api.Workspace()),timed('PlanManage',()=>api.PlanManage(0)),timed('Library',()=>api.Library('',[],'name'))]);
 S.focus=1;S.cols[1].sel.clear();S.cols[1].cursor=1;document.activeElement?.blur();pop.close();
 out.keyState={screen:S.screen,popup:!!S.popup,view:S.cols[1].view.length};
 const start=performance.now();
 for(let i=0;i<10;i++)document.body.dispatchEvent(new KeyboardEvent('keydown',{key:' ',code:'Space',bubbles:true}));
 out.optimisticMs=Math.round(performance.now()-start);
 for(let i=0;i<1200;i++){if(!S.editing&&out.spaceCalls.length===10)break;await sleep(25)}
 out.spaceDrainMs=Math.round(performance.now()-start);out.editorRows=document.querySelectorAll('#board .row').length;out.sidebarLists=S.shell?.plans?.length;
 await reads;
 for(const [name,id] of [['home page','home'],['list management page','plans'],['library page','library'],['editor page','plan']])await timed(name,()=>go(id));
 for(let i=0;i<2400;i++){const ts=await api.Tasks();if(!ts.running&&!ts.queued){out.tasks=ts;break}await sleep(50)}
 window.fetch=originalFetch;out.durationMs=Math.round(performance.now()-began);return out;
 })()`);
 fs.writeFileSync(output,JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
 if(execute&&(result.spaceCalls.length!==10||result.errors.length))throw Error('Keyboard or browser check failed; inspect output');
}finally{ws?.close();chrome?.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
