// Grades one run: culprit from answer.json (or the explanation), fix by starting the game with the submitted files layered over the case.
// usage: node grade.mjs <resultDir>     (D:/RimArcho-test/mcp-eval/results/<case>-<group>-<run>)
//        GRADE_REUSE_LOG=1 reuses <resultDir>/graded.Player.log instead of starting the game (re-scoring only).
import fs from 'fs'; import path from 'path'; import {execFileSync} from 'child_process';
import {build, run} from './sandbox.mjs';
import {judge} from './logdiff.mjs';

const ROOT = 'D:/RimArcho-test/mcp-eval';
// A clean log is not enough: disabling the broken Mod, or patching around it elsewhere, also clears the log.
//   keepEnabled: Mods that must stay in the active list; touch: the submission must change files under one of these prefixes
//   (matched case-insensitively against `<packageId>/...`, delete.json entries included); `ModsConfig.xml` alone counts
//   where the list is the fix. touchAll: every prefix must be changed (one fault per prefix).
const RULES = {
  c1: {keepEnabled: ['khamenman.OutfitStandsPLus'], touch: ['khamenman.OutfitStandsPLus/']},
  c2: {keepEnabled: ['khamenman.OutfitStandsPLus'], touch: ['khamenman.OutfitStandsPLus/', 'odysseypatch.1trickPwnyta/', 'ModsConfig.xml']},
  c3: {keepEnabled: ['DarkDaskin.SmartFirefight'], touch: ['DarkDaskin.SmartFirefight/1.6/Assemblies/']},   // the fault is in the DLL: it must be rebuilt
  c4: {keepEnabled: ['OskarPotocki.VanillaFactionsExpanded.Core'], touch: ['ModsConfig.xml']},
  c5: {keepEnabled: ['mlie.10xcarryingcapacity'], touch: ['mlie.10xcarryingcapacity/']},                       // patching Outfit Stands Plus would hide the symptom only
  h1: {keepEnabled: ['vanillaexpanded.vfecore'], touch: ['vanillaexpanded.vfecore/']},
  // the stale DLL can go (delete.json), be replaced, or lose the race (Outfit Stands Plus listed first)
  h2: {keepEnabled: ['khamenman.OutfitStandsPLus', 'odysseypatch.1trickPwnyta'], touch: ['odysseypatch.1trickPwnyta/1.6/Assemblies/', 'ModsConfig.xml']},
  // touchAll: each fault is fixed where it is
  h5: {keepEnabled: ['khamenman.OutfitStandsPLus', 'mlie.10xcarryingcapacity'], touch: [], touchAll: ['khamenman.OutfitStandsPLus/', 'mlie.10xcarryingcapacity/']},
  // the caller is the fault: its DLL must be rebuilt; a null check in Cyanobot's Genes' postfix hides it
  h4: {keepEnabled: ['mossbrook.colonistmutterings', 'cyanobot.CyanobotsGenes'], touch: ['mossbrook.colonistmutterings/1.6/Assemblies/']},
  // h8: the missing check is in PE's own assembly: it must be rebuilt (Socio Butterfly stays on)
  h8: {keepEnabled: ['ferny.ProgressionEducation', 'LovelyDovey.Recreation.WithEuterpe'], touch: ['ferny.ProgressionEducation/1.6/Assemblies/']},
  // h9: no fault; the right answer changes nothing
  h9: {control: true},
  // the comp's own code is the fault: SSR's DLL must be rebuilt; a for loop in the profiler, or the profiler off, hides it
  h7: {keepEnabled: ['ssr.body', 'tallowbyte.pawncompprofiler'], touch: ['ssr.body/1.6/Assemblies/']},
  // Charmweavers' transpiler is the fault: its DLL must change (rebuilt, or a patch DLL of its own beside it); HAR's
  // transpiler is fine on vanilla IL, so changing HAR, or turning either Mod off, only hides it
  h3: {keepEnabled: ['ReSplice.XOTR.Charmweavers', 'erdelf.HumanoidAlienRaces'], touch: ['ReSplice.XOTR.Charmweavers/1.6/Assemblies/']},
  // the retexture Mod's patch removes the beards: it must change (hide them instead, or drop the operation)
  h6: {keepEnabled: ['hourai.vanillahairsretex.1.6'], touch: ['hourai.vanillahairsretex.1.6/']},
};
const resDir = path.resolve(process.argv[2]); const name = path.basename(resDir); const caseId = name.split('-')[0];
const caseDir = `${ROOT}/cases/${caseId}`; const key = JSON.parse(fs.readFileSync(`${caseDir}/answer.json`, 'utf8'));
const layer = `${ROOT}/graded/${name}/layer`; fs.rmSync(`${ROOT}/graded/${name}`, {recursive: true, force: true}); fs.mkdirSync(layer, {recursive: true});
const result = {run: name, culprit: null, fix: null};

