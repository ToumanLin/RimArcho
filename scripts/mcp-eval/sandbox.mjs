// Builds an isolated RimWorld 1.6 sandbox for one eval case and records its Player.log.
// Real game files and real Mods are junctioned (read-only use); only overlay Mods are copies.
// usage: node sandbox.mjs <caseDir> [--run] [--seconds N]
import fs from 'fs'; import path from 'path'; import {spawn, execFileSync} from 'child_process';

const GAME = 'D:/rimworld16';
// Pinned 1.6 config templates (ModsConfig.xml: version and expansions; Prefs.xml). Never the host
// player's live Config: once a 1.7 game rewrote it, every graded log gained PrefsData XML errors.
const USERCFG = 'D:/RimArcho-test/mcp-eval/cases/base/Config';

// submitted JSON may start with a BOM, and a one-item list may be a bare string (Windows PowerShell 5's
// ConvertTo-Json does both): [].concat() turns either into a list
const readJson = f => JSON.parse(fs.readFileSync(f, 'utf8').trimStart());
export function loadCase(dir) { return JSON.parse(fs.readFileSync(path.join(dir, 'case.json'), 'utf8')); }

// case.json: { id, base: "../base-mods.json", add: [{pkg, src}], overlay: [{pkg, folder, files: "overlay/<folder>"}], drop: [pkg] }
// layerDir (a submission): files at <packageId>/<path>, ModsConfig.xml, enable.json (re-enabled Mods), delete.json (removed files)
export function build(caseDir, runDir, layerDir) {
  const c = loadCase(caseDir);
  const base = JSON.parse(fs.readFileSync(path.resolve(caseDir, c.base), 'utf8'));
  fs.rmSync(runDir, {recursive: true, force: true});
  const g = path.join(runDir, 'game'); fs.mkdirSync(path.join(g, 'Mods'), {recursive: true});
  for (const n of fs.readdirSync(GAME)) {
    const p = path.join(GAME, n); if (n === 'Mods' || n === 'steamapps') continue;
    if (fs.statSync(p).isDirectory()) fs.symlinkSync(p, path.join(g, n), 'junction');
    else fs.copyFileSync(p, path.join(g, n));
  }
  const active = [];
  const enable = layerDir && fs.existsSync(path.join(layerDir, 'enable.json')) ? [].concat(readJson(path.join(layerDir, 'enable.json'))).map(p => p.toLowerCase()) : [];
  // delete.json: files the submission removes, as `<packageId>/<path in the Mod>`
  const del = layerDir && fs.existsSync(path.join(layerDir, 'delete.json')) ? [].concat(readJson(path.join(layerDir, 'delete.json'))).map(f => String(f).replaceAll('\\', '/')) : [];
  const isDropped = m => c.drop?.some(d => d.toLowerCase() === m.pkg.toLowerCase()) && !enable.includes(m.pkg.toLowerCase());
  const mods = [...base];
  for (const a of c.add || []) { const i = a.before ? mods.findIndex(m => m.pkg.toLowerCase() === a.before.toLowerCase()) : -1; if (i >= 0) mods.splice(i, 0, a); else mods.push(a); }
  for (const m of mods) {
    if (!isDropped(m)) active.push(m.pkg);   // dropped Mods stay installed but are not enabled
    if (m.path.startsWith(GAME + '/Data') || m.path.startsWith('D:\rimworld16\Data')) continue; // official content is already in Data
    const ov = c.overlay?.find(o => o.pkg.toLowerCase() === m.pkg.toLowerCase());
    const dest = path.join(g, 'Mods', m.pkg);
    const layer = layerDir && fs.existsSync(path.join(layerDir, m.pkg)) ? path.join(layerDir, m.pkg) : null;
    const gone = del.filter(f => f.toLowerCase().startsWith(m.pkg.toLowerCase() + '/')).map(f => f.slice(m.pkg.length + 1));
    if (ov || layer || gone.length) {
      copyDir(m.path, dest); if (ov) copyDir(path.resolve(caseDir, ov.files), dest); if (layer) copyDir(layer, dest);
      for (const f of gone) fs.rmSync(path.join(dest, f), {force: true});
    }
    else fs.symlinkSync(m.path, dest, 'junction');
  }
  if (layerDir && fs.existsSync(path.join(layerDir, 'ModsConfig.xml'))) {   // the submission changed the active list
    const xml = fs.readFileSync(path.join(layerDir, 'ModsConfig.xml'), 'utf8').match(/<activeMods>[\s\S]*?<\/activeMods>/)[0];
    const ids = [...xml.matchAll(/<li>([^<]+)<\/li>/g)].map(m => m[1]);
    active.length = 0; active.push(...ids);
  }
  const cfg = path.join(runDir, 'userdata', 'Config'); fs.mkdirSync(cfg, {recursive: true});
  let x = fs.readFileSync(path.join(USERCFG, 'ModsConfig.xml'), 'utf8');
  x = x.replace(/<activeMods>[\s\S]*?<\/activeMods>/, '<activeMods>\n' + active.map(p => `    <li>${p.toLowerCase()}</li>`).join('\n') + '\n  </activeMods>');
  fs.writeFileSync(path.join(cfg, 'ModsConfig.xml'), x);
  fs.copyFileSync(path.join(USERCFG, 'Prefs.xml'), path.join(cfg, 'Prefs.xml'));
  return {game: g, userdata: path.join(runDir, 'userdata'), log: path.join(runDir, 'Player.log'), active};
}

function copyDir(src, dst) { fs.cpSync(src, dst, {recursive: true, force: true, dereference: true}); }

export async function run(env, seconds = 150) {
  const exe = path.join(env.game, 'RimWorldWin64.exe');
  const child = spawn(exe, [`-savedatafolder=${env.userdata}`, '-logFile', env.log], {cwd: env.game, stdio: 'ignore', detached: false, env: {...process.env, HARMONY_LOG_FILE: path.join(path.dirname(env.log), 'harmony.log.txt')}});
  let lastSize = -1, stable = 0; const t0 = Date.now();
  while (Date.now() - t0 < seconds * 1000) {
    await new Promise(r => setTimeout(r, 5000));
    const sz = fs.existsSync(env.log) ? fs.statSync(env.log).size : 0;
    stable = sz === lastSize && sz > 0 ? stable + 1 : 0; lastSize = sz;
    if (stable >= 8 && sz > 8000 && Date.now() - t0 > 60000) break;   // 40 s of silence once past the Mod loading output: at the main menu
  }
  try { execFileSync('taskkill', ['/PID', String(child.pid), '/T', '/F']); } catch {}
  return fs.existsSync(env.log);
}

if (process.argv[1].endsWith('sandbox.mjs')) {
  const caseDir = path.resolve(process.argv[2]); const c = loadCase(caseDir);
  const fixed = process.argv.includes('--fixed'); const layer = fixed ? path.join(caseDir, 'answer') : (process.argv.includes('--layer') ? process.argv[process.argv.indexOf('--layer') + 1] : undefined);
  const runDir = `D:/RimArcho-test/mcp-eval/run/${c.id}${fixed ? '-fixed' : (layer ? '-graded' : '')}`;
  const env = build(caseDir, runDir, layer);
  console.log('built', env.active.length, 'mods ->', runDir);
  if (process.argv.includes('--run')) {
    const i = process.argv.indexOf('--seconds');
    console.log('log written:', await run(env, i > 0 ? +process.argv[i + 1] : 150));
  }
}
