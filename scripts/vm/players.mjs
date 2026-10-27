import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { runChild } from './subprocess.mjs';
import { createHash } from 'node:crypto';
import { discover } from './observe.mjs';
import { sleep } from './cdp.mjs';
import { allowed, canonical, classify as baseClassify, classifyKey, coverage, difference, exclusions, random, stateKey } from './player-policy.mjs';

const hash=s=>createHash('sha256').update(s).digest('hex').slice(0,20);
const quoted=s=>JSON.stringify(s);
const replayCommand=args=>'node '+args.map(s=>"'"+s.replaceAll("'","''")+"'").join(' ');
export async function drive(ctx) {
  const {c,options,result,out,currentPlan,snapshot,settled,check,save}=ctx;
  const seed=Number(options.seed||271027), steps=Number(options.steps||200), minutes=Number(options.minutes||60);
  if(!Number.isSafeInteger(seed)||!Number.isInteger(steps)||steps<1||!Number.isFinite(minutes)||minutes<=0)throw Error('seed must be an integer, steps positive integer, minutes positive');
  if(options.allow&&!['local','external'].includes(options.allow))throw Error('--allow local|external');
  const classify=(control,state)=>baseClassify(control,{...state,sharedContent:!!options.baseline});
  const rng=random(seed), pick=a=>a[Math.floor(rng()*a.length)], started=performance.now(), deadline=started+minutes*60000;
  Object.assign(result,{mode:options.mode,seed,steps,minutes,allow:options.allow||'safe',canonicalExclusions:exclusions,knownFailures:[{id:'F2',oracle:'undo canonical equality',description:'stage metadata outside undo; fix belongs to stage-undo branch'}]});
  const ledger=new Map(),states=new Map(),queue=[],trace=[],edges=[],sequences=new Map(),pageDetails=new Map();
  const maxStates=Number(options['max-states']||200);
  const previousLedger=options.ledger?JSON.parse(readFileSync(options.ledger,'utf8')):null;
  let sampleAt=0,statusAt=0,continuousFailure=null,expectedEnd,nonUndoableBranch=false,needsReset=false;
  const read=()=>c.evaluate('__vm.read(false)');
  const controls=()=>c.evaluate(`(${discover.toString()})()`);
  const fail=(name,detail)=>{check(name,false,detail);continuousFailure ||= {name,detail};};
  async function oracles(label,ready=true) {
    let s=await read();
    if(ready&&(s.loading||s.editing||s.busy.env)){
      try{s=await settled();}catch(e){fail('no stuck loading',{label,error:e.message});}
    }
    if(s.blank||s.readError||(!s.content&&!s.loading))fail('no blank/error page',{label,page:s.page,error:s.readError});
    if(!s.sidebar.includes('plan'))fail('sidebar intact',{label,sidebar:s.sidebar});
    if(s.errors.length)fail('no console/script errors',{label,errors:s.errors});
    if(s.noGame)fail('no false no-game state',{label});
    if(ready&&(s.loading||s.editing||s.busy.env))fail('no stuck loading',{label,loading:s.loading,editing:s.editing});
    const batch=await c.evaluate(`({samples:__vm.samples.slice(${sampleAt}),statuses:__vm.statuses.slice(${statusAt}),sampleAt:__vm.samples.length,statusAt:__vm.statuses.length})`);
    sampleAt=batch.sampleAt;statusAt=Math.max(0,batch.statusAt-1);
    for(const p of batch.samples.filter(p=>p.armed)){
      if(p.blank||(!p.content&&!p.loading))fail('continuous no blank page',{label,sample:p});
      if(!p.sidebar.includes('plan'))fail('continuous sidebar intact',{label,sample:p});
      if(p.gapMs>1500)fail('responsive main thread',{label,gapMs:p.gapMs});
      const k=JSON.stringify([p.env,p.problemPlan,p.listVersion]);
      if(sequences.has(k)&&p.sequence<sequences.get(k))fail('problems sequence monotonic',{label,key:k,before:sequences.get(k),after:p.sequence});
      sequences.set(k,p.sequence);
    }
    for(let i=1;i<batch.statuses.length;i++){
      const a=batch.statuses[i-1],b=batch.statuses[i];
      if(a.text&&!/loading|checking|正在|加载/i.test(a.text)&&/loading|加载/i.test(b.text)&&a.inputCount===b.inputCount)fail('results not overwritten by loading',{label,a,b});
    }
    return s;
  }
  function identity(control,state,s) {
    // The DOM is virtualized. Rows are representative action classes, with their
    // concrete target retained for replay; ordinary descendants remain separate.
    const label=control.label.replaceAll(out,'<run>').replaceAll(out.replaceAll('\\','/'),'<run>');
    const stable=control.row?['row',control.area,control.rowKind,control.identity?.['data-kind']||'',/missing|absent/i.test(control.label)?'missing':'present']:[control.tag,control.role,control.identity,control.locator,label];
    // A sidebar button does not become a new control in every popup/focus state.
    // Keep popup/menu context for their controls, and page/list/view context for
    // page controls. Retain all graph occurrences separately in the ledger.
    const context=control.owner==='nav'?['sidebar']:['popup','menu'].includes(control.owner)?state:[s.page||'plan',s.plan,s.modes||[]];
    return hash(JSON.stringify([context,...stable]));
  }
  function record(found,s,key,recipe) {
    const added=[];
    for(const control of found){
      // A container and its pointer-styled text child are one target; nested real
      // buttons are still independently discovered by discover().
      const id=identity(control,key,s);
      if(ledger.has(id)){
        const old=ledger.get(id);if(!old.states.includes(key))old.states.push(key);
        const policy=classify(control,s),eligible=allowed(policy,options.allow)&&control.visible&&control.enabled&&!control.blockedByLayer;
        if(!old.eligible&&eligible&&!old.attemptedAt)Object.assign(old,control,{key:id,state:key,recipe,eligible:true,outcome:'unreachable',reason:'not attempted'});
        continue;
      }
      const policy=classify(control,s), permitted=allowed(policy,options.allow),eligible=permitted&&control.visible&&control.enabled&&!control.blockedByLayer;
      const row={...control,key:id,state:key,states:[key],page:control.owner==='nav'?'sidebar':s.page||'plan',plan:s.plan,tier:policy.tier,classification:policy.reason,blocked:policy.blocked,eligible,recipe,outcome:'unreachable',reason:'not attempted',discoveredAt:new Date().toISOString()};
      if(!permitted){row.outcome='skipped';row.reason=policy.blocked||policy.tier==='unknown'?policy.reason:`${policy.tier} tier disabled (use --allow ${policy.tier==='external'?'external':'local'})`;}
      else if(!eligible){row.reason=!control.visible?'not visible in this state':!control.enabled?'disabled; requires application precondition':'covered by page/popup/menu; not an eligible edge in this state';}
      ledger.set(id,row);added.push(row);
      if(control.context&&control.visible){const right={...row,key:hash(id+'context'),label:control.label+' [context menu]',sourceLabel:control.label,action:{kind:'click',locator:control.locator,button:'right'}};if(!ledger.has(right.key)){ledger.set(right.key,right);added.push(right);}}
    }
    return added;
  }
  async function enqueue(recipe) {
    const s=await read(),found=await controls(),key=stateKey(s,found),id=hash(key);
    record(found,s,id,recipe);
    if(!states.has(id)){const node={key:id,signature:key,page:s.page||'plan',popup:s.popup,menu:s.menu,recipe};states.set(id,node);if(states.size<=maxStates)queue.push(node);}
    return id;
  }
  async function point(action) {
    return c.evaluate(`(()=>{const e=document.querySelector(${quoted(action.locator)});if(!e||e.disabled||e.closest('[inert]'))return null;const b=e.getBoundingClientRect(),x=b.x+b.width/2,y=b.y+b.height/2,h=document.elementFromPoint(x,y);return b.width&&b.height&&(h===e||e.contains(h))?{x,y}:null})()`);
  }
  const targetControl=action=>c.evaluate(`(()=>{const target=document.querySelector(${quoted(action.locator)});return (${discover.toString()})().find(c=>document.querySelector(c.locator)===target)||null})()`);
  async function locate(action) {
    for(let tries=0;tries<12;tries++){
      const p=await point(action);if(p)return p;
      const scroll=await c.evaluate(`(()=>{const e=document.querySelector(${quoted(action.locator)});if(!e||e.closest('[hidden],[inert]'))return null;const b=e.getBoundingClientRect();for(let a=e.parentElement;a;a=a.parentElement){const r=a.getBoundingClientRect();if(a.scrollHeight>a.clientHeight+4&&/(auto|scroll)/.test(getComputedStyle(a).overflowY)&&(b.bottom>r.bottom||b.top<r.top))return {x:Math.max(4,Math.min(innerWidth-4,r.x+r.width/2)),y:Math.max(4,Math.min(innerHeight-4,r.y+r.height/2)),delta:b.bottom>r.bottom?360:-360};}return null})()`);
      if(!scroll){if(await c.evaluate('document.getAnimations().some(a=>a.playState==="running")')){await sleep(60);continue;}return null;}
      await input({kind:'wheel',...scroll});await sleep(60);
    }
    return null;
  }
  async function input(action,logged=true) {
    if(action.kind==='key'){
      const policy=classifyKey(action.combo,await read());
      if(!allowed(policy,options.allow))throw Error('policy blocked keyboard input: '+policy.reason);
    }
    const pointer=action.kind==='click'?await locate(action):null;
    if(action.kind==='click'){
      if(!pointer)throw Error('unreachable control: '+action.locator);
      const actual=await targetControl(action),policy=actual&&classify(actual,await read());
      if(!policy||!allowed(policy,options.allow))throw Error('policy blocked replay target: '+action.locator);
    }
    const entry={index:trace.length,atMs:performance.now()-started,action,started:new Date().toISOString()};
    if(logged){trace.push(entry);save('trace',{schema:1,seed,actions:trace});}
    try {
      if(action.kind==='key')await c.key(action.combo,{repeats:action.repeats||0,beforeRepeat:async()=>{const policy=classifyKey(action.combo,await read());if(!allowed(policy,options.allow))throw Error('policy blocked held key repeat: '+policy.reason);}});
      else if(action.kind==='text')await c.call('Input.insertText',{text:action.text});
      else if(action.kind==='wheel')await c.call('Input.dispatchMouseEvent',{type:'mouseWheel',x:action.x||700,y:action.y||420,deltaX:0,deltaY:action.delta||420});
      else if(action.kind==='branch'){
        // Embed branch inputs so copied VM evidence has no dependency on an
        // unreturned guest branch file or on the original result directory.
        action.actions ||= JSON.parse(readFileSync(action.traceFile,'utf8')).actions;
        const branchFile=join(out,'replay-branch-'+entry.index+'.json');
        writeFileSync(branchFile,JSON.stringify({actions:action.actions}));
        const child=await runChild([...replayArgs(branchFile),'--branch','true'],Math.floor(Math.min(600000,Math.max(1000,deadline-performance.now()))));
        entry.exitCode=child.status;entry.branchOutput=child.stdout.slice(-4000);entry.error=child.error?.message;
        try{
          const summary=JSON.parse(child.stdout),report=JSON.parse(readFileSync(join(summary.output,'result.json'),'utf8'));
          entry.functionalStatus=report.functionalStatus;entry.branchResult=join(summary.output,'result.json');
          entry.nonUndoableChanges=report.nonUndoableChanges||null;entry.failures=report.failures;entry.storage=report.storage;
          result.performanceFailures.push(...report.performanceFailures.map(f=>({...f,branchControl:action.control})));
        }catch{}
        if(entry.functionalStatus!=='passed')fail('isolated branch',{control:action.control,error:entry.error,output:entry.branchOutput});
      }
      else {
        for(let n=1;n<=(action.count||1);n++){
          if(n>1){
            const p=await point(action),actual=p&&await targetControl(action),policy=actual&&classify(actual,await read());
            if(!p||Math.abs(p.x-pointer.x)>2||Math.abs(p.y-pointer.y)>2||!policy||!allowed(policy,options.allow)){
              entry.cancelledRepeats={from:n,reason:'target moved, disappeared or changed to a disallowed tier'};break;
            }
            (entry.repeatTargets||=[]).push({click:n,tier:policy.tier,label:actual.label,locator:actual.locator});
          }
          await c.mouse(action.button||'left',pointer.x,pointer.y,n);entry.issuedClicks=n;
        }
      }
      if(action.delayMs)await sleep(action.delayMs);
      entry.finishedMs=performance.now()-started;
    }catch(e){entry.error=e.message;throw e;}
    return entry;
  }
  const key=combo=>({kind:'key',combo});
  const hasRestoration=(before,after)=>after.plans.some(p=>{
    const old=before.plans.find(b=>b.name===p.name),wanted=old?.workspace?.[0]?.entries;
    return old&&(p.undoDepth>old.undoDepth||(p.workspace?.[0]?.entries!==wanted&&(p.undo?.some(x=>x.entries===wanted)||p.redo?.some(x=>x.entries===wanted))));
  });
  async function root() {
    await input(key('Escape'));await input(key('Escape'));
    let s=await read();
    if(s.navLayout==='top'){await input(key('Control+KeyB'));await sleep(400);s=await read();}
    if(s.page!==null){
      // Sidebar reflow animates after page races. Wait before targeting geometry;
      // page shortcuts can be ignored while a native form input owns focus.
      await settled();await sleep(350);
      await input({kind:'click',locator:'#nav .nav-item[data-id="plan"]'});await settled(null,15000);s=await read();
    }
    if(s.plan!==currentPlan){const plan=s.plans.find(p=>p.name===currentPlan);if(!plan)throw Error('original list was removed');await openPlan(plan);}
    // A native input can retain focus after a dismissed view. Real row input
    // returns keyboard ownership to the editor before focus/undo shortcuts.
    const focusRow=(await controls()).find(x=>x.row&&x.area==='1'&&x.hittable&&x.visible&&classify(x,s).tier==='safe');
    if(focusRow)await input({kind:'click',locator:focusRow.locator});
    await input(key('Digit2'));await input(key('Home'));await settled();
  }
  async function openPlan(plan) {
    const locator=`#nav [data-id="plan:${plan.id}"]`;
    if(await locate({locator}))await input({kind:'click',locator});
    else {
      const s=await read(),row=(await controls()).find(x=>x.row&&x.area==='1'&&x.hittable&&classify(x,s).tier==='safe');
      if(row)await input({kind:'click',locator:row.locator});
      await input(key('Shift+KeyP'));await settled();
      const state=await read(),index=state.plans.findIndex(p=>p.id===plan.id);
      if(index<0)throw Error('list no longer available: '+plan.name);
      await input({kind:'click',locator:`#popup .opts > .opt:nth-child(${index+1})`});
    }
    await settled(null,15000,plan.name);
    if((await read()).plan!==plan.name)throw Error('list cannot be edited in the current game: '+plan.name);
  }
  async function normalizeDetails(page) {
    const details=await c.evaluate(`(${discover.toString()})().filter(x=>x.tag==='SUMMARY').map(x=>({locator:x.locator,open:document.querySelector(x.locator).parentElement.open}))`);
    if(!pageDetails.has(page)){pageDetails.set(page,details);return;}
    for(const wanted of pageDetails.get(page)){
      const actual=details.find(x=>x.locator===wanted.locator);
      if(actual&&actual.open!==wanted.open){await input({kind:'click',locator:wanted.locator});await settled();}
    }
  }
  async function restore(recipe) {
    if(needsReset){if(!await undoTo(startSnapshot,'reset'))throw Error('canonical root cannot be restored');needsReset=false;}
    await root();await normalizeDetails(null);
    for(const action of recipe){const page=(await read()).page;await input(action);await settled();const next=(await read()).page;if(next!==page)await normalizeDetails(next);}
    return oracles('return/replay');
  }
  async function undoTo(before,label) {
    await root();
    // Undo applies to the open list. Visit all lists by real sidebar input.
    const after=await snapshot(label+'-before-undo');
    for(const p of after.plans){
      const original=before.plans.find(b=>b.name===p.name);if(!original)continue;
      if(JSON.stringify(p.workspace)===JSON.stringify(original.workspace)&&p.undoDepth<=original.undoDepth)continue;
      const ui=(await read()).plans.find(b=>b.name===p.name);if(!ui)continue;
      if((await read()).plan!==p.name)await openPlan(ui);
      const wanted=original.workspace?.[0]?.entries,actual=p.workspace?.[0]?.entries;
      let n=Math.max(0,p.undoDepth-original.undoDepth),combo='KeyU';
      if(wanted!==actual){
        const u=p.undo.findIndex(step=>step.entries===wanted),r=p.redo.findIndex(step=>step.entries===wanted);
        if(u>=0)n=u+1;else if(r>=0){n=r+1;combo='Control+KeyR';}
      }
      for(let i=0;i<n;i++){
        // The global U key can be consumed by a page-level status undo. Choose
        // the explicit workspace command through the real palette instead.
        if(combo==='KeyU'){
          await input(key('Control+KeyK'));await settled();
          const choice=(await controls()).find(x=>x.owner==='popup'&&x.visible&&/^undo(?:\s|$)/i.test(x.label));
          if(!choice)throw Error('workspace undo command is unavailable');
          await input({kind:'click',locator:choice.locator});
        }else await input(key(combo));
        await settled();
      }
    }
    await root();
    const restored=await snapshot(label+'-undone'),d=difference(canonical(before),canonical(restored));
    needsReset=!!difference(canonical(startSnapshot),canonical(restored));
    if(d){const known=d.path.includes('staging');fail('undo canonical equality',{label,knownFailure:known?'F2':null,firstDifference:d});}
    return !d;
  }
  async function attempt(row,recipe) {
    const start=performance.now();
    const action=row.action||{kind:'click',locator:row.locator};
    row.action=action;row.attemptedAt=new Date().toISOString();
    try {
      await restore(recipe);
      if(hash(stateKey(await read(),await controls()))!==row.state){row.outcome='unreachable';row.reason='state changed during real-input recipe replay';return;}
      if(action.kind==='click'&&!await locate(action)){row.outcome='unreachable';row.reason='not hittable after real scroll/replay';return;}
      // Re-check the actual target immediately before input. A stale selector must
      // never silently turn a safe button into a newly rendered destructive one.
      if(action.kind==='click'){
        const actual=await targetControl(action),s=await read();
        if(!actual){row.outcome='unreachable';row.reason='target disappeared';return;}
        if(!row.row&&(actual.label!==(row.sourceLabel??row.label)||JSON.stringify(actual.identity)!==JSON.stringify(row.identity))){row.outcome='unreachable';row.reason='semantic target changed during replay';return;}
        const policy=classify(actual,s);
        if(policy.tier!==row.tier||!allowed(policy,options.allow)){row.outcome='skipped';row.reason='target classification changed during replay';return;}
      }
      const before=await snapshot('edge-'+row.key+'-before');
      const failures=result.failures.length;
      await input(action);await settled();await oracles('edge '+row.key);
      const after=await snapshot('edge-'+row.key+'-after');
      needsReset=!!difference(canonical(startSnapshot),canonical(after));
      const child=await enqueue([...recipe,action]);edges.push({from:row.state,to:child,control:row.key});
      await input(key('Escape'));await input(key('Escape'));
      const changed=difference(canonical(before),canonical(after));
      if(changed){
        row.change=changed;
        const undoable=hasRestoration(before,after);
        if(undoable)await undoTo(before,'edge-'+row.key);
        else if(row.tier==='safe')fail('safe action preserves canonical domain',{key:row.key,firstDifference:changed});
        else row.nonUndoable={reason:'persistent write; branch discarded',firstDifference:changed};
      }
      await restore(recipe);
      const returned=await read();
      row.returnVerified=hash(stateKey(returned,await controls()))===row.state;
      if(!row.returnVerified)fail('return state equality',{control:row.key,expected:row.state,actual:hash(stateKey(returned,await controls()))});
      const returnedSnapshot=await snapshot('edge-'+row.key+'-return');
      needsReset=!!difference(canonical(startSnapshot),canonical(returnedSnapshot));
      if(!changed){const d=difference(canonical(before),canonical(returnedSnapshot));if(d)fail('cancel/return canonical equality',{control:row.key,firstDifference:d});}
      row.outcome=result.failures.length===failures?'pressed-ok':'failed';
      row.asserted=row.outcome==='pressed-ok';row.oracles=result.failures.slice(failures);row.reason=undefined;
    }catch(e){row.outcome=/unreachable|not hittable/.test(e.message)?'unreachable':'failed';row.reason=e.message;if(row.outcome==='failed')fail('crawler edge',{key:row.key,error:e.stack});}
    finally {row.elapsedMs=performance.now()-start;saveLedger();}
  }
  function saveLedger() {
    const all=[...ledger.values()],pages=coverage(all),previous=previousLedger;
    const old=new Set((previous?.controls||[]).map(c=>c.key)),now=new Set(all.map(c=>c.key));
      const report={schema:1,seed,mode:options.mode,scope:'discovered controls within reachable fixture states; repeated virtual rows grouped by action/area/missing class',complete:queue.length===0&&states.size<=maxStates&&performance.now()<deadline,states:[...states.values()],edges,controls:all,pages,skippedByTier:Object.fromEntries(['safe','local-destructive','external','unknown'].map(t=>[t,all.filter(c=>c.tier===t&&c.outcome==='skipped').length])),diff:previous?{new:[...now].filter(k=>!old.has(k)),removed:[...old].filter(k=>!now.has(k))}:null};
    save('ledger',report);result.playerCoverage={pages,skippedByTier:report.skippedByTier,states:states.size,controls:all.length,complete:report.complete};
    writeFileSync(join(out,'ledger.md'),`# ${options.mode} coverage\n\nSeed: ${seed}. ${states.size} states, ${all.length} controls.\n\nCoverage = asserted / permitted, visible, enabled edges without an overlay. Offscreen eligible controls stay in the denominator; hidden/disabled controls remain recorded as unreachable.\n\n| Page | Discovered | Eligible | Asserted | Coverage | Failed | Skipped | Unreachable |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n${Object.entries(pages).map(([name,p])=>`| ${name} | ${p.discovered} | ${p.eligible} | ${p.asserted} | ${p.coveragePercent??'—'}% | ${p.failed} | ${p.skipped} | ${p.unreachable} |`).join('\n')}\n\nSkipped by tier: ${JSON.stringify(report.skippedByTier)}\n\n${previous?`Ledger diff: ${report.diff.new.length} new, ${report.diff.removed.length} removed.`:'No previous ledger supplied.'}\n`);
  }
  // Keyboard recipes supplement DOM discovery, including all editor columns and
  // popups that have no permanently visible button. OpTable/palette are observed.
  const recipes=[['Control+KeyK'],['Shift+F10'],['KeyG'],['KeyM'],['KeyN'],['KeyX'],['Shift+KeyP'],['Shift+KeyH'],['KeyI'],['KeyE'],['Shift+Slash'],['Digit1'],['Digit2'],['Digit3'],['Digit4'],['Backslash']];
  // Opening an imported/saved plan can materialize its workspace for the first
  // time. Do this as explicit setup, before the canonical start checkpoint.
  const initialUI=await read();
  for(const plan of initialUI.plans){
    if(!initialUI.sidebar.includes('plan:'+plan.id))continue;
    try{await input({kind:'click',locator:`#nav [data-id="plan:${plan.id}"]`});await settled();}
    catch(e){(result.setupUnreachablePlans||=[]).push({name:plan.name,reason:e.message});}
  }
  await root();
  const startSnapshot=await snapshot('player-start');
  expectedEnd=startSnapshot;
  await input(key('Control+Backslash')); // real documented refnums switch, isolated home only
  await settled();
  save('operation-seeds',await c.evaluate(`(async()=>{const {S}=await import('/js/state.js');return {ops:S.start?.ops,commands:S.start?.commands,nav:S.shell?.nav}})()`));
  let originalFailure=null;
  try {
    if(options.mode==='crawl') {
      await root();await enqueue([]);
      // All sidebar pages are explicit roots; popup branching cannot starve them.
      const nav=(await controls()).filter(x=>x.owner==='nav'&&(x.identity['data-id']||x.identity.id));
      for(const n of nav){
        if(!n.enabled||!n.visible||!n.hittable)continue;
        const rows=[...ledger.values()].filter(r=>r.owner==='nav'&&r.locator===n.locator&&r.eligible&&!r.attemptedAt);
        for(const row of rows)await attempt(row,[]);
      }
      for(const r of recipes){
        await root();const s=await read(),from=hash(stateKey(s,await controls()));
        const recipe=r.map(key),id=hash(from+'key:'+r.join(','));
        const row={key:id,state:from,page:'plan',tier:'safe',eligible:true,visible:true,enabled:true,owner:'keyboard',label:r.join(', '),action:recipe[0],recipe:[],outcome:'unreachable',reason:'not attempted',classification:'documented local key'};
        ledger.set(id,row);await attempt(row,[]);
      }
      let pageCursor=0;
      const pageOrder=[...new Set(queue.map(n=>n.page))];
      const rank=r=>r.owner==='nav'?4:/^#(?:side-split|navtoggle|status|where)/.test(r.locator||'')?3:r.row?2:1;
      while(queue.length&&performance.now()<deadline){
        for(const n of queue)if(!pageOrder.includes(n.page))pageOrder.push(n.page);
        let at=-1;
        for(let tries=0;tries<pageOrder.length&&at<0;tries++){
          const page=pageOrder[pageCursor++%pageOrder.length];at=queue.findIndex(n=>n.page===page);
        }
        const node=queue.splice(at,1)[0];
        const rows=[...ledger.values()].filter(r=>r.state===node.key&&r.eligible&&r.visible&&r.enabled&&!r.attemptedAt).sort((a,b)=>rank(a)-rank(b));
        const row=rows[0];
        if(row){
          // Confirming persistent operations runs in another fresh runner case.
          if(row.tier!=='safe')await isolatedEdge(row,node.recipe);
          else await attempt(row,node.recipe);
          if(rows.length>1)queue.push(node);
        }
        saveLedger();
        if(continuousFailure?.name==='undo canonical equality')break;
      }
      result.crawlLimit=performance.now()>=deadline?'minutes':states.size>maxStates?'max-states':queue.length?'failure':null;
      if(result.crawlLimit)for(const r of ledger.values())if(r.reason==='not attempted')r.reason='crawl stopped: '+result.crawlLimit;
    } else await walk();
  }catch(e){fail('player driver',{error:e.stack});}
  finally {
    try{
      await input(key('Escape'));
      if(nonUndoableBranch){const final=await snapshot('player-end-nonundoable');const d=difference(canonical(expectedEnd),canonical(final));if(d)fail('nonundoable branch stable after cancel',{firstDifference:d});}
      else await undoTo(expectedEnd,'player-end');
      await oracles('end');
    }catch(e){fail('end restoration',{error:e.message});}
    save('trace',{schema:1,seed,actions:trace});saveLedger();
    result.playerElapsedMs=performance.now()-started;
    if(result.failures.length){
      const args=replayArgs(join(out,'trace.json'));
      result.replay={command:replayCommand(args),seed,trace:join(out,'trace.json')};
      save('replay',result.replay);
      if(options.mode==='walk'&&options.shrink!=='false')await shrink();
    }
  }

  function replayArgs(file) {
    return [join(ctx.repo,'scripts/vm/run.mjs'),'--mode','walk','--driver','W','--seed',String(seed),'--steps',String(steps),'--minutes',String(minutes),'--tools',ctx.tools,'--out',join(out,'retries'),'--replay',file,'--shrink','false',...(options.baseline?['--baseline',options.baseline,'--reuse-content','true']:[]),...(options.allow?['--allow',options.allow]:[])];
  }
  async function isolatedEdge(row,recipe) {
    row.attemptedAt=new Date().toISOString();
    const parentBefore=await snapshot('branch-'+row.key+'-parent-before');
    const file=join(out,'branch-'+row.key+'.json');save('branch-'+row.key,{actions:[...recipe,row.action||{kind:'click',locator:row.locator}].map(action=>({action}))});
    // Child runs have a fresh fixture/full filesystem copy; never reuse a dirty root.
    const entry=await input({kind:'branch',traceFile:file,control:row.key});
    const parentAfter=await snapshot('branch-'+row.key+'-parent-return');
    row.returnVerified=!difference(canonical(parentBefore),canonical(parentAfter));
    if(!row.returnVerified)fail('branch parent canonical equality',{control:row.key});
    row.outcome=entry.functionalStatus==='passed'&&row.returnVerified?'pressed-ok':'failed';row.asserted=row.outcome==='pressed-ok';row.branchReplay=file;row.nonUndoable=entry.nonUndoableChanges;row.oracles=entry.failures||[];row.reason=entry.error||entry.branchOutput;saveLedger();
  }
  async function walk() {
    await root();await enqueue([]);
    const replay=options.replay?JSON.parse(readFileSync(options.replay,'utf8')):null;
    if(replay){
      const replayStart=performance.now(),firstAt=replay.actions[0]?.atMs||0;
      for(let i=0;i<replay.actions.length;i++){const entry=replay.actions[i];if(performance.now()>=deadline)throw Error('replay exceeded minute budget');const wait=(entry.atMs||firstAt)-firstAt-(performance.now()-replayStart);if(wait>0)await sleep(wait);await input(entry.action||entry);if(i%10===9)await oracles('replay',false);}
      await settled();await oracles('replay settled');
      if(options.branch==='true'){
        const after=await snapshot('branch-after'),d=difference(canonical(startSnapshot),canonical(after));
        const undoable=hasRestoration(startSnapshot,after);
        if(d&&!undoable&&!d.path.includes('/staging')){nonUndoableBranch=true;expectedEnd=after;result.nonUndoableChanges={reason:'explicitly permitted persistent operation in a disposable branch; no workspace undo exists',beforeCanonicalHash:hash(JSON.stringify(canonical(startSnapshot))),afterCanonicalHash:hash(JSON.stringify(canonical(after))),firstDifference:d,beforeCheckpoint:'player-start',afterCheckpoint:result.checkpoints.at(-1).label};}
      }
      return;
    }
    const pageKeys=['Shift+Digit1','Shift+Digit2','Shift+Digit3','Shift+Digit4','Shift+Digit5','Shift+Digit6','Shift+Digit7','Shift+Digit8'];
    for(let i=0;i<steps&&performance.now()<deadline;i++){
      const beforeFailures=result.failures.length,s=await read(),fast=rng()<.68,delayMs=fast?20+Math.floor(rng()*81):180+Math.floor(rng()*421),group=Math.floor(rng()*10);
      let actions=[],chosenRow=null;
      if(group<3)actions=Array.from({length:2+Math.floor(rng()*4)},()=>({...key(pick(pageKeys)),delayMs}));
      else if(group===3)actions=[key(pick(['Control+KeyK','KeyI','KeyE','Shift+F10'])),{...key('Escape'),delayMs},key('Escape')];
      else if(group===4)actions=[key('Escape'),{...key('Alt+ArrowLeft'),delayMs},key('Escape')];
      else if(group<8){
        // Real editor operations; deliberately interleave queued writes and navigation.
        await root();
        const col=pick(['Digit1','Digit2','Digit3']);
        actions=[key(col),key(pick(['Home','ArrowDown','End'])),{...key(pick(['Space','Space','Space','Shift+KeyJ','Shift+KeyK','Shift+KeyS','KeyT','KeyU'])),repeats:rng()<.3?2:0,delayMs}];
      } else {
        const found=await controls(),state=hash(stateKey(s,found));record(found,s,state,[]);
        const candidates=found.filter(x=>x.visible&&x.enabled&&x.hittable&&allowed(classify(x,s),options.allow)&&(!previousLedger||previousLedger.controls.some(old=>(old.page===(s.page||'plan')||old.page==='sidebar'&&x.owner==='nav')&&old.locator===x.locator&&allowed(old,options.allow))));
        const target=candidates.length?pick(candidates):null;
        chosenRow=target?ledger.get(identity(target,state,s)):null;
        if(target&&classify(target,s).tier!=='safe'){
          await isolatedEdge(chosenRow,trace.filter(t=>t.action.kind!=='branch').map(t=>t.action));
          result.executedSteps=i+1;if(result.failures.length>beforeFailures)break;continue;
        }
        actions=target?[{kind:'click',locator:target.locator,count:pick([1,1,2,3]),delayMs},...(rng()<.5?[key('Escape')]:[])]:[key('Escape')];
      }
      for(const action of actions){try{await input(action);}catch(e){if(/unreachable control|policy blocked/.test(e.message)){(result.skippedActions||=[]).push({step:i,action,reason:e.message});}else fail('walk action',{step:i,error:e.message});break;}}
      // The observer samples throughout a burst. Drain it after the group so that
      // querying thousands of domain rows cannot turn 20ms spam into slow input.
      await oracles('walk '+i,false);
      if(chosenRow){
        try{await settled();await oracles('random control '+i);await snapshot('random-control-'+i);chosenRow.outcome=result.failures.length===beforeFailures?'pressed-ok':'failed';chosenRow.asserted=chosenRow.outcome==='pressed-ok';chosenRow.attemptedAt=new Date().toISOString();}catch(e){chosenRow.outcome='failed';chosenRow.reason=e.message;fail('random control',{step:i,error:e.message});}
      }
      if(i%10===9){try{await settled();await oracles('checkpoint '+i);await snapshot('walk-'+i);}catch(e){fail('checkpoint',{step:i,error:e.message});}}
      result.executedSteps=i+1;save('trace',{schema:1,seed,actions:trace});
      if(result.failures.length>beforeFailures){originalFailure=result.failures[beforeFailures];break;}
    }
    await settled();await oracles('walk complete');
  }
  async function shrink() {
    // Replays always use a fresh runner copy. Keep a candidate only if the same
    // oracle fails; unrelated setup/selector failures cannot shrink an app bug.
    const target=originalFailure||result.failures[0];
    const signature=f=>JSON.stringify([f?.name,f?.detail?.knownFailure||null,f?.detail?.firstDifference?.path||null]);
    let best=trace.map(t=>({action:t.action})),attempts=[];
    const max=Number(options['shrink-runs']||6);
    async function retry(actions){
      const file=join(out,'shrink-candidate.json');writeFileSync(file,JSON.stringify({actions}));
      const r=await runChild(replayArgs(file),180000);
      let reproduced=false,output=null;
      try{const j=JSON.parse(r.stdout);output=j.output;const report=JSON.parse(readFileSync(join(output,'result.json'),'utf8'));reproduced=report.failures.some(f=>signature(f)===signature(target));}catch{}
      attempts.push({length:actions.length,reproduced,output,error:r.error?.message});return reproduced;
    }
    // Shorter prefixes, then removal chunks (including middle actions).
    for(let n=Math.floor(best.length/2);n>0&&attempts.length<Math.ceil(max/2);n=Math.floor(n/2))if(await retry(best.slice(0,n)))best=best.slice(0,n);
    for(let chunk=Math.ceil(best.length/2);chunk>=1&&attempts.length<max;chunk=Math.floor(chunk/2))for(let at=0;at<best.length&&attempts.length<max;at+=chunk){const candidate=best.slice(0,at).concat(best.slice(at+chunk));if(candidate.length&&await retry(candidate)){best=candidate;break;}}
    save('minimal-trace',{schema:1,seed,actions:best});save('shrink',{oracle:target?.name,signature:signature(target),originalLength:trace.length,minimalLength:best.length,attempts,command:replayCommand(replayArgs(join(out,'minimal-trace.json'))),minimality:'bounded prefix/chunk reduction, not proof of global minimum'});
  }
}
