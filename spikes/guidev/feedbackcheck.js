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
    [...document.querySelectorAll('.guide .way button')].find(b => b.textContent === 'choose a pack…').click();
    await new Promise(r => setTimeout(r, 0));
    assert(!document.querySelector('.guide .typed input').hasAttribute('aria-invalid'), 'pack error marked URL invalid');
    assert([...document.querySelectorAll('.guide .way button')].find(b => b.textContent === 'choose a pack…').getAttribute('aria-invalid') === 'true', 'pack picker error not linked');
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
  await test('A4: segmented state follows a programmatic switch before the next frame', async () => {
    const {seg, setSeg} = await import('/js/screens.js');
    const strip = seg(['Fixture first', 'Fixture second'], 0, () => {});
    document.querySelector('#screen').replaceChildren(strip);
    const buttons = [...strip.querySelectorAll('button')];
    assert(buttons[0].getAttribute('aria-pressed') === 'true' && buttons[1].getAttribute('aria-pressed') === 'false', 'initial segment state');
    setSeg(strip, 1);
    assert(buttons[0].getAttribute('aria-pressed') === 'false' && buttons[1].getAttribute('aria-pressed') === 'true', 'changed segment state');
  });
  await test('A5/A16: equal setting labels retain distinct identities and local action context', async () => {
    const screens = await import('/js/screens.js');
    handlers.set('Settings', () => [{title:'Fixture settings',items:[
      {label:'Same label',edit:'text',value:'one',argv:['config','set','fixture / one','{}']},
      {label:'Same label',edit:'text',value:'two',argv:['config','set','fixture / two','{}']},
      {key:'fixture.account',label:'Fixture account',actions:[{label:'log in',argv:['account','fixture','login']}]},
    ]}]);
    handlers.set('Backgrounds', () => ({setting:{key:'plain',focus:'center',stars:[],files:[]},groups:[{name:'game',items:[{key:'plain',label:'plain'}]}]}));
    handlers.set('Tasks', () => ({rows:[]}));
    await screens.settings();
    const rows = [...document.querySelectorAll('.settings > .set')];
    assert(rows.length === 3 && new Set(rows.map(r=>r.id)).size === 3 && rows.every(r=>r.id && !/\s/.test(r.id)), 'setting record ids');
    assert(rows.every(r=>r.getAttribute('role') === 'group' && r.querySelector('.name').id === r.getAttribute('aria-labelledby')), 'setting row context');
    assert(rows[0].querySelector('input').getAttribute('aria-labelledby') === rows[0].getAttribute('aria-labelledby'), 'field label no longer local');
    assert(rows[2].querySelector('button').textContent === 'log in', 'child verb got noisier');
  });
  await test('A5: both search filters keep their clear action inside their visible field label', async () => {
    const {createQuery} = await import('/js/filesearchquery.js');
    const card=document.createElement('div');document.querySelector('#screen').replaceChildren(card);
    const query=createQuery({text:'fixture',files:'About',mods:'Fixture Mod',groups:[]},card,{run(){},shortcuts(){},stop(){},focusResults(){},visible(){return true;},consume(){},layout(){}});
    query.buildQuery();
    for (const input of [query.filesBar.input,query.modsBar.input]) {
      const group=input.closest('[role=group]'),label=group && document.getElementById(group.getAttribute('aria-labelledby'));
      assert(label && label.textContent === input.getAttribute('aria-label') && group.contains(input), 'filter context missing');
      assert(group.querySelector('button'), 'clear action left the field');
    }
    query.destroy();
  });
  await test('A4/A16: task selection is independent of the cursor and survives a group transition', async () => {
    const record=(id)=>({id,title:'Same task',obj:'Same Mod',group:'today',state:'fail',stateText:'failed',line:'Fixture refusal',time:'12:00',items:[],steps:[],acts:[{id:'retry',label:'try again',key:'r'}]});
    let rows=[record('fixture task / one'),record('fixture task / two')];
    handlers.set('Tasks',()=>({rows,running:0,queued:0}));
    await tasks.open();
    const press=(key,ctrlKey=false)=>tasks.key(new KeyboardEvent('keydown',{key,ctrlKey,cancelable:true}));
    press('Escape');press('2');press('Home');press(' ',true);
    const selected=document.querySelector('.tk-row[aria-selected=true]'),identity=selected.id;
    assert(selected.getAttribute('role') === 'row' && selected.closest('[role=grid]').getAttribute('aria-multiselectable') === 'true', 'unsupported selection structure');
    const described=selected.getAttribute('aria-describedby').split(' ').map(id=>document.getElementById(id)?.textContent).join(' ');
    assert(described.includes('failed') && described.includes('Fixture refusal') && described.includes('12:00'), 'grid row lost its line or time: '+described);
    press('ArrowDown');
    assert(document.getElementById(identity).getAttribute('aria-selected') === 'true' && !document.getElementById(identity).classList.contains('cur'), 'cursor changed selection');
    press(' ',true);
    assert(document.querySelectorAll('.tk-row[aria-selected=true]').length === 2, 'multi-selection missing');
    rows=rows.map(r=>({...r,group:'queue',state:'queue'}));await tasks.event({});
    for (const row of document.querySelectorAll('.tk-row')) {
      assert(!/\s/.test(row.id) && row.getAttribute('aria-selected') === 'true', 'selection or identity lost on refresh');
      for (const attr of ['aria-labelledby','aria-describedby']) for (const id of row.getAttribute(attr).split(' ')) assert(document.getElementById(id), 'task reference dangling');
    }
    assert(document.getElementById(identity) && document.querySelector('.tk-sum button'), 'task identity or actions lost');
    press('Escape');assert(!document.querySelector('.tk-row[aria-selected=true]'), 'Esc did not clear selection');
  });
  await test('A16: duplicate pack names and texture names use caller-owned record ids after sorting', async () => {
    handlers.set('Packs',()=>({rows:[{name:'Fixture / A',mods:1},{name:'Fixture / B',mods:1}]}));
    await packs.list();
    const ids=[...document.querySelectorAll('.data-row')].map(r=>r.id);
    assert(ids.length===2 && new Set(ids).size===2 && ids.every(id=>id && !/\s/.test(id)), 'pack ids');
    handlers.set('Pack',()=>({name:'Fixture / A',facts:[],mods:[{key:'pkg.a/folder one',name:'Same Mod',order:1},{key:'pkg.b/folder two',name:'Same Mod',order:2}]}));
    await packs.pack('Fixture / A');
    const rows=[...document.querySelectorAll('.pack-mods .data-row')];
    assert(rows.length===2 && rows[0].id!==rows[1].id && rows.every(r=>r.id && !r.hasAttribute('aria-label')), 'duplicate pack-entry identity or extra name');
    const {createTable}=await import('/js/datatable.js');
    const table=createTable({key:'fixture.columns',columns:[{key:'name',title:'name',w:120}],redraw(){}});
    const box=document.createElement('div'),head=document.createElement('div');box.append(head);table.draw(box,head);
    const row=document.createElement('div');row.append(document.createElement('span'));table.row(row,0,'fixture-texture-one');
    table.row(row,3,'fixture-texture-two');
    assert(row.id==='fixture-texture-two' && row.getAttribute('aria-rowindex')==='5', 'recycled row kept prior identity');
  });
  await test('A16: search row ids retain Mod and file identity across expansion', async () => {
    const {createResults}=await import('/js/filesearchtree.js');
    const card=document.createElement('div');document.querySelector('#screen').replaceChildren(card);card.style.height='300px';
    const X={byID:new Map(),modsFound:[],rows:[],cursor:0,expanded:new Set(),selected:null};
    const result=createResults(X,card,{readFile(){},markSelected(){}},{},{key(){},stop(){},run(){}});
    result.buildResults();
    const file={path:'About/About.xml',rel:'About/About.xml',count:2,lines:[{line:1,text:'match one'},{line:2,text:'match two'}]};
    result.applyDeltas([{id:'mod / one',name:'Same Mod',file},{id:'mod / two',name:'Same Mod',file}]);result.drawTree();
    await new Promise(requestAnimationFrame);
    let files=[...card.querySelectorAll('.fs-file')];
    assert(files.length===2 && files[0].id!==files[1].id, 'same file in different Mods shares id');
    const identity=files[0].id;files[0].click();await new Promise(requestAnimationFrame);
    files=[...card.querySelectorAll('.fs-file')];
    const lines=[...card.querySelectorAll('.fs-line')];
    assert(files.some(r=>r.id===identity) && lines.length===2 && lines[0].id!==lines[1].id, 'expanded file or match identity lost');
    assert([...card.querySelectorAll('[id]')].every(r=>!/\s/.test(r.id)), 'search id contains whitespace');result.destroy();
  });
  await test('A10: unsaved facts compose with an existing copy description and clear after save', async () => {
    const lists=await import('/js/lists.js');
    S.screen='';S.board={env:'fixture',status:{}};
    const it={id:'fixture-mod',name:'Same Mod',ref:'#fixture',kind:'local',pos:1,copyCount:2,copyLabel:'local copy',unsaved:true};
    lists.setItems(1,[it]);S.cols[1].view=[0];lists.renderRows(1);
    let row=document.getElementById('editor-1-fixture-mod');
    assert(row.getAttribute('aria-description')==='local copy; unsaved' && !row.hasAttribute('aria-describedby'), 'unsaved replaced or duplicated copy description');
    it.unsaved=false;lists.renderRows(1);row=document.getElementById('editor-1-fixture-mod');
    assert(row.getAttribute('aria-description')==='local copy', 'saved row retained unsaved');
    lists.setStagingOpen(true);assert(document.querySelector('[data-area="2"] .head').getAttribute('aria-expanded')==='true', 'staging opened without state');
    lists.setStagingOpen(false);assert(document.querySelector('[data-area="2"] .head').getAttribute('aria-expanded')==='false', 'staging closed without state');
  });
  await test('A5: closed filter suggestions never retain a missing controlled list', async () => {
    const {createBar}=await import('/js/filterbar.js');
    const bar=createBar({data:()=>({groups:[]})});document.querySelector('#screen').replaceChildren(bar.el);
    assert(!bar.input.hasAttribute('aria-controls'), 'closed combobox has a missing list');
    bar.input.focus();bar.input.dispatchEvent(new Event('input',{bubbles:true}));
    if(bar.input.hasAttribute('aria-controls'))assert(document.getElementById(bar.input.getAttribute('aria-controls')), 'open suggestion reference missing');
    bar.input.blur();
    assert(!bar.input.hasAttribute('aria-controls'), 'blurred combobox retained suggestions');bar.destroy();
  });
  await test('Search disclosure retains the provider key target until the event has finished', async () => {
    const search=await import('/js/filesearch.js');
    handlers.set('FileSearchInfo',()=>({files:'About',mods:'',groups:[]}));
    handlers.set('BeginFileSearch',()=>42);handlers.set('StartFileSearch',()=>true);handlers.set('StopFileSearch',()=>true);
    handlers.set('ReadSearchFile',()=>({lines:[]}));
    handlers.set('FileSearchUpdate',()=>({done:true,next:1,summary:'2 matches',deltas:[{id:'fixture-mod',name:'Fixture Mod',file:{path:'About/About.xml',rel:'About/About.xml',count:2,lines:[{line:1,text:'fixture'},{line:2,text:'fixture'}]}}]}));
    S.envSeq=9001;await search.open();
    const input=document.querySelector('.fs-text-box input');input.value='fixture';input.dispatchEvent(new Event('input',{bubbles:true}));
    await search.run();await new Promise(resolve=>setTimeout(resolve,0));
    const file=document.querySelector('.fs-file');assert(file,'fixture file missing');
    file.focus();const expand=new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true,cancelable:true});file.dispatchEvent(expand);
    assert(expand.defaultPrevented && file.isConnected,'provider expansion removed its target during dispatch');
    await new Promise(requestAnimationFrame);
    const opened=document.getElementById(file.id);assert(opened?.getAttribute('aria-expanded')==='true','expansion did not render');
    const collapse=new KeyboardEvent('keydown',{key:'ArrowLeft',bubbles:true,cancelable:true});opened.dispatchEvent(collapse);
    assert(collapse.defaultPrevented && opened.isConnected,'provider collapse removed its target during dispatch');
    await new Promise(requestAnimationFrame);
    assert(document.getElementById(file.id)?.getAttribute('aria-expanded')==='false','collapse did not render');search.leave();
  });
  await test('Library disclosure uses the existing chevron while ordinary row clicks still open a copy', async () => {
    const library=await import('/js/library.js');
    handlers.set('Library',()=>({shown:2,total:2,columns:[{key:'name',title:'mod',shown:true}],rows:[
      {id:'fixture-lineage',name:'Same Mod',copies:2,insts:[{id:'one',ref:'#one',label:'local'},{id:'two',ref:'#two',label:'workshop'}]},
      {id:'fixture-next',name:'Next Mod',copies:1,single:true,insts:[{id:'next',ref:'#next',name:'Next Mod'}]},
    ]}));
    await library.open();
    await library.relang();
    let row=document.querySelector('.lib-row.lin'),chevron=row.querySelector('.chev');
    assert(!row.hasAttribute('aria-expanded') && chevron.getAttribute('role')==='button' && chevron.tabIndex===-1,'state remains on a non-toggle row or changes Tab order');
    assert(document.getElementById(chevron.getAttribute('aria-labelledby')).textContent==='Same Mod','disclosure name not local');
    chevron.click();row=document.querySelector('.lib-row.lin');
    assert(row.querySelector('.chev').getAttribute('aria-expanded')==='false','chevron failed to collapse');
    row.click();row=document.querySelector('.lib-row.lin');
    assert(row.querySelector('.chev').getAttribute('aria-expanded')==='true' && document.querySelector('.lib-row.inst.cur'),'row click changed');
    row.querySelector('.chev').click();document.querySelector('.lib-row.lin .chev').click();
    assert(document.querySelector('.lib-row.lin .chev').getAttribute('aria-expanded')==='true','repeated native toggle route failed');
  });
  say('', '');
}
