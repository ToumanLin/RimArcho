#!/usr/bin/env node
// rimworld-probe: create, build and run a probe mod in an isolated RimWorld sandbox (Windows, Unity builds 1.6 and earlier).
//
//   node probe.mjs new   <probeDir> [--id probe.scenario]
//   node probe.mjs build <probeDir> --game <gameDir> --harmony <harmonyModDir>
//   node probe.mjs run   --game <gameDir> --work <workDir> --name <run> --mods "<pkgId>=<folder>,..." [--no-dlc ideology,...] [--seconds 600]
//   node probe.mjs clean --work <workDir>
//
// run: --mods lists every non-official mod in load order, Harmony and the probe included. Official content
// (Core and the DLCs found in <gameDir>/Data) loads first; --no-dlc leaves listed DLCs off.
// The sandbox is <workDir>/game: the game's folders and every mod folder are junctions (nothing is copied but the
// small top-level files), and saves, config and the log go to <workDir>/runs/<name>. Steam is not started.
import fs from 'fs'; import path from 'path'; import os from 'os'; import crypto from 'crypto';
import {spawn, execFileSync} from 'child_process'; import {fileURLToPath} from 'url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const TEMPLATE = path.join(HERE, '..', 'template');
const MARK = /\[Probe\] (RESULT|EXCEPTION)/;

function opts(argv) {
  const o = {_: []};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i].startsWith('--')) { const k = argv[i].slice(2); o[k] = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[++i] : true; }
    else o._.push(argv[i]);
  }
  return o;
}
function need(o, k) { if (!o[k]) { console.error(`missing --${k}`); process.exit(2); } return o[k]; }

function cmdNew(o) {
  const dir = path.resolve(need({dir: o._[1]}, 'dir'));
  if (fs.existsSync(dir)) { console.error(`${dir} already exists`); process.exit(2); }
  fs.cpSync(TEMPLATE, dir, {recursive: true});
  if (o.id) {
    const about = path.join(dir, 'About', 'About.xml');
    fs.writeFileSync(about, fs.readFileSync(about, 'utf8').replace('probe.scenario', o.id));
  }
  console.log(`created ${dir}\nedit src/Scenario.cs, then: node probe.mjs build "${dir}" --game <gameDir> --harmony <harmonyModDir>`);
}

function cmdBuild(o) {
  const dir = path.resolve(o._[1] || '.');
  const game = path.resolve(need(o, 'game')), harmony = path.resolve(need(o, 'harmony'));
  const r = spawn('dotnet', ['build', '-c', 'Release', '-nologo', '-v', 'q', `-p:RimWorldDir=${game}`, `-p:HarmonyDir=${harmony}`], {cwd: dir, stdio: 'inherit', shell: false});
  r.on('exit', code => {
    if (code === 0) console.log(`built ${path.join(dir, 'Assemblies', 'Probe.dll')}`);
    process.exit(code);
  });
}

