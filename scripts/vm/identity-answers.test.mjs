import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const source = (name) => readFileSync(new URL(`../../internal/surface/gui/assets/js/${name}.js`, import.meta.url), 'utf8');
const part = (text, start, end) => text.slice(text.indexOf(start), text.indexOf(end, text.indexOf(start)));
function el(tag, cls = '', text = '') {
  const node = { tag, className: cls, textContent: text, children: [], dataset: {},
    append(...children) { this.children.push(...children); },
    prepend(...children) { this.children.unshift(...children); }, focus() { this.focused = true; },
  };
  node.classList = {
    add(...names) { node.className += ' ' + names.join(' '); },
    toggle(name, on) { if (on) this.add(name); },
  };
  Object.defineProperty(node, 'innerHTML', { set() { node.children = []; } });
  return node;
}
function all(node) { return [node, ...node.children.flatMap(all)]; }
const buttons = (node) => all(node).filter((n) => n.tag === 'button');
const answers = ['same', 'copy', 'unrelated'].map((answer, i) => ({ kind: 'answer', answer, label: answer,
  key: ['s', 'c', 'u'][i], title: `effect of ${answer} on lists and history`, confirm: 'confirm', warn: `warning for ${answer}` }));
const why = { kind: 'view', label: 'why?', question: 1, argv: ['identity', 'why', 'q:1'] };
const item = { text: 'Are these the same?', why: 'Four files match but the name differs', actions: [...answers, why], mods: [], status: 2 };

test('all three surfaces separate a quiet why control from the three equal answers', () => {
  for (const screen of ['screens', 'problems', 'badgepop']) {
    const calls = [], box = el('div');
    const context = createContext({ el, box, pinned: true, reasonsOf: () => [item],
      runNeed: (it, a) => calls.push(a), needWrite: (a) => a.kind === 'answer' ? 'answer' : '', hide() {},
      archo: {}, T: (s) => s, Tf: (s) => s, N: { cur: [0, 0], open: new Set(), ask: null }, markNeeds() {},
      // Can it run's card (screens.js): the cursor, done lines and the other column
      setCur() {}, snapshot() {}, doAction() {}, jumpTo() {}, linkBlock: () => el('div'), confirmRow: () => el('div'),
      statusIcon: () => '', subject: () => '', actionWrite: (a) => a.kind === 'answer' ? 'answer' : '',
      needActionWrite: (a) => a.kind === 'answer' ? 'answer' : '',
      runAction: (it, a) => calls.push(a), runNeedAction: (it, a) => calls.push(a),
      button: (label, key, run, cls, write) => { const b = el('button', `btn ${cls}`, label); b.onclick = run; if (write) b.dataset.envWrite = write; return b; },
      // needtext.js (name links): plain text is enough for these button checks
      needText: (tag, cls, text) => el(tag, cls, text), needParts: () => el('ul'), needEvidence: () => el('div'),
      close() {},
    });
    const js = source(screen);
    const code = screen === 'screens' ? part(js, 'function needCard(', '// markNeeds moves')
      : screen === 'problems' ? part(js, 'function card(', 'async function fixAutomatically(') : part(js, 'function draw(', 'function place(');
    const fn = screen === 'screens' ? 'needCard' : screen === 'problems' ? 'card' : 'draw';
    runInContext(code + `\nglobalThis.cardUnderTest = ${fn};`, context);
    const row = context.cardUnderTest(item, 0) || box;
    const bs = buttons(row);
    assert.equal(bs.length, 4);
    const quiet = bs.find((b) => b.textContent === 'why?');
    assert.ok(quiet.className.includes('identity-why'));
    const answerButtons = bs.filter((b) => b !== quiet);
    assert.deepEqual(answerButtons.map((b) => b.textContent), ['same', 'copy', 'unrelated']);
    assert.ok(answerButtons.every((b) => !/primary|more|identity-why/.test(b.className)));
    const parent = (child) => all(row).find((n) => n.children.includes(child));
    assert.notEqual(parent(quiet), parent(answerButtons[0]), 'why sits under the text, outside the answer group');
    answerButtons.forEach((b, i) => {
      assert.equal(b.title, answers[i].title);
      assert.equal(b.dataset.envWrite, 'answer');
      b.onclick({ stopPropagation() {} });
      assert.equal(calls.at(-1).answer, answers[i].answer);
    });
    assert.equal(quiet.dataset.envWrite, undefined);
    quiet.onclick({ stopPropagation() {} });
    assert.equal(calls.at(-1), why);
  }
});

