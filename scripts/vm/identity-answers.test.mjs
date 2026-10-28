import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const source = (name) => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8');
const part = (text, start, end) => text.slice(text.indexOf(start), text.indexOf(end, text.indexOf(start)));
function el(tag, cls = '', text = '') {
  return { tag, className: cls, textContent: text, children: [], dataset: {},
    classList: { add() {} }, append(...children) { this.children.push(...children); } };
}
function buttons(node) {
  return [ ...(node.tag === 'button' ? [node] : []), ...node.children.flatMap(buttons) ];
}
const answers = ['same', 'copy', 'unrelated'].map((answer) => ({ kind: 'answer', answer, label: answer,
  title: `effect of ${answer} on lists and history`, confirm: 'confirm', warn: `warning for ${answer}` }));
const why = { kind: 'view', label: 'why?', argv: ['identity', 'why', 'q:1'] };
const item = { text: 'Are these the same?', actions: [...answers, why], mods: [], status: 2 };

test('can it run and editor problems expose every identity answer with equal styling and tooltips', () => {
  for (const screen of ['screens', 'problems']) {
    const calls = [];
    const context = createContext({ el, archo: {}, T: (s) => s, N: { cur: 0, open: new Set() }, markNeeds() {},
      statusIcon: () => '', subject: () => '', actionWrite: (a) => a.kind === 'answer' ? 'answer' : '',
      needActionWrite: (a) => a.kind === 'answer' ? 'answer' : '',
      runAction: (it, a) => calls.push(a), runNeedAction: (it, a) => calls.push(a),
      button: (label, key, run, cls, write) => { const b = el('button', `btn ${cls}`, label); b.onclick = run; if (write) b.dataset.envWrite = write; return b; },
    });
    const js = source(screen);
    const code = screen === 'screens' ? part(js, 'function needCard(', '// markNeeds moves')
      : part(js, 'function card(', '// subject:');
    runInContext(code + `
globalThis.cardUnderTest = ${screen === 'screens' ? 'needCard' : 'card'};`, context);
    const bs = buttons(context.cardUnderTest(item, 0));
    assert.deepEqual(bs.map((b) => b.textContent), ['same', 'copy', 'unrelated', 'why?']);
    assert.ok(bs.every((b) => !b.className.includes('primary') && !b.className.includes('more')));
    bs.slice(0, 3).forEach((b, i) => {
      assert.equal(b.title, answers[i].title);
      assert.equal(b.dataset.envWrite, 'answer');
      b.onclick({ stopPropagation() {} });
      assert.equal(calls.at(-1).answer, answers[i].answer);
    });
    assert.equal(bs[3].dataset.envWrite, undefined, 'why remains available during a game switch');
    bs[3].onclick({ stopPropagation() {} });
    assert.equal(calls.at(-1), why);
  }
});

test('each answer waits for confirmation and why only inspects the selected question', async () => {
  const executed = [], confirmations = [], inspections = [], panels = [];
  const context = createContext({ ui: {}, T: (s) => s,
    api: { Inspect: async (argv) => { inspections.push(argv); return ['package differs', 'three contents match']; } },
    pop: { confirm: (...args) => confirmations.push(args), text: (...args) => panels.push(args) },
    doAction: (a) => executed.push(a),
  });
  runInContext(part(source('screens'), 'async function runAction(', 'async function doAction('), context);
  for (const a of answers) {
    await context.runAction(item, a);
    assert.equal(executed.length, 0, 'clicking an answer cannot mutate before confirmation');
    const [label, lines, , confirm] = confirmations.pop();
    assert.equal(label, a.label);
    assert.equal(lines[0], a.warn);
    confirm();
    assert.equal(executed.pop(), a);
  }
  await context.runAction(item, why);
  assert.deepEqual(inspections, [why.argv]);
  assert.equal(panels.length, 1);
  assert.equal(executed.length, 0);
});