if (fs.existsSync(`${resDir}/submit.zip`)) execFileSync('C:/Windows/System32/tar.exe', ['-xf', `${resDir}/submit.zip`, '-C', layer]);
// a BOM is common (Windows PowerShell 5 writes UTF-8 with one) and JSON.parse rejects it
const readJson = f => JSON.parse(fs.readFileSync(f, 'utf8').trimStart());
let answer = null; try { answer = readJson(`${layer}/answer.json`); fs.rmSync(`${layer}/answer.json`); } catch {}
const named = new Set([].concat(answer?.culprit || []).map(p => String(p).toLowerCase()));
result.culpritNamed = [...named];
result.culprit = key.required.every(p => named.has(p.toLowerCase()));
result.culpritVia = result.culprit ? 'list' : null;
result.decoyNamedOnly = !result.culprit && (key.decoy || []).some(d => named.has(d.pkg.toLowerCase()));
if (!result.culprit && !(key.decoy || []).some(d => named.has(d.pkg.toLowerCase()))) {
  // the list can be wrong while the explanation is right (a dependent Mod listed, the framework explained): accept the text, flagged for a human look
  const text = String(answer?.explanation || '').toLowerCase();
  const names = key.required.map(p => [p.toLowerCase(), (key.culprit.find(c => c.pkg === p)?.name || p).toLowerCase()]);
  if (names.every(n => n.some(s => text.includes(s)))) { result.culprit = true; result.culpritVia = 'explanation'; }
}
result.submitted = fs.readdirSync(layer, {recursive: true}).filter(f => fs.statSync(path.join(layer, f)).isFile());

if (RULES[caseId]?.control) {
  // no-fault control: correct = nothing named and nothing changed. The answer file is still required: an empty hand-in is a miss, not a pass.
  const deletedC = fs.existsSync(`${layer}/delete.json`) ? [].concat(readJson(`${layer}/delete.json`)).filter(Boolean) : [];
  result.control = true; result.answered = answer !== null;
  result.changed = result.submitted.length > 0 || deletedC.length > 0;
  result.falsePositive = result.changed || named.size > 0;
  result.culprit = result.answered && named.size === 0;
  result.fix = !result.changed;
  result.fixNote = result.changed ? 'false positive: files were changed in a case with no fault' : (result.answered ? undefined : 'no answer.json');
  delete result.culpritVia; delete result.decoyNamedOnly;
  fs.writeFileSync(`${resDir}/grade.json`, JSON.stringify(result, null, 2)); console.log(JSON.stringify(result, null, 2));
  process.exit(0);
}
if (!result.submitted.length) { result.fix = false; result.fixNote = 'nothing submitted'; }
else {
  const runDir = `${ROOT}/run/${name}`; let log, reached = true;
  if (process.env.GRADE_REUSE_LOG && fs.existsSync(`${resDir}/graded.Player.log`)) log = `${resDir}/graded.Player.log`;
  else {
    const env = build(caseDir, runDir, layer); await run(env, env.ticks ? 600 : 420); log = env.log; fs.copyFileSync(log, `${resDir}/graded.Player.log`);
    // a runtime case is judged only on a map that ran its ticks: a game that never got there has no tick errors either
    if (env.ticks) { reached = env.reached; result.ticksReached = reached; }
  }
  const j = judge(caseDir, log);
  result.fixClean = j.stillBroken.length === 0 && reached; result.newErrors = j.stillBroken.slice(0, 8);
  if (!reached) result.fixNote = 'the test map did not reach its tick count (no autosave)';
  const rule = RULES[caseId] || {keepEnabled: [], touch: []};
  // a deleted file counts as a change at its path
  const deleted = fs.existsSync(`${layer}/delete.json`) ? [].concat(readJson(`${layer}/delete.json`)).map(String) : [];
  const sub = [...result.submitted, ...deleted].map(f => f.replaceAll('\\', '/').toLowerCase());
  const cfg = sub.includes('modsconfig.xml') ? fs.readFileSync(`${layer}/ModsConfig.xml`, 'utf8').toLowerCase() : null;
  const stillOn = rule.keepEnabled.every(p => cfg === null || cfg.includes(`<li>${p.toLowerCase()}</li>`));
  const hit = t => sub.some(f => f.startsWith(t.toLowerCase()));
  const touched = (rule.touch.length === 0 || rule.touch.some(hit)) && (rule.touchAll || []).every(hit);
  result.fix = result.fixClean && stillOn && touched;
  if (result.fixClean && !result.fix) result.fixNote = !stillOn ? 'log is clean only because the broken Mod was disabled' : 'log is clean but the fault was not fixed where it is (workaround elsewhere)';
}
fs.writeFileSync(`${resDir}/grade.json`, JSON.stringify(result, null, 2)); console.log(JSON.stringify(result, null, 2));