test('row and panel answers use the same confirmation before any mutation', async () => {
  const executed = [], confirmations = [], comparisons = [];
  const context = createContext({ ui: {}, T: (s) => s,
    api: { Inspect: () => assert.fail('identity comparison cannot use text Inspect') },
    pop: { confirm: (...args) => confirmations.push(args), identity: (...args) => comparisons.push(args) },
    doAction: (a) => executed.push(a),
  });
  runInContext(part(source('screens'), 'async function runAction(', 'async function doAction('), context);
  for (const a of answers) {
    await context.runAction(item, a);
    assert.equal(executed.length, 0);
    const [label, lines, , confirm] = confirmations.pop();
    assert.equal(label, a.label); assert.equal(lines[0], a.warn);
    confirm(); assert.equal(executed.pop(), a);
  }
  await context.runAction(item, why);
  const [qid, answer] = comparisons.pop();
  assert.equal(qid, 1);
  await answer(answers[1]);
  assert.equal(executed.length, 0);
  confirmations.pop()[3]();
  assert.equal(executed.pop(), answers[1]);
});

function panelContext(read) {
  let root;
  const picked = [];
  const context = createContext({ el, S: { popup: null }, T: (s) => s,
    Tf: (s, ...args) => s.replace(/%[ds]/g, () => args.shift()),
    api: { IdentityWhy: read },
    copyablePath: (node, path) => { node.title = path; node.dataset.path = path; },
    open: (kind, title, build) => { root = el('div'); const state = { kind, title }; context.S.popup = state; build(root, state); },
    close: () => { context.S.popup = null; }, say: () => assert.fail('unexpected error'), errText: String,
    run: (action) => { assert.equal(context.S.popup, null, 'comparison closes before confirmation'); picked.push(action); },
  });
  runInContext(part(source('popups'), 'export async function identity(', '// choose offers').replace('export ', ''), context);
  return { context, picked, root: () => root };
}

test('comparison uses normal table text, short folders with path tooltips, five files and equal confirmed answers', async () => {
  const data = { headline: '4 files match but names differ', rows: [
    { label: 'name', new: 'Q Mod B', previous: 'Q Mod A', match: false },
    { label: 'author', new: 'Tester', previous: 'Tester', match: true },
    { label: 'folder', new: 'qmod-b', previous: 'qmod-a', newPath: 'D:/fixture/qmod-b', previousPath: 'D:/fixture/qmod-a', match: false },
  ], files: ['Defs/1.xml', 'Defs/2.xml', 'Defs/3.xml', 'Defs/4.xml', 'Defs/5.xml'], more: 2, answers };
  const panel = panelContext(async (qid) => { assert.equal(qid, 1); return data; });
  await panel.context.identity(1, panel.context.run);
  const nodes = all(panel.root());
  assert.ok(nodes.some((n) => n.tag === 'table'));
  assert.ok(!nodes.some((n) => n.tag === 'pre'));
  assert.ok(nodes.some((n) => n.className === 'match'));
  assert.ok(nodes.some((n) => n.className === 'different'));
  assert.ok(!nodes.some((n) => String(n.textContent).includes('D:/fixture')));
  assert.equal(nodes.find((n) => n.dataset.path === 'D:/fixture/qmod-b').textContent, 'qmod-b');
  assert.equal(nodes.filter((n) => n.tag === 'li').length, 6);
  assert.ok(nodes.some((n) => n.textContent === 'and 2 more files'));
  const bs = buttons(panel.root());
  assert.ok(bs.every((b) => !/primary|go/.test(b.className)));
  panel.context.S.popup.keys({ key: 'Enter' });
  assert.equal(panel.picked.length, 0, 'Enter does not pick a default answer');
  assert.equal(bs[0].focused, true, 'initial focus is Close');
  const same = bs.find((b) => b.textContent === 'same');
  assert.equal(same.title, answers[0].title); assert.equal(same.dataset.envWrite, 'answer');
  same.onclick(); assert.equal(panel.picked[0], answers[0]);
  await panel.context.identity(1, panel.context.run);
  panel.context.S.popup.keys({ key: 'u', preventDefault() {} });
  assert.equal(panel.picked.at(-1), answers[2]);
});

test('a late comparison cannot replace a closed or newer popup', async () => {
  let resolve;
  const panel = panelContext(() => new Promise((done) => { resolve = done; }));
  const pending = panel.context.identity(1, panel.context.run);
  const old = panel.root();
  panel.context.S.popup = { kind: 'other' };
  resolve({ headline: 'late', rows: [], answers });
  await pending;
  assert.equal(old.children.length, 1);
  assert.equal(old.children[0].textContent, 'loading…');
  assert.equal(panel.context.S.popup.kind, 'other');
});
