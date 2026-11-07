import {S, $, el, say} from '/app/js/state.js';
import {setText} from '/app/js/api.js';
import {icon} from '/app/js/icons.js';
import * as shell from '/app/js/shell.js';
import * as screens from '/app/js/screens.js';
import * as originalHome from '/app/js/home.js';
import * as packs from '/app/js/packs.js';
import {fixture, applied, applyRow, homeData, shellData, issueFor, translations} from './fixtures.js';

setText(translations);
S.start = {lang:'en',commands:[],ops:[]};
S.board = {env:'main',envs:['main','1.6'],cols:[[],[],[]],stamp:'draft',needs:3};
S.screen = 'home';
let cursor = fixture.applied;
const ui = {
  renderWhere: () => shell.sync(), renderHints: renderHints,
  go: async id => {
    if (id === 'home') return draw();
    const names = {needs:'can it run',log:'game log',changes:'mod library / changes',library:'the library',plan:'lists',packs:'packs',textures:'texture optimisation',env:'game',settings:'settings',tasks:'background tasks'};
    say('', `draft entry: ${names[id] || (id.startsWith('plan:') ? 'edit list' : id.startsWith('pack:') ? 'packs' : id)}`);
  },
  reload: async () => { syncData(); shell.sync(); shell.renderLive?.(); if (fixture.design === 'original' && fixture.running) await originalHome.home(); },
  run: async name => { if (name === 'launch') launch(); },
  cmd: () => say('', 'draft entry: rescan'),
  planMenu: () => say('', 'draft entry: list menu'),
};
screens.initScreens(ui);
packs.initPacks(ui);
shell.initShell(ui);
if (fixture.design === 'original') originalHome.initHome(ui);
$('#tasks-line').innerHTML = `<span class="ic">${icon.tasks}</span><span class="lbl">background tasks</span>`;
$('#tasks-line').onclick = () => ui.go('tasks');

