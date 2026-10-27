// A reviewed semantic policy, not a list of DOM buttons. Unknowns always fail closed.
export const exclusions = [
  'undo/redo stack payloads and depths: undo adds redo history; original snapshots retain both',
  'environment current_plan: opening a different list is navigation, not an undoable edit',
  'problem publication/analysis caches, revision, sequence and computed observations',
  'plan_collection.checked_at: source refresh timestamp; source/hash/content fields remain compared',
  'UI settings, reference numbers, focus, selection, scroll, page visits, clipboard, logs and task history',
];
export function canonical(s) {
  return Object.fromEntries(['sources','instances','links','environments','plans','lineage_data','lineage_tag','instance_note','plan_collection','user_rule','modsConfigHashes','modsConfig'].map(k=>[k,
    k==='plans' ? s.plans.map(({undo,redo,undoDepth,redoDepth,...p})=>p) :
    k==='environments' ? s.environments.map(({current_plan,...e})=>e) : k==='plan_collection'?s[k]?.map(({checked_at,...row})=>row):s[k]]));
}
export function difference(a,b,path='') {
  if(Object.is(a,b))return null;
  if(a===null||b===null||typeof a!=='object'||typeof b!=='object'||Array.isArray(a)!==Array.isArray(b))return {path,left:a,right:b};
  for(const k of [...new Set([...Object.keys(a),...Object.keys(b)])].sort()) { const d=difference(a[k],b[k],path+'/'+k);if(d)return d; }
  return null;
}
export function random(seed) { let n=Number(seed)>>>0;return ()=>{n+=0x6D2B79F5;let t=n;t=Math.imul(t^(t>>>15),t|1);t^=t+Math.imul(t^(t>>>7),t|61);return ((t^(t>>>14))>>>0)/4294967296;}; }
const safeCommands = new Set(['full','detail','search','menu','filter','fold','expand','collapse','history','palette','help','keys','import','export','position','tags','note','swap','move-to','enable-at','plan','plans','home','needs','library','changes','log','settings','env','packs','tasks','textures','guide','undo','redo','sort','preview','toggle','enable','disable','add','take-out','put-back','stage','stage-all','restore-all','up','down','up10','down10']);
const externalCommands = new Set(['folder','workshop','workshop-id','steam','subscription-page','subscription-check','subscription-update','resubscribe','unsubscribe','launch','download','open','browse','save-file','open-file']);
const localCommands = new Set(['delete','remove','separate','pin','pin-update','unpin','move','unhide','textures-clean','subscription-remove','subscription-remove-fast','save','apply']);
export function classifyKey(combo,state={}) {
  if(combo==='Control+Backslash')return {tier:'safe',reason:'reference-number UI toggle in the isolated home'};
  if(['Shift+KeyL','Control+KeyO','Control+KeyW','Control+KeyS'].includes(combo))return {tier:'external',reason:'documented game/external shortcut'};
  if(combo==='Enter')return state.popup==='tags'?{tier:'local-destructive',reason:'commit persistent tag'}:state.popup==='position'?{tier:'safe',reason:'undoable position choice'}:{tier:'unknown',reason:'Enter could confirm an unclassified menu/dialog action'};
  if(combo==='Space'&&(state.popup||state.menu))return {tier:'unknown',reason:'Space could activate an unclassified dialog choice'};
  if(combo==='Space'&&state.page!==undefined&&state.page!==null&&state.page!=='library')return {tier:'unknown',reason:'Space is only reviewed for editor/library workspace actions'};
  if(/^(?:Escape|Space|Tab|Shift\+Tab|Home|End|PageUp|PageDown|(?:(?:Shift|Alt)\+)?Arrow(?:Up|Down|Left|Right)|(?:Shift\+)?Digit[1-8]|Control\+(?:Key[ABKRYZ]|Space)|Shift\+F10|Shift\+Key[HP]|Key[EGIMNTUX]|(?:Shift\+)?Key[JKST]|Backslash|Slash|Shift\+Slash)$/.test(combo))return {tier:'safe',reason:'reviewed navigation/view/workspace shortcut'};
  return {tier:'unknown',reason:'unclassified keyboard shortcut '+combo};
}
function classifyBase(control, state={}) {
  if(control.action?.kind==='key') return classifyKey(control.action.combo,state);
  if(control.action?.kind==='wheel') return {tier:'safe',reason:'real scrolling'};
  const id=control.identity||{}, cmd=id['data-cmd'] || id['data-op'] || '', write=id['data-env-write'], text=control.label?.trim().toLowerCase()||'';
  if(/^go back to this version/.test(text))return {tier:'safe',reason:'saved-version checkout advertises undo; verify on a disposable DB branch',isolate:true,mustUndo:true};
  if(control.semantics?.some(x=>['breadcrumb','table-header','plan-choice','export-format','appearance','filter-choice'].includes(x)))return {tier:'safe',reason:'reviewed UI navigation/view control: '+control.semantics.join(', ')};
  if(control.owner==='nav'&&id['data-id']==='subscribe')return {tier:'safe',reason:'open subscription form without submitting or previewing a URL'};
  if(id['data-act']==='goto')return {tier:'safe',reason:'task result navigation'};
  if(id['data-act']==='forget')return {tier:'local-destructive',reason:'remove task record in isolated DB'};
  if(control.owner==='popup'&&state.popupTitle==='lists'&&state.plans?.some(p=>text===p.name.toLowerCase()||text.startsWith(p.name.toLowerCase()+' ')))return {tier:'safe',reason:'known list choice from the observed shell'};
  if(id['data-game']||id['data-id']?.startsWith('game|')||id['data-id']?.startsWith('row|')||/use this (?:one|;)|switch to .* and use/.test(text))return {tier:'local-destructive',reason:'apply list/game selection may change active environment or deployment links'};
  if(control.href && !control.href.startsWith('#')) return {tier:'external',reason:'link opens a browser or network target'};
  // Mod titles may contain 'Steam', 'download' or 'RimWorld'. A reviewed
  // selection row is distinct from an operation nested inside that row.
  if(control.row&&!cmd&&!write)return {tier:'safe',reason:'reviewed row selection/context'};
  if(control.inputType==='file' || /(?:save as|choose|browse|select) (?:a |the )?(?:file|folder|directory)|open.*(?:browser|steam|folder)|launch|rimworld|steamcmd|download|subscribe|unsubscribe|resubscribe|check for updates|refresh.*(?:workshop|steam)|workshop|steam page|web page|ctrl\+s|工坊|启动游戏|下载|退订|订阅/.test(text)) return {tier:'external',reason:'native picker, external app or network operation'};
  if(externalCommands.has(cmd)||externalCommands.has(write)||/subscribe|steam|download|launch|network|browse|file-picker/.test(write||''))return {tier:'external',reason:'external operation '+(cmd||write)};
  // A cancel remains safe even inside a destructive confirmation.
  if(/^(?:cancel|close|back|skip for now|no|取消|关闭|返回|否|×|✕|←)(?:\s|$)/.test(text))return {tier:'safe',reason:'dismissal'};
  if(['toggle','enable','disable','add','take-out','sort','undo','redo','move-to','enable-at','up','down'].includes(write))return {tier:'safe',reason:'reviewed undoable workspace edit '+write};
  if(localCommands.has(cmd) || /delete|trash|recycle|remove|uninstall|permanent|confirm move|apply|save changes|save list|create list|new list|rename|清空|删除|回收|卸载|确认|保存|重命名/.test(text) || id['data-env-write'])return {tier:'local-destructive',reason:'persistent local write / confirmation; disposable branch required'};
  if(control.owner==='nav'&&(/^(?:plan(?::\d+)?|home|needs|library|changes|log|settings|env|packs|tasks|textures|guide|plans)$/.test(id['data-id']||'')||['navtoggle','nav-settings','tasks-line','env'].includes(id.id)))return {tier:'safe',reason:'reviewed sidebar route/toggle'};
  if(safeCommands.has(cmd))return {tier:'safe',reason:'reviewed local command '+cmd};
  if(control.row)return {tier:'safe',reason:'representative row selection/context'};
  if(/click to copy|left click: only these|view full details|technical information|see all in change history|what is wrong with this list|go back to this version|drag to move, right click: columns|the list.s page in all lists|^clear$|^sidebar$|^location$|^\d+ staging \d+$/i.test(text))return {tier:'safe',reason:'reviewed view, filter, clipboard or undoable history control'};
  if(['text','search','number','password','email','url','textarea'].includes(control.inputType)||control.tag==='TEXTAREA')return {tier:'safe',reason:'focus editable field without submitting'};
  if(control.tag==='SUMMARY'||['tab','separator'].includes(control.role)||control.tag==='SELECT')return {tier:'safe',reason:'view/focus control'};
  if(/^(?:sort|undo|redo|preview|import|export|help|keys|commands|search|filter|filters|all|enabled|disabled|staging|details?|history|problems?|versions?|settings|appearance|backgrounds?|general|advanced|language|about|sources?|games?|lists?|plans?|tasks?|changes?|log|home|textures?|packs?|guide|show|hide|expand|collapse|more|menu|move to|enable at|tags?|note|返回|排序|撤销|重做|导入|导出|详情|帮助)(?:\s|$|…|\.|\d|[↕▼▶▸⌄])/i.test(text))return {tier:'safe',reason:'reviewed navigation, view or cancelable editor'};
  if(control.owner==='popup' && state.popup==='palette')return {tier:'unknown',reason:'palette option requires command classification'};
  return {tier:'unknown',reason:'no reviewed semantic classification'};
}
export function classify(control,state={}) {
  const policy=classifyBase(control,state);
  if(!state.sharedContent)return policy;
  const id=control.identity||{},cmd=id['data-cmd']||id['data-op']||'',write=id['data-env-write'],text=control.label?.trim().toLowerCase()||'';
  const dbOnly=new Set(['create-plan','new-plan','rename-plan','remove-plan','use-plan','plan-unfollow','tags','note']);
  if(state.popupTitle==='delete the list'&&/^delete(?:\s|$)/.test(text))return {tier:'local-destructive',reason:'reviewed plan deletion confirmation; private DB only'};
  const workspace=['toggle','enable','disable','add','take-out','sort','undo','redo','move-to','enable-at','up','down'];
  if((write&&!dbOnly.has(write)&&!workspace.includes(write))||localCommands.has(cmd)||id['data-game']||/use this (?:one|;)|switch to .* and use/.test(text)||/delete|trash|recycle|move to…|clean|uninstall|permanent|apply/.test(text)&&!dbOnly.has(write))return {...policy,blocked:'shared baseline: operation may change content or deployment files; requires a private full copy'};
  return policy;
}
export function allowed(policy,allow) {
  if(typeof policy==='object'){if(policy.blocked)return false;policy=policy.tier;}
  return policy==='safe'||policy==='local-destructive'&&['local','external'].includes(allow)||policy==='external'&&allow==='external';
}
export function stateKey(s,controls=[]) {
  const menu=s.menu?controls.filter(c=>c.owner==='menu'&&c.visible).map(c=>c.label):[];
  return JSON.stringify([s.page||'plan',s.plan,s.popup,s.popupTitle,s.menu,s.page===null?s.focus:null,s.navLayout,s.modes||[],menu]);
}
export function coverage(controls) {
  const pages={};
  for(const c of controls){const p=pages[c.page]||= {discovered:0,eligible:0,asserted:0,passed:0,failed:0,skipped:0,unreachable:0,tiers:{}};p.discovered++;p.tiers[c.tier]=(p.tiers[c.tier]||0)+1;if(c.eligible)p.eligible++;if(c.outcome==='pressed-ok')p.passed++;if(c.asserted)p.asserted++;if(c.outcome==='failed')p.failed++;if(c.outcome==='skipped')p.skipped++;if(c.outcome==='unreachable')p.unreachable++;}
  for(const p of Object.values(pages)){p.coveragePercent=p.eligible?Math.round(p.asserted/p.eligible*10000)/100:null;p.discoveredPassPercent=Math.round(p.passed/p.discovered*10000)/100;}
  return pages;
}
