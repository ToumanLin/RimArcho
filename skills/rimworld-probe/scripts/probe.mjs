#!/usr/bin/env node
// rimworld-probe: create, build and run a probe mod in an isolated RimWorld sandbox (Windows; 1.6 Unity and 1.7 Waylith).
//
//   node probe.mjs new   <probeDir> [--id probe.scenario]
//   node probe.mjs build <probeDir> --game <gameDir> --harmony <harmonyModDir>
//   node probe.mjs run   --game <gameDir> --work <workDir> --name <run> --mods "<pkgId>=<folder>,..." [--no-dlc ideology,...] [--seconds 600]
//   node probe.mjs clean --work <workDir>
//
// build detects the game version and builds <probeDir>/<version>/Assemblies/Probe.dll.
// run: --mods lists every non-official mod in load order, Harmony and the probe included. Official content
// (Core and the DLCs found in <gameDir>/Data) loads first; --no-dlc leaves listed DLCs off.
// The sandbox is <workDir>/game-<version>: the game's folders and every mod folder are junctions, the game's top-level
// files are hard links (copies only if the work folder is on another drive). User data (config, saves, the 1.7 shader
// and texture caches) is <workDir>/userdata-<version>, reused between runs; each run's log, probe lines and screenshot go
// to <workDir>/runs/<name>. The sandbox has no steam_appid.txt, so the game stays off Steam even if the client runs.
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

