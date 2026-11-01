import { gzipSync } from 'node:zlib';
import { createHash } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir, cpus, totalmem, platform } from 'node:os';
import { resolve, join, dirname, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import { connect, sleep } from './cdp.mjs';
import { observe, discover } from './observe.mjs';
import { fingerprints } from './checkpoints.mjs';
import { treeBytes, cleanupRun, hostOutput } from './storage.mjs';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
// A VM guest gets an exported copy of the scripts with no Git; RIMARCHO_VM_BUILD names its commit.
const common=spawnSync('git',['-C',repo,'rev-parse','--path-format=absolute','--git-common-dir'],{encoding:'utf8',windowsHide:true});
const hasGit=common.status===0;
assert(hasGit||process.env.RIMARCHO_VM_BUILD,'run from a Git checkout, or set RIMARCHO_VM_BUILD in an exported copy');
const repositoryRoot=hasGit?resolve(common.stdout.trim(),'..'):repo;
const options = {};
for (let i = 2; i < process.argv.length; i += 2) {
  const key = process.argv[i];
  if (!key.startsWith('--') || !process.argv[i+1]) throw Error('options need --name value');
  options[key.slice(2)] = process.argv[i+1];
}
const driver = options.driver || 'W', scenario = Number(options.scenario || 1);
const mode = options.mode;
assert(!mode || ['crawl','walk'].includes(mode), 'use --mode crawl|walk');
assert(!mode || driver === 'W', 'crawl/walk require real window driver W');
assert(['W','A','C'].includes(driver) && scenario >= 1 && scenario <= 4, 'use --driver W|A|C --scenario 1|2|3|4');
const outside = path => { for(const root of [repo,repositoryRoot]) { const r=relative(root,resolve(path));assert(r.startsWith('..')||isAbsolute(r),`results and test homes must be outside the repository: ${path}`); } return resolve(path); };
const guest=process.env.RIMARCHO_VM_GUEST==='1';
assert(options.allow!=='external'||guest,'external actions are enabled only in the test VM');
assert(!options.baseline || guest,'host runs support only the generated fixture; real baselines are VM-only');
assert(!options.baseline || options['reuse-content']==='true','VM baselines require --reuse-content true (no full content copy)');
assert(!options.baseline || mode || scenario!==2,'scenario 2 deployment switching needs private content; skipped on a shared baseline');
// --real-home true|<dir>: the guest player's own home, used directly. The run backs up its DB
// first and restores it afterwards; child replays (--start-db) get a private DB from that backup.
const realHome=options['real-home']?resolve(options['real-home']==='true'?join(process.env.APPDATA||'','RimArcho'):options['real-home']):null;
const childOfReal=!!(realHome&&options['start-db']);
assert(!realHome || guest,'the real home is used only in the test VM; revert snapshot RimArcho-Real-Vulkan after the round');
assert(!realHome || !options.baseline,'--real-home replaces --baseline');
assert(!realHome || mode || scenario!==2,'scenario 2 deployment switching changes content; refused on the real home');
const outputBase = outside(guest ? options.out : hostOutput(options.out));
mkdirSync(outputBase, { recursive: true });
const out = mkdtempSync(join(outputBase, `${mode || 's'+scenario}-${driver}-`));
const tools = resolve(options.tools || 'D:/RimArcho-test/tools');
const exe = name => join(tools, name + (platform() === 'win32' ? '.exe' : ''));
const result = { schema: 1, scenario, driver, seed: 271027, fixture: realHome ? 'real-home' : options.baseline ? 'real-copy' : 'small', output: out, started: new Date().toISOString(), host: { cpu: cpus()[0]?.model, cores: cpus().length, memory: totalmem(), platform: platform() }, timings: [], checks: [], coverage: [], checkpoints: [], failures: [], performanceFailures: [] };
const save = (name, data) => {
 const json=JSON.stringify(data,null,2);
 if(options.archive==='true' && (name==='window-trace'||/^\d{2}-/.test(name)))writeFileSync(join(out,name+'.json.gz'),gzipSync(json));
 else writeFileSync(join(out,name+'.json'),json);
 if(options.archive==='true' && ['perf','browser-cpu','window-trace','result'].includes(name))writeFileSync(join(dirname(outputBase),out.split(/[\\/]/).pop()+'-'+name+'.json'+(name==='window-trace'?'.gz':'')),name==='window-trace'?gzipSync(json):json);
};
const command = (file, args, env, timeout = 15000) => {
  const r = spawnSync(file, args, { encoding: 'utf8', timeout, env: { ...process.env, ...env }, windowsHide: true, maxBuffer: 128 * 1024 * 1024 });
  if (r.error || r.status !== 0) throw Error(`${file} ${args.join(' ')}: ${r.error?.message || r.stderr || r.stdout}`);
  return r.stdout;
};
const home = realHome && !childOfReal ? realHome : join(out, 'case', 'home');
const realStart = join(out, 'real-start.db');
let c, child, api, env, currentPlan, checkpointSeq = 0, eventStream;
const events = [], requestWaiters = new Map(), browserTimeline=[];let traceComplete;
const check = (name, ok, detail) => { result.checks.push({ name, ok: !!ok, detail }); if (!ok) result.failures.push({name,detail}); };
const timed = async (name, fn, budgetMs = 2000) => { const start=performance.now(); try { return await fn(); } finally { const ms=performance.now()-start;result.timings.push({name,ms,budgetMs});if(budgetMs!==null&&ms>budgetMs)result.performanceFailures.push({name,ms,budgetMs}); } };
const domain = snap => ({ sources:snap.sources, instances:snap.instances, links:snap.links, environments: snap.environments, plans: snap.plans, lineage_data: snap.lineage_data, lineage_tag:snap.lineage_tag, instance_note:snap.instance_note, user_rule:snap.user_rule, modsConfigHashes:snap.modsConfigHashes });
const payload = snap => snap.plans.map(p => ({ name:p.name, entries:p.workspace, staging:p.staging }));
const entries = (snap,name=currentPlan) => snap.plans.find(p=>p.name===name)?.workspace?.[0]?.entries;
const undoDepth = (snap,name=currentPlan) => snap.plans.find(p=>p.name===name)?.undoDepth;
async function snapshot(label) {
  let data,ui;
  for(let attempt=0;attempt<3;attempt++){
    data=JSON.parse(command(exe('vmstate'), ['--home',home,'--plan',currentPlan]));
    ui=c ? await c.evaluate('window.__vm.read()') : null;
    // A background publication can begin a new page read while vmstate runs.
    // Capture a settled checkpoint instead of diagnosing its loading view as blank.
    if(!mode||!ui||!(ui.loading||ui.editing||ui.busy.env))break;
    await settled();
  }
  const artifact = `${String(++checkpointSeq).padStart(2,'0')}-${label}`;
  const digest=createHash('sha256').update(JSON.stringify(data)).digest('hex');
  result.checkpoints.push({label,sha256:digest,domainHashes:fingerprints(data),counts:{instances:data.instances.length,plans:data.plans.length},surface:ui?{page:ui.page,plan:ui.plan,popup:ui.popup}:null});
  if(ui) {
    if(ui.page===null && data.observation && (!mode || ui.plan===currentPlan)){
      const expected=data.observation.entries.map(e=>e.i||'pkg:'+e.m);
      const shown=ui.columns[1].entries.map(e=>e.id||e.ref);
      check(label+': window enabled list matches domain',JSON.stringify(shown)===JSON.stringify(expected),{expected:expected.length,shown:shown.length});
    }
    const ids=ui.columns.flatMap(col=>col.entries.map(e=>e.id).filter(Boolean));
    check(label+': no duplicate instances in columns',new Set(ids).size===ids.length);
    check(label+': page has content',!ui.blank && ui.content,ui.page);
    check(label+': sidebar lists intact',ui.plans?.length===data.plans.length,{ui:ui.plans?.length,db:data.plans.length});
    check(label+': no console/script errors',ui.errors.length===0,ui.errors);
    if(!mode)check(label+': open plan matches',ui.plan===currentPlan,{expected:currentPlan,actual:ui.plan});
  }
  return data;
}
async function until(fn, ms=15000) {
  const end=performance.now()+ms;
  while(performance.now()<end) { const r=await fn(); if(r) return r;await sleep(50); }
  throw Error(`condition timed out after ${ms}ms; state=${c?JSON.stringify(await c.evaluate('(()=>{const s=__vm.read(false);return {page:s.page,plan:s.plan,popup:s.popup,loading:s.loading,editing:s.editing,busy:s.busy,content:s.content,status:s.status,navLayout:s.navLayout,activeElement:s.activeElement,errors:s.errors}})()')):'no window'}`);
}
async function settled(page, ms=10000, plan) {
  let last='', stable=0;
  return until(async()=>{
    const s=await c.evaluate(mode?'window.__vm.read(false)':'window.__vm.read()');
    if(s.blank || s.readError) throw Error(`DOM oracle: ${s.readError || 'blank page'}`);
    // Background analysis status can change continuously on a large library.
    // Page loading/editing and list publication gate readiness; focus/rows gate
    // visual stability without repeatedly serializing all problem/instance data.
    const signature=JSON.stringify([s.page,s.plan,mode?s.columnState:s.columns,mode&&s.busy.analyze?null:s.status,s.popup,s.problems?.listVersion]);
    const ready=!s.loading && !s.editing && !s.busy.env && s.content && s.sidebar.includes('plan') && (page===undefined || s.page===page) && (plan===undefined || s.plan===plan);
    stable=ready && signature===last ? stable+1 : 0;last=signature;
    return stable>=5 && s;
  },ms);
}
async function environmentReady(page) {
  const start=performance.now();
  try { return await timed('environment-switch-ready',()=>settled(page,15000),5000); }
  catch(e){
    check('environment switch meets 15s hard deadline',false,e.message);
    // Keep the failure, then collect eventual completion/restoration for diagnosis.
    const state=await settled(page,120000);
    result.timings.push({name:'environment-switch-eventual',ms:performance.now()-start,budgetMs:15000});
    return state;
  }
}
async function press(combo, opts) { await c.key(combo,opts); }
async function editor() {
  await press('Escape');
  const state=await uiState();
  if(state.page===null && state.plan===currentPlan) return settled(null,15000);
  const plan=state.plans.find(p=>p.name===currentPlan);
  assert(plan,`plan unavailable: ${currentPlan}`);
  if(state.page!==null || !state.sidebar.includes(`plan:${plan.id}`)) {
    await c.click('#nav .nav-item[data-id="plan"]');await settled(null,15000);
  }
  await c.click(`#nav [data-id="plan:${plan.id}"]`);
  await until(async()=>(await uiState()).plan===currentPlan);
  await settled(null,15000);
}
async function popupFeedback(name, action, opened) {
  await timed(name,async()=>{await action();await until(async()=>!!(await uiState()).popup===opened,15000)},200);
}
async function enabledSecond() {
  await press('Digit2');await press('Home');await press('ArrowDown');
}
async function dismiss() { await press('Escape');await settled(); }
async function uiState() { return c.evaluate('window.__vm.read()'); }
async function windowEvidence() {
  await c.evaluate('__vm.stop()');
  const evidence=await c.evaluate('(()=>{const {read,stop,timer,inputs,events,errors,samples,feedback,statuses,...meta}=__vm;return meta})()');
  // Long real-content crawls can accumulate tens of MB. Bound each CDP response
  // instead of serializing the entire observer through a single 15s request.
  for(const name of ['inputs','events','errors','samples','feedback','statuses']) {
    const length=await c.evaluate(`__vm.${name}.length`);evidence[name]=[];
    for(let at=0;at<length;at+=100)evidence[name].push(...await c.evaluate(`__vm.${name}.slice(${at},${at+100})`));
  }
  return evidence;
}
async function clickText(selector,text) {
  for(let i=0;i<24;i++){
    const point=await c.evaluate(`(()=>{for(const e of document.querySelectorAll(${JSON.stringify(selector)})){if(e.disabled || e.textContent.trim()!==${JSON.stringify(text)})continue;const b=e.getBoundingClientRect(),x=b.x+b.width/2,y=b.y+b.height/2,h=document.elementFromPoint(x,y);if(b.width&&b.height&&(h===e||e.contains(h)))return {x,y};}return null})()`);
    if(point){await c.mouse('left',point.x,point.y);return point;}
    if(i===23)throw Error(`visible control unavailable: ${selector} ${text}`);
    const p=await c.evaluate('(()=>{const b=document.querySelector("#screen").getBoundingClientRect();return {x:b.x+b.width/2,y:b.y+b.height/2}})()');
    await c.call('Input.dispatchMouseEvent',{type:'mouseWheel',...p,deltaX:0,deltaY:400});await sleep(50);
  }
}
async function cancelUnsubscribe() {
  const observed=JSON.parse(command(exe('vmstate'),['--home',home,'--plan',currentPlan])).observation;
  const target=await c.evaluate(`(()=>{const kinds=${JSON.stringify(observed.kinds)};return window.__vm.read().columns.slice(0,2).flatMap((c,k)=>c.entries.map((e,i)=>({...e,k,i}))).find(e=>kinds[e.id]==='workshop')})()`);
  if(!target){result.coverage.push({action:'unsubscribe cancellation',status:'unsupported: no Workshop row'});return;}
  for(const close of ['cancel','esc','backdrop']) {
    await press(target.k===0?'Digit1':'Digit2');await press('Home');
    if(target.i)await c.key('ArrowDown',{repeats:target.i-1});
    const b=await snapshot(`before-unsubscribe-${close}`);
    await press('Delete');await until(async()=>!!(await uiState()).popup);
    check('unsubscribe confirmation is for Workshop',await c.evaluate('document.querySelector("#popup").innerText.toLowerCase().includes("unsubscribe")'));
    if(close==='cancel')await c.click('#popup .btns button:not(.go)');
    else if(close==='esc')await press('Escape');
    else {const p=await c.evaluate('(()=>{const b=document.querySelector("#popup").getBoundingClientRect();return {x:b.x+4,y:b.y+4}})()');await c.mouse('left',p.x,p.y);}
    await settled();check(`unsubscribe ${close} preserves domain`,JSON.stringify(domain(await snapshot(`unsubscribe-${close}`)))===JSON.stringify(domain(b)));
  }
}
async function repeatedSubscription() {
  // A loopback peer closes the TLS handshake. This exercises real submit/queue/failure
  // handling without remote accounts, certificates, fixture hooks or product changes.
  const server=createServer(socket=>socket.destroy());
  await new Promise((ok,fail)=>{server.once('error',fail);server.listen(0,'127.0.0.1',ok)});
  const url=`https://127.0.0.1:${server.address().port}/phase1a.git`;
  const tasks=()=>c.evaluate('(async()=>{const {api}=await import("/js/state.js");return api.Tasks()})()');
  try {
    for(const clicks of [2,3]) {
      const before=await snapshot(`before-subscription-${clicks}`),known=new Set((await tasks()).rows.map(r=>r.id));
      await press('Shift+Digit6');await settled('library');
      await press('Shift+Digit6');await settled('library');
  await c.click('#nav [data-id="subscribe"]');await settled('subscribe');
      await c.click('.subscribe-omni input');await c.call('Input.insertText',{text:url});
      await until(()=>c.evaluate('!!document.querySelector(".subscribe-foot button.primary:not(:disabled)")'));
      const point=await c.click('.subscribe-foot button.primary');
      for(let i=2;i<=clicks;i++)await c.mouse('left',point.x,point.y,i);
      await until(async()=>{const rows=(await tasks()).rows.filter(r=>!known.has(r.id));return rows.length&&rows.every(r=>!['queue','run'].includes(r.state))},15000);
      const added=(await tasks()).rows.filter(r=>!known.has(r.id));save(`subscription-${clicks}-tasks`,added);
      check(`subscription ${clicks}-click queues one request`,added.length===1,{tasks:added.map(r=>({kind:r.kind,state:r.state,line:r.line}))});
      check(`subscription ${clicks}-click reports local transport failure`,added.length===1&&added[0].state==='fail');
      await editor();const after=await snapshot(`after-subscription-${clicks}`);
      check(`subscription ${clicks}-click failure preserves plans and instances`,JSON.stringify(payload(before))===JSON.stringify(payload(after))&&JSON.stringify(before.instances)===JSON.stringify(after.instances));
    }
    result.coverage.push({action:'subscription submit double/triple click',status:'asserted: loopback TLS failure; successful download fixture remains Phase 1b'});
  } finally {server.close();}
}
async function cancelExportOverwrite() {
  if(platform()!=='win32') {result.coverage.push({action:'export overwrite cancellation',status:'unsupported: Windows native helper'});return;}
  const path=join(out,'existing.archolist.json'),marker='existing export must survive cancellation\n';
  writeFileSync(path,marker);
  await press('KeyE');await settled('plan-export');
  await c.click('#screen .fmt');await settled('plan-export');
  const evidence=join(out,'native-save.json');
  const helper=spawn('powershell.exe',['-NoProfile','-File',join(repo,'scripts/vm/native-save-cancel.ps1'),'-AppProcessId',String(child.pid),'-ExistingFile',path,'-Evidence',evidence],{windowsHide:true,stdio:['ignore','pipe','pipe']});
  let logs='';helper.stdout.on('data',b=>logs+=b);helper.stderr.on('data',b=>logs+=b);
  const completion=new Promise((ok,fail)=>{helper.on('error',fail);helper.on('exit',code=>code===0?ok():fail(Error(`native save cancellation: ${logs}`)));});
  // Observe helper failure immediately even if the click itself fails.
  completion.catch(()=>{});
  try {await clickText('#screen button','save as a file…');await completion;}
  finally {if(helper.exitCode===null)helper.kill();}
  await settled('plan-export');
  check('export overwrite cancellation preserves existing bytes',readFileSync(path,'utf8')===marker);
  result.coverage.push({action:'export overwrite cancellation',status:'asserted: native Windows control No then Cancel',evidence:'native-save.json'});
  await editor();
}
async function importPreview() {
  await editor();await press('KeyI');await settled('plan-import');
  await c.click('.xc-body textarea.xc-text');await press('Control+KeyA');
  await c.call('Input.insertText',{text:'ludeon.rimworld'});
  await c.click('.xc-body .xc-row.end button.primary');
  await until(()=>c.evaluate('!!document.querySelector("[data-env-write=import-apply]")'));
  await settled('plan-import');
}
async function freshProblems() {
  if(driver==='C'){result.coverage.push({action:'live problem freshness',status:'unsupported: C has no live publication'});return;}
  const start=performance.now();
  await until(async()=>{
    const p=c?await c.evaluate('(async()=>{const {api}=await import("/js/state.js");return api.ListProblems()})()'):await api('ListProblems');
    return p&&!p.updating;
  },120000);
  const p=c?await c.evaluate('(async()=>{const {api}=await import("/js/state.js");return api.ListProblems()})()'):await api('ListProblems');
  const n=c?await c.evaluate('(async()=>{const {api}=await import("/js/state.js");return api.Needs()})()'):await api('Needs');
  const board=c?(await uiState()).problems:(await api('Board')).problems;
  save('fresh-problems',{listProblems:p,needs:n,board});
  check('fresh problems context agrees',p.environment===board.environment&&p.plan===board.plan&&p.listVersion===board.listVersion&&p.environment===n.environment&&p.plan===n.plan&&p.listVersion===n.listVersion);
  check('per-mod problem ownership agrees',JSON.stringify(p.mods)===JSON.stringify(board.mods));
  check('fresh problems counts agree',p.count===board.count&&p.count===n.count,{list:p.count,board:board.count,needs:n.count});
  result.timings.push({name:'fresh-problems',ms:performance.now()-start,budgetMs:120000});
  for(let i=0;i<5;i++)await timed('ListProblems-hot-read',()=>c?c.evaluate('(async()=>{const {api}=await import("/js/state.js");return api.ListProblems()})()'):api('ListProblems'),100);
}
async function act(name,...args) {
  if(driver==='C') {
    const map={Sort:['sort'],Undo:['undo'],Redo:['redo'],Preview:['sort','--dry-run'],CompleteDependencies:['loadout','complete-dependencies'],ReplaceRecommended:['loadout','replace-recommended',...args[0]||[]],UseInstalled:['loadout','use-installed',...args[0]||[]]};
    return command(exe('archo'),['--open-plan',currentPlan,...map[name]],env);
  }
  return api(name,...args);
}
async function listen(base) {
  const controller=new AbortController();eventStream=controller;
  const response=await fetch(base+'/events',{signal:controller.signal});
  const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='';
  (async()=>{try{while(true){const {value,done}=await reader.read();if(done)return;buffer+=decoder.decode(value,{stream:true});let at;while((at=buffer.indexOf('\n\n'))>=0){const part=buffer.slice(0,at);buffer=buffer.slice(at+2);const line=part.split('\n').find(l=>l.startsWith('data: '));if(!line)continue;const e=JSON.parse(line.slice(6));events.push(e);if(e.name==='environment'){const p=requestWaiters.get(e.data.request);if(p){requestWaiters.delete(e.data.request);p(e.data)}}}}}catch(e){if(!controller.signal.aborted)result.failures.push({name:'event stream',detail:String(e)})}})();
}
async function setup() {
  const preparing=performance.now();
  for(const name of ['vmstate','archo',...(driver==='W'?['RimArcho']:driver==='A'?['guidev']:[])]) assert(existsSync(exe(name)),`build ${exe(name)} first (scripts/vm/README.md)`);
  const sq=p=>`'${p.replaceAll("'","''")}'`;
  if(realHome) {
    assert(existsSync(join(realHome,'archo.db')),'real home has no archo.db: '+realHome);
    if(childOfReal) {
      mkdirSync(home,{recursive:true});cpSync(options['start-db'],join(home,'archo.db'));
      if(existsSync(join(realHome,'config.toml')))cpSync(join(realHome,'config.toml'),join(home,'config.toml'));
      // Packs default to <home>/packs; point the private DB at the real folder.
      command('sqlite3',[join(home,'archo.db'),`INSERT INTO meta(key,value) VALUES('cfg:pack.dir',${sq(join(realHome,'packs'))}) ON CONFLICT(key) DO NOTHING;`]);
      result.sharedContent={home:realHome,policy:'private DB from the parent start state; real content reused; content writes skipped'};
    } else {
      command('sqlite3',['-readonly',join(realHome,'archo.db'),`.backup ${sq(realStart)}`],{},120000);
      for(const f of ['config.toml','tasks.json'])if(existsSync(join(realHome,f)))cpSync(join(realHome,f),join(out,'real-start-'+f));
      result.sharedContent={home:realHome,policy:'guest real home used directly; DB, config and tasks restored after the run; content writes skipped'};
    }
    // The player policy and walker read English labels (the copied baseline set this too).
    command(exe('archo'),['config','set','ui.lang','en'],{RIMARCHO_HOME:home});
  } else if(options.baseline) {
    const baseline=outside(options.baseline);assert(existsSync(join(baseline,'isolated.json')),'baseline needs isolated.json from prepare-real.ps1');
    if(JSON.parse(readFileSync(join(baseline,'isolated.json'),'utf8').replace(/^\uFEFF/,'' )).synthetic)result.fixture='synthetic-scale';
    command(exe('vmstate'),['--mode','audit','--home',join(baseline,'home'),'--root',baseline],{},30000);
    mkdirSync(home,{recursive:true});
    command('sqlite3',['-readonly',join(baseline,'home','archo.db'),`.backup '${join(home,'archo.db').replaceAll("'","''")}'`]);
    if(existsSync(join(baseline,'home','config.toml')))cpSync(join(baseline,'home','config.toml'),join(home,'config.toml'));
    result.sharedContent={baseline,policy:'private DB only; baseline content reused; filesystem writes skipped'};
  } else command(exe('vmstate'),['--mode','fixture','--root',dirname(home)],{},30000);
  if(mode){
    // A copied player's old undo history predates this experiment. Random Undo
    // followed by an edit could otherwise destroy the redo path back to start.
    // Establish the same boundary as the fixture, on this case's DB only.
    const removed=Number(command('sqlite3',[join(home,'archo.db'),'SELECT count(*) FROM workspace_undo; DELETE FROM workspace_undo;']).trim());
    result.setupUndoBoundary={removedSteps:removed,reason:'isolated case starts with empty undo/redo history; workspace/staging/content retained'};
    if(options.allow!=='external'){
      // Opening plan management otherwise refreshes stale followed collections
      // through Go networking, outside CDP request interception. Defer only the
      // copied refresh timestamps through this case budget; retain follow data.
      const until=Math.ceil(Date.now()/1000+Number(options.minutes||60)*60+600);
      command('sqlite3',[join(home,'archo.db'),`UPDATE plan_collection SET checked_at=${until};`]);
      result.deferredExternalEffects=[{operation:'automatic followed-collection network refresh',until,scope:'copied DB checked_at only'}];
    }
  }
  // The real home keeps the player's own APPDATA/LOCALAPPDATA (lock library, monitor, subscriptions).
  env=realHome?{RIMARCHO_HOME:home}:{RIMARCHO_HOME:home, APPDATA:join(out,'appdata'), LOCALAPPDATA:join(out,'localappdata')};
  const initial=JSON.parse(command(exe('vmstate'),['--home',home]));
  currentPlan=options.plan || initial.plans.find(p=>p.name==='default')?.name || initial.plans[0].name;
  result.plan=currentPlan;result.content={instances:initial.instances.length,plans:initial.plans.length};
  result.build=hasGit?command('git',['-C',repo,'rev-parse','HEAD']).trim():process.env.RIMARCHO_VM_BUILD;
  result.workingTree=hasGit?command('git',['-C',repo,'status','--short']).trim():'exported copy';
  result.node=process.version;result.toolHashes=Object.fromEntries(['archo','vmstate',driver==='W'?'RimArcho':driver==='A'?'guidev':null].filter(Boolean).map(n=>[n,createHash('sha256').update(readFileSync(exe(n))).digest('hex')]));
  result.preparationMs=performance.now()-preparing;
  if(driver==='W'&&!mode&&scenario===3&&!options.baseline&&!realHome)command(exe('archo'),['config','set','subscribed.dir',join(out,'subscriptions')],env);
  if(driver==='C') return;
  const launching=performance.now();
 result.milestones=[];const milestone=name=>result.milestones.push({name,ms:performance.now()-launching,epochMs:Date.now()});
  const freePort = () => new Promise((ok,fail) => { const server=createServer();server.on('error',fail);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(()=>ok(port))}); });
  const port=Number(options.port || (driver==='W' && process.env.RIMARCHO_DEVTOOLS_PORT) || await freePort());
  const addr=`127.0.0.1:${port}`;
  // Refuse an existing listener rather than driving another window's user data.
  try {await fetch(`http://${addr}/${driver==='W'?'json/list':''}`,{signal:AbortSignal.timeout(300)});throw Error(`port ${port} already in use`);}catch(e){if(e.message.includes('already in use'))throw e;}
  milestone("process-spawn");
  result.processStartEpochMs=Date.now();
  child=spawn(exe(driver==='W'?'RimArcho':'guidev'),driver==='W'?[]:['-home',home,'-addr',addr,'-exe',exe('archo')],{env:{...process.env,...env,RIMARCHO_PERF:options.perf==='true'?'1':'',RIMARCHO_DEVTOOLS_PORT:String(port)},cwd:tools,windowsHide:true,stdio:['ignore','pipe','pipe']});
  let logs='';child.stdout.on('data',b=>logs=(logs+b).slice(-256*1024));child.stderr.on('data',b=>logs=(logs+b).slice(-256*1024));child.on('exit',()=>writeFileSync(join(out,'process.log'),logs));
  await until(async()=>{try{return (await fetch(`http://${addr}/${driver==='W'?'json/list':''}`,{signal:AbortSignal.timeout(500)})).ok}catch{return false}},15000);
  milestone('devtools-listening');
  if(driver==='W') {
    await until(async()=>{try{return (await (await fetch(`http://${addr}/json/list`)).json()).some(t=>t.type==='page'&&!t.url.startsWith('devtools:'));}catch{return false}},15000);
    c=await connect(addr);await until(async()=>{try{return await c.evaluate("location.host === 'wails.localhost'")}catch{return false}},15000);await until(async()=>c.evaluate("(async()=>{const {S}=await import('/js/state.js');return !!S.shell?.plan;})()"),15000);
    milestone('shell-plan-visible');
    if(options.profile==='true'){result.profileBrowserStart=await c.evaluate('performance.now()');await c.call('Profiler.enable');await c.call('Profiler.start');}
    if(options.timeline==='true') {
      c.on(e=>{if(e.method==='Tracing.dataCollected')browserTimeline.push(...e.params.value);if(e.method==='Tracing.tracingComplete')traceComplete?.()});
      await c.call('Tracing.start',{categories:'toplevel,blink.user_timing,devtools.timeline,disabled-by-default-devtools.timeline.stack',options:'record-as-much-as-possible'});
      result.timelineBrowserStart=await c.evaluate("performance.mark('f4-trace-sync');performance.getEntriesByName('f4-trace-sync').at(-1).startTime");
    }
    result.windowConnectedEpochMs=Date.now();
    await c.evaluate(`(${observe.toString()})()`);
    await settled(undefined,15000);milestone('home-stable');
    if((await uiState()).popup){result.coverage.push({action:'startup-dialog',status:'dismissed with real Esc',text:await c.evaluate("document.querySelector('#popup')?.innerText")});await press('Escape');milestone('startup-esc');await settled();milestone('prompt-dismissed-stable');}
    await editor();milestone('editor-stable');
    await c.evaluate('window.__vm.armed=true');
  } else {
    const base='http://'+addr;await listen(base);
    let request=Date.now()*1000;
    api=async(name,...args)=>{
      const method={Sort:'Run',Preview:'Preview'}[name]||name;
      if(name==='Sort')args=['sort'];if(name==='Preview')args=['sort'];
      if(['Workspace','UseEnv'].includes(method)) {
        const id=++request;
        const completion=new Promise((ok,fail)=>{const timer=setTimeout(()=>{requestWaiters.delete(id);fail(Error(`${method} event timeout`))},15000);requestWaiters.set(id,d=>{clearTimeout(timer);d.error?fail(Error(d.error)):ok(d)})});
        const response=await fetch(base+'/call',{method:'POST',body:JSON.stringify({m:method,a:[id,...args]}),signal:AbortSignal.timeout(15000)});const j=await response.json();if(j.error||!j.result)throw Error(j.error||'not acknowledged');return completion;
      }
      const response=await fetch(base+'/call',{method:'POST',body:JSON.stringify({m:method,a:args}),signal:AbortSignal.timeout(15000)});const j=await response.json();if(j.error)throw Error(j.error);return j.result;
    };
    await api('Init'); const ps=await api('Shell');const p=ps.plans.find(p=>p.name===currentPlan);await api('OpenPlan',p.id);
  }
  const ms=performance.now()-launching,budgetMs=initial.instances.length>1000?5000:2000;
  result.timings.push({name:'cold-window-or-service-ready',ms,budgetMs});
  if(ms>budgetMs)result.performanceFailures.push({name:'cold-window-or-service-ready',ms,budgetMs});
}

