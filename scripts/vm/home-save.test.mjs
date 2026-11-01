// Start (06 §2.3): edits not yet written to the game get save in the launch block's corner, a
// button of its own (w); nothing when there is nothing to save or the game runs.
import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const js = readFileSync(new URL('../../internal/surface/gui/assets/js/home.js', import.meta.url), 'utf8');
const part = (start, end) => js.slice(js.indexOf(start), js.indexOf(end, js.indexOf(start)));
function el(tag, cls = '', text = '') {
  const node = { tag, className: cls, textContent: text, children: [], dataset: {},
    append(...children) { this.children.push(...children); }, prepend(...children) { this.children.unshift(...children); } };
  return node;
}
const all = (node) => [node, ...node.children.flatMap(all)];

function launchContext(live = null) {
  const calls = [];
  const context = createContext({ el, T: (s) => s, Tf: (s, ...a) => s.replace(/%[ds]/g, () => a.shift()),
    S: { live, busy: {} }, launching: 0, arrived: false, note: () => null,
    running: () => !!(live && live.running), launch: () => calls.push('launch'), save: () => calls.push('save'),
    step: (n, cls) => el('section', 'hm-step ' + cls) });
  runInContext(part('function canSave(', '// save writes') + part('function launchStep(', '// problemOf:') +
    '\nglobalThis.launchStep = launchStep;', context);
  return { context, calls };
}
const plan = { kind: 'plan', current: true, unsaved: 3 };

test('3 unsaved edits: Save beside ▶, not inside it; a click saves without launching', () => {
  const { context, calls } = launchContext();
  const s = context.launchStep(plan);
  const block = s.children.find((n) => n.className === 'hm-launch');
  const [go, sv] = block.children;
  assert.equal(go.className.split(' ')[0], 'hm-go');
  assert.equal(sv.className, 'hm-save');
  assert.ok(!all(go).includes(sv), 'not a button inside the launch button');
  assert.equal(sv.children[0].textContent, 'save 3 changes');
  assert.equal(sv.children[1].textContent, 'w');
  assert.equal(sv.dataset.envWrite, 'save');
  // the block's corner said "3 unsaved": the button says it now
  assert.ok(!all(go).some((n) => n.textContent === '3 unsaved'));
  sv.onclick();
  assert.deepEqual(calls, ['save']);
});

test('nothing to save, another row, or the game running: no Save', () => {
  for (const [f, live] of [[{ ...plan, unsaved: 0 }, null], [{ ...plan, current: false }, null], [{ kind: 'pack', unsaved: 3 }, null], [plan, { running: true, errors: 0 }]]) {
    const { context } = launchContext(live);
    const s = context.launchStep(f);
    assert.ok(!all(s).some((n) => n.className === 'hm-save'), JSON.stringify([f, live]));
  }
});