// 1.7 is the Waylith engine on .NET 10: different executable, assemblies in the game folder, a fixed log path.
function gameInfo(game) {
  if (!fs.existsSync(path.join(game, 'Version.txt'))) { console.error(`not a RimWorld folder (no Version.txt): ${game}`); process.exit(2); }
  const full = fs.readFileSync(path.join(game, 'Version.txt'), 'utf8').trim();
  const version = full.split('.').slice(0, 2).join('.');
  const waylith = fs.existsSync(path.join(game, 'Waylith.dll'));
  return {full, version, waylith, exe: waylith ? 'RimWorld.exe' : 'RimWorldWin64.exe'};
}

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
  const {version} = gameInfo(game);
  const args = ['build', '-c', 'Release', '-nologo', '-v', 'q', `-p:GameVersion=${version}`, `-p:RimWorldDir=${game}`, `-p:HarmonyDir=${harmony}`];
  const r = spawn('dotnet', args, {cwd: dir, stdio: 'inherit', shell: false});
  r.on('exit', code => {
    if (code === 0) console.log(`built ${path.join(dir, version, 'Assemblies', 'Probe.dll')} for ${version}`);
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

function buildGame(game, g, mods) {
  fs.mkdirSync(path.join(g, 'Mods'), {recursive: true});
  for (const n of fs.readdirSync(game)) {
    const p = path.join(game, n), d = path.join(g, n);
    // Without steam_appid.txt the game does not attach to a running Steam client: no Steam Cloud sync of the player's
    // save folder, no Workshop scan.
    if (n === 'steam_appid.txt') { fs.rmSync(d, {force: true}); continue; }
    if (n === 'Mods' || n === 'steamapps' || fs.existsSync(d)) continue;
    if (fs.statSync(p).isDirectory()) fs.symlinkSync(p, d, 'junction');
    else { try { fs.linkSync(p, d); } catch { fs.copyFileSync(p, d); } }
  }
  for (const m of mods) {
    const d = path.join(g, 'Mods', m.pkg);
    if (fs.existsSync(d)) {
      if (path.resolve(fs.realpathSync(d)) === path.resolve(fs.realpathSync(m.dir))) continue;
      unlinkJunction(d);
    }
    fs.symlinkSync(m.dir, d, 'junction');
  }
}

function writeConfig(info, userdata, active, known) {
  const cfg = path.join(userdata, 'Config'); fs.mkdirSync(cfg, {recursive: true});
  const li = a => a.map(p => `    <li>${p}</li>`).join('\n');
  fs.writeFileSync(path.join(cfg, 'ModsConfig.xml'), `<?xml version="1.0" encoding="utf-8"?>
<ModsConfigData>
  <version>${info.full}</version>
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
  <adaptiveTrainingEnabled>False</adaptiveTrainingEnabled>
  <langFolderName>English</langFolderName>
  <resetModsConfigOnCrash>False</resetModsConfigOnCrash>
  <volumeMaster>0</volumeMaster>
</PrefsData>
`);
  // A save left from an earlier run must not load instead of the quicktest map (dev mode loads one named "autostart").
  fs.rmSync(path.join(userdata, 'Saves'), {recursive: true, force: true});
  fs.rmSync(path.join(userdata, 'probe.png'), {force: true});
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
  if (work.toLowerCase().startsWith(game.toLowerCase())) { console.error('--work must be outside the game folder'); process.exit(2); }
  const info = gameInfo(game);
  const off = String(o['no-dlc'] || '').split(',').filter(Boolean).map(s => 'ludeon.rimworld.' + s.toLowerCase());
  const known = dlcs(game);
  const active = ['ludeon.rimworld', ...known.filter(d => !off.includes(d)), ...mods.map(m => m.pkg)];
  // Harmony must load before the official content, as players have it.
  const h = active.indexOf('brrainz.harmony'); if (h > 0) { active.splice(h, 1); active.unshift('brrainz.harmony'); }

  const g = path.join(work, `game-${info.version}`);
  buildGame(game, g, mods);
  const userdata = path.join(work, `userdata-${info.version}`);
  writeConfig(info, userdata, active, known);
  const run = path.join(work, 'runs', name);
  fs.rmSync(run, {recursive: true, force: true}); fs.mkdirSync(run, {recursive: true});
  const log = path.join(run, 'Player.log');
  console.log(`${name} (${info.full}): ${active.join(' > ')}`);

  const before = hash(realPrefs());
  let child;
  if (info.waylith) {
    // 1.7 writes its file log to the player's LocalLow folder whatever the arguments; -NoFileLog turns it off and the
    // console output becomes the log.
    const out = fs.openSync(log, 'a');
    child = spawn(path.join(g, info.exe), [`-savedatafolder=${userdata}`, '-NoFileLog', '-quicktest', '-probequit'],
      {cwd: g, stdio: ['ignore', out, out], windowsHide: true});
  } else {
    child = spawn(path.join(g, info.exe), [`-savedatafolder=${userdata}`, '-logFile', log, '-quicktest', '-probequit',
      '-screen-fullscreen', '0', '-screen-width', '1280', '-screen-height', '720'], {cwd: g, stdio: 'ignore'});
  }
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
  const lines = text.split(/\r?\n/).filter(l => l.includes('[Probe]')).map(l => l.slice(l.indexOf('[Probe]')));
  fs.writeFileSync(path.join(run, 'probe-lines.txt'), lines.join('\n') + '\n');
  // Unity file log: "Exception ..." / "Error ..." at line start; 1.7 console: "HH:MM:SS  ERROR message".
  const errors = text.split(/\r?\n/).filter(l => /^(Exception|Error|Could not)|^\d\d:\d\d:\d\d\s+(ERROR|FATAL)\b|\[Probe\] EXCEPTION/.test(l));
  console.log(`ended: ${reason} after ${Math.round((Date.now() - t0) / 1000)} s; log ${log}`);
  for (const l of lines) console.log('  ' + l);
  if (!lines.length) console.log('  (no [Probe] lines: check the log for load errors, or raise --seconds)');
  if (errors.length) console.log(`log has ${errors.length} error lines, first: ${errors[0].slice(0, 200)}`);
  const shot = path.join(userdata, 'probe.png');
  if (fs.existsSync(shot)) { fs.copyFileSync(shot, path.join(run, 'probe.png')); console.log(`screenshot ${path.join(run, 'probe.png')}`); }
  if (hash(realPrefs()) !== before) console.log(`WARNING: the real ${realPrefs()} changed during this run. Report it; do not edit it.`);
  process.exit(lines.some(l => l.includes('[Probe] RESULT')) ? 0 : 1);
}

// Remove a junction without touching what it points to.
function unlinkJunction(p) {
  const st = fs.lstatSync(p);
  if (!st.isSymbolicLink()) throw new Error(`refusing to remove ${p}: not a junction`);
  fs.rmdirSync(p);
}

// Removes the sandbox game folders. Junctions are unlinked first; the remaining top-level files are hard links or
// copies, so deleting them leaves the real game intact. User data and run logs stay.
function cmdClean(o) {
  const work = path.resolve(need(o, 'work'));
  const games = fs.existsSync(work) ? fs.readdirSync(work).filter(n => /^game(-|$)/.test(n)).map(n => path.join(work, n)) : [];
  if (!games.length) { console.log('nothing to clean'); return; }
  for (const g of games) {
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
    console.log(`removed ${n} junctions and ${g}`);
  }
  console.log(`user data and run logs stay in ${work}`);
}

const o = opts(process.argv.slice(2));
const cmd = {new: cmdNew, build: cmdBuild, run: cmdRun, clean: cmdClean}[o._[0]];
if (!cmd) { console.error('usage: probe.mjs new|build|run|clean (see the header of this file)'); process.exit(2); }
await cmd(o);
