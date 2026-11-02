import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const js = name => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
function element(tag, cls, text) {
  return { tag, cls, textContent: text, children: [], dataset: {}, classList: { add() {} },
    setAttribute() {}, append(...nodes) { this.children.push(...nodes); }, addEventListener() {},
    replaceChildren(...nodes) { this.textContent = ''; this.children = nodes; },
    querySelectorAll(selector) { return this.children.flatMap(c => [...(c.tag === selector ? [c] : []), ...(c.querySelectorAll?.(selector) || [])]); },
    scrollIntoView() {}, scrollTop: 0, clientHeight: 400, scrollHeight: 20000 };
}

test('background problem updates use the individual reasons from Go', () => {
  const S = { board: { problems: { environment: 'game', sequence: 1 } }, shell: { plan: { id: 7 } } };
  const context = createContext({ S, el: element, keyCaps: (k) => element("kbd", "", k) });
  runInContext(js('problems') + '\nglobalThis.applyEvent = event;', context);
  const reasons = { a: [{ text: 'does not list 1.7 (lists 1.6)', actions: [{ kind: 'version-mute' }] }] };
  const payload = { environment: 'game', plan: 7, sequence: 2, count: 1, status: { a: 2 }, reasons,
    items: [{ counted: true, fold: true, mods: ['a'], text: '365 mods do not list 1.7', actions: [{ kind: 'version-list' }] }] };
  assert.equal(context.applyEvent(payload), true);
  assert.equal(S.board.reasons, reasons);
  assert.equal(S.board.reasons.a[0].actions[0].kind, 'version-mute');
  assert.equal(context.applyEvent({ ...payload, sequence: 1 }), false);
  assert.equal(context.applyEvent({ ...payload, environment: 'other' }), false);
  assert.equal(context.applyEvent({ ...payload, plan: 8 }), false);
  assert.equal(context.applyEvent({ ...payload, sequence: 3, count: 0, status: {}, reasons: {} }), true);
  assert.deepEqual(S.board.status, {});
  assert.deepEqual(S.board.reasons, {});
});

function panelHarness() {
  const S = { envSeq: 1, screenSeq: 1, shell: { plan: { id: 7 } } }, document = {};
  let box, confirmation;
  const pop = {
    open(kind, title, build) { box = el('div'); S.popup = { kind, title }; build(box, S.popup); },
    close() { S.popup = null; },
    confirm(title, lines, label, go) { S.popup = { kind: 'confirm' }; confirmation = { title, lines, label, go }; },
  };
  function el(...args) {
    const node = element(...args);
    node.focus = () => { document.activeElement = node; };
    node.click = () => { if (!node.disabled) return node.onclick?.(); };
    return node;
  }
  const ignored = new Set(), calls = [], shown = [], reads = [];
  const rows = Array.from({ length: 365 }, (_, i) => ({ id: `mod-${i}`, name: `Mod ${i}`, supported: i < 352 ? 'lists 1.6' : 'lists 1.5' }));
  const action = (refs, undo, batch, label) => ({ kind: undo ? 'version-unmute' : 'version-mute', refs,
    label, confirm: batch && !undo ? 'confirm' : '', warn: 'Every list and game; reversible.' });
  function data(undo) {
    const groups = [rows.slice(0, 352), rows.slice(352)].map((members, i) => {
      members = members.filter(row => ignored.has(row.id) === undo);
      return { title: `${i ? 'only lists 1.5 or earlier' : 'lists 1.6'}  ${members.length}`,
        action: action(members.map(r => r.id), undo, true, undo ? 'restore group' : 'ignore group'),
        rows: members.map(row => ({ ...row, action: action([row.id], undo, false, undo ? 'restore' : 'ignore') })) };
    }).filter(g => g.rows.length);
    return { title: undo ? `${ignored.size} mods' 1.7 warnings are ignored` : 'does not list 1.7 as supported', groups,
      all: action(groups.flatMap(g => g.rows.map(r => r.id)), undo, true, undo ? 'restore all' : 'ignore all') };
  }
  const api = { async VersionWarnings(mm, undo) { reads.push([mm, undo]); return data(undo); } };
  const context = createContext({ el, keyCaps: (k) => el("kbd", "", k), S, document, pop, api, T: s => s, errText: String });
  runInContext(js('versionpanel') + '\nglobalThis.openList=openVersionList;', context);
  const run = async (_, a) => { calls.push(a); for (const id of a.refs) { if (a.kind === 'version-unmute') ignored.delete(id); else ignored.add(id); } };
  const open = undo => context.openList({ title: 'version list', gameVersion: '1.7', versionMuted: undo }, run, id => shown.push(id));
  return { S, document, context, api, open, calls, reads, shown, ignored, get box() { return box; }, get confirmation() { return confirmation; } };
}

