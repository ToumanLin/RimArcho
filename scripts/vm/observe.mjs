// Installed only by the test driver. All imported application state is observed, never edited.
export async function observe() {
  const { S } = await import('/js/state.js');
  const { on } = await import('/js/api.js');
  const vm = window.__vm = { inputs: [], events: [], errors: [], samples: [], feedback: [], armed: false, statuses: [], start: performance.now() };
  const now = () => performance.now() - vm.start;
  const context = () => ({ page: S.screen, plan: S.shell?.plan?.name, env: S.board?.problems?.environment, listVersion: S.board?.problems?.listVersion });
  for (const type of ['input', 'keydown', 'keyup', 'mousedown', 'mouseup', 'click', 'dblclick', 'pointerdown', 'pointerup']) {
    addEventListener(type, e => {
      const col=S.cols[S.focus];
      const input={ at: now(), type, key: e.key, code: e.code, shift: e.shiftKey, ctrl: e.ctrlKey, repeat: e.repeat, button: e.button, detail: e.detail, trusted: e.isTrusted, target: e.target.closest?.("button,.row")?.outerHTML.slice(0,500), focus:S.focus, editing:S.editing||0, popup:S.popup?.kind, activeTag:document.activeElement?.tagName, selection:col?[...col.sel]:[], cursor:col?.items[col?.view[col?.cursor]]?.id, ...context() };
      vm.inputs.push(input);
      if(type==='keydown'||type==='pointerdown')vm.lastAction={index:vm.inputs.length-1,input};
    }, true);
  }
  addEventListener('error', e => vm.errors.push({ at: now(), message: String(e.message) }));
  addEventListener('unhandledrejection', e => vm.errors.push({ at: now(), message: String(e.reason?.stack || e.reason) }));
  const originalError = console.error.bind(console);
  console.error = (...args) => { vm.errors.push({ at: now(), message: args.map(String).join(' ') }); originalError(...args); };
  for (const name of ['environment', 'list-problems', 'scan', 'analyze', 'tasks']) on(name, data => vm.events.push({ at: now(), name, data }));
  let lastFeedback=-1;
  const mutation=new MutationObserver(()=>{
    const action=vm.lastAction;if(!action||action.index===lastFeedback)return;
    lastFeedback=action.index;
    vm.feedback.push({inputIndex:action.index,inputAt:action.input.at,at:now(),ms:now()-action.input.at,...context()});
  });
  mutation.observe(document.querySelector('#app'),{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:['class','hidden','disabled','aria-busy']});
  vm.read = (detailed=true) => {
    const screen = document.querySelector('#screen'), status = document.querySelector('#status')?.textContent || '';
    const cols = detailed && (S.cols || []).map(c => ({ entries: c.items.map(i => ({ id: i.id, ref: i.ref, packageId: i.packageId, missing: i.missing })), selected: [...c.sel], cursor: c.cursor }));
    return { ...context(), status, popup: S.popup?.kind || null, busy: S.busy, editing: S.editing || 0, problems: S.board?.problems, plans: S.shell?.plans, columns: cols, focus: S.focus,
      loading: !!document.querySelector('.page-loading, [aria-busy="true"]'),
      readError: document.querySelector('.page-read-error')?.innerText,
      blank: !document.querySelector('#app')?.firstElementChild,
      noGame: !!S.board?.envs?.length && /no games? yet|还没有游戏/.test(screen?.textContent||''),
      sidebar: [...document.querySelectorAll('#nav [data-id]')].map(e => e.dataset.id),
      content: S.screen === null ? !!document.querySelector('#board .rows') : !!screen?.innerText.trim(),
      errors: vm.errors, activeElement: document.activeElement?.outerHTML.slice(0, 300) };
  };
  let lastStatus = '', last = performance.now();
  vm.timer = setInterval(() => {
    const t = performance.now(), s = vm.read(false);
    vm.samples.push({ armed: vm.armed, at: now(), gapMs: t-last, page: s.page, plan: s.plan, plansCount:s.plans?.length, noGame:s.noGame, status: s.status, blank: s.blank, content: s.content, loading: s.loading, sidebar: s.sidebar, env:s.env, problemPlan:s.problems?.plan, sequence:s.problems?.sequence, listVersion: s.listVersion, popup: s.popup });
    last = t;
    if (s.status !== lastStatus) { vm.statuses.push({ at: now(), text: s.status, inputCount: vm.inputs.length }); lastStatus = s.status; }
  }, 25);
  vm.stop=()=>{clearInterval(vm.timer);mutation.disconnect();};
  return context();
}
export function discover() {
  const selector = 'button,a,input,select,textarea,summary,[role="button"],[role="tab"],[role="menuitem"],[tabindex],[data-cmd],[data-env-write],.row';
  return [...document.querySelectorAll(selector)].map(e => {
    const b = e.getBoundingClientRect(), x = b.x+b.width/2, y = b.y+b.height/2, hit = document.elementFromPoint(x,y);
    const label = (e.getAttribute('aria-label') || e.title || e.innerText || e.name || '').trim().replace(/\s+/g,' ').slice(0,200);
    const identity = Object.fromEntries([...e.attributes].filter(a => /^(data-|name|id)/.test(a.name)).map(a => [a.name,a.value]));
    return { tag: e.tagName, role: e.getAttribute('role'), label, identity, bounds: {x:b.x,y:b.y,width:b.width,height:b.height}, visible: !!(b.width && b.height), enabled: !e.disabled && !e.closest('[inert]'), hittable: hit === e || e.contains(hit), key: [document.querySelector('#nav .on')?.dataset.id, document.querySelector('#popup:not([hidden])')?.innerText.slice(0,80), e.tagName, e.getAttribute('role'), JSON.stringify(identity),label].join('|') };
  });
}
