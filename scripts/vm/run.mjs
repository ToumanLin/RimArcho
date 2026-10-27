import { createHash } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:net';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir, cpus, totalmem, platform } from 'node:os';
import { resolve, join, dirname, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import { connect, sleep } from './cdp.mjs';
import { observe, discover } from './observe.mjs';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const common=spawnSync('git',['-C',repo,'rev-parse','--path-format=absolute','--git-common-dir'],{encoding:'utf8',windowsHide:true});
assert(common.status===0,'run from a Git checkout');
const repositoryRoot=resolve(common.stdout.trim(),'..');
const options = {};
for (let i = 2; i < process.argv.length; i += 2) {
  const key = process.argv[i];
  if (!key.startsWith('--') || !process.argv[i+1]) throw Error('options need --name value');
  options[key.slice(2)] = process.argv[i+1];
}
const driver = options.driver || 'W', scenario = Number(options.scenario || 1);
assert(['W','A','C'].includes(driver) && scenario >= 1 && scenario <= 4, 'use --driver W|A|C --scenario 1|2|3|4');
const outside = path => { for(const root of [repo,repositoryRoot]) { const r=relative(root,resolve(path));assert(r.startsWith('..')||isAbsolute(r),`results and test homes must be outside the repository: ${path}`); } return resolve(path); };
const outputBase = outside(options.out || join(tmpdir(), 'RimArcho-vm-results'));
mkdirSync(outputBase, { recursive: true });
const out = mkdtempSync(join(outputBase, `s${scenario}-${driver}-`));
const tools = resolve(options.tools || join(tmpdir(), 'RimArcho-vm-tools'));
const exe = name => join(tools, name + (platform() === 'win32' ? '.exe' : ''));
const result = { schema: 1, scenario, driver, seed: 271027, fixture: options.baseline ? 'real-copy' : 'small', output: out, started: new Date().toISOString(), host: { cpu: cpus()[0]?.model, cores: cpus().length, memory: totalmem(), platform: platform() }, timings: [], checks: [], coverage: [], checkpoints: [], failures: [], performanceFailures: [] };
const save = (name, data) => writeFileSync(join(out, name + '.json'), JSON.stringify(data,null,2));
const command = (file, args, env, timeout = 15000) => {
  const r = spawnSync(file, args, { encoding: 'utf8', timeout, env: { ...process.env, ...env }, windowsHide: true, maxBuffer: 128 * 1024 * 1024 });
  if (r.error || r.status !== 0) throw Error(`${file} ${args.join(' ')}: ${r.error?.message || r.stderr || r.stdout}`);
  return r.stdout;
};
const home = join(out, 'case', 'home');
let c, child, api, env, currentPlan, checkpointSeq = 0, eventStream;
const events = [], requestWaiters = new Map();
const check = (name, ok, detail) => { result.checks.push({ name, ok: !!ok, detail }); if (!ok) result.failures.push({name,detail}); };
const timed = async (name, fn, budgetMs = 2000) => { const start=performance.now(); try { return await fn(); } finally { const ms=performance.now()-start;result.timings.push({name,ms,budgetMs});if(budgetMs!==null&&ms>budgetMs)result.performanceFailures.push({name,ms,budgetMs}); } };
const domain = snap => ({ sources:snap.sources, instances:snap.instances, links:snap.links, environments: snap.environments, plans: snap.plans, lineage_data: snap.lineage_data, lineage_tag:snap.lineage_tag, instance_note:snap.instance_note, user_rule:snap.user_rule, modsConfigHashes:snap.modsConfigHashes });
const payload = snap => snap.plans.map(p => ({ name:p.name, entries:p.workspace, staging:p.staging }));
const entries = (snap,name=currentPlan) => snap.plans.find(p=>p.name===name)?.workspace?.[0]?.entries;
const undoDepth = (snap,name=currentPlan) => snap.plans.find(p=>p.name===name)?.undoDepth;
async function snapshot(label) {
  const data = JSON.parse(command(exe('vmstate'), ['--home',home,'--plan',currentPlan]));
  const ui = c ? await c.evaluate('window.__vm.read()') : null;
  const artifact = `${String(++checkpointSeq).padStart(2,'0')}-${label}`;
  save(artifact,{ domain:data, surface:ui });result.checkpoints.push({ label, artifact:artifact+'.json' });
  if(ui) {
    if(ui.page===null && data.observation){
      const expected=data.observation.entries.map(e=>e.i||'pkg:'+e.m);
      const shown=ui.columns[1].entries.map(e=>e.id||e.ref);
      check(label+': window enabled list matches domain',JSON.stringify(shown)===JSON.stringify(expected),{expected:expected.length,shown:shown.length});
    }
    const ids=ui.columns.flatMap(col=>col.entries.map(e=>e.id).filter(Boolean));
    check(label+': no duplicate instances in columns',new Set(ids).size===ids.length);
    check(label+': page has content',!ui.blank && ui.content,ui.page);
    check(label+': sidebar lists intact',ui.plans?.length===data.plans.length,{ui:ui.plans?.length,db:data.plans.length});
    check(label+': no console/script errors',ui.errors.length===0,ui.errors);
    check(label+': open plan matches',ui.plan===currentPlan,{expected:currentPlan,actual:ui.plan});
  }
  return data;
}
async function until(fn, ms=15000) {
  const end=performance.now()+ms;
  while(performance.now()<end) { const r=await fn(); if(r) return r;await sleep(50); }
  throw Error(`condition timed out after ${ms}ms`);
}
async function settled(page, ms=10000) {
  let last='', stable=0;
  return until(async()=>{
    const s=await c.evaluate('window.__vm.read()');
    if(s.blank || s.readError) throw Error(`DOM oracle: ${s.readError || 'blank page'}`);
    const signature=JSON.stringify([s.page,s.plan,s.columns,s.status,s.popup,s.problems?.listVersion]);
    const ready=!s.loading && !s.editing && !s.busy.env && s.content && s.sidebar.includes('plan') && (page===undefined || s.page===page);
    stable=ready && signature===last ? stable+1 : 0;last=signature;
    return stable>=5 && s;
  },ms);
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
async function dismiss() { await press('Escape');await settled(); }
async function uiState() { return c.evaluate('window.__vm.read()'); }
async function clickText(selector,text) {
  for(let i=0;i<24;i++){
    const point=await c.evaluate(`(()=>{for(const e of document.querySelectorAll(${JSON.stringify(selector)})){if(e.disabled || e.textContent.trim()!==${JSON.stringify(text)})continue;const b=e.getBoundingClientRect(),x=b.x+b.width/2,y=b.y+b.height/2,h=document.elementFromPoint(x,y);if(b.width&&b.height&&(h===e||e.contains(h)))return {x,y};}return null})()`);
    if(point){await c.mouse('left',point.x,point.y);return point;}
    if(i===23)throw Error(`visible control unavailable: ${selector} ${text}`);
    const p=await c.evaluate('(()=>{const b=document.querySelector("#screen").getBoundingClientRect();return {x:b.x+b.width/2,y:b.y+b.height/2}})()');
    await c.call('Input.dispatchMouseEvent',{type:'mouseWheel',...p,deltaX:0,deltaY:400});await sleep(50);
  }
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
  if(options.baseline) {
    const baseline=outside(options.baseline);assert(existsSync(join(baseline,'isolated.json')),'baseline needs isolated.json from prepare-real.ps1');
    command(exe('vmstate'),['--mode','audit','--home',join(baseline,'home'),'--root',baseline],{},30000);
    if(scenario===2 && driver==='W' && options['owned-files']!=='true') {
      // UseEnv can create/remove deployment links: this scenario owns a full filesystem copy.
      command('pwsh',['-NoProfile','-File',join(repo,'scripts/vm/prepare-real.ps1'),'-SourceHome',join(baseline,'home'),'-Root',dirname(home),'-Tools',tools],{},1800000);
    } else {
    mkdirSync(home,{recursive:true});
    command('sqlite3',['-readonly',join(baseline,'home','archo.db'),`.backup '${join(home,'archo.db').replaceAll("'","''")}'`]);
    for(const name of ['config.toml','cache']) if(existsSync(join(baseline,'home',name))) cpSync(join(baseline,'home',name),join(home,name),{recursive:true});
    }
  } else command(exe('vmstate'),['--mode','fixture','--root',dirname(home)],{},30000);
  env={RIMARCHO_HOME:home, APPDATA:join(out,'appdata'), LOCALAPPDATA:join(out,'localappdata')};
  const initial=JSON.parse(command(exe('vmstate'),['--home',home]));
  currentPlan=options.plan || initial.plans.find(p=>p.name==='default')?.name || initial.plans[0].name;
  result.plan=currentPlan;result.content={instances:initial.instances.length,plans:initial.plans.length};
  result.build=command('git',['-C',repo,'rev-parse','HEAD']).trim();
  result.workingTree=command('git',['-C',repo,'status','--short']).trim();
  result.node=process.version;result.toolHashes=Object.fromEntries(['archo','vmstate',driver==='W'?'RimArcho':driver==='A'?'guidev':null].filter(Boolean).map(n=>[n,createHash('sha256').update(readFileSync(exe(n))).digest('hex')]));
  result.preparationMs=performance.now()-preparing;
  if(driver==='C') return;
  const launching=performance.now();
  const freePort = () => new Promise((ok,fail) => { const server=createServer();server.on('error',fail);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(()=>ok(port))}); });
  const port=Number(options.port || (driver==='W' && process.env.RIMARCHO_DEVTOOLS_PORT) || await freePort());
  const addr=`127.0.0.1:${port}`;
  // Refuse an existing listener rather than driving another window's user data.
  try {await fetch(`http://${addr}/${driver==='W'?'json/list':''}`,{signal:AbortSignal.timeout(300)});throw Error(`port ${port} already in use`);}catch(e){if(e.message.includes('already in use'))throw e;}
  child=spawn(exe(driver==='W'?'RimArcho':'guidev'),driver==='W'?[]:['-home',home,'-addr',addr,'-exe',exe('archo')],{env:{...process.env,...env,RIMARCHO_DEVTOOLS_PORT:String(port)},cwd:tools,windowsHide:true,stdio:['ignore','pipe','pipe']});
  let logs='';child.stdout.on('data',b=>logs+=b);child.stderr.on('data',b=>logs+=b);child.on('exit',()=>writeFileSync(join(out,'process.log'),logs));
  await until(async()=>{try{return (await fetch(`http://${addr}/${driver==='W'?'json/list':''}`,{signal:AbortSignal.timeout(500)})).ok}catch{return false}},15000);
  if(driver==='W') {
    c=await connect(addr);await until(async()=>{try{return await c.evaluate("location.host === 'wails.localhost'")}catch{return false}},15000);await until(async()=>c.evaluate("(async()=>{const {S}=await import('/js/state.js');return !!S.shell?.plan;})()"),15000);
    await c.evaluate(`(${observe.toString()})()`);
    await settled(undefined,15000);
    if((await uiState()).popup){result.coverage.push({action:'startup-dialog',status:'dismissed with real Esc',text:await c.evaluate("document.querySelector('#popup')?.innerText")});await press('Escape');await settled();}
    await editor();
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
  if(c){await press('Shift+KeyS');await settled();await dismiss();}else await act('Sort');
  const sorted=await snapshot('sorted');
  if(entries(before)!==entries(sorted)) {
    check('sort is one undo step',undoDepth(sorted)===Math.min(100,undoDepth(before)+1));
    if(c){await press('KeyU');await settled();}else await act('Undo');
    const undone=await snapshot('sort-undone');check('undo restores entries',entries(undone)===entries(before));
    if(c){await press('Control+KeyR');await settled();}else await act('Redo');
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
  for(const count of [1,10,100]) for(const cadence of [20,50,100]) {
    await dismiss();
    const available=await c.evaluate('(async()=>{const {S}=await import("/js/state.js");return S.cols[0].view.length})()');
    if(available<count){result.coverage.push({action:'selection-'+count,cadence,status:'insufficient fixture rows',available});continue;}
    await c.click('.area[data-area="0"] .row');await press('Home');
    if(count>1)await c.key('Shift+ArrowDown',{repeats:count-2});
    const before=await snapshot(`selection-${count}-${cadence}`);
    result.timings.push({name:`space-${count}-${cadence}`,trace:await c.burst(Array(20).fill('Space'),cadence)});
    await timed(`space-${count}-${cadence}-settled`,()=>settled(),10000);
    const after=await snapshot(`space-${count}-${cadence}`);
    const stack=after.plans.find(p=>p.name===currentPlan).undo;
    const n=stack.findIndex(step=>step.entries===entries(before))+1;
    if(n>0){await c.burst(Array(n).fill('KeyU'),20);await settled();check(`space-${count}-${cadence} undo restores entries`,entries(await snapshot(`undo-${count}-${cadence}`))===entries(before));}
    else check(`space-${count}-${cadence} unchanged or undoable`,entries(after)===entries(before));
  }
  // Exercise held Space and the enabled/staging columns with real repeat keydown events.
  await c.click('.area[data-area="0"] .row');await c.key('Space',{repeats:4});await settled();await snapshot('held-space');
  await c.click('.area[data-area="1"] .row[data-i="1"]');
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
      result.timings.push({name:'navigation-burst',trace:await c.burst(['Shift+Digit4','Shift+Digit2','Shift+Digit1','Shift+Digit5'],20)});
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
      await press('Shift+Digit4');await settled('library',15000);
      check('environment switch adopts destination',await c.evaluate(`(async()=>{const {S}=await import('/js/state.js');return S.board.env===${JSON.stringify(destination)}})()`));
      await press('Shift+Digit8');await settled('env');
      // Raw environment IDs are intentionally absent in canonical snapshots; read displayed names.
      const row=await c.evaluate(`(()=>{const rows=[...document.querySelectorAll('.envrow')];return rows.findIndex(r=>r.querySelector('.name')?.textContent!==${JSON.stringify(destination)})})()`);
      await c.click(`.envrow:nth-child(${row+1}) button`);await press('Shift+Digit1');await settled('home',15000);
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
      await c.mouse('left',point.x,point.y,clicks);await settled();
      const a=await snapshot(`after-enable-${clicks}`);
      check(`enable ${clicks}-click has no duplicate entries`,new Set(JSON.parse(entries(a)).map(e=>e.i||"missing:"+e.m)).size===JSON.parse(entries(a)).length);
      if(entries(a)!==entries(b)){await press('KeyU');await settled();check(`enable ${clicks}-click undo restores entries`,entries(await snapshot(`enable-${clicks}-undone`))===entries(b));}
    }
    for(const clicks of [2,3]){
      const b=await snapshot(`before-import-${clicks}`);
      await importPreview();await c.click('.xc-preview fieldset .xc-check:nth-of-type(2) input');
      const point=await c.click('[data-env-write="import-apply"]');
      await c.mouse('left',point.x,point.y,clicks);await settled('plans');await editor();
      const a=await snapshot(`after-import-${clicks}`);
      check(`import ${clicks}-click creates no plans`,a.plans.length===b.plans.length);
      check(`import ${clicks}-click is one undo step`,undoDepth(a)===Math.min(100,undoDepth(b)+1));
      await press('KeyU');await settled();check(`import ${clicks}-click undo restores entries`,entries(await snapshot(`import-${clicks}-undone`))===entries(b));
    }
    result.coverage.push({action:'subscription submit double/triple click',status:'blocked: requires online subscription fixture; Phase 1b'});
  }
}
async function scenario4() {
  const before=await snapshot('before-cancel');
  if(!c){
    await act('Preview');result.coverage.push({action:'cancel/Esc/focus',status:'unsupported: W-only; preview/dry-run tested'});
    const b=await snapshot('before-stage');
    if(driver==='A'){
      const row=(await api('Board')).cols[1].find((r,i)=>i>0&&r.id);
      if(row)await api('Stage',1,[row.id]);
    }else{
      const list=JSON.parse(entries(b));
      const m=list.slice(1).map(e=>b.instances.find(m=>m.id===e.i)).find(m=>m&&b.instances.filter(i=>i.package_id===m.package_id).length===1);
      assert(m,'stage needs a uniquely resolvable enabled package');
      command(exe('archo'),['--open-plan',currentPlan,'stage','add','pkg:'+m.package_id],env);
    }
    const a=await snapshot('after-stage');
    check('stage changes enabled entries',entries(a)!==entries(b));
    await act('Undo');const u=await snapshot('stage-undone');
    check('stage undo restores entries and staging payload',JSON.stringify(payload(u))===JSON.stringify(payload(b)),{finding:'Stage metadata is outside workspace undo (plan §6.1 / P0-2)'});
  }
  else {
    // Plan deletion is destructive only after confirmation. Test cancel, Esc and backdrop.
    for(const close of ['cancel','esc','backdrop']) {
      const selector='#top [data-env-write="remove-plan"], #topbar [data-env-write="remove-plan"], [data-env-write="remove-plan"]';
      await c.click(selector);await until(async()=>!!(await uiState()).popup);
      if(close==='cancel')await c.click('#popup .btns button:not(.go)');
      else if(close==='esc')await press('Escape');
      else {const p=await c.evaluate('(()=>{const b=document.querySelector("#popup").getBoundingClientRect();return {x:b.x+4,y:b.y+4}})()');await c.mouse('left',p.x,p.y);}
      await settled();check(`delete ${close} closes only dialog`,!(await uiState()).popup);
      const after=await snapshot('cancel-'+close);check(`delete ${close} does not write`,JSON.stringify(domain(before))===JSON.stringify(domain(after)));
    }
    for(const close of ['cancel','esc','backdrop']){
      await c.click('.area[data-area="0"] .row');const b=await snapshot(`before-mod-delete-${close}`);
      await press('Delete');await until(async()=>!!(await uiState()).popup);
      if(close==='cancel')await c.click('#popup .btns button:not(.go)');
      else if(close==='esc')await press('Escape');
      else {const p=await c.evaluate('(()=>{const b=document.querySelector("#popup").getBoundingClientRect();return {x:b.x+4,y:b.y+4}})()');await c.mouse('left',p.x,p.y);}
      await settled();check(`mod deletion ${close} preserves payload and files`,JSON.stringify(domain(await snapshot(`mod-delete-${close}`)))===JSON.stringify(domain(b)));
    }
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
    result.coverage.push({action:'export overwrite cancellation',status:'blocked/external: native Windows save dialog requires UIA; CDP has no access'});
    const cancelBefore=await snapshot('before-import-cancel');
    await importPreview();await press('Escape');await settled();await editor();
    check('import preview Esc does not write',JSON.stringify(domain(await snapshot('import-cancelled')))===JSON.stringify(domain(cancelBefore)));
    for(const key of ['KeyM','KeyG','Shift+F10']) {await c.click('.area[data-area="0"] .row');await press(key);await settled();await press('Escape');await settled();}
    // A real enable followed by undo, then a real move and undo.
    await c.click('.area[data-area="0"] .row');const initial=await snapshot('before-enable');
    await press('Space');await settled();await snapshot('after-enable');await press('KeyU');await settled();
    check('enable undo restores list',entries(await snapshot('enable-undone'))===entries(initial));
    await c.click('.area[data-area="1"] .row[data-i="1"]');const moveBefore=await snapshot('before-move');
    await press('Shift+KeyJ');await settled();const moved=await snapshot('after-move');
    if(entries(moved)!==entries(moveBefore)){await press('KeyU');await settled();check('move undo restores list',entries(await snapshot('move-undone'))===entries(moveBefore));}
    await c.click('.area[data-area="1"] .row[data-i="1"]');const stageBefore=await snapshot('before-stage');
    await press('KeyT');await settled();const staged=await snapshot('after-stage');
    if(entries(staged)!==entries(stageBefore)){await press('KeyU');await settled();const undone=await snapshot('stage-undone');check('stage undo restores entries and staging payload',JSON.stringify(payload(undone))===JSON.stringify(payload(stageBefore)),{finding:'Stage metadata is outside workspace undo (plan §6.1 / P0-2)',before:stageBefore.plans.find(p=>p.name===currentPlan).staging,after:undone.plans.find(p=>p.name===currentPlan).staging});}
  }
  const after=await snapshot('after-cancel');check('previews and cancelled actions preserve game config',JSON.stringify(after.modsConfigHashes)===JSON.stringify(before.modsConfigHashes));
  await sortUndo();
}
try {
  await timed('prepare-and-start',setup,null);
  if(c)await editor();
  await timed('scenario',()=>[scenario1,scenario2,scenario3,scenario4][scenario-1](),300000);
  await freshProblems();
  if(c){
    await settled();
    const evidence=await c.evaluate('(()=>{__vm.stop();const {read,stop,timer,...v}=__vm;return v})()');save('window-trace',evidence);
    check('all observed input is trusted',evidence.inputs.every(i=>i.trusted));check('no transient blank page',evidence.samples.filter(s=>s.armed).every(s=>!s.blank && (s.content || s.loading)));
    check('sidebar never loses lists',evidence.samples.filter(s=>s.armed).every(s=>s.sidebar.includes('plan') && s.plansCount===result.content.plans));
    check('never falsely displays no game',evidence.samples.filter(s=>s.armed).every(s=>!s.noGame));
    check('no console errors',evidence.errors.length===0,evidence.errors);
    const sequences=new Map();
    for(const sample of evidence.samples.filter(s=>s.armed)){
      const key=JSON.stringify([sample.env,sample.problemPlan,sample.listVersion]);
      if(sequences.has(key)&&sample.sequence<sequences.get(key))check('problem sequence never goes backwards',false,{key,before:sequences.get(key),after:sample.sequence});
      sequences.set(key,sample.sequence);
    }
    for(let i=1;i<evidence.statuses.length;i++){const a=evidence.statuses[i-1],b=evidence.statuses[i];if(a.text && !/loading|checking|正在|加载/i.test(a.text) && /loading|加载/i.test(b.text) && a.inputCount===b.inputCount)result.failures.push({name:'result overwritten by loading without input',a,b});}
    result.feedback={measurement:'first observed DOM mutation after latest real input; 25ms state sampler separately records stalls',samples:evidence.feedback.length,maxMs:Math.max(0,...evidence.feedback.map(f=>f.ms)),over200ms:evidence.feedback.filter(f=>{const i=evidence.inputs[f.inputIndex];return f.ms>200&&i?.type==='keydown'&&i.shift&&/^Digit[12458]$/.test(i.code)&&!i.popup})};
    result.performanceFailures.push(...result.feedback.over200ms.map(f=>({name:'first feedback',...f,budgetMs:200})));
    const stalls=evidence.samples.filter(s=>s.gapMs>500);result.performanceFailures.push(...stalls.map(s=>({name:'main-thread stall',...s,budgetMs:500})));
    if(options.discover==='true')save('controls',await c.evaluate(`(${discover.toString()})()`));
    await c.shot(join(out,'final.png'));
  }
} catch(e) {
  result.failures.push({name:'runner',detail:e.stack});
  if(c){try{await c.shot(join(out,'failure.png'));save('failure-window',await c.evaluate('window.__vm?.read()'));save('window-trace',await c.evaluate('(()=>{const {read,stop,timer,...v}=__vm;return v})()'));}catch{}}
} finally {
  eventStream?.abort();c?.close();
  if(child?.pid){if(platform()==='win32')spawnSync('taskkill',['/PID',String(child.pid),'/T','/F'],{windowsHide:true});else child.kill();}
  save('events',events);result.finished=new Date().toISOString();result.functionalStatus=result.failures.length?'failed':'passed';result.status=result.failures.length||result.performanceFailures.length?'failed':'passed';save('result',result);
  console.log(JSON.stringify({status:result.status,driver,scenario,output:out,failures:result.failures,performanceFailures:result.performanceFailures},null,2));
  process.exitCode=result.failures.length||result.performanceFailures.length?1:0;
}