// The real player config. The sandbox must not change it; the runner reports if it did.
function realPrefs() {
  return path.join(os.homedir(), 'AppData', 'LocalLow', 'Ludeon Studios', 'RimWorld by Ludeon Studios', 'Config', 'Prefs.xml');
}
function hash(f) { return fs.existsSync(f) ? crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex') : 'missing'; }

// Official expansions in release order: later DLCs inherit defs from earlier ones, so alphabetical order breaks loading.
const RELEASE_ORDER = ['royalty', 'ideology', 'biotech', 'anomaly', 'odyssey'];
function dlcs(game) {
  const data = path.join(game, 'Data');
  const rank = n => { const i = RELEASE_ORDER.indexOf(n); return i < 0 ? RELEASE_ORDER.length : i; };
  return fs.readdirSync(data).filter(n => fs.statSync(path.join(data, n)).isDirectory() && n !== 'Core')
    .map(n => n.toLowerCase()).sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
    .map(n => 'ludeon.rimworld.' + n);
}

function buildGame(game, work, mods) {
  const g = path.join(work, 'game');
  fs.mkdirSync(path.join(g, 'Mods'), {recursive: true});
  for (const n of fs.readdirSync(game)) {
    const p = path.join(game, n), d = path.join(g, n);
    if (n === 'Mods' || n === 'steamapps' || fs.existsSync(d)) continue;
    if (fs.statSync(p).isDirectory()) fs.symlinkSync(p, d, 'junction');
    else fs.copyFileSync(p, d);
  }
  for (const m of mods) {
    const d = path.join(g, 'Mods', m.pkg);
    if (fs.existsSync(d)) {
      if (path.resolve(fs.realpathSync(d)) === path.resolve(fs.realpathSync(m.dir))) continue;
      unlinkJunction(d);
    }
    fs.symlinkSync(m.dir, d, 'junction');
  }
  return g;
}

function writeConfig(game, userdata, active, known) {
  const cfg = path.join(userdata, 'Config'); fs.mkdirSync(cfg, {recursive: true});
  const version = fs.readFileSync(path.join(game, 'Version.txt'), 'utf8').trim();
  const li = a => a.map(p => `    <li>${p}</li>`).join('\n');
  fs.writeFileSync(path.join(cfg, 'ModsConfig.xml'), `<?xml version="1.0" encoding="utf-8"?>
<ModsConfigData>
  <version>${version}</version>
  <activeMods>
${li(active)}
  </activeMods>
  <knownExpansions>
${li(known)}
  </knownExpansions>
</ModsConfigData>
`);
  // Windowed and small, so the game does not take over the screen; dev mode for Ultrafast and dev tools.
  fs.writeFileSync(path.join(cfg, 'Prefs.xml'), `<?xml version="1.0" encoding="utf-8"?>
<PrefsData>
  <screenWidth>1280</screenWidth>
  <screenHeight>720</screenHeight>
  <fullscreen>False</fullscreen>
  <uiScale>1</uiScale>
  <runInBackground>True</runInBackground>
  <pauseOnLoad>False</pauseOnLoad>
  <pauseOnError>False</pauseOnError>
  <automaticPauseMode>Never</automaticPauseMode>
  <devMode>True</devMode>
  <langFolderName>English</langFolderName>
  <resetModsConfigOnCrash>False</resetModsConfigOnCrash>
  <volumeMaster>0</volumeMaster>
</PrefsData>
`);
}

async function cmdRun(o) {
  const game = path.resolve(need(o, 'game')), work = path.resolve(need(o, 'work')), name = need(o, 'name');
  const seconds = +(o.seconds || 600);
  const mods = need(o, 'mods').split(',').map(s => s.trim()).filter(Boolean).map(s => {
    const i = s.indexOf('='); if (i < 0) { console.error(`--mods entry needs pkgId=folder: ${s}`); process.exit(2); }
    const dir = path.resolve(s.slice(i + 1));
    if (!fs.existsSync(path.join(dir, 'About', 'About.xml'))) { console.error(`no About/About.xml in ${dir}`); process.exit(2); }
    return {pkg: s.slice(0, i).toLowerCase(), dir};
  });
  if (path.resolve(work).toLowerCase().startsWith(path.resolve(game).toLowerCase())) { console.error('--work must be outside the game folder'); process.exit(2); }
  const off = String(o['no-dlc'] || '').split(',').filter(Boolean).map(s => 'ludeon.rimworld.' + s.toLowerCase());
  const known = dlcs(game);
  const active = ['ludeon.rimworld', ...known.filter(d => !off.includes(d)), ...mods.map(m => m.pkg)];
  // Harmony must load before the official content, as players have it.
  const h = active.indexOf('brrainz.harmony'); if (h > 0) { active.splice(h, 1); active.unshift('brrainz.harmony'); }

  const g = buildGame(game, work, mods);
  const run = path.join(work, 'runs', name);
  fs.rmSync(run, {recursive: true, force: true});
  const userdata = path.join(run, 'userdata'), log = path.join(run, 'Player.log');
  writeConfig(game, userdata, active, known);
  console.log(`${name}: ${active.join(' > ')}`);

  const before = hash(realPrefs());
  const args = [`-savedatafolder=${userdata}`, '-logFile', log, '-quicktest', '-probequit',
    '-screen-fullscreen', '0', '-screen-width', '1280', '-screen-height', '720'];
  const child = spawn(path.join(g, 'RimWorldWin64.exe'), args, {cwd: g, stdio: 'ignore'});
  let exited = false; child.on('exit', () => { exited = true; });
  const t0 = Date.now(); let reason = 'timeout';
  while (Date.now() - t0 < seconds * 1000) {
    await new Promise(r => setTimeout(r, 2000));
    if (exited) { reason = 'game exited'; break; }
    const text = fs.existsSync(log) ? fs.readFileSync(log, 'utf8') : '';
    if (MARK.test(text)) { reason = 'result'; await new Promise(r => setTimeout(r, 8000)); break; }
  }
  if (!exited) { try { execFileSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], {stdio: 'ignore'}); } catch {} }
  await new Promise(r => setTimeout(r, 1500));

  const text = fs.existsSync(log) ? fs.readFileSync(log, 'utf8') : '';
  const lines = text.split(/\r?\n/).filter(l => l.includes('[Probe]'));
  fs.writeFileSync(path.join(run, 'probe-lines.txt'), lines.join('\n') + '\n');
  const errors = text.split(/\r?\n/).filter(l => /^(Exception|Error|Could not)|\[Probe\] EXCEPTION/.test(l));
  console.log(`ended: ${reason} after ${Math.round((Date.now() - t0) / 1000)} s; log ${log}`);
  for (const l of lines) console.log('  ' + l);
  if (!lines.length) console.log('  (no [Probe] lines: check the log for load errors, or raise --seconds)');
  if (errors.length) console.log(`log has ${errors.length} error lines, first: ${errors[0].slice(0, 200)}`);
  const shot = path.join(userdata, 'probe.png');
  if (fs.existsSync(shot)) console.log(`screenshot ${shot}`);
  if (hash(realPrefs()) !== before) console.log(`WARNING: the real ${realPrefs()} changed during this run. Report it; do not edit it.`);
  process.exit(lines.some(l => l.includes('[Probe] RESULT')) ? 0 : 1);
}

