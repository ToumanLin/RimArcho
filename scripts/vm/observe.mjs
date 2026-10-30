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
  // Domain payloads are independently captured by vmstate. Keep scalar event
  // metadata here: analysis events may contain the entire real mod library.
  for (const name of ['environment', 'list-problems', 'scan', 'analyze', 'tasks']) on(name, data => vm.events.push({ at: now(), name, data: data&&typeof data==='object'?Object.fromEntries(Object.entries(data).filter(([,v])=>v===null||['string','number','boolean'].includes(typeof v))):data, payload:'scalar metadata; full domain in checkpoints' }));
  let lastFeedback=-1;
  const mutation=new MutationObserver(()=>{
    const action=vm.lastAction;if(!action||action.index===lastFeedback)return;
    lastFeedback=action.index;
    vm.feedback.push({inputIndex:action.index,inputAt:action.input.at,at:now(),ms:now()-action.input.at,...context()});
  });
  mutation.observe(document.querySelector('#app'),{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:['class','hidden','disabled','aria-busy']});
  const modePath=e=>{const parts=[];for(let n=e;n&&n!==document.body;n=n.parentElement){if(n.id){parts.unshift('#'+n.id);break;}const peers=[...n.parentElement.children].filter(x=>x.tagName===n.tagName);parts.unshift(n.tagName+':nth-of-type('+(peers.indexOf(n)+1)+')');}return parts.join('>');};
  vm.read = (detailed=true) => {
    const screen = document.querySelector('#screen'), status = document.querySelector('#status')?.textContent || '';
    const renderedColumns=S.screen===null ? S.cols.slice(0,2).map((col,k)=>({expected:col.view.length,rendered:document.querySelectorAll(`.area[data-area="${k}"] .rows .row`).length})) : [];
    const cols = detailed && (S.cols || []).map(c => ({ entries: c.items.map(i => ({ id: i.id, ref: i.ref, packageId: i.packageId, missing: i.missing })), selected: [...c.sel], cursor: c.cursor }));
    return { ...context(), status, popup: S.popup?.kind || null, popupTitle:S.popup?.title || '', menu:!!document.querySelector('#menu:not([hidden])'), modes:[...document.querySelectorAll('.tabs .on,[role="tab"][aria-selected="true"],details[open],.fold.closed,.fold.collapsed')].map(e=>[modePath(e),{...e.dataset},e.className,e.tagName]), busy: S.busy, editing: S.editing || 0, problems: detailed ? S.board?.problems : Object.fromEntries(['environment','plan','sequence','listVersion','revision','count','updating'].map(k=>[k,S.board?.problems?.[k]])), columnState:(S.cols||[]).map(c=>[c.items.length,c.view.length,c.cursor,[...c.sel]]), plans: S.shell?.plans, columns: cols, renderedColumns, focus: S.focus,
      loading: !!document.querySelector('#screen .page-loading, #screen[aria-busy="true"]'),
      pendingRegions:[...document.querySelectorAll('#screen [aria-busy="true"]')].map(e=>e.getAttribute('aria-label')||e.className),
      readError: document.querySelector('.page-read-error')?.innerText,
      blank: !document.querySelector('#app')?.firstElementChild,
      noGame: !!S.board?.envs?.length && /no games? yet|还没有游戏/.test(screen?.textContent||''),
      sidebar: [...document.querySelectorAll('#nav [data-id]')].map(e => e.dataset.id),
      content: S.screen === null ? !!document.querySelector('#board .rows') && renderedColumns.every(col=>!col.expected || col.rendered>0) : !!screen?.innerText.trim(),
      navLayout:document.querySelector('#app')?.classList.contains('nav-top')?'top':'side',
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
  const selector = 'button,a[href],input:not([type="hidden"]),select,textarea,summary,label:has(> input),[role="button"],[role="tab"],[role="menuitem"],[role="option"],[role="checkbox"],[role="switch"],[role="slider"],[tabindex],[data-cmd],[data-env-write],[onclick],[contenteditable="true"],.row,.lrow,.opt,.fold > .head';
  const candidates = new Set(document.querySelectorAll(selector));
  // Delegated handlers and row descendants need more than the CONTROL selector.
  for(const e of document.querySelectorAll('#app *')) if(e.onclick || e.oncontextmenu || getComputedStyle(e).cursor==='pointer') {
    if(!e.closest('.refnums') && (!e.parentElement || getComputedStyle(e.parentElement).cursor!=='pointer' || e.onclick || e.oncontextmenu)) candidates.add(e);
  }
  const path = e => {
    const parts=[];
    for(let n=e;n && n!==document.body;n=n.parentElement) {
      if(n.id){parts.unshift('#'+CSS.escape(n.id));break;}
      const semantic=['data-cmd','data-env-write','data-id','data-key','data-area','name'].find(k=>n.hasAttribute(k));
      if(semantic)parts.unshift(n.tagName.toLowerCase()+'['+semantic+'='+JSON.stringify(n.getAttribute(semantic))+']');
      else {const siblings=[...n.parentElement.children].filter(s=>s.tagName===n.tagName);parts.unshift(n.tagName.toLowerCase()+':nth-of-type('+(siblings.indexOf(n)+1)+')');}
    }
    return parts.join(' > ');
  };
  return [...candidates].map(e => {
    const b = e.getBoundingClientRect(), x = b.x+b.width/2, y = b.y+b.height/2, hit = document.elementFromPoint(x,y);
    const label = (e.getAttribute('aria-label') || e.title || e.innerText || e.name || '').trim().replace(/\s+/g,' ').slice(0,200);
    const identity = Object.fromEntries([...e.attributes].filter(a => /^(data-|name|id)/.test(a.name)).map(a => [a.name,a.value]));
    const row=e.matches('.row,.lrow,.lib-row,.log-row,.chg-row,.needs-row'), area=e.closest('[data-area]')?.dataset.area;
    const owner=e.closest('#popup:not([hidden])')?'popup':e.closest('#menu:not([hidden])')?'menu':e.closest('#nav,#nav-settings,#tasks-line,#env')?'nav':'page';
    const locator=path(e);
    const blockedByLayer=!!(hit?.closest('#popup:not([hidden]),#menu:not([hidden])')&&!e.closest('#popup:not([hidden]),#menu:not([hidden])'))||!!(e.closest('#board,#planhead')&&!document.querySelector('#screen')?.hidden);
    const semantics=[['breadcrumb','#crumbs button'],['table-header','.th[data-k]'],['plan-choice','.pm-list .pm-row'],['export-format','.fmts .fmt'],['appearance','.accent-swatch,.theme-segments button'],['filter-choice','button[id^="filter-options-"]']].filter(([,selector])=>e.matches(selector)).map(([name])=>name);
    return { semantics, tag: e.tagName, role: e.getAttribute('role'), label, identity, locator, owner, area, row, rowKind:row?[...e.classList].filter(c=>!['cur','on','sel','selected'].includes(c)).join(' '):null, inputType:e.type, href:e.getAttribute('href'), context:!!e.oncontextmenu || row, blockedByLayer, bounds: {x:b.x,y:b.y,width:b.width,height:b.height}, visible: !!(b.width && b.height), enabled: !e.matches(':disabled') && !e.closest('[inert]'), hittable: hit === e || e.contains(hit), key: [e.tagName, e.getAttribute('role'),JSON.stringify(identity),locator,label].join('|') };
  });
}
