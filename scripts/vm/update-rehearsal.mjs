// The window part of the update rehearsal (17 §5, update-rehearsal.ps1): in the candidate's
// window, settings › "check for updates", then "install update" and its confirmation, as a
// player clicks them (trusted mouse events at each control's center). Then it follows the
// self-update task until the window goes away, waits for the restarted window and reads
// its version. Read-only calls (Settings, Tasks) observe; only the clicks act.
//
//   node scripts/vm/update-rehearsal.mjs <ip>:9223 <candidate version> <rehearsal version>
//
// Prints PASS / FAIL / NOTE lines and a last line "RESULT {json}"; exit code = failures.
import { connect, sleep } from './cdp.mjs';

const SVC = 'github.com/ToumanLin/RimArcho/internal/surface/gui.API.';
const goCall = (c, method, ...args) =>
  c.evaluate(`import('/wails/runtime.js').then(m => m.Call.ByName(${JSON.stringify(SVC + method)}, ...${JSON.stringify(args)}))`);

// version is the settings row "RimArcho" (view.SelfUpdateItem): the running program's version.
async function version(c) {
  const groups = await goCall(c, 'Settings');
  for (const g of groups || []) for (const it of g.items || []) if (it.key === 'archo.version') return it.value;
  return '';
}

async function until(what, ms, f, every = 500) {
  const end = Date.now() + ms;
  let last;
  while (Date.now() < end) {
    try { last = await f(); if (last) return last; } catch (e) { last = undefined; }
    await sleep(every);
  }
  throw Error(`${what}: not after ${Math.round(ms / 1000)} s`);
}

// A window whose page answers: DevTools up, the app's Go service reachable.
async function attach(addr, ms) {
  return until('a window answering on ' + addr, ms, async () => {
    let c;
    try {
      c = await connect(addr, { timeoutMs: 5000 });
      const v = await version(c);
      if (v) return { c, v };
    } catch { }
    c?.close();
    return null;
  }, 1000);
}

const [addr, from, to] = process.argv.slice(2);
if (!addr || !from || !to) { console.error('usage: update-rehearsal.mjs <ip>:<port> <candidate version> <rehearsal version>'); process.exit(2); }
let fails = 0;
const result = { from, to };
const say = (ok, name, why = '') => { if (!ok) fails++; console.log(`${ok ? 'PASS' : 'FAIL'} window: ${name}${!ok && why ? ` (${why})` : ''}`); };

try {
  const { c, v } = await attach(addr, 90000);
  result.before = v;
  say(v === from, `the candidate's window runs ${from}`, `it runs ${v}`);
  if (v !== from) throw Error('wrong window');
  await c.click('#nav-settings');
  await until('settings page', 15000, () => c.evaluate(`!!document.querySelector('[data-setting-action="update --check"], [data-setting-action="update"]')`));
  // check for updates (the test release service answers; the Monitor item and Lanzou too)
  if (!(await c.evaluate(`!!document.querySelector('[data-setting-action="update"]')`))) {
    await c.click('[data-setting-action="update --check"]');
    await until('"install update" after the check', 90000, async () => {
      if (await c.evaluate(`!!document.querySelector('[data-setting-action="update"]')`)) return true;
      // the row is redrawn when the check task ends; reopen settings if it was not
      const t = await goCall(c, 'Tasks');
      const row = (t?.rows || []).find(r => r.kind === 'self-check');
      if (row && row.state !== 'run' && row.state !== 'queue') {
        await c.click('#nav-settings').catch(() => {});
      }
      return false;
    }, 1500);
  }
  const line = await c.evaluate(`document.querySelector('[data-setting-action="update"]')?.closest('[data-setting-row]')?.innerText || ''`);
  say(line.includes(to), `settings offers ${to}`, line.replace(/\s+/g, ' ').slice(0, 160));
  await c.click('[data-setting-action="update"]');
  await until('the confirmation', 10000, () => c.evaluate(`!!document.querySelector('.btns .go')`));
  const started = Date.now();
  await c.click('.btns .go');
  say(true, 'install update confirmed');
  // Follow the task until the window closes (it restarts) or the task ends without it.
  let task = null, gone = false;
  while (Date.now() - started < 300000) {
    try {
      const t = await goCall(c, 'Tasks');
      const row = (t?.rows || []).find(r => r.kind === 'self-update');
      if (row) {
        if (!task || row.state !== task.state || row.line !== task.line) console.log(`NOTE window: task ${row.state}: ${row.line || ''}${row.say ? ' | ' + row.say : ''}`);
        task = { state: row.state, line: row.line, say: row.say, dur: row.dur };
      }
      if (task && !['run', 'queue'].includes(task.state) && Date.now() - started > 5000) {
        // ended: a restart closes the window within a few seconds; a failure leaves it open
        await sleep(5000);
        await goCall(c, 'Tasks');
        break;
      }
    } catch (e) { gone = true; break; }
    await sleep(500);
  }
  if (!gone) { try { await sleep(3000); await goCall(c, 'Tasks'); } catch { gone = true; } }
  c.close();
  result.task = task;
  const text = `${task?.line || ''} ${task?.say || ''}`;
  say(!!task && /restarting|正在重启/.test(text) && text.includes(to), 'the task ends in "restarting"', task ? `${task.state}: ${text.trim()}` : 'no self-update task seen');
  say(gone, 'the old window closed', 'still answering');
  // The restarted window: same DevTools port (the environment is passed on).
  try {
    const n = await attach(addr, 90000);
    result.after = n.v;
    say(n.v === to, `the new window reports ${to}`, `it reports ${n.v}`);
    n.c.close();
  } catch (e) { say(false, `the new window reports ${to}`, e.message); }
} catch (e) {
  say(false, 'driving the window', e.message);
}
console.log('RESULT ' + JSON.stringify(result));
process.exit(fails);