// Remove a junction without touching what it points to.
function unlinkJunction(p) {
  const st = fs.lstatSync(p);
  if (!st.isSymbolicLink()) throw new Error(`refusing to remove ${p}: not a junction`);
  fs.rmdirSync(p);
}

function cmdClean(o) {
  const g = path.join(path.resolve(need(o, 'work')), 'game');
  if (!fs.existsSync(g)) { console.log('nothing to clean'); return; }
  let n = 0;
  for (const dir of [path.join(g, 'Mods'), g]) {
    if (!fs.existsSync(dir)) continue;
    for (const e of fs.readdirSync(dir)) {
      const p = path.join(dir, e);
      if (fs.lstatSync(p).isSymbolicLink()) { unlinkJunction(p); n++; }
    }
  }
  const left = [];
  (function walk(d) { for (const e of fs.readdirSync(d)) { const p = path.join(d, e); const st = fs.lstatSync(p); if (st.isSymbolicLink()) left.push(p); else if (st.isDirectory()) walk(p); } })(g);
  if (left.length) { console.error(`links still present, not deleting: ${left.join(', ')}`); process.exit(1); }
  fs.rmSync(g, {recursive: true, force: true});
  console.log(`removed ${n} junctions and ${g}; run logs stay in ${path.join(path.resolve(o.work), 'runs')}`);
}

const o = opts(process.argv.slice(2));
const cmd = {new: cmdNew, build: cmdBuild, run: cmdRun, clean: cmdClean}[o._[0]];
if (!cmd) { console.error('usage: probe.mjs new|build|run|clean (see the header of this file)'); process.exit(2); }
await cmd(o);
