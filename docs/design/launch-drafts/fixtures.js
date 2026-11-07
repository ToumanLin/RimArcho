// HomeRow/Home-shaped fixtures follow internal/view/shell.go. A list with no
// analysis stays unchecked; a pack is described as checked when created.
const params = new URLSearchParams(location.search);
export const fixture = {
  design: params.get('design') || 'a', background: params.get('background') || 'image',
  scenario: params.get('scenario') || 'issue', applied: 'main-colony',
  remembered: new Map([['main','main-colony'],['1.6','old-save']]),
  currentPlans: new Map([['main','main-colony'],['1.6','old-save']]),
  running: params.get('scenario') === 'running',
  runningRow: 'main-colony',
  games: [{name:'main',version:'1.7',label:'main · 1.7'}, {name:'1.6',version:'1.6',label:'1.6 · 1.6'}],
  rows: [
    {id:'main-colony',kind:'plan',name:'Main colony',env:'main · 1.7',envName:'main',versions:'1.7',mods:487,played:'played today',plan:1,page:'plan:1',at:6},
    {id:'industrial',kind:'pack',name:'Industrial pack',env:'main · 1.7',envName:'main',versions:'1.6–1.7',mods:312,played:'played yesterday',at:5},
    {id:'vanilla',kind:'plan',name:'Light vanilla',env:'main · 1.7',envName:'main',versions:'1.7',mods:64,played:'played Oct 21',plan:2,page:'plan:2',at:4},
    {id:'old-save',kind:'plan',name:'Continue old save',env:'1.6 · 1.6',envName:'1.6',versions:'1.6',mods:206,played:'played Oct 20',plan:3,page:'plan:3',at:3},
    {id:'medieval',kind:'pack',name:'Medieval',env:'1.6 · 1.6',envName:'1.6',versions:'1.5–1.6',mods:156,played:'played Oct 18',at:2},
    {id:'new-list',kind:'plan',name:'new list',env:'main · 1.7',envName:'main',versions:'1.7',mods:5,played:'never played',plan:4,page:'plan:4',at:0},
  ],
};
export const applied = () => fixture.rows.find(r => r.id === fixture.applied);
export function applyRow(id) {
  const row = fixture.rows.find(r => r.id === id);
  if (!row) return;
  fixture.applied = id; fixture.remembered.set(row.envName,id);
  if (row.kind === 'plan') fixture.currentPlans.set(row.envName,id);
}
export const currentPlan = () => fixture.rows.find(r => r.id === fixture.currentPlans.get(applied().envName));
export function issueFor(row) {
  if (!row || row.kind === 'pack' || fixture.scenario === 'clear' || fixture.scenario === 'unchecked' || row.id !== 'main-colony') return null;
  return {text:'patches in 2 mods found no target',why:'Patches with no match often cause errors: 37% of such mods errored in shared logs (4.5% for a random mod).'};
}
export function homeData() {
  if (fixture.scenario === 'empty') return {rows:[],featured:-1,lastRunErrors:0};
  const current = applied();
  const rows = fixture.rows.map(r => ({...r,current:r.id === currentPlan().id,otherEnv:r.envName !== current.envName,
    state:r.kind === 'pack' ? 'checked when created' : fixture.scenario === 'unchecked' || r.id === 'new-list' ? 'not checked yet' : issueFor(r) ? '2 problems will break the game' : 'ready to launch',
    level:issueFor(r) ? 'bad' : (fixture.scenario === 'unchecked' && r.kind === 'plan') || r.id === 'new-list' ? '' : 'ok',
  }));
  return {rows,featured:rows.findIndex(r => r.id === fixture.applied),lastRun:'Main colony · today',lastRunDay:'today',lastRunErrors:fixture.scenario === 'issue' ? 3 : 0,
    changes:{title:'the current plan since you last played',text:fixture.scenario === 'issue' && currentPlan().id === 'main-colony' ? '5 mods updated, 1 needs attention' : 'nothing changed'},launch:issueFor(current)};
}
export function shellData() {
  const count = fixture.scenario === 'issue' && currentPlan().id === 'main-colony' ? 3 : 0, plan = currentPlan();
  const nav = [{title:'playing',items:[{id:'home',label:'start'},{id:'needs',label:'can it run',badge:count},{id:'log',label:'game log'}]},
    {title:'library',items:[{id:'library',label:'the library',kids:[{id:'library',label:'all'},{id:'changes',label:'changes'}]},
    {id:'plan',label:'lists',kids:fixture.rows.filter(r => r.kind === 'plan' && r.envName === plan.envName).map(r => ({id:r.page,label:r.name}))},
    {id:'packs',label:'packs',kids:fixture.rows.filter(r => r.kind === 'pack' && r.envName === plan.envName).map(r => ({id:'pack:'+r.name,label:r.name}))},
    {id:'textures',label:'texture optimisation'},{id:'env',label:'game'}]}];
  return {nav,env:plan.env,envs:fixture.scenario === 'empty' ? 0 : fixture.games.length,plan:{id:plan.plan || 1,name:plan.name,page:plan.page},plans:[],crumbs:{home:[{id:'home',label:'start'}]}};
}
export const translations = {
  'settings':'settings','background tasks':'background tasks','start':'start','can it run':'can it run','game log':'game log','mod library':'the library','lists':'lists','pack':'packs','list':'lists','games':'game','game':'game',
  'launch the game':'launch the game','launch':'start','edit':'edit','apply':'apply','%d mods':'%d mods','move':'move','apply / edit':'apply / edit','pages':'pages','all keys':'all keys',
  'last game':'last game','no errors':'no errors','no errors yet':'no errors yet','%d errors':'%d errors','the game is running':'game running','game running since %s':'game running since %s','pack %s running since %s':'pack %s running since %s',
  'a game is already running':'a game is already running','no game yet: add one on the games page':'no game yet: add one on the games page',
  'navigation at the top':'navigation at the top','navigation at the side':'navigation at the side','use this; no game file is written':'use this; no game file is written',
  'switch to %s and use this; no game file is written':'switch to %s and use this; no game file is written','which list runs is taken from the launch records':'which list runs is taken from the launch records',
};
