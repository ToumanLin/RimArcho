// Can it run in the GUI (06 §2.4, §5; 06 §10 G29): the two columns, an item's buttons, the
// confirmation asked in the item and the done lines an action leaves.
import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const js = readFileSync(new URL('../../internal/surface/gui/assets/js/screens.js', import.meta.url), 'utf8');
const part = (start, end) => js.slice(js.indexOf(start), js.indexOf(end, js.indexOf(start)));
function el(tag, cls = '', text = '') {
  const node = { tag, className: cls, textContent: text, children: [], dataset: {},
    append(...children) { this.children.push(...children); },
    prepend(...children) { this.children.unshift(...children); },
  };
  node.classList = { add(...names) { node.className += ' ' + names.join(' '); }, toggle() {}, remove() {} };
  Object.defineProperty(node, 'innerHTML', { set() { node.children = []; } });
  return node;
}
// the context's arrays are another realm's: compare them as plain data
const plain = (x) => JSON.parse(JSON.stringify(x));
const all = (node) => [node, ...node.children.flatMap(all)];
const buttons = (node) => all(node).filter((n) => n.tag === 'button');
const freshN = () => ({ data: null, cols: [[], []], cur: [0, 0], at: ['', ''], col: 0, open: new Set(), els: [[], []],
  hintsShut: false, done: [], batch: 0, before: null, ready: false, ask: null, others: false, env: '' });

function cardContext(calls) {
  const context = createContext({ el, N: freshN(), T: (s) => s, Tf: (s, ...a) => s.replace(/%[ds]/g, () => a.shift()),
    archo: {}, statusIcon: () => '', needText: (tag, cls, text) => el(tag, cls, text), needParts: () => el('ul'),
    needEvidence: () => el('div', 'ev-line'), linkBlock: () => el('div'), jumpTo: (id) => calls.push(['jump', id]),
    setCur() {}, snapshot: () => calls.push(['snapshot']), doAction: (a, o) => calls.push(['do', a.kind, o]),
    runAction: (it, a, here) => calls.push(['run', a.kind, here]), actionWrite: (a) => a.kind,
    confirmYes: () => calls.push(['yes']), drawNeeds: () => calls.push(['draw']),
    button: (label, key, run, cls = '', write) => { const b = el('button', `btn ${cls}`, label); b.onclick = run; b.key = key; if (write) b.dataset.envWrite = write; return b; },
  });
  runInContext(part('function needCard(', '// markNeeds moves') + part('function confirmRow(', 'function confirmYes(') +
    '\nglobalThis.needCard = needCard;', context);
  return context;
}

const choice = { id: 'n1', kind: 'mod.incompatible', text: 'A and B cannot run together', section: 0, status: 1,
  evidence: { kind: 'declared', label: 'declared by the mod' },
  actions: [{ kind: 'choose-disable', key: 'space', label: 'disable one', options: [{ label: 'disable A' }, { label: 'disable B' }] },
    { kind: 'problem', key: 'enter', label: 'details' }],
  related: [{ id: 'r1', text: 'in the last run: A — 3 errors' }] };

test('every action is a button at the foot, one per option of a choice, none behind ⋯', () => {
  const calls = [];
  const row = cardContext(calls).needCard(choice, 0, 0);
  const foot = row.children.find((n) => n.className === 'need-foot');
  assert.ok(foot, 'the buttons sit in the foot');
  assert.ok(foot.children.some((n) => n.className === 'ev-line'), 'the evidence shares the foot');
  const bs = buttons(foot);
  assert.deepEqual(bs.map((b) => b.textContent), ['disable A', 'disable B', 'details']);
  assert.ok(!bs.some((b) => /more/.test(b.className)));
  assert.ok(bs.slice(0, 2).every((b) => b.className.includes('option') && !b.className.includes('primary')), 'no option is the default');
  bs[1].onclick();
  assert.deepEqual(calls.slice(-2), [['snapshot'], ['do', 'choose-disable', 1]]);
  bs[2].onclick();
  assert.deepEqual(calls.at(-1), ['run', 'problem', true]);
  // the other column's item is a link
  const rel = buttons(row).find((b) => b.className === 'need-rel');
  assert.equal(rel.textContent, '→ in the last run: A — 3 errors');
  rel.onclick({ stopPropagation() {} });
  assert.deepEqual(calls.at(-1), ['jump', 'r1']);
});

test('a confirmation is asked in the item, in place of its buttons', () => {
  const calls = [];
  const context = cardContext(calls);
  const remove = { kind: 'textures-clean', key: 'C', label: 'remove the converted textures', confirm: 'press C again', warn: 'the DDS are deleted' };
  const it = { id: 'n2', kind: 'texture.stale', text: 'X changed', section: 0, actions: [remove] };
  context.N.ask = { id: 'n2', act: remove };
  const row = context.needCard(it, 0, 0);
  assert.ok(!row.children.some((n) => n.className === 'need-foot'));
  const box = row.children.find((n) => n.className === 'need-confirm');
  assert.equal(box.children[0].textContent, 'the DDS are deleted');
  const [yes, no] = buttons(box);
  assert.equal(yes.textContent, remove.label); assert.ok(yes.className.includes('primary')); assert.equal(yes.key, 'enter');
  assert.equal(no.key, 'esc');
  yes.onclick(); assert.deepEqual(calls.at(-1), ['yes']);
  no.onclick(); assert.equal(context.N.ask, null);
  // another item's card keeps its buttons
  assert.ok(context.needCard({ ...it, id: 'n3' }, 0, 1).children.some((n) => n.className === 'need-foot'));
});

