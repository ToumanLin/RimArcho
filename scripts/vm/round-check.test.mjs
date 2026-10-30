import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { check, expectedPhases, jsonLines, steps } from './round-check.mjs';

const script = 'out C:\\r\nmenu\nnew difficulty=Peaceful\nplay 120\nsave Round 1\nload Round 1\nplay 60\nmenu\nquit\n';
const phase = (p, save) => JSON.stringify({ message: `[Archo Monitor] phase ${p}`, context: save ? { phase: p, save } : { phase: p }, kind: 'phase' });
const goodMonitor = [
  JSON.stringify({ kind: 'session', context: { version: '0.9.1', game: '1.6.4871', skipped: [] } }),
  phase('menu'), phase('new'), phase('play'), phase('save', 'Round 1'), phase('play'), phase('menu'),
  JSON.stringify({ kind: 'exit', context: { entries: 5 } }),
].join('\n');
const goodDrive = [
  { ev: 'start', n: 0, game: '1.6.4871 rev1261', data: 'C:\\data' },
  ...['menu', 'new', 'play', 'save', 'load', 'play', 'menu', 'quit'].map((s, i) => ({ ev: 'step', n: i + 1, step: s, state: 'ok', sec: 10 })),
  { ev: 'note', n: 5, msg: 'dialog Mods mismatch: … -> Load anyway' },
  { ev: 'done', n: 8 },
].map(e => JSON.stringify(e)).join('\n');
const goodPlayer = ['menu', 'new', 'play', 'save', 'play', 'menu'].map(p => `[Archo Monitor] phase ${p}`).join('\n');

function evidence(files) {
  const dir = mkdtempSync(join(tmpdir(), 'round-check-'));
  const all = { 'round.txt': script, 'drive.log': goodDrive, 'monitor.jsonl': goodMonitor, 'player-phases.txt': goodPlayer, 'watch.json': '{"status":"exited"}', ...files };
  for (const [name, text] of Object.entries(all)) if (text !== null) writeFileSync(join(dir, name), text);
  return dir;
}

function run(files) {
  const dir = evidence(files);
  try { return check(dir); } finally { rmSync(dir, { recursive: true, force: true }); }
}

test('steps and expected phases follow the script', () => {
  assert.deepEqual(steps(script).map(s => s.kind), ['menu', 'new', 'play', 'save', 'load', 'play', 'menu', 'quit']);
  assert.equal(steps(script)[3].arg, 'Round 1');
  assert.deepEqual(expectedPhases(script), [{ phase: 'menu' }, { phase: 'new' }, { phase: 'play' }, { phase: 'save', save: 'Round 1' }, { phase: 'play' }, { phase: 'menu' }]);
  // Loading from the main menu right after start: no second "menu".
  assert.deepEqual(expectedPhases('menu\nload A\nquit').map(p => p.phase), ['menu', 'save', 'play']);
});

test('a complete round passes', () => {
  const r = run({});
  assert.equal(r.failures, 0, r.lines.join('\n'));
  assert.ok(r.lines.includes('NOTE step 5: dialog Mods mismatch: … -> Load anyway'));
  assert.ok(r.lines.some(l => l.startsWith('PASS monitor: phases menu > new > play > save(Round 1) > play > menu')));
});

test('a save name with the .rws extension still matches', () => {
  const r = run({ 'monitor.jsonl': goodMonitor.replace('"save":"Round 1"', '"save":"Round 1.rws"') });
  assert.equal(r.failures, 0, r.lines.join('\n'));
});

test('a failed step fails the round and only compares the phases that ran', () => {
  const drive = goodDrive.split('\n').slice(0, 3).concat([
    JSON.stringify({ ev: 'step', n: 3, step: 'play', state: 'fail', msg: 'game time did not move' }),
    JSON.stringify({ ev: 'fail', n: 3, msg: 'game time did not move' }),
  ]).join('\n');
  const mon = goodMonitor.split('\n').slice(0, 4).join('\n');
  const r = run({ 'drive.log': drive, 'monitor.jsonl': mon, 'player-phases.txt': 'phase menu\n[Archo Monitor] phase menu\n[Archo Monitor] phase new\n[Archo Monitor] phase play', 'watch.json': '{"status":"fail","killed":true}' });
  assert.ok(r.lines.includes('FAIL drive: step 3 play (game time did not move)'), r.lines.join('\n'));
  assert.ok(r.lines.some(l => l.startsWith('FAIL drive: all 8 steps done')));
  assert.ok(r.lines.some(l => l.startsWith('FAIL game: quit to the desktop by itself (watch: fail, killed)')));
  assert.ok(r.lines.some(l => l === 'PASS monitor: phases menu > new > play'), r.lines.join('\n'));
  assert.ok(r.lines.includes('PASS player.log: the same phase lines'), r.lines.join('\n'));
});

test('wrong phase order, missing exit and skipped hooks are failures', () => {
  const mon = [
    JSON.stringify({ kind: 'session', context: { skipped: ['Verse.Game.FinalizeInit'] } }),
    phase('menu'), phase('new'), phase('save', 'Other'), phase('play'), phase('menu'),
  ].join('\n');
  const r = run({ 'monitor.jsonl': mon });
  assert.ok(r.lines.includes('FAIL monitor: every hook installed (skipped: Verse.Game.FinalizeInit)'));
  assert.ok(r.lines.some(l => l.startsWith('FAIL monitor: phases menu > new > play > save(Round 1)') && l.includes('has menu > new > save(Other) > play > menu')), r.lines.join('\n'));
  assert.ok(r.lines.some(l => l.startsWith('FAIL monitor: the load names the save "Round 1"')));
  assert.ok(r.lines.includes('FAIL monitor: exit line last (no exit line)'));
});

test('the game must run the Monitor RimArcho bundles, unless kept on purpose', () => {
  const status = 'Archo Monitor\n  bundled: 0.9.1 (C:\\RimArcho\\monitor\\ArchoMonitor)\n  installed: 0.9.1, 0.9.0-dev (C:\\local\\monitor)\n';
  assert.ok(run({ 'monitor.txt': status }).lines.includes('PASS monitor: the bundled Archo Monitor 0.9.1 ran'));
  const old = goodMonitor.replace('"version":"0.9.1"', '"version":"0.9.0-dev"');
  const r = run({ 'monitor.txt': status, 'monitor.jsonl': old });
  assert.ok(r.lines.includes('FAIL monitor: the bundled Archo Monitor ran (the game ran 0.9.0-dev; RimArcho bundles 0.9.1)'), r.lines.join('\n'));
  const kept = run({ 'monitor.txt': 'kept: the installed Monitor (-KeepMonitor)\n' + status, 'monitor.jsonl': old });
  assert.equal(kept.failures, 0, kept.lines.join('\n'));
  assert.ok(kept.lines.some(l => l.startsWith('NOTE monitor: the game ran the installed 0.9.0-dev')));
  // No monitor.txt (an older round's evidence): nothing to compare.
  assert.ok(!run({}).lines.some(l => l.includes('bundled')));
});

test('no drive.log means the driver never loaded', () => {
  const r = run({ 'drive.log': null, 'watch.json': '{"status":"no-driver","killed":true}' });
  assert.ok(r.lines.includes('FAIL drive: started in the game (no drive.log: the game never loaded ArchoDrive.dll)'), r.lines.join('\n'));
});

test('jsonLines keeps lines it cannot parse', () => {
  assert.deepEqual(jsonLines('\uFEFF{"a":1}\n\nnot json\n'), [{ a: 1 }, { ev: 'unparsed', raw: 'not json' }]);
});
