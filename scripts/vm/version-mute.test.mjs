import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const js = name => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
function element(tag, cls, text) {
  return { tag, cls, textContent: text, children: [], dataset: {}, setAttribute() {}, append(...nodes) { this.children.push(...nodes); }, addEventListener() {} };
}

test('background problem updates use the individual reasons from Go', () => {
  const S = { board: { problems: { environment: 'game', sequence: 1 } }, shell: { plan: { id: 7 } } };
  const context = createContext({ S, el: element });
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

test('the version list opens on demand and each button acts on its own mod', () => {
  let notifications = 0;
  const context = createContext({ el: element, notification: () => notifications++ });
  runInContext(js('versionwarnings') + '\nonVersionList("test",notification);globalThis.list=versionList;globalThis.toggle=toggleVersionList;', context);
  const item = { id: 'version-warning', actions: [{ kind: 'version-list', entries: [
    { text: 'Alpha', action: { kind: 'version-mute', refs: ['pkg:a'], label: 'mute Alpha' } },
    { text: 'Beta', action: { kind: 'version-unmute', refs: ['pkg:b'], label: 'restore Beta' } },
  ] }] };
  const calls = [];
  assert.equal(context.list(item, () => {}), null);
  context.toggle(item);
  const list = context.list(item, (it, action) => calls.push([it, action]));
  assert.equal(list.children.length, 2);
  list.children[1].children[1].onclick({ stopPropagation() {} });
  assert.equal(calls[0][0], item);
  assert.deepEqual(calls[0][1].refs, ['pkg:b']);
  assert.equal(calls[0][1].kind, 'version-unmute');
  context.toggle(item);
  assert.equal(context.list(item, () => {}), null);
  assert.equal(notifications, 2);
});