async function sortUndo() {
  const before=await snapshot('before-sort');
  await timed('sort-to-settled',async()=>{if(c){await press('Shift+KeyS');await settled();await dismiss();}else await act('Sort');},result.content.instances>1000?5000:2000);
  const sorted=await snapshot('sorted');
  if(entries(before)!==entries(sorted)) {
    check('sort is one undo step',undoDepth(sorted)===Math.min(100,undoDepth(before)+1));
    await timed('sort-undo-to-settled',async()=>{if(c){await press('KeyU');await settled();}else await act('Undo');},5000);
    const undone=await snapshot('sort-undone');check('undo restores entries',entries(undone)===entries(before));
    await timed('sort-redo-to-settled',async()=>{if(c){await press('Control+KeyR');await settled();}else await act('Redo');},5000);
    const redone=await snapshot('sort-redone');check('redo restores sorted entries',entries(redone)===entries(sorted));
  } else check('idempotent sort adds no history',undoDepth(sorted)===undoDepth(before));
  return sorted;
}
async function scenario1() {
  result.coverage.push({action:'multi-selection suggested placement',C:'unsupported: D2/P0-2',A:'domain-only',W:'real input bursts'});
  if(!c){
    if(driver==='A'){const b=await api('Board');const it=b.cols[0].find(i=>i.id);if(it){const before=await snapshot('before-toggle');await api('Toggle',0,[it.id],[it.ref]);await snapshot('enabled');await api('Undo');check('toggle undo restores entries',entries(await snapshot('toggle-undone'))===entries(before));}}
    await sortUndo();return;
  }

  const initial=await snapshot('before-bursts');
  const inventory=(await uiState()).columns.flatMap(col=>col.entries.map(e=>e.id).filter(Boolean)).sort();
  for(const count of [1,10,100]) for(const cadence of [20,50,100]) {
    await dismiss();
    const available=await c.evaluate('(async()=>{const {S}=await import("/js/state.js");return S.cols[0].view.length})()');
    if(available<count){result.coverage.push({action:'selection-'+count,cadence,status:'insufficient fixture rows',available});continue;}
    const observed=JSON.parse(command(exe('vmstate'),['--home',home,'--plan',currentPlan])).observation;
    const activePackages=observed.entries.map(e=>observed.packages[e.i]||e.m);
    const chosen=await c.evaluate(`(()=>{const packages=${JSON.stringify(observed.packages)},eligible=${JSON.stringify(observed.eligible)},seen=new Set(${JSON.stringify(activePackages)}),out=[];const col=window.__vm.read().columns[0];return col.entries.map((e,i)=>({id:e.id,i})).filter(e=>{const p=packages[e.id];if(!p||!eligible[e.id]||seen.has(p))return false;seen.add(p);return true}).slice(0,${count})})()`);
    if(chosen.length<count){result.coverage.push({action:'legal-selection-'+count,status:'insufficient distinct loadable packages',available:chosen.length});continue;}
    await c.click('.area[data-area="0"] .row');await press('Home');
    let position=0;
    for(const item of chosen){
      if(item.i>position)await c.key('ArrowDown',{repeats:item.i-position-1});
      await press('Control+Space');position=item.i;
    }
    check(`selection ${count}-${cadence} contains legal unique packages`,(await uiState()).columns[0].selected.length===count);
    const before=await snapshot(`selection-${count}-${cadence}`);
    result.timings.push({name:`space-${count}-${cadence}`,trace:await c.burst(Array(20).fill('Space'),cadence)});
    await timed(`space-${count}-${cadence}-settled`,()=>settled(),10000);
    const after=await snapshot(`space-${count}-${cadence}`);
    check(`space-${count}-${cadence} conserves instances across columns`,JSON.stringify((await uiState()).columns.flatMap(col=>col.entries.map(e=>e.id).filter(Boolean)).sort())===JSON.stringify(inventory));
    const stack=after.plans.find(p=>p.name===currentPlan).undo;
    const n=stack.findIndex(step=>step.entries===entries(before))+1;
    if(n>0){await c.burst(Array(n).fill('KeyU'),20);await settled();check(`space-${count}-${cadence} undo restores entries`,entries(await snapshot(`undo-${count}-${cadence}`))===entries(before));}
    else check(`space-${count}-${cadence} unchanged or undoable`,entries(after)===entries(before));
  }
  // Exercise held Space and the enabled/staging columns with real repeat keydown events.
  await c.click('.area[data-area="0"] .row');await c.key('Space',{repeats:4});await settled();await snapshot('held-space');
  await enabledSecond();
  await c.key('Shift+KeyJ');await settled();await snapshot('shift-lowercase-j');
  await c.key('Shift+KeyK',{uppercase:true});await settled();await snapshot('shift-uppercase-k');
  result.timings.push({name:'enabled-space-burst',trace:await c.burst(Array(20).fill('Space'),50)});await settled();await snapshot('enabled-space-burst');
  await c.click('.area[data-area="0"] .row');await press('KeyT');await settled();await press('Digit3');
  result.timings.push({name:'staging-space-burst',trace:await c.burst(Array(20).fill('Space'),50)});await settled();await snapshot('staging-space-burst');
  await press('Shift+KeyT');await settled();await press('KeyU');await settled();
  check('bursts keep all plans',(await uiState()).plans.length===initial.plans.length);

}
async function scenario2() {
  const before=await snapshot('before-navigation');
  if(!c){
    if(driver==='A'){const jobs=[api('Workspace'),api('Workspace'),api('Workspace')];const values=await Promise.all(jobs);check('latest workspace completes',values.at(-1).board && !values.at(-1).stale);}
    result.coverage.push({action:'page/selection/loading interleaving',status:'unsupported: W-only; A covers asynchronous workspace events, C read-only projection'});
  }else{
    for(let round=0;round<5;round++){
      result.timings.push({name:'navigation-burst',trace:await c.burst(['Shift+Digit6','Shift+Digit2','Shift+Digit1','Shift+Digit4'],20)});
      await timed('navigation-final-editor',()=>settled(null),5000);
      check('last requested page wins',(await uiState()).page===null);
    }
    // Distinct plan requests while the preceding plan is still loading.
    const plans=await c.evaluate('(async()=>{const {S}=await import("/js/state.js");return S.shell.plans})()');
    if(plans.length>1){const other=plans.find(p=>p.name!==currentPlan),original=plans.find(p=>p.name===currentPlan);
      for(let i=0;i<10;i++){await c.click(`#nav [data-id="plan:${other.id}"]`);await sleep(50);await c.click(`#nav [data-id="plan:${original.id}"]`);await sleep(50);}
      await settled(null);check('last requested plan wins',(await uiState()).plan===currentPlan);
    }
  }
  if(c){
    const original=(await uiState()).env;
    await press('Shift+Digit8');await settled('env');
    const alternatives=await c.evaluate('[...document.querySelectorAll(".envrow:not(.on) .name")].map(e=>e.textContent)');
    if(alternatives.length){
      const destination=alternatives[0];
      await clickText('.envrow:not(.on) button','use this one');
      // Navigate while UseEnv is still in flight. Names are deliberately distinct from IDs.
      await press('Shift+Digit6');await environmentReady('library');
      check('environment switch adopts destination',await c.evaluate(`(async()=>{const {S}=await import('/js/state.js');return S.board.env===${JSON.stringify(destination)}})()`));
      await press('Shift+Digit8');await settled('env');
      // Raw environment IDs are intentionally absent in canonical snapshots; read displayed names.
      const row=await c.evaluate(`(()=>{const rows=[...document.querySelectorAll('.envrow')];return rows.findIndex(r=>r.querySelector('.name')?.textContent!==${JSON.stringify(destination)})})()`);
      await c.click(`.envrow:nth-child(${row+1}) button`);await press('Shift+Digit1');await environmentReady('home');
      check('environment switch returns to original ID',(await uiState()).env===original);
      await editor();
    }else result.coverage.push({action:'environment switch',status:'unsupported: only one environment'});
  }
  const after=await snapshot('after-navigation');check('navigation does not edit plans',JSON.stringify(payload(before))===JSON.stringify(payload(after)));check('navigation keeps history',JSON.stringify(before.plans)===JSON.stringify(after.plans));
}
async function scenario3() {
  await sortUndo();
  const before=await snapshot('before-repeat-sort');
  if(!c){for(let i=0;i<3;i++)await act('Sort');}else{
    await editor();const point=await c.click('.area[data-area="1"] [data-cmd="sort"]');
    await c.mouse('left',point.x,point.y,2);await c.mouse('left',point.x,point.y,3);await settled();await dismiss();
    result.timings.push({name:'sort-key-burst',trace:await c.burst(Array(10).fill('Shift+KeyS'),20)});await settled();await dismiss();
    await press('Shift+KeyS',{uppercase:true});await settled();await dismiss();
  }
  const after=await snapshot('after-repeat-sort');check('repeated sorts preserve canonical list',entries(after)===entries(before));check('repeated sorts do not grow undo',undoDepth(after)===undoDepth(before));
  if(c){
    await press('Shift+Digit1');await settled('home');await editor();const returned=await snapshot('after-return');check('sorted result survives navigation',entries(returned)===entries(after));
    for(const clicks of [2,3]){
      const point=await c.click('.area[data-area="0"] .row');
      const b=await snapshot(`before-enable-${clicks}`);
      for(let click=2;click<=clicks;click++)await c.mouse('left',point.x,point.y,click);await settled();
      const a=await snapshot(`after-enable-${clicks}`);
      check(`enable ${clicks}-click has no duplicate entries`,new Set(JSON.parse(entries(a)).map(e=>e.i||"missing:"+e.m)).size===JSON.parse(entries(a)).length);
      if(entries(a)!==entries(b)){await press('KeyU');await settled();check(`enable ${clicks}-click undo restores entries`,entries(await snapshot(`enable-${clicks}-undone`))===entries(b));}
    }
    for(const clicks of [2,3]){
      const b=await snapshot(`before-import-${clicks}`);
      await importPreview();await c.click('.xc-preview fieldset .xc-check:nth-of-type(2) input');
      const point=await c.click('[data-env-write="import-apply"]');
      for(let click=2;click<=clicks;click++)await c.mouse('left',point.x,point.y,click);await settled('plans');await editor();
      const a=await snapshot(`after-import-${clicks}`);
      check(`import ${clicks}-click creates no plans`,a.plans.length===b.plans.length);
      check(`import ${clicks}-click is one undo step`,undoDepth(a)===Math.min(100,undoDepth(b)+1));
      await press('KeyU');await settled();check(`import ${clicks}-click undo restores entries`,entries(await snapshot(`import-${clicks}-undone`))===entries(b));
    }
    await repeatedSubscription();
  }
}
async function deletionPrerequisite() {
  if(!c||(await uiState()).plans.length!==1)return;
  await c.click('[data-env-write="remove-plan"]');await sleep(350);await settled();
  result.lastPlanRefusal={popup:(await uiState()).popup,status:await c.evaluate('document.querySelector("#status")?.textContent'),message:await c.evaluate('document.querySelector("#say")?.textContent||document.querySelector(".toast")?.textContent||""')};
  check('last plan deletion refuses without confirmation',!(await uiState()).popup&&/only list|cannot be deleted|\u552f\u4e00|\u4e0d\u80fd\u5220\u9664/.test(result.lastPlanRefusal.status));
  await c.click('#nav [data-id="plans"]');await settled('plans');await press('KeyN');await settled('plans');
  await c.click('.pm-name');await press('Control+KeyA');await c.call('Input.insertText',{text:'VM cancellation companion'});
  await until(()=>c.evaluate('!!document.querySelector("[data-env-write=create-plan]:not(:disabled)")'));
  await c.click('[data-env-write="create-plan"]');await settled();await editor();
  result.setupAdditionalPlans=(await uiState()).plans.length-1;
  check('delete cancellation prerequisite creates second plan',result.setupAdditionalPlans===1);
}
async function guestProbes() {
  await deletionPrerequisite();
  await c.click('[data-env-write="remove-plan"]');await until(async()=>!!(await uiState()).popup);
  result.deletePlanPreview=await c.evaluate('document.querySelector("#popup")?.innerText');
  await dismiss();
  await press('Shift+Digit6');await settled('library');
  await c.click('#nav [data-id="subscribe"]');await settled('subscribe');
  await c.click('.subscribe-omni input');await c.call('Input.insertText',{text:'https://127.0.0.1:9/phase1a.git'});
  await until(()=>c.evaluate('!!document.querySelector(".subscribe-preview")?.textContent.trim()'));
  result.subscriptionPrerequisite={gitAvailable:spawnSync('git',['--version'],{windowsHide:true}).status===0,preview:await c.evaluate('document.querySelector(".subscribe-preview")?.textContent'),subscribeDisabled:await c.evaluate('document.querySelector(".subscribe-foot button.primary")?.disabled')};
  if(result.subscriptionPrerequisite.subscribeDisabled&&/unresolved location change/.test(result.subscriptionPrerequisite.preview)){
    // Diagnosis on the case DB only: a preparer used cfg:subscribed.dir that did
    // not match its relocated indexed source. Do not move or create content.
    const rows=JSON.parse(command('sqlite3',['-json',join(home,'archo.db'),"SELECT root FROM source WHERE label='subscribed' AND status='ok' LIMIT 1;"])||'[]');
    result.subscriptionPrerequisite.indexedSourceFound=rows.length>0;
    if(rows.length){
      command('sqlite3',[join(home,'archo.db'),`UPDATE meta SET value='${rows[0].root.replaceAll("'","''")}' WHERE key='cfg:subscribed.dir';`]);
      await c.click('.subscribe-omni input');await press('Control+KeyA');await c.call('Input.insertText',{text:'https://127.0.0.1:9/phase1a.git'});await sleep(400);
      await until(()=>c.evaluate('!!document.querySelector(".subscribe-preview")?.textContent.trim()'));
      result.subscriptionPrerequisite.afterDirectoryAlignment={preview:await c.evaluate('document.querySelector(".subscribe-preview")?.textContent'),subscribeDisabled:await c.evaluate('document.querySelector(".subscribe-foot button.primary")?.disabled'),scope:'copied DB config aligned to existing baseline source; no content changes or submit'};
    }
  }
  await press('Escape');await editor();
}
async function scenario4() {
  await deletionPrerequisite();
  const before=await snapshot('before-cancel');
  const staging = snap => snap.plans.map(p=>({name:p.name,staging:p.staging}));
  const stageHistory = async (b,a,disable) => {
    const step=async redo=>{if(c){await press(redo?'Control+KeyR':'KeyU');await settled();}else await act(redo?'Redo':'Undo');};
    await step(false);const u=await snapshot('stage-undone');
    check('stage undo restores entries and staging payload',JSON.stringify(payload(u))===JSON.stringify(payload(b)),{finding:'Staging/history payload mismatch (spec 04 §2.2)'});
    await step(true);const r=await snapshot('stage-redone');
    check('stage redo restores entries and staging payload',JSON.stringify(payload(r))===JSON.stringify(payload(a)));
    await step(false);await snapshot('stage-undone-again');
    await disable();const d=await snapshot('stage-undone-then-disabled');
    check('ordinary disable changes enabled entries',entries(d)!==entries(b));
    check('stage undo then disable never resurrects staging',JSON.stringify(staging(d))===JSON.stringify(staging(b)));
    await step(false);const restored=await snapshot('stage-disable-undone');
    check('disable undo preserves original stage payload',JSON.stringify(payload(restored))===JSON.stringify(payload(b)));
  };
  if(!c){
    await act('Preview');result.coverage.push({action:'cancel/Esc/focus',status:'unsupported: W-only; preview/dry-run tested'});
    const b=await snapshot('before-stage');
    let disable;
    if(driver==='A'){
      const row=(await api('Board')).cols[1].find((r,i)=>i>0&&r.id);
      assert(row,'stage needs an enabled mod');
      await api('Stage',1,[row.id]);
      disable=()=>api('Toggle',1,[row.id],[]);
    }else{
      const list=JSON.parse(entries(b));
      const m=list.slice(1).map(e=>b.instances.find(m=>m.id===e.i)).find(m=>m&&b.instances.filter(i=>i.package_id===m.package_id).length===1);
      assert(m,'stage needs a uniquely resolvable enabled package');
      command(exe('archo'),['--open-plan',currentPlan,'stage','add','pkg:'+m.package_id],env);
      disable=()=>command(exe('archo'),['--open-plan',currentPlan,'disable','pkg:'+m.package_id],env);
    }
    const a=await snapshot('after-stage');
    check('stage changes enabled entries',entries(a)!==entries(b));
    await stageHistory(b,a,disable);
  }
  else {
    // Plan deletion is destructive only after confirmation. Test cancel, Esc and backdrop.
    for(const close of ['cancel','esc','backdrop']) {
      const selector='#top [data-env-write="remove-plan"], #topbar [data-env-write="remove-plan"], [data-env-write="remove-plan"]';
      await popupFeedback('plan-delete-confirm-ready',()=>c.click(selector),true);
      await popupFeedback(`dialog-close-${close}`,async()=>{
        if(close==='cancel')await c.click('#popup .btns button:not(.go)');
        else if(close==='esc')await press('Escape');
        else {const p=await c.evaluate('(()=>{const b=document.querySelector("#popup").getBoundingClientRect();return {x:b.x+4,y:b.y+4}})()');await c.mouse('left',p.x,p.y);}
      },false);
      await settled();check(`delete ${close} closes only dialog`,!(await uiState()).popup);
      const after=await snapshot('cancel-'+close);check(`delete ${close} does not write`,JSON.stringify(domain(before))===JSON.stringify(domain(after)));
    }
    for(const close of ['cancel','esc','backdrop']){
      await c.click('.area[data-area="0"] .row');const b=await snapshot(`before-mod-delete-${close}`);
      await press('Delete');await until(async()=>!!(await uiState()).popup);
      await popupFeedback(`dialog-close-${close}`,async()=>{
        if(close==='cancel')await c.click('#popup .btns button:not(.go)');
        else if(close==='esc')await press('Escape');
        else {const p=await c.evaluate('(()=>{const b=document.querySelector("#popup").getBoundingClientRect();return {x:b.x+4,y:b.y+4}})()');await c.mouse('left',p.x,p.y);}
      },false);
      await settled();check(`mod deletion ${close} preserves payload and files`,JSON.stringify(domain(await snapshot(`mod-delete-${close}`)))===JSON.stringify(domain(b)));
    }
    await cancelUnsubscribe();
    await c.click('#nav-settings');await settled('settings');
    const directoryBefore=await snapshot('before-directory-cancel');
    for(const close of ['skip','esc']){
      await clickText('button','move to…');await until(async()=>(await uiState()).popup==='directory');
      await c.click('#popup input.field');await c.call('Input.insertText',{text:join(out,'never-moved')});
      await clickText('#popup button','preview move');
      await until(()=>c.evaluate('[...document.querySelectorAll("#popup button")].some(b=>b.textContent.trim()==="confirm move")'));
      if(close==='skip')await clickText('#popup button','skip for now');else await press('Escape');
      await settled('settings');check(`directory ${close} preserves payload and files`,JSON.stringify(domain(await snapshot(`directory-${close}`)))===JSON.stringify(domain(directoryBefore)));
    }
    await editor();
    const exportBefore=await snapshot('before-export-cancel');
    await cancelExportOverwrite();
    check('export cancellation preserves domain',JSON.stringify(domain(await snapshot('export-cancelled')))===JSON.stringify(domain(exportBefore)));
    const cancelBefore=await snapshot('before-import-cancel');
    await importPreview();await press('Escape');await settled();await editor();
    check('import preview Esc does not write',JSON.stringify(domain(await snapshot('import-cancelled')))===JSON.stringify(domain(cancelBefore)));
    for(const key of ['KeyM','KeyG','Shift+F10']) {await c.click('.area[data-area="0"] .row');await press(key);await settled();await press('Escape');await settled();}
    // A real enable followed by undo, then a real move and undo.
    await c.click('.area[data-area="0"] .row');const initial=await snapshot('before-enable');
    await press('Space');await settled();await snapshot('after-enable');await press('KeyU');await settled();
    check('enable undo restores list',entries(await snapshot('enable-undone'))===entries(initial));
    await enabledSecond();const moveBefore=await snapshot('before-move');
    await press('Shift+KeyJ');await settled();const moved=await snapshot('after-move');
    if(entries(moved)!==entries(moveBefore)){await press('KeyU');await settled();check('move undo restores list',entries(await snapshot('move-undone'))===entries(moveBefore));}
    await enabledSecond();const stageBefore=await snapshot('before-stage');
    await press('KeyT');await settled();const staged=await snapshot('after-stage');
    check('stage changes enabled entries',entries(staged)!==entries(stageBefore));
    await stageHistory(stageBefore,staged,async()=>{await enabledSecond();await press('Space');await settled();});
  }
  const after=await snapshot('after-cancel');check('previews and cancelled actions preserve game config',JSON.stringify(after.modsConfigHashes)===JSON.stringify(before.modsConfigHashes));
  await sortUndo();
}
try {
  await timed('prepare-and-start',setup,null);
  if(c)await editor();
  if(options.probe==='true')await guestProbes();
  else if(mode) {
    const { drive } = await import('./players.mjs');
    await drive({ c, options, result, out, repo, home, tools, currentPlan, snapshot, settled, check, editor, command, exe, save });
  } else if(options["cold-only"]!=="true") {
    await timed('scenario',()=>[scenario1,scenario2,scenario3,scenario4][scenario-1](),300000);
    await freshProblems();
  }
  if(c){
    await settled();
    const evidence=await windowEvidence();result.observationCounts=Object.fromEntries(Object.entries(evidence).filter(([,v])=>Array.isArray(v)).map(([k,v])=>[k,v.length]));
    // App keyboard handlers may invoke .click() internally (e.g. Tab to a list).
    // Physical key/pointer arrival must be trusted; derived synthetic clicks are evidence.
    check('all observed key/pointer input is trusted',evidence.inputs.filter(i=>/^(key|pointer|mouse)/.test(i.type)).every(i=>i.trusted));check('no transient blank page',evidence.samples.filter(s=>s.armed).every(s=>!s.blank && (s.content || s.loading)));
    check('sidebar never loses lists',evidence.samples.filter(s=>s.armed).every(s=>s.sidebar.includes('plan') && (mode || s.plansCount>=result.content.plans&&s.plansCount<=result.content.plans+(result.setupAdditionalPlans||0))));
    check('never falsely displays no game',evidence.samples.filter(s=>s.armed).every(s=>!s.noGame));
    check('no console errors',evidence.errors.length===0,evidence.errors);
    const sequences=new Map();
    for(const sample of evidence.samples.filter(s=>s.armed)){
      const key=JSON.stringify([sample.env,sample.problemPlan,sample.listVersion]);
      if(sequences.has(key)&&sample.sequence<sequences.get(key))check('problem sequence never goes backwards',false,{key,before:sequences.get(key),after:sample.sequence});
      sequences.set(key,sample.sequence);
    }
    for(let i=1;i<evidence.statuses.length;i++){const a=evidence.statuses[i-1],b=evidence.statuses[i];if(a.text && !/loading|checking|\u6b63\u5728|\u52a0\u8f7d/i.test(a.text) && /loading|\u52a0\u8f7d/i.test(b.text) && a.inputCount===b.inputCount)result.failures.push({name:'result overwritten by loading without input',a,b});}
    result.feedback={measurement:'first observed DOM mutation after latest real input; 25ms state sampler separately records stalls',samples:evidence.feedback.length,maxMs:Math.max(0,...evidence.feedback.map(f=>f.ms)),over200ms:evidence.feedback.filter(f=>{const i=evidence.inputs[f.inputIndex];return f.ms>200&&i?.type==='keydown'&&i.shift&&/^Digit[12458]$/.test(i.code)&&!i.popup})};
    result.performanceFailures.push(...result.feedback.over200ms.map(f=>({name:'first feedback',...f,budgetMs:200})));
    const stalls=evidence.samples.filter(s=>s.gapMs>500);result.performanceFailures.push(...stalls.map(s=>({name:'main-thread stall',...s,budgetMs:500})));
    if(options.discover==='true')save('controls',await c.evaluate(`(${discover.toString()})()`));
    if(options.timeline==='true'){const done=new Promise(ok=>traceComplete=ok);await c.call('Tracing.end');await done;writeFileSync(join(dirname(outputBase),out.split(/[\\/]/).pop()+'-timeline.json.gz'),gzipSync(JSON.stringify({traceEvents:browserTimeline})));}
    if(options.profile==='true'){const p=await c.call('Profiler.stop');save('browser-cpu',p.result.profile);}
    if(options.perf==='true')save('perf',await c.evaluate('window.__f4'));
  }
} catch(e) {
  result.failures.push({name:'runner',detail:e.stack});
  if(c){try{result.failureSurface=await c.evaluate('(()=>{const s=__vm.read(false);return {page:s.page,status:s.status,popup:s.popup,errors:s.errors}})()');}catch{}}
} finally {
  eventStream?.abort();c?.close();
  if(child?.pid){if(platform()==='win32')spawnSync('taskkill',['/PID',String(child.pid),'/T','/F'],{windowsHide:true});else child.kill();}
  if(realHome&&!childOfReal&&existsSync(realStart)){
    // Each seed starts from the same real state; the snapshot revert is the final reset.
    try{
      command('sqlite3',[join(realHome,'archo.db'),'.timeout 20000',`.restore '${realStart.replaceAll("'","''")}'`],{},120000);
      for(const f of ['config.toml','tasks.json'])if(existsSync(join(out,'real-start-'+f)))cpSync(join(out,'real-start-'+f),join(realHome,f));
      result.realHomeRestored=true;
    }catch(e){result.realHomeRestored=false;result.failures.push({name:'real home restore',detail:e.message});}
  }
  try{result.storage=cleanupRun(out);}catch(e){result.failures.push({name:'cleanup',detail:e.message});}
  result.finished=new Date().toISOString();result.functionalStatus=result.failures.length?'failed':'passed';result.status=result.failures.length||result.performanceFailures.length?'failed':'passed';save('result',result);
  console.log(JSON.stringify({status:result.status,driver,scenario,...(mode?{mode,seed:result.seed,steps:result.executedSteps}:{}),output:out,failures:result.failures,performanceFailures:result.performanceFailures},null,2));
  process.exitCode=result.failures.length||result.performanceFailures.length?1:0;
}