function layoutContext() {
  // the status line: what a sort said, with its undo link
  const status = { childNodes: [{ nodeName: '#text', textContent: 'sorted, 2 moved ' }, { nodeName: 'A', textContent: 'undo' }],
    querySelector: (s) => (s === 'a' ? {} : null) };
  const context = createContext({ N: freshN(), S: { live: null, board: { env: 'e' } }, T: (s) => s,
    $: (s) => (s === '#status' ? status : s === '#screen' ? { dataset: {} } : null) });
  context.status = status;
  runInContext(part('function orderNeeds(', 'export async function needs(') +
    part('function snapshot(', 'async function undoDone(') +
    part('export async function needs(', '// snapshot:').replace('export ', '') +
    '\nObject.assign(globalThis, { orderNeeds, snapshot, actionDone, settleDone, undoBatch, needs });', context);
  return context;
}

test('the left column holds the three sections, the right the run; merged and other pages\' items are left out', () => {
  const c = layoutContext();
  c.N.data = { items: [{ id: 'r', section: 3 }, { id: 'h', section: 1, status: -1 }, { id: 'x', section: 0 }, { id: 'q', section: 2 },
    { id: 'm', section: 0, into: 'x' }, { id: 'e', section: 4 }] };
  c.orderNeeds();
  assert.deepEqual(plain(c.N.cols.map((col) => col.map((x) => x.id))), [['x', 'h', 'q'], ['r']]);
  c.N.hintsShut = true;
  c.orderNeeds();
  assert.deepEqual(plain(c.N.cols[0].map((x) => x.id)), ['x', 'q'], 'a shut Things to watch keeps only its heading');
});

test('what an action resolves stays as a done line where it was, with undo on the latest batch', () => {
  const c = layoutContext();
  c.N.data = { items: [
    { id: 'a', kind: 'dep.missing', group: 1, section: 0, text: 'Lib is missing' },
    { id: 'b', kind: 'order.violated', group: 0, section: 0, title: '7 mods are out of order' },
    { id: 'c', kind: 'dep.inactive', group: 1, section: 0, text: 'Core is not enabled' },
    { id: 'r', kind: 'log', group: 7, section: 3, text: 'A — 3 errors' }] };
  c.N.env = 'e'; // the page was read for this game
  c.orderNeeds();
  c.snapshot();
  // the action is still running: a refresh in between leaves the snapshot alone
  const after = { items: [{ id: 'b2', kind: 'order.violated', group: 0, section: 0, title: '6 mods are out of order' },
    { id: 'c', kind: 'dep.inactive', group: 1, section: 0, text: 'Core is not enabled' }, { id: 'r', kind: 'log', group: 7, section: 3 }] };
  c.api = { Needs: async () => after };
  c.beginPageRead = () => ({ fail() {}, current: () => true, finish() {} });
  c.drawNeeds = () => {};
  return c.needs(true).then(async () => {
    assert.equal(c.N.done.length, 0);
    assert.ok(c.N.before, 'still waiting for the action');
    // the action said what it did; a background check then put its own words in the
    // status line: the done line keeps the action's
    c.actionDone({ text: 'enabled Lib', undo: true });
    c.status.childNodes = [{ nodeName: '#text', textContent: 'analysis finished' }];
    c.status.querySelector = () => null;
    await c.needs(true);
    // a vanished; b became b2 (same kind: changed, not done); c is still there
    assert.deepEqual(plain(c.N.done.map((d) => [d.id, d.text, d.title, d.undo])), [['a', 'enabled Lib', 'Lib is missing', true]]);
    assert.equal(c.undoBatch(), c.N.done[0].batch);
    assert.deepEqual(plain(c.N.cols[0].map((x) => x.id)), ['a', 'b2', 'c'], 'the done line keeps its place');
    // leaving the page clears them
    await c.needs(false);
    assert.equal(c.N.done.length, 0);
  });
});

test('a sort, which says nothing back, takes the status line as it is when the sort ends', () => {
  const c = layoutContext();
  c.N.data = { items: [{ id: 'b', kind: 'order.violated', group: 0, section: 0, title: '7 mods are out of order' }] };
  c.orderNeeds();
  c.snapshot();
  c.actionDone();
  c.status.childNodes = [{ nodeName: '#text', textContent: 'analysis finished' }];
  c.settleDone({ items: [] });
  assert.deepEqual(plain(c.N.done.map((d) => [d.id, d.text, d.undo])), [['b', 'sorted, 2 moved', true]]);
  assert.equal(c.N.before, null);
  assert.equal(c.N.ready, false);
});

test('a sort that makes the last run stale leaves no done lines on its cards', () => {
  // VM round 2026-11-05: Sort put "sorted, …" with undo on all 13 last-run cards.
  const c = layoutContext();
  c.N.data = { items: [{ id: 'b', kind: 'order.violated', group: 0, section: 0, title: '7 mods are out of order' },
    { id: 'log1', group: 3, section: 3, title: 'Furniture — 12 errors' }, { id: 'log2', group: 3, section: 3, title: 'Other — 1 error' }] };
  c.orderNeeds();
  c.snapshot();
  c.actionDone();
  c.settleDone({ items: [] });
  assert.deepEqual(plain(c.N.done.map((d) => d.id)), ['b']);
});