test('365 version rows load only on opening, stay in one scroll body, and names navigate', async () => {
  const h = panelHarness();
  assert.equal(h.reads.length, 0);
  await h.open(false);
  assert.deepEqual(h.reads, [['1.7', false]]);
  const [head, body] = h.box.children;
  assert.equal(head.children[0].textContent, 'does not list 1.7 as supported');
  assert.equal(body.cls, 'body');
  const rows = body.children.filter(n => n.cls === 'version-row');
  assert.equal(rows.length, 365);
  assert.equal(body.children.filter(n => n.cls === 'version-group').length, 2);
  for (const row of rows) {
    assert.equal(row.children.length, 3);
    assert.match(row.children[1].textContent, /^lists 1\.[56]$/);
    assert.equal(row.children[2].textContent, 'ignore');
  }
  rows[364].children[0].onclick();
  assert.deepEqual(h.shown, ['mod-364']);
  assert.equal(h.S.popup, null);
  const css = readFileSync(new URL('../../internal/surface/gui/assets/style.css', import.meta.url), 'utf8');
  assert.match(css, /\.pop \.body[^}]*max-height:\s*60vh[^}]*overflow-y:\s*auto/);
  assert.match(css, /\.version-panel \.version-row[^}]*grid-template-columns:/);
});

test('single ignore refreshes in place; group and all ignore confirm; restore all is reversible', async () => {
  const h = panelHarness();
  await h.open(false);
  let body = h.box.children[1];
  body.scrollTop = 120;
  await body.children.find(n => n.cls === 'version-row').children[2].onclick();
  assert.equal(h.calls.length, 1);
  assert.deepEqual([...h.calls[0].refs], ['mod-0']);
  assert.equal(h.box.children[1].scrollTop, 120);
  const group = h.box.children[1].children.find(n => n.cls === 'version-group');
  group.children[1].onclick();
  assert.equal(h.S.popup.kind, 'confirm');
  assert.equal(h.calls.length, 1);
  assert.match(h.confirmation.lines[0], /Every list and game/);
  await h.confirmation.go();
  assert.equal(h.ignored.size, 352);
  assert.equal(h.calls[1].refs.length, 351);
  h.box.children[0].children[1].onclick();
  assert.equal(h.calls.length, 2);
  h.S.popup = null; // cancel confirmation: no write
  assert.equal(h.ignored.size, 352);
  await h.open(false);
  h.box.children[0].children[1].onclick();
  await h.confirmation.go();
  assert.equal(h.ignored.size, 365);
  await h.open(true);
  assert.equal(h.box.children[0].children[1].textContent, 'restore all');
  const restore = h.box.children[1].children.find(n => n.cls === 'version-row').children[2];
  assert.equal(restore.textContent, 'restore');
  await restore.onclick();
  assert.equal(h.ignored.size, 364);
  await h.box.children[0].children[1].onclick();
  assert.equal(h.ignored.size, 0);
});

test('closing or switching context discards a late version-list response', async () => {
  for (const change of [h => { h.S.popup = null; }, h => { h.S.envSeq++; }, h => { h.S.screenSeq++; }, h => { h.S.shell.plan.id++; }]) {
    const h = panelHarness();
    let reply;
    h.api.VersionWarnings = () => new Promise(resolve => { reply = resolve; });
    const pending = h.open(false);
    const body = h.box.children[1];
    change(h);
    reply({ title: 'late data', groups: [], all: {} });
    await pending;
    assert.equal(body.textContent, 'loading…');
    assert.equal(body.children.length, 0);
  }
});

test('keyboard reaches row controls, scrolls, and keeps focus after refreshing', async () => {
  const h = panelHarness();
  await h.open(false);
  const p = h.S.popup, [head, body, foot] = h.box.children;
  const key = (key, more = {}) => p.keys({ key, preventDefault() {}, ...more });
  assert.equal(h.document.activeElement, foot.children[0]);
  key('Tab');
  assert.equal(h.document.activeElement, head.children[1]);
  key('Tab'); // group action
  key('Tab'); // first name
  key('Tab'); // first ignore
  const action = h.document.activeElement;
  assert.equal(action.textContent, 'ignore');
  await key(' ');
  await new Promise(setImmediate);
  assert.equal(h.ignored.size, 1);
  assert.ok(h.box.querySelectorAll('button').includes(h.document.activeElement));
  key('Tab', { shiftKey: true });
  assert.equal(h.document.activeElement.cls, 'version-name');
  key('PageDown');
  assert.equal(body.scrollTop, 360);
  key('End');
  assert.equal(body.scrollTop, body.scrollHeight);
  key('Home');
  assert.equal(body.scrollTop, 0);
  // Enter activates the focused name, not the header's batch action.
  key('Enter');
  assert.deepEqual(h.shown, ['mod-1']);
  assert.equal(h.calls.length, 1);
});
