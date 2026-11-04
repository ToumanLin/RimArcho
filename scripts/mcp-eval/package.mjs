// Packs one case for the VM: manifest (which Mod folder each package id uses), ModsConfig.xml, a Player.log with the
// host sandbox paths rewritten to the VM layout, and the overlay files. Output: D:/RimArcho-test/mcp-eval/pkg/<case>/
// usage: node package.mjs <case>...
import fs from 'fs'; import path from 'path';
import {loadCase} from './sandbox.mjs';

const ROOT = 'D:/RimArcho-test/mcp-eval';
const WS = 'D:/SteamLibrary/steamapps/workshop/content/294100';
const LIBS = new Set(['2019000693', '3578444636', '3530229241', '_3700765558_alt_decapitate']);

function source(p) {
  p = p.split('\\').join('/');
  if (p.startsWith('D:/rimworld16/Data/')) return null;                       // official content lives in the game folder
  if (p.startsWith(WS + '/')) return 'ws:' + p.slice(WS.length + 1);
  const name = p.split('/').pop();
  const link = name.startsWith('_Archo_env_') ? fs.readlinkSync(p.replace(/\/$/, '')).split('\\').join('/').split('/').pop() : null;   // _<workshopId>_
  if (link) return 'ws:' + link.replace(/^_|_$/g, '');
  if (LIBS.has(name)) return 'lib:' + name;
  throw new Error('no VM source for ' + p);
}

// The case log as the VM's player would have it: the sandbox game folder becomes the guest game C:\Eval\RimWorld,
// in the Mono path lines too. archo reads `Mono path[0]` to tell whose game wrote a log (03 §6.4); without it, or
// with a host path, the log has no known owner, so archo before 1.0.1 skipped the attribution analysis (review
// 2026-11-04 §2.4). The sandbox's command line and save-folder lines would give the host layout away: dropped.
export function vmLog(text) {
  return text.replace(/D:([\\/])RimArcho-test[\\/]mcp-eval[\\/]run[\\/][\w-]+[\\/]game/gi, (m, s) => 'C:' + s + 'Eval' + s + 'RimWorld')
    .split(/\r?\n/).filter(l => !/^Command line arguments:|^Save data folder overridden/.test(l)).join('\r\n');
}

for (const id of process.argv.slice(2)) {
  const caseDir = `${ROOT}/cases/${id}`; const c = loadCase(caseDir);
  const base = JSON.parse(fs.readFileSync(`${ROOT}/cases/base-mods.json`, 'utf8'));
  const dropped = new Set((c.drop || []).map(d => d.toLowerCase()));
  const mods = [...base];
  for (const a of c.add || []) { const i = a.before ? mods.findIndex(m => m.pkg.toLowerCase() === a.before.toLowerCase()) : -1; if (i >= 0) mods.splice(i, 0, a); else mods.push(a); }
  const manifest = mods.map(m => ({pkg: m.pkg, src: source(m.path), overlay: !!c.overlay?.some(o => o.pkg.toLowerCase() === m.pkg.toLowerCase()), active: !dropped.has(m.pkg.toLowerCase())})).filter(m => m.src);
  const out = `${ROOT}/pkg/${id}`; fs.rmSync(out, {recursive: true, force: true}); fs.mkdirSync(out, {recursive: true});
  fs.writeFileSync(`${out}/manifest.json`, JSON.stringify(manifest, null, 1));
  fs.copyFileSync(`${ROOT}/run/${id}/userdata/Config/ModsConfig.xml`, `${out}/ModsConfig.xml`);
  for (const o of c.overlay || []) fs.cpSync(path.resolve(caseDir, o.files), `${out}/overlay/${o.pkg}`, {recursive: true});
  const log = vmLog(fs.readFileSync(`${caseDir}/Player.log`, 'utf8'));
  fs.writeFileSync(`${out}/Player.log`, log);
  const owned = /^Mono path\[0\] = 'C:\/Eval\/RimWorld\/RimWorldWin64_Data\/Managed'/m.test(log);
  console.log(id, manifest.length, 'mods;', manifest.filter(m => m.src.startsWith('lib:')).length, 'from lib;',
    /RimArcho-test/.test(log) ? 'LOG STILL HAS HOST PATHS' : 'log clean;', owned ? 'game path C:\\Eval\\RimWorld' : 'NO GUEST GAME PATH');
}
