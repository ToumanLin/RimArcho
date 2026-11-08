// Builds the prepared save of case h8: a quicktest map with a Progression: Education classroom in session and a Socio Butterfly
// birthday party that holds the students, made by the helper Mod mods/pe-setup (an authoring tool, never in the case).
// usage: node make-pe-save.mjs      (after `node cases.mjs h8`; writes D:/RimArcho-test/mcp-eval/cases/h8/save/autostart.rws)
import fs from 'fs'; import path from 'path'; import {spawn, execFileSync} from 'child_process';
import {build} from './sandbox.mjs';

const ROOT = 'D:/RimArcho-test/mcp-eval'; const SRC = path.join(import.meta.dirname, 'mods', 'pe-setup');
const HELPER = 'rimarcho.eval.pesetup';

// the helper Mod, built next to the other generated Mods
const mod = `${ROOT}/cases/_mods/${HELPER}`; fs.rmSync(mod, {recursive: true, force: true});
fs.cpSync(path.join(SRC, 'About'), path.join(mod, 'About'), {recursive: true});
const tmp = `${ROOT}/_build/pe-setup`; fs.rmSync(tmp, {recursive: true, force: true}); fs.mkdirSync(tmp, {recursive: true});
fs.cpSync(path.join(SRC, 'Source'), path.join(tmp, 'Source'), {recursive: true}); fs.copyFileSync(path.join(SRC, 'pe-setup.csproj'), path.join(tmp, 'pe-setup.csproj'));
execFileSync('dotnet', ['build', '-c', 'Release', '-nologo', '-v', 'q'], {cwd: tmp, stdio: 'pipe'});
fs.mkdirSync(`${mod}/1.6/Assemblies`, {recursive: true}); fs.copyFileSync(`${tmp}/bin/PeEvalSetup.dll`, `${mod}/1.6/Assemblies/PeEvalSetup.dll`);

// a throw-away case: h8's Mods plus the helper, without the save
const dir = `${ROOT}/cases/_h8-setup`; fs.rmSync(dir, {recursive: true, force: true}); fs.mkdirSync(dir, {recursive: true});
const c = JSON.parse(fs.readFileSync(`${ROOT}/cases/h8/case.json`, 'utf8'));
delete c.runtime; c.id = '_h8-setup'; c.add.push({pkg: HELPER, name: 'PE Eval Scenario Setup', path: mod});
fs.writeFileSync(`${dir}/case.json`, JSON.stringify(c, null, 2));
const env = build(dir, `${ROOT}/run/_h8-setup`);
fs.rmSync(`${env.userdata}/Saves`, {recursive: true, force: true});
const child = spawn(path.join(env.game, 'RimWorldWin64.exe'), [`-savedatafolder=${env.userdata}`, '-logFile', env.log, '-quicktest'], {cwd: env.game, stdio: 'ignore'});
const out = `${env.userdata}/Saves/autostart.rws`; const t0 = Date.now();
while (Date.now() - t0 < 240000 && !(fs.existsSync(out) && child.exitCode !== null)) await new Promise(r => setTimeout(r, 3000));
try { execFileSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], {stdio: 'ignore'}); } catch {}
if (!fs.existsSync(out)) { console.error('no save written; see', env.log); process.exit(1); }

// the shipped save must not know the helper: drop it from the Mod list in the header, or the game warns about a mismatch
let x = fs.readFileSync(out, 'utf8');
const head = x.match(/<modIds>[\s\S]*?<\/modIds>\s*<modSteamIds>[\s\S]*?<\/modSteamIds>\s*<modNames>[\s\S]*?<\/modNames>/);
console.log('header:', head ? head[0].length + ' chars' : 'not found');
if (head) {
  const ids = [...head[0].matchAll(/<modIds>([\s\S]*?)<\/modIds>/g)][0][1].match(/<li>[^<]*<\/li>/g);
  const drop = ids.findIndex(l => l.toLowerCase().includes(HELPER));
  if (drop >= 0) {
    const fix = block => block.replace(/(<(modIds|modSteamIds|modNames)>)([\s\S]*?)(<\/\2>)/g, (m, o, n, body, cl) => { const li = body.match(/<li>[^<]*<\/li>|<li\s*\/>/g) || []; li.splice(drop, 1); return o + '\n    ' + li.join('\n    ') + '\n  ' + cl; });
    x = x.replace(head[0], fix(head[0]));
  }
}
x = x.replace(/[ \t]*<li Class="PeEvalSetup\.PeSetupComponent" \/>\r?\n/, '');   // and its GameComponent entry
if (x.includes('PeEvalSetup')) throw new Error('the helper is still in the save');
fs.mkdirSync(`${ROOT}/cases/h8/save`, {recursive: true}); fs.writeFileSync(`${ROOT}/cases/h8/save/autostart.rws`, x);
fs.copyFileSync(env.log, `${ROOT}/cases/h8/save/setup.Player.log`);
console.log('saved', `${ROOT}/cases/h8/save/autostart.rws`, x.length, 'bytes');
