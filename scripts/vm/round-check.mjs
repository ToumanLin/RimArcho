// Checks the evidence of one game round (round.ps1): the driver's steps, Archo Monitor's
// phases in ArchoMonitor.log and Player.log, and how the game ended. Prints PASS / FAIL /
// NOTE lines; exit code = number of failures.
//
//   node scripts/vm/round-check.mjs <evidence dir>
//
// The evidence dir holds round.txt (the script), drive.log, monitor.jsonl (the session,
// phase and exit lines of ArchoMonitor.log), player-phases.txt (Player.log's
// "[Archo Monitor]" lines) and watch.json (how the round ended).
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

export function jsonLines(text) {
  const out = [];
  for (const line of (text || '').replace(/^﻿/, '').split(/\r?\n/)) {
    const t = line.trim();
    if (!t) continue;
    try { out.push(JSON.parse(t)); } catch { out.push({ ev: 'unparsed', raw: t }); }
  }
  return out;
}

// steps reads the round script the way ArchoDrive's parser does (kind and argument only).
export function steps(script) {
  const out = [];
  for (let line of (script || '').split(/\r?\n/)) {
    line = line.replace(/#.*/, '').trim();
    if (!line) continue;
    const [kind, ...rest] = line.split(/\s+/);
    if (kind.toLowerCase() === 'out') continue;
    out.push({ kind: kind.toLowerCase(), arg: rest.filter(w => !w.includes('=')).join(' ') });
  }
  return out;
}

// expectedPhases: what Archo Monitor writes for the script (06 §6). Loading ends at the
// main menu; a new colony is "new" then "play"; loading a save is "save" then "play"; the
// main menu is "menu". The Monitor never repeats a phase, so neither does this.
export function expectedPhases(script) {
  const seq = [{ phase: 'menu' }];
  const push = (phase, save) => { if (seq[seq.length - 1].phase !== phase) seq.push(save ? { phase, save } : { phase }); };
  for (const s of steps(script)) {
    if (s.kind === 'new') { push('new'); push('play'); }
    else if (s.kind === 'load') { push('save', s.arg); push('play'); }
    else if (s.kind === 'menu') push('menu');
  }
  return seq;
}

export function check(dir) {
  const lines = [];
  const pass = (ok, name, why) => lines.push(`${ok ? 'PASS' : 'FAIL'} ${name}${!ok && why ? ` (${why})` : ''}`);
  const note = s => lines.push(`NOTE ${s}`);
  const read = f => existsSync(join(dir, f)) ? readFileSync(join(dir, f), 'utf8').replace(/^﻿/, '') : null;

  const script = read('round.txt');
  const watch = JSON.parse(read('watch.json') || '{}');
  const drive = jsonLines(read('drive.log'));
  const plan = steps(script);
  const quits = plan.length > 0 && plan[plan.length - 1].kind === 'quit';

  // The driver.
  const start = drive.find(e => e.ev === 'start');
  pass(!!start, 'drive: started in the game', drive.length ? 'no start line' : 'no drive.log: the game never loaded ArchoDrive.dll');
  if (start) note(`game ${start.game}; data ${start.data}`);
  for (const e of drive) {
    if (e.ev === 'step' && e.state === 'ok') pass(true, `drive: step ${e.n} ${e.step} ${Math.round(e.sec)} s${e.note ? ` — ${e.note}` : ''}`);
    else if (e.ev === 'step' && e.state === 'fail') pass(false, `drive: step ${e.n} ${e.step}`, e.msg);
    else if (e.ev === 'fail' && !drive.some(x => x.ev === 'step' && x.state === 'fail')) pass(false, 'drive', e.msg);
    else if (e.ev === 'note' && /^(dialog|closed) /.test(e.msg)) note(`step ${e.n}: ${e.msg}`);
  }
  const done = drive.some(e => e.ev === 'done');
  const okSteps = drive.filter(e => e.ev === 'step' && e.state === 'ok').length;
  pass(done && okSteps === plan.length, `drive: all ${plan.length} steps done`, `${okSteps} done; watch: ${watch.status || 'unknown'}`);

  // How the game ended.
  if (quits) pass(watch.status === 'exited', 'game: quit to the desktop by itself', `watch: ${watch.status || 'unknown'}${watch.killed ? ', killed' : ''}`);
  else note(`game left running (the script has no quit): watch ${watch.status}`);

  // Archo Monitor.
  const mon = jsonLines(read('monitor.jsonl'));
  const session = mon.find(e => e.kind === 'session');
  pass(!!session, 'monitor: session line', 'no session line in ArchoMonitor.log');
  if (session?.context?.skipped?.length) pass(false, 'monitor: every hook installed', `skipped: ${session.context.skipped.join(', ')}`);
  else if (session) pass(true, 'monitor: every hook installed');
  if (session) note(`monitor ${session.context?.version}, game ${session.context?.game}`);
  // The Monitor this RimArcho bundles is the one under test, unless round.ps1 -KeepMonitor.
  const status = read('monitor.txt');
  const bundled = /^\s*bundled:\s*(\S+)\s+\(/m.exec(status || '')?.[1];
  if (session && bundled) {
    const ran = session.context?.version;
    if (ran === bundled) pass(true, `monitor: the bundled Archo Monitor ${bundled} ran`);
    else if (/^kept:/m.test(status)) note(`monitor: the game ran the installed ${ran}; RimArcho bundles ${bundled} (-KeepMonitor)`);
    else pass(false, 'monitor: the bundled Archo Monitor ran', `the game ran ${ran}; RimArcho bundles ${bundled}`);
  }
  const want = expectedPhases(script);
  if (!done) {
    // Phases are compared only for the steps that ran.
    const ran = plan.slice(0, okSteps).map(s => s.kind + (s.arg ? ' ' + s.arg : '')).join('\n');
    want.splice(0, want.length, ...expectedPhases(ran));
  }
  const got = mon.filter(e => e.kind === 'phase').map(e => ({ phase: e.context?.phase, save: e.context?.save }));
  const show = seq => seq.map(p => p.phase + (p.save ? `(${p.save})` : '')).join(' > ') || '(none)';
  const same = got.length === want.length && got.every((p, i) => p.phase === want[i].phase);
  pass(same, `monitor: phases ${show(want)}`, `ArchoMonitor.log has ${show(got)}`);
  for (const w of want.filter(p => p.save)) {
    const g = got.find(p => p.phase === 'save' && saveName(p.save) === w.save);
    pass(!!g, `monitor: the load names the save "${w.save}"`, `save phases: ${got.filter(p => p.phase === 'save').map(p => p.save).join(', ') || 'none'}`);
  }
  const exit = mon.find(e => e.kind === 'exit');
  if (quits && done) pass(!!exit && mon.indexOf(exit) === mon.length - 1, 'monitor: exit line last', exit ? 'lines after the exit line' : 'no exit line');
  if (exit) note(`monitor exit: ${exit.context?.entries} entries`);

  // Player.log carries the same phase lines (19 §4).
  const pl = (read('player-phases.txt') || '').split(/\r?\n/).map(l => /\[Archo Monitor\] phase (\w+)/.exec(l)?.[1]).filter(Boolean);
  const plSame = pl.length === want.length && pl.every((p, i) => p === want[i].phase);
  pass(plSame, 'player.log: the same phase lines', `Player.log has ${pl.join(' > ') || '(none)'}`);

  return { lines, failures: lines.filter(l => l.startsWith('FAIL')).length };
}

function saveName(s) {
  return (s || '').replace(/\.rws$/i, '');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const dir = process.argv[2];
  if (!dir) { console.error('usage: round-check.mjs <evidence dir>'); process.exit(2); }
  const r = check(dir);
  for (const l of r.lines) console.log(l);
  console.log(`${r.failures ? 'FAIL' : 'PASS'} round: ${r.failures} failed`);
  process.exit(r.failures);
}
