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
    {id:'main-colony',kind:'plan',name:'主力殖民地',env:'main · 1.7',envName:'main',versions:'1.7',mods:487,played:'今天玩过',plan:1,page:'plan:1',at:6},
    {id:'industrial',kind:'pack',name:'工业整合包',env:'main · 1.7',envName:'main',versions:'1.6–1.7',mods:312,played:'昨天玩过',at:5},
    {id:'vanilla',kind:'plan',name:'轻量原版',env:'main · 1.7',envName:'main',versions:'1.7',mods:64,played:'10 月 21 日玩过',plan:2,page:'plan:2',at:4},
    {id:'old-save',kind:'plan',name:'旧档继续',env:'1.6 · 1.6',envName:'1.6',versions:'1.6',mods:206,played:'10 月 20 日玩过',plan:3,page:'plan:3',at:3},
    {id:'medieval',kind:'pack',name:'中世纪',env:'1.6 · 1.6',envName:'1.6',versions:'1.5–1.6',mods:156,played:'10 月 18 日玩过',at:2},
    {id:'new-list',kind:'plan',name:'新列表',env:'main · 1.7',envName:'main',versions:'1.7',mods:5,played:'还没玩过',plan:4,page:'plan:4',at:0},
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
  return {text:'2 个 Mod 的补丁没有找到目标',why:'落空的补丁通常会报红字：分享的日志里这样的 Mod 有 37% 出错（随便挑一个是 4.5%）。'};
}
export function homeData() {
  if (fixture.scenario === 'empty') return {rows:[],featured:-1,lastRunErrors:0};
  const current = applied();
  const rows = fixture.rows.map(r => ({...r,current:r.id === currentPlan().id,otherEnv:r.envName !== current.envName,
    state:r.kind === 'pack' ? '创建时检查过' : fixture.scenario === 'unchecked' || r.id === 'new-list' ? '还没检查' : issueFor(r) ? '2 个问题会让游戏出错' : '可以启动',
    level:issueFor(r) ? 'bad' : (fixture.scenario === 'unchecked' && r.kind === 'plan') || r.id === 'new-list' ? '' : 'ok',
  }));
  return {rows,featured:rows.findIndex(r => r.id === fixture.applied),lastRun:'主力殖民地 · 今天',lastRunDay:'今天',lastRunErrors:fixture.scenario === 'issue' ? 3 : 0,
    changes:{title:'当前列表自上次游玩以来',text:fixture.scenario === 'issue' && currentPlan().id === 'main-colony' ? '5 个 Mod 更新，1 个需要处理' : '没有变化'},launch:issueFor(current)};
}
export function shellData() {
  const count = fixture.scenario === 'issue' && currentPlan().id === 'main-colony' ? 3 : 0, plan = currentPlan();
  const nav = [{title:'在玩',items:[{id:'home',label:'启动'},{id:'needs',label:'能不能跑',badge:count},{id:'log',label:'游戏日志'}]},
    {title:'库',items:[{id:'library',label:'Mod 库',kids:[{id:'library',label:'全部'},{id:'changes',label:'变化'}]},
    {id:'plan',label:'列表',kids:fixture.rows.filter(r => r.kind === 'plan' && r.envName === plan.envName).map(r => ({id:r.page,label:r.name}))},
    {id:'packs',label:'整合包',kids:fixture.rows.filter(r => r.kind === 'pack' && r.envName === plan.envName).map(r => ({id:'pack:'+r.name,label:r.name}))},
    {id:'textures',label:'贴图优化'},{id:'env',label:'游戏'}]}];
  return {nav,env:plan.env,envs:fixture.scenario === 'empty' ? 0 : fixture.games.length,plan:{id:plan.plan || 1,name:plan.name,page:plan.page},plans:[],crumbs:{home:[{id:'home',label:'启动'}]}};
}
export const translations = {
  'settings':'设置','background tasks':'后台任务','start':'启动','can it run':'能不能跑','game log':'游戏日志','mod library':'Mod 库','lists':'列表','pack':'整合包','list':'列表','games':'游戏','game':'游戏',
  'launch the game':'启动游戏','launch':'启动','edit':'编辑','apply':'应用','%d mods':'%d 个 Mod','move':'移动','apply / edit':'应用 / 编辑','pages':'页面','all keys':'全部按键',
  'last game':'上次游戏','no errors':'没有报错','no errors yet':'还没有报错','%d errors':'%d 个错误','the game is running':'游戏运行中','game running since %s':'游戏运行中 · %s 起','pack %s running since %s':'整合包 %s 运行中 · %s 起',
  'a game is already running':'已有游戏在运行','no game yet: add one on the games page':'还没有游戏，在游戏页添加一个',
  'navigation at the top':'导航放在顶部','navigation at the side':'导航放在侧边','use this; no game file is written':'应用这个列表；不写入游戏文件',
  'switch to %s and use this; no game file is written':'切换到 %s 并应用；不写入游戏文件','which list runs is taken from the launch records':'运行的是哪个列表取自启动记录',
};
