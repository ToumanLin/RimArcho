// Synthetic feedback regression cases, invoked by /dev/pagecheck. No product APIs.
export async function checkFeedback({ handlers, test, assert, deferred }) {
  const st = await import('/js/state.js');
  const { progress, fieldError } = await import('/js/feedbacksemantics.js');
  const { shimmer } = await import('/js/motion.js');
  const tasks = await import('/js/tasks.js');
  const packs = await import('/js/packs.js');
  const { S, say, busyStart } = st;
  const status = document.querySelector('#status');
  await test('A12: footer has one live outcome, failures are assertive and undo stays outside Tab', async () => {
    say('ok', 'Fixture list saved', () => {});
    assert(status.getAttribute('role') === 'status' && status.getAttribute('aria-live') === 'polite', 'success not polite');
    const undo = status.querySelector('a');
    assert(undo.getAttribute('role') === 'button' && undo.tabIndex === -1 && undo.textContent === 'undo', 'undo semantics');
    const text = status.firstElementChild;
    // Identical event delivery must retain the existing nodes.
    say('ok', 'Fixture list saved', st.undoStatus);
    const outcome = status.firstElementChild;
    say('ok', 'Fixture list saved', st.undoStatus);
    assert(status.firstElementChild === outcome, 'unchanged outcome replaced text');
    say('fail', 'Fixture list refused');
    assert(status.getAttribute('role') === 'alert' && status.getAttribute('aria-live') === 'assertive', 'failure not assertive');
    assert(!status.querySelector('a') && text.isConnected === false, 'previous undo survives refusal');
  });
  await test('A11: a start shows before its reply; a refused start takes back only its own line', async () => {
    S.busy.scan = false;
    const reply = deferred();
    const request = busyStart('scan', 'scanning…', () => reply.promise);
    const bar = status.querySelector('[role=progressbar]');
    assert(status.getAttribute('aria-busy') === 'true' && bar?.textContent === 'scanning…' && !bar.hasAttribute('aria-valuenow'), 'no immediate indeterminate progress');
    // the run fails before its reply arrives: the failure stays on the line
    S.busy.scan = false; say('fail', 'Fixture scan failed');
    reply.resolve(true); await request;
    assert(status.textContent === 'Fixture scan failed' && status.getAttribute('role') === 'alert' && !S.busy.scan, 'reply replaced the failure');
    const refused = deferred();
    const second = busyStart('analyze', 'checking the mods files…', () => refused.promise);
    refused.resolve(false); await second;
    assert(status.textContent === '' && !S.busy.analyze, 'refused start kept its line');
    const late = deferred();
    const third = busyStart('analyze', 'checking the mods files…', () => late.promise);
    say('ok', 'Fixture newer action'); late.resolve(false); await third;
    assert(status.textContent === 'Fixture newer action' && !S.busy.analyze, 'refused start cleared a newer message');
    say('busy', 'checking the mods files… 3 / 8');
    const counted = status.querySelector('[role=progressbar]');
    assert(counted.getAttribute('aria-valuenow') === '3' && counted.getAttribute('aria-valuemax') === '8', 'shown count not exposed');
    say('', '');
  });
  await test('A11: unknown values and skeletons stay truthful', async () => {
    const box = document.createElement('div'), label = document.createElement('span'); label.textContent = 'Fixture bytes';
    progress(box, label);
    assert(!box.hasAttribute('aria-valuenow'), 'indeterminate falsely claims zero');
    progress(box, label, 0, 40);
    assert(box.getAttribute('aria-valuenow') === '0' && box.getAttribute('aria-valuemax') === '40', 'measured zero lost');
    const skel = shimmer('loading…');
    assert(skel.getAttribute('role') === 'progressbar' && !skel.hasAttribute('aria-valuenow'), 'skeleton value');
  });
  await test('A12: field errors attach and clear; stale subscription preview failures do not replace new choices', async () => {
    const field = document.createElement('input'), error = document.createElement('div');
    const sync = fieldError(field, error);
    error.textContent = 'Fixture name taken'; sync();
    assert(field.getAttribute('aria-invalid') === 'true' && field.getAttribute('aria-describedby') === error.id, 'error link missing');
    error.textContent = ''; sync();
    assert(!field.hasAttribute('aria-invalid') && !field.hasAttribute('aria-describedby'), 'stale error link');
    const {addForm} = await import('/js/subscribe.js');
    const old = deferred(), current = deferred();
    handlers.set('SubscribePreview', req => req.steamcmd ? current.promise : old.promise);
    const form = addForm(); document.querySelector('#screen').replaceChildren(form.el);
    form.setText('???');
    const box = form.el.querySelector('.subscribe-omni input'), bad = form.el.querySelector('.subscribe-bad');
    await new Promise(r => setTimeout(r, 0));
    assert(box.getAttribute('aria-invalid') === 'true' && box.getAttribute('aria-describedby') === bad.id, 'not recognised is not tied to the paste box');
    form.setText('1001001001');
    form.el.querySelectorAll('.opt')[1].click();
    current.resolve({lines:['New choice ready'],apply:true}); await new Promise(r => setTimeout(r, 0));
    old.reject(new Error('Old choice failed')); await new Promise(r => setTimeout(r, 0));
    assert(form.el.querySelector('.subscribe-preview').textContent === 'New choice ready', 'old preview failure replaced current choice');
    handlers.delete('SubscribePreview');
  });
  await test('A11/A12: task reads and duplicate end events preserve the latest task state', async () => {
    const a = deferred(), b = deferred(); let count = 0;
    handlers.set('Tasks', () => (++count === 1 ? a.promise : b.promise));
    const first = tasks.open(), second = tasks.open();
    b.resolve({rows:[],running:0,queued:0}); await second;
    a.resolve({rows:[{id:'old',title:'Old task'}]}); await first;
    assert(!document.querySelector('.tk-row[data-id=old]'), 'old tasks read overwrote latest read');
    S.screen = 'home'; handlers.set('Tasks', () => ({rows:[]}));
    await tasks.event({ended:{id:'feedback-once',ok:true,text:'Fixture task complete'}});
    const text = status.firstElementChild;
    await tasks.event({ended:{id:'feedback-once',ok:true,text:'Fixture task complete'}});
    assert(status.firstElementChild === text, 'duplicate task completion delivered twice');
  });
  await test('A11: a long run ends on the line after other messages', async () => {
    const textures = await import('/js/textures.js');
    S.screen = 'home'; S.packBuild = {name:'Fixture pack',frac:.2,text:'1 / 5'};
    say('ok', 'Fixture other action');
    packs.packEvent({name:'Fixture pack',done:true,ok:false,error:'Fixture build failed'});
    assert(status.textContent === 'Fixture build failed' && status.getAttribute('role') === 'alert' && S.packBuild === null, 'build failure dropped');
    textures.event({frac:.25,text:'1 / 4'});
    say('ok', 'Fixture other action');
    textures.event({done:true,ok:false,error:'Fixture texture failure'});
    assert(status.textContent === 'Fixture texture failure', 'texture failure dropped');
    say('', '');
  });
  await test('A11: a delayed pack reply cannot replace its failure', async () => {
    S.packBuild = null;
    const reply = deferred();
    handlers.set('PackNameCheck', () => '');
    handlers.set('Packs', () => ({rows:[],unfinished:[]}));
    handlers.set('BuildPageResult', () => ({name:'Ack fixture pack',plan:'Fixture',tex:0,texChoices:[],steps:[],ok:true}));
    handlers.set('PackBuild', () => reply.promise);
    await packs.build();
    document.querySelector('.bd-go').click();
    const buildName = document.querySelector('#pk-name').value;
    packs.packEvent({name:buildName,frac:.25,text:'1 / 4'});
    const bar = document.querySelector('.bd-prog [role=progressbar]');
    assert(document.getElementById(bar.getAttribute('aria-labelledby')).textContent.includes(buildName) && bar.getAttribute('aria-valuenow') === '0.25', 'build progress not named by its pack');
    packs.packEvent({name:buildName,done:true,ok:false,error:'Fixture build failed'});
    reply.resolve('Fixture build accepted');
    await new Promise(r => setTimeout(r, 0));
    assert(status.textContent === 'Fixture build failed', 'late pack reply replaced failure');
  });
  await test('A11: guide scan completion ignores duplicate results and late progress', async () => {
    const guide = await import('/js/guide.js'), {emit} = await import('/dev/pagecheck-runtime.js');
    handlers.set('GuideFind', () => [{name:'Fixture game',game:'fixture',version:'1.6'}]);
    handlers.set('GuideAdd', request => { emit('environment', {request,names:['Fixture game']}); return true; });
    handlers.set('GuideScan', () => true);
    await guide.start('env');
    document.querySelector('.guide-foot .primary').click();
    await new Promise(r => setTimeout(r, 0));
    guide.guideEvent({i:0,step:true,ok:true,text:'Fixture scan done'});
    guide.guideEvent({done:true,ok:true});
    const content = document.querySelector('#screen').textContent;
    guide.guideEvent({i:0,text:'Late scan count'});
    guide.guideEvent({done:true,ok:false});
    assert(document.querySelector('#screen').textContent === content, 'ended guide scan changed');
    S.busy.scan = false;
    document.querySelector('.guide-foot .primary').click();
    const url = document.querySelector('.guide .typed input');
    url.value = 'bad'; url.dispatchEvent(new KeyboardEvent('keydown', {key:'Enter',bubbles:true,cancelable:true}));
    await new Promise(r => setTimeout(r, 0));
    assert(document.querySelector('.guide .typed input').getAttribute('aria-invalid') === 'true', 'guide URL error not linked');
    handlers.set('PickPackFile', () => { throw Error('Fixture pack picker failed'); });
    [...document.querySelectorAll('.guide .typed button')].find(b => b.textContent === 'choose a pack…').click();
    await new Promise(r => setTimeout(r, 0));
    assert(!document.querySelector('.guide .typed input').hasAttribute('aria-invalid'), 'pack error marked URL invalid');
    assert([...document.querySelectorAll('.guide .typed button')].find(b => b.textContent === 'choose a pack…').getAttribute('aria-invalid') === 'true', 'pack picker error not linked');
  });
  await test('A12: shared import errors follow the selected source control', async () => {
    const exchange = await import('/js/exchange.js');
    S.screen = 'home';
    handlers.set('ExchangeInfo', () => ({dialogs:true}));
    handlers.set('ImportPreviewFor', () => { throw Error('Fixture source rejected'); });
    await exchange.importFrom('file', 'Fixture.xml');
    await new Promise(r => setTimeout(r, 0));
    let text = document.querySelector('.xc-text'), pick = document.querySelector('.xc-body .xc-row .primary');
    assert(pick.getAttribute('aria-invalid') === 'true' && !text.hasAttribute('aria-invalid'), 'file failure assigned to paste field');
    await exchange.importFrom('text', 'bad fixture');
    await new Promise(r => setTimeout(r, 0));
    text = document.querySelector('.xc-text'); pick = document.querySelector('.xc-body .xc-row .primary');
    assert(text.getAttribute('aria-invalid') === 'true' && !pick.hasAttribute('aria-invalid'), 'paste failure assigned to file chooser');
  });
  await test('A7: shared menus name their target, expose checks/disabled items and restore their opener', async () => {
    const pop = await import('/js/popups.js');
    let menu = document.querySelector('#menu');
    if (!menu) { menu = document.createElement('div'); menu.id = 'menu'; menu.hidden = true; document.body.append(menu); }
    const opener = document.createElement('button'); opener.textContent = 'Fixture Mod'; document.body.append(opener);
    pop.menuOpener(opener); opener.focus(); let runs = 0;
    const actions = [{ label: 'Open', key: 'o', run: () => runs++ }, { sep: true },
      { label: 'Size', checked: true, key: '✓', run: () => runs++ },
      { label: 'Date', checked: false, run: () => runs++ }, { label: 'Unavailable', disabled: true, run: () => runs++ }];
    pop.menu(100, 100, actions, true, { name: opener.textContent, opener });
    const buttons = [...menu.querySelectorAll('button')];
    assert(menu.getAttribute('role') === 'menu' && menu.getAttribute('aria-label') === 'Fixture Mod', 'menu role/name');
    assert(opener.getAttribute('aria-controls') === menu.id && opener.getAttribute('aria-expanded') === 'true', 'menu/opener link');
    assert(buttons[1].getAttribute('role') === 'menuitemcheckbox' && buttons[1].getAttribute('aria-checked') === 'true' && buttons[2].getAttribute('aria-checked') === 'false', 'checked states');
    assert(buttons[3].disabled && menu.querySelector('[role=separator]') && document.activeElement === buttons[0], 'disabled/separator/initial focus');
    const key = key => document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
    key('End'); assert(document.activeElement === buttons[2] && runs === 0, 'navigation committed or reached disabled item');
    key('Home'); key('Enter'); assert(runs === 1 && menu.hidden && document.activeElement === opener, 'menu item did not run exactly once');
    pop.menu(100, 100, actions, true, { name: 'Fixture Mod', opener }); key('Escape');
    assert(menu.hidden && document.activeElement === opener && opener.getAttribute('aria-expanded') === 'false', 'key menu Esc restoration');
    pop.menu(100, 100, actions, false, { name: 'Fixture Mod', opener }); pop.hideMenu(true);
    assert(document.activeElement === opener && opener.getAttribute('aria-expanded') === 'false', 'mouse menu Esc restoration');
    let backs = 0; pop.menu(100, 100, actions, true, { name: 'Fixture Mod', opener, back: () => backs++ }); key('Escape');
    assert(backs === 1 && menu.hidden && opener.getAttribute('aria-expanded') === 'false', 'Esc did not use the menu back function');
    pop.menu(100, 100, [actions[4]], true, { name: 'Fixture Mod', opener });
    key('ArrowDown'); assert(document.activeElement === menu && runs === 1, 'all-disabled menu lost focus or executed');
    key('Escape'); assert(document.activeElement === opener, 'all-disabled menu did not restore focus');
    opener.remove();
  });
  await test('A7: command results expose the active option and navigation never executes', async () => {
    const pop = await import('/js/popups.js'); const { buildCommandPanel } = await import('/js/command-panel.js');
    pop.close(); const opener = document.createElement('button'); opener.textContent = 'Fixture opener'; document.body.append(opener); opener.focus();
    handlers.set('Palette', () => [{ id: 'fixture-one', name: 'Fixture One', desc: 'Fixture One' }, { id: 'fixture-two', name: 'Fixture Two', desc: 'second' }]);
    handlers.set('CommandStatus', () => ({})); let runs = 0;
    pop.open('palette', 'command palette', (box, p) => buildCommandPanel(box, p, { close: pop.close, pick: () => runs++ }));
    await new Promise(r => setTimeout(r, 20));
    const input = document.querySelector('.command-panel input'); input.value = ''; input.dispatchEvent(new Event('input'));
    await new Promise(r => setTimeout(r, 20));
    input.focus(); // Previous import checks can still have an animation-frame focus queued.
    const list = document.getElementById(input.getAttribute('aria-controls'));
    const active = () => document.getElementById(input.getAttribute('aria-activedescendant'));
    assert(input.getAttribute('role') === 'combobox' && list.getAttribute('role') === 'listbox' && list.getAttribute('aria-labelledby') === 'command-panel-title', 'command combo/list name');
    assert(active().getAttribute('aria-selected') === 'true' && active().textContent === 'Fixture OneFixture One' && active().lastElementChild.getAttribute('aria-hidden') === 'true', 'initial active option');
    S.popup.keys(new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, cancelable: true }));
    assert(active().textContent === 'Fixture Twosecond' && list.querySelectorAll('[aria-selected=true]').length === 1 && runs === 0, 'result navigation committed/selected wrong option');
    assert(active().getAttribute('aria-posinset') === '2' && active().getAttribute('aria-setsize') === '2', 'option position/size');
    S.popup.keys(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }));
    assert(runs === 1 && !S.popup && document.activeElement === opener, 'command result did not run exactly once/restore focus: ' + runs + '/' + S.popup?.kind + '/' + document.activeElement?.outerHTML);
    opener.remove();
  });
  await test('A7: plan/sort/game choosers expose options without committing on arrows', async () => {
    const pop = await import('/js/popups.js'); let runs = 0;
    const opener = document.createElement('button'); opener.textContent = 'Fixture chooser'; document.body.append(opener); opener.focus();
    pop.choose('Fixture choices', ['Alpha', 'Beta'], () => runs++, 1);
    const list = document.querySelector('#popup [role=listbox]');
    assert(document.activeElement === list && list.getAttribute('aria-labelledby') && document.getElementById(list.getAttribute('aria-activedescendant')).textContent === 'Beta', 'chooser initial role/name/focus/active');
    S.popup.keys(new KeyboardEvent('keydown', { key: 'ArrowUp', cancelable: true }));
    assert(runs === 0 && list.querySelector('[aria-selected=true]').textContent === 'Alpha', 'chooser arrows committed');
    assert(opener.getAttribute('aria-controls') === list.id, 'chooser opener link');
    pop.popupKey(new KeyboardEvent('keydown', { key: 'Escape' }));
    assert(document.activeElement === opener && runs === 0 && !opener.hasAttribute('aria-controls'), 'chooser Esc did not restore or left its list link'); opener.remove();
  });
  await test('A7: the version-warning dialog contains named entries and Esc restores its opener', async () => {
    const pop = await import('/js/popups.js'); pop.close();
    const opener = document.createElement('button'); opener.textContent = 'Fixture versions'; document.body.append(opener); opener.focus();
    const action = { kind: 'version-mute', label: 'ignore' }; let runs = 0;
    handlers.set('VersionWarnings', () => ({ title: 'Fixture version warnings', all: action, groups: [{ title: 'Fixture group', action,
      rows: [{ id: 'fixture-one', name: 'Fixture One', supported: '1.6', action }, { id: 'fixture-two', name: 'Fixture Two', supported: '1.5', action }] }] }));
    await (await import('/js/versionpanel.js')).openVersionList({ title: 'Fixture versions', gameVersion: '1.7' }, () => runs++, () => runs++);
    const box = document.querySelector('.version-panel'), list = box.querySelector('[role=list]');
    assert(box.getAttribute('role') === 'dialog' && document.getElementById(box.getAttribute('aria-labelledby')).textContent === 'Fixture version warnings', 'version dialog name');
    assert(list.querySelectorAll('[role=listitem]').length === 2 && opener.getAttribute('aria-controls') === box.id, 'version list/opener link');
    S.popup.keys(new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true }));
    assert(runs === 0, 'version-list arrows committed'); pop.popupKey(new KeyboardEvent('keydown', { key: 'Escape' }));
    assert(document.activeElement === opener && opener.getAttribute('aria-expanded') === 'false', 'version-list Esc restoration'); opener.remove();
  });
  await test('A7: the log picker opens from Enter without the page consuming it', async () => {
    const pop = await import('/js/popups.js'); pop.close(); pop.hideMenu();
    handlers.set('LogPage', () => ({ path: 'fixture-log', files: [{ path: 'fixture-log', label: 'Player.log', when: '12:00' },
      { path: 'fixture-previous', label: 'Previous Player.log', when: '11:00' }], rows: [], sections: [], monitors: [] }));
    await (await import('/js/screens.js')).gameLog('fixture-log', true);
    const picker = document.querySelector('.picker'); picker.focus();
    const e = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }); picker.dispatchEvent(e);
    const menu = document.querySelector('#menu');
    assert(e.defaultPrevented && !menu.hidden && menu.getAttribute('aria-label') === 'Player.log', 'log picker Enter did not open its named menu');
    assert(document.activeElement === menu.querySelector('button') && menu.querySelector('[aria-current=page]'), 'log picker initial focus/current file');
    document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    assert(document.activeElement === picker && menu.hidden, 'log menu Esc restoration');
  });
  say('', '');
}