function syncData() {
  S.shell = shellData();
  S.board.needs = S.shell.nav.flatMap(g => g.items).find(i => i.id === 'needs')?.badge || 0;
  const row = applied(), running = fixture.rows.find(r => r.id === fixture.runningRow);
  S.board.env = row.envName;
  S.live = {running:fixture.running,items:[],since:'14:05',errors:fixture.running ? 3 : 0,pack:fixture.running && running.kind === 'pack' ? running.name : ''};
}
function renderHints() {
  const items = [['↑↓','move'],['enter','apply / edit'],['L','start'],['shift+1–8','pages'],['?','all keys']];
  const box = $('#hints'); box.replaceChildren();
  items.forEach(([key, text], i) => {
    if (i) box.append(el('span','sep','·'));
    box.append(el('b','',key),' '+text);
  });
}
function getRows() {
  const rows = homeData().rows || [];
  const sorted = [...rows].sort((a,b) => b.at - a.at || a.name.localeCompare(b.name));
  return [sorted.find(r => r.id === fixture.applied),...sorted.filter(r => r.id !== fixture.applied)].filter(Boolean);
}
function visibleRows() {
  const rows = getRows();
  return fixture.design === 'c' ? rows.filter(r => r.envName === applied().envName) : rows;
}
function rowLine(r) {
  const on = r.id === fixture.applied;
  const row = el('div','hm-row'+(on ? ' on' : '')+(cursor === r.id ? ' cur' : ''));
  row.dataset.row = r.id;
  row.setAttribute('role','listitem');
  row.title = `${r.env} · ${r.state}`;
  const name = el('div','n');
  name.append(el('span','kind',r.kind === 'plan' ? 'lists' : 'packs'),el('span','t',r.name));
  const sub = el('div','s num',`${r.versions} · ${r.mods} mods · ${r.played}`);
  const button = el('button','act',(on ? 'edit' : 'apply')+' ↵');
  button.title = on ? 'open editor' : r.otherEnv ? `switch to ${r.envName} and use this; no game file is written` : 'apply; no game file is written';
  button.onclick = e => {e.stopPropagation(); act(r);};
  row.append(name,sub,button);
  row.onclick = () => setCursor(r.id);
  row.ondblclick = () => act(r);
  return row;
}
function setCursor(id) {
  cursor = id;
  document.querySelectorAll('[data-row]').forEach(row => row.classList.toggle('cur',row.dataset.row === id));
}
async function act(r) {
  if (r.id === fixture.applied) return ui.go(r.kind === 'plan' ? r.page : 'pack:'+r.name);
  await apply(r.id);
}
async function apply(id, focusLabel) {
  applyRow(id); cursor = id;
  await draw();
  if (focusLabel) document.querySelector(`select[aria-label="${focusLabel}"]`)?.focus({preventScroll:true});
}
function chooseGame(name, focusLabel) {
  const next = fixture.remembered.get(name) || fixture.rows.find(r => r.envName === name)?.id;
  if (next) return apply(next,focusLabel);
}
function field(label, select, caption = '') {
  const group = el('div','sequence-field');
  const lab = el('label','sequence-label',label); lab.htmlFor = select.id;
  const control = el('div','sequence-select');
  const arrow = el('span','select-chevron'); arrow.innerHTML = icon.chev;
  control.append(select,arrow);
  group.append(lab,control);
  if (caption) group.append(el('span','sequence-caption num',caption));
  return group;
}
function gameField() {
  const select = el('select'); select.id = 'game-select'; select.setAttribute('aria-label','game');
  fixture.games.forEach(g => {
    const option = el('option','',g.label); option.value = g.name; option.selected = g.name === applied().envName; select.append(option);
  });
  select.onchange = () => chooseGame(select.value,'game');
  return field('game',select);
}
function targetField() {
  const select = el('select'); select.id = 'target-select'; select.setAttribute('aria-label','list or pack');
  const rows = getRows().filter(r => r.envName === applied().envName);
  for (const [kind,label] of [['plan','lists'],['pack','packs']]) {
    const group = el('optgroup'); group.label = label;
    rows.filter(r => r.kind === kind).forEach(r => {
      const option = el('option','',r.name); option.value = r.id; option.selected = r.id === fixture.applied; group.append(option);
    });
    if (group.children.length) select.append(group);
  }
  select.onchange = () => apply(select.value,'list or pack');
  const r = applied();
  return field('list or pack',select,`${r.versions} · ${r.mods} mods`);
}
function launchStack() {
  const box = el('div','stack');
  const button = el('button','btn launch primary'); button.dataset.launch = '';
  const ic = el('span','ic'); ic.innerHTML = icon.play;
  button.append(ic,el('span','','launch the game'),el('kbd','','L'));
  button.disabled = fixture.running;
  const running = fixture.rows.find(r => r.id === fixture.runningRow);
  button.title = fixture.running ? `a game is already running: ${running.env} · ${running.name}` : `launch ${applied().env} · ${applied().name}`;
  button.onclick = launch;
  box.append(button);
  const hint = issueFor(applied());
  if (hint) {
    const issue = el('button','issue bad hint');
    const warning = el('span','ic'); warning.innerHTML = icon.warn;
    const text = el('span','t'); text.append(el('span','what',hint.text),el('span','why',hint.why));
    issue.append(warning,text,el('span','go','→'));
    issue.title = hint.text+'\n'+hint.why;
    issue.onclick = () => ui.go('needs'); box.append(issue);
  } else if (fixture.scenario === 'unchecked' && applied().kind === 'plan') {
    const note = el('button','issue unchecked','mod files have not been checked yet →');
    note.onclick = () => ui.go('needs'); box.append(note);
  }
  return box;
}
function sequence() {
  const box = el('section','hm-launch launch-sequence'); box.setAttribute('aria-label','choose a game, list or pack, then launch');
  box.append(gameField(),el('span','sequence-arrow','›'),targetField(),el('span','sequence-arrow','›'),launchStack());
  return box;
}
function activity() {
  const box = el('div','hm-act');
  const line = (label,text,id) => {
    const b = el('button','hm-actline'); b.append(el('span','t',label),el('span','x',text),el('span','go','→')); b.onclick = () => ui.go(id); return b;
  };
  box.append(line('last game',fixture.running ? 'game running · 3 errors' : fixture.scenario === 'issue' ? 'today · 3 errors' : 'today · no errors',fixture.running || fixture.scenario === 'issue' ? 'needs' : 'log'));
  const changes = homeData().changes;
  if (changes) box.append(line(changes.title,changes.text,'changes'));
  return box;
}
function gamesColumn() {
  const pane = el('div','games-column'); pane.append(el('div','column-label','game'));
  fixture.games.forEach(g => {
    const b = el('button','game-choice'+(applied().envName === g.name ? ' on' : ''));
    b.dataset.game = g.name;
    b.setAttribute('aria-pressed',String(applied().envName === g.name));
    b.append(el('span','game-name',g.name),el('span','game-version',g.version));
    b.onclick = async () => {await chooseGame(g.name); document.querySelector(`[data-game="${g.name}"]`)?.focus({preventScroll:true});};
    pane.append(b);
  });
  return pane;
}
async function launch() {
  if (fixture.running || fixture.scenario === 'empty') return;
  const r = applied(); fixture.running = true; fixture.runningRow = r.id;
  await draw();
  say('ok',`launch submitted: ${r.env} · ${r.name} (draft preview)`);
}
async function draw() {
  syncData(); shell.sync(); shell.renderLive?.(); renderHints();
  if (fixture.design === 'original') { await originalHome.home(); return; }
  originalHome.setBackdrop(fixture.background === 'plain' ? {} : {url:'/background.png',dim:30,blur:0,focus:'center'});
  const screen = $('#screen'); screen.replaceChildren();
  const page = screens.page(`home draft-home layout-${fixture.design}`,'');
  screen.append(page);
  if (fixture.scenario === 'empty') {
    const empty = el('div','card empty-state','no game yet: add one on the games page');
    const add = el('button','btn','add a game'); add.onclick = () => ui.go('env'); empty.append(add); page.append(empty); return;
  }
  const menu = el('div','hm-menu');
  const list = el('div','hm-list'); list.setAttribute('role','list'); list.setAttribute('aria-label','lists and packs');
  if (fixture.design === 'c') menu.append(el('div','column-label','list or pack'));
  visibleRows().forEach(r => list.append(rowLine(r))); menu.append(list);
  if (fixture.design === 'c') {
    page.append(gamesColumn(),menu,activity());
    const launcher = el('div','hm-launch');
    const where = el('div','where'); where.append(applied().env+' · ',el('b','',applied().name));
    launcher.append(where,launchStack()); page.append(launcher);
  } else page.append(menu,activity(),sequence());
}
document.addEventListener('keydown',e => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {e.preventDefault(); shell.toggleNav(); return;}
  if (e.target.matches('select,input,button') || e.ctrlKey || e.metaKey || e.altKey) return;
  if (fixture.scenario === 'empty') return;
  if (fixture.design === 'original') {originalHome.homeKey(e); return;}
  const rows = visibleRows();
  if (['ArrowDown','ArrowUp','Home','End'].includes(e.key)) {
    e.preventDefault();
    const i = Math.max(0,rows.findIndex(r => r.id === cursor));
    const next = e.key === 'Home' ? 0 : e.key === 'End' ? rows.length-1 : (i+(e.key === 'ArrowDown' ? 1 : rows.length-1))%rows.length;
    if (rows[next]) {setCursor(rows[next].id); document.querySelector(`[data-row="${cursor}"]`)?.scrollIntoView({block:'nearest'});}
  } else if (e.key === 'Enter') {e.preventDefault(); const row = rows.find(r => r.id === cursor); if (row) act(row);}
  else if (e.key === 'L' || e.key === 'l' && e.shiftKey) {e.preventDefault(); launch();}
});
await draw(); shell.settle();
