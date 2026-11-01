import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

function harness() {
  function el(tag, cls = '', text = '') {
    return { tag, className: cls, textContent: text, children: [], dataset: {},
      append(...nodes) { this.children.push(...nodes); }, setAttribute() {},
      classList: { add() {}, remove() {}, toggle() {} },
      click() { return this.onclick({ preventDefault() {}, stopPropagation() {} }); } };
  }
  const calls = [], state = { screen: 'needs', busy: {} };
  const context = createContext({ S: state, el, keyCaps: (k) => el("kbd", "", k), T: x => x, Tf: (s, n) => s.replace('%d', n),
    document: { createTextNode: text => ({ textContent: text }) },
    api: { OpenURL: async url => calls.push(['url', url]) }, say() {}, errText: String,
    statusIcon: n => `icon:${n}` });
  function load(file, names) {
    const source = readFileSync(new URL(`../../internal/surface/gui/assets/js/${file}`, import.meta.url), 'utf8')
      .replace(/^import .*;\r?$/gm, '').replace(/\bexport /g, '');
    runInContext(`(function(){${source}\nObject.assign(globalThis, {${names.join(',')}});})()`, context);
  }
  load('needtext.js', ['initNeedText', 'needText', 'needParts', 'needEvidence']);
  context.initNeedText({ go: async page => { calls.push(['go', page]); state.screen = ''; },
    palette: async ref => calls.push(['mod', ref.id]) });
  return { context, state, calls, load };
}

test('each name activates its own target; keyboard activation stays local', async () => {
  const h = harness();
  const parts = [{ text: 'TakeCover', id: 'one' }, { text: ' 与 ' }, { text: 'Achtung!', id: 'two' }];
  const row = h.context.needText('div', '', '', parts);
  assert.equal(row.children.length, 3);
  assert.equal(row.children[1].tag, undefined);
  await row.children[2].click();
  assert.deepEqual(h.calls, [['go', 'plan'], ['mod', 'two']]);
  h.calls.length = 0;
  await row.children[0].click();
  assert.deepEqual(h.calls, [['mod', 'one']]);
  const e = { key: 'Enter', preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; } };
  row.children[2].onkeydown(e);
  assert.equal(e.prevented && e.stopped, true);
  assert.deepEqual(h.calls.at(-1), ['mod', 'two']);
});

test('known missing mods open the Workshop, unknown ones stay text, and markup stays literal', async () => {
  const h = harness();
  const url = 'https://steamcommunity.com/sharedfiles/filedetails/?id=1234';
  const row = h.context.needText('div', '', '', [{ text: '<img onerror=bad>', url }, { text: ' unknown' }]);
  assert.equal(row.children[0].textContent, '<img onerror=bad>');
  assert.equal(row.children[0].innerHTML, undefined);
  await row.children[0].click();
  assert.deepEqual(h.calls, [['url', url]]);
  assert.equal(row.children[1].onclick, undefined);
});

test('sentences keep inline text flow on every problem surface', () => {
  const h = harness();
  const parts = [{ text: 'TakeCover', id: 'one' }, { text: ' 与 ' }, { text: 'Achtung!', id: 'two' }, { text: ' 不兼容' }];
  for (const cls of ['title word-facts', 'why word-facts', 'why sub', 'pp-t', 'pp-why', 'bp-t', 'bp-why']) {
    const node = h.context.needText('div', cls, '', parts);
    assert.equal(node.className.split(/\s+/).includes('word-facts'), false, cls);
    assert.equal(node.children.map(x => x.textContent).join(''), 'TakeCover 与 Achtung! 不兼容');
  }
  const plain = h.context.needText('div', 'title word-facts', 'plain', null);
  assert.equal(plain.className, 'title');
  const list = h.context.needParts({ parts: ['TakeCover 与 Achtung! 不兼容'], content: { parts: [parts] } });
  assert.equal(list.children[0].tag, 'li');
  assert.equal(list.children[0].children.map(x => x.textContent).join(''), 'TakeCover 与 Achtung! 不兼容');
});

test('badge popover keeps its own mod plain and links the other mod in title and why', () => {
  const h = harness();
  h.load('badgepop.js', ['draw', 'box']);
  const parts = [{ text: 'Fortress Citadel', id: 'one' }, { text: ' + ' }, { text: 'Framework', id: 'two' }];
  h.state.board = { reasons: { one: [{ status: -1, text: 'pair', why: 'pair', content: { text: parts, why: parts } }] } };
  h.context.draw({ id: 'one' }, -1);
  const body = h.context.box.children[0].children[1];
  for (const line of body.children) {
    assert.equal(line.children[0].tag, undefined);
    assert.equal(line.children[0].textContent, 'Fortress Citadel');
    assert.equal(line.children[2].tag, 'a');
    assert.equal(line.children[2].textContent, 'Framework');
  }
  assert.equal(parts[0].id, 'one', 'shared view data must keep the link for other surfaces');
});

test('editor panel shows evidence only on hints', () => {
  const h = harness();
  h.load('problems.js', ['card']);
  for (const status of [-1, 2, 4]) {
    const card = h.context.card({ text: 'item', status, evidence: { kind: status === -1 ? 'corpus' : 'declared', label: 'source' } });
    const body = card.children[1];
    assert.equal(body.children.some(x => x.className === 'ev-line'), status === -1);
  }
});

test('folds are semantic lists and evidence includes the supplied cutoff', () => {
  const h = harness();
  const list = h.context.needParts({ parts: ['First', 'Second'], content: { parts: [[{ text: 'First', id: 'one' }], null] } });
  assert.equal(list.tag, 'ul');
  assert.deepEqual(list.children.map(x => x.tag), ['li', 'li']);
  assert.equal(list.children[0].children[0].textContent, 'First');
  assert.equal(list.children[1].textContent, 'Second');
  const evidence = h.context.needEvidence({ kind: 'corpus', label: '分享的日志（截至 2040-03）', source: '40/50' });
  assert.equal(evidence.children[0].textContent, '分享的日志（截至 2040-03）');
});

test('background updates preserve hint links and evidence without counting them or hiding warnings', () => {
  const h = harness();
  h.load('problems.js', ['event', 'chip', 'card']);
  h.state.board = { problems: { environment: 'fixture', sequence: 1 } };
  const hint = { status: -1, counted: false, mods: ['one'], text: 'One and Two',
    content: { text: [{ text: 'One', id: 'one' }, { text: ' and ' }, { text: 'Two', id: 'two' }] },
    evidence: { kind: 'corpus', label: 'shared logs (up to 2040-03)' }, actions: [{ kind: 'fold' }] };
  // The event carries the row icons and reasons as view.indexNeeds builds them; the
  // window adopts them as sent (codex/version-mute moved this to the server).
  const update = items => {
    const status = {}, reasons = {};
    for (const it of items) {
      if (!it.counted && it.status !== -1) continue;
      for (const id of it.fold ? it.mods : it.mods.slice(0, 1)) {
        if (!status[id] || it.status > status[id]) status[id] = it.status;
        (reasons[id] ||= []).push({ status: it.status, text: it.title || it.text, content: it.content, evidence: it.evidence,
          section: it.section, actions: (it.actions || []).filter(a => a.kind !== 'fold') });
      }
    }
    for (const id in reasons) reasons[id].sort((x, y) => y.status - x.status);
    return { environment: 'fixture', sequence: 2, count: items.filter(x => x.counted && x.section !== 4).length, items, status, reasons };
  };
  h.context.event(update([hint]));
  assert.equal(h.state.board.needs, 0);
  assert.equal(h.state.board.status.one, -1);
  assert.equal(h.state.board.reasons.one[0].content.text[2].id, 'two');
  assert.equal(h.state.board.reasons.one[0].evidence.label, hint.evidence.label);
  assert.equal(h.state.board.reasons.one[0].actions.length, 0);
  assert.equal(h.context.chip().children[1].textContent, 'suggestions');
  const warning = { counted: true, status: 2, mods: ['one'], text: 'warning', section: 0 };
  for (const items of [[warning, hint], [hint, warning]]) {
    h.context.event(update(items));
    assert.equal(h.state.board.needs, 1);
    assert.equal(h.state.board.status.one, 2);
    assert.equal(h.state.board.reasons.one[0].text, 'warning');
    assert.equal(h.context.chip().children[1].textContent, '1 problems');
  }
  const card = h.context.card({ ...hint, actions: [], why: '', section: 0 });
  const title = card.children[1].children[0];
  assert.equal(title.children[0].tag, 'a');
  assert.equal(title.children[2].tag, 'a');
  assert.equal(title.onclick, undefined);
  h.context.event(update([{ counted: true, status: 2, mods: ['hidden'], text: 'hidden', section: 4 }]));
  assert.equal(h.context.chip(), null, 'hidden housekeeping alone must not create a problem chip');
});
