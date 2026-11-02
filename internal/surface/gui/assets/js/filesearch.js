// Mod library › Search files (06 §2.9). Query completion and filters are shared with the library.
import { S, $, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { beginPageRead, page, area, button, hintsChanged } from "./screens.js";
import { copyablePath, copy } from "./fields.js";
import { createBar } from "./filterbar.js";
import { queryParts } from "./searchsuggest.js";
import { wireSeparator } from "./separator.js";
import { virtualRows } from "./virtualrows.js";
import { syntaxParts } from "./searchsyntax.js";

const kinds = ["C#", "Defs", "patches", "About", "translations", "textures", "sounds"];
const X = {text:"",files:"Defs patches About",mods:"",case:false,regex:false,initialized:false,env:null,
  groups:[],modsFound:[],byID:new Map(),rows:[],expanded:new Set(),cursor:0,selected:null,line:0,
  at:null,update:null,running:false,request:null,since:0,seq:0,visit:0,readSeq:0,filterSeq:0,
  wrap:false,wrapTouched:false,timer:0,filterTimer:0};
let root, queryCard, resultsCard, viewerCard, tree, summary, notes, stopButton, validation;
let textInput, filesBar, modsBar, treeRows, codeRows, observer;
let begins=Promise.resolve();
const visible=()=>S.screen==="search" && root?.isConnected;
const consume=e=>{e.preventDefault();e.stopPropagation();};

export async function open() {
  leave();
  const visit=++X.visit;
  const read=beginPageRead("search",{label:"loading…",retry:open});
  root=page("filesearch","");root.replaceChildren();
  const left=el("div","fs-left"),gap=el("div","fs-split");
  queryCard=el("section","card fs-query");
  resultsCard=el("section","card fs-results");
  viewerCard=el("section","card fs-viewer");
  left.append(queryCard,resultsCard);root.append(left,gap,viewerCard);
  $("#screen").replaceChildren(root);
  area(queryCard,null,focusInput);area(resultsCard,null,focusResults);
  buildQuery();buildResults();drawViewer();
  let side=null;
  const width=()=>root.getBoundingClientRect().width;
  const place=()=>root.style.setProperty("--fs-side",side===null?"32%":side+"px");
  wireSeparator(gap,{key:"ui.gui.search.side",enabled:()=>innerWidth>=900,
    range:()=>[260,Math.max(260,width()-300)],get:()=>side??width()*.32,
    set:v=>{side=v;place();},reset:()=>{side=null;place();},serialize:()=>side===null?"":String(side),
    restore:v=>{side=v && Number.isFinite(+v)?+v:null;place();}});
  place();
  observer=new MutationObserver(()=>{if(!visible())leave();});
  observer.observe($("#screen"),{childList:true});
  read.finish();
  api.Setting("ui.gui.search.wrap").then(v=>{
    if(visible() && visit===X.visit && !X.wrapTouched){X.wrap=v==="true";applyWrap();}
  }).catch(()=>{});
  try {
    const info=await api.FileSearchInfo();
    if(!visible() || visit!==X.visit)return;
    if(!X.initialized || X.env!==S.envSeq){X.files=info.files;X.mods=info.mods;X.env=S.envSeq;X.initialized=true;}
    X.groups=info.groups || [];
    filesBar.setText(X.files);modsBar.setText(X.mods);refreshPanel();
    if(X.mods!==info.mods)refreshFilters();
  } catch(e){if(visible() && visit===X.visit)say("fail",errText(e));}
  hintsChanged();
}

function fileGroup(target=false) {
  const parts=queryParts(X.files.trim() || "Defs patches About");
  return {key:"files",title:T("files"),noCounts:true,...(target?{target:"files"}:{}),
    values:kinds.map(value=>({filter:value,label:T(value),count:0,
      on:parts.find(p=>p.value.toLowerCase()===value.toLowerCase())?.negative?-1:
        parts.some(p=>p.value.toLowerCase()===value.toLowerCase())?1:0}))};
}
function panelData() {return {groups:[fileGroup(true),...X.groups]};}
function fieldRow(label,content) {
  const row=el("div","fs-field"),name=el("label","fs-label",T(label));
  const input=content.tagName==="INPUT"?content:content.querySelector("input");
  if(input){input.id="fs-"+label;input.setAttribute("aria-label",T(label));name.htmlFor=input.id;}
  row.append(name,content);return row;
}
function buildQuery() {
  const fields=el("div","fs-fields"),top=el("div","fs-text-box");
  textInput=el("input","fs-input");textInput.spellcheck=false;textInput.value=X.text;textInput.placeholder=T("text to find");
  textInput.oninput=()=>{X.text=textInput.value;};
  textInput.onkeydown=e=>{
    if(e.isComposing)return;
    if(shortcuts(e)){e.stopPropagation();return;}
    if(e.key==="Enter"){consume(e);run();}
    else if(e.key==="ArrowDown"){consume(e);filesBar.focus();}
    else if(e.key==="Escape"){consume(e);if(X.running)stop();else{ textInput.blur();focusResults(); }}
  };
  top.append(textInput);
  for(const [name,label,title,key] of [["case","Aa","case sensitive","alt+c"],["regex",".*","regular expression","alt+r"]]){
    const b=button(label,"",()=>toggle(name),"quiet small fs-toggle");
    b.dataset.option=name;b.title=T(title);b.setAttribute("aria-label",T(title));
    b.append(keyCaps(key));b.setAttribute("aria-pressed",String(X[name]));top.append(b);
  }
  fields.append(fieldRow("search",top));
  const key=e=>{if(e.key==="Escape"&&X.running){consume(e);stop();return true;}return shortcuts(e);};
  filesBar=createBar({bare:true,noPanel:true,placeholder:T("files"),data:()=>({groups:[fileGroup()]}),
    change:(_open,reason)=>{X.files=filesBar.input.value;validateKinds();refreshPanel();if(reason==="filter"&&X.text.trim())run();},
    submit:run,key,toggle:()=>modsBar.toggle()});
  fields.append(fieldRow("files",filesBar.el));
  validation=el("p","fs-validation");validation.hidden=true;validation.id="fs-file-error";filesBar.input.setAttribute("aria-describedby",validation.id);
  fields.append(validation);
  modsBar=createBar({placeholder:T("mods"),data:panelData,targets:{files:filesBar},focusRoot:()=>queryCard,deferOutside:true,
    splitKey:"ui.gui.search.filters",count:()=>queryParts(X.files).length+queryParts(X.mods).length,
    change:(onlyOpen,reason)=>{if(!onlyOpen){X.mods=modsBar.input.value;refreshFilters();if(reason==="filter"&&X.text.trim())run();}},
    submit:run,key,leave:focusResults});
  const modsLabel=el("label","fs-label",T("mods"));modsLabel.htmlFor="fs-mods";
  modsBar.input.id="fs-mods";modsBar.input.setAttribute("aria-label",T("mods"));
  modsBar.el.querySelector(".fbar-row").prepend(modsLabel);
  fields.append(modsBar.el);queryCard.append(fields);
  filesBar.setText(X.files);modsBar.setText(X.mods);refreshPanel();
}
function validateKinds(){
  const unknown=X.files.trim().split(/\s+/).filter(Boolean).find(w=>!kinds.some(k=>k.toLowerCase()===w.replace(/^-/,'').toLowerCase()));
  validation.textContent=unknown?Tf("unknown file kind: %s",unknown):"";
  validation.hidden=!unknown;filesBar.input.setAttribute("aria-invalid",String(!!unknown));
  return !unknown;
}
function refreshPanel(){modsBar?.draw();filesBar?.draw();}
function refreshFilters(){
  clearTimeout(X.filterTimer);const seq=++X.filterSeq,visit=X.visit,query=X.mods;
  X.filterTimer=setTimeout(async()=>{
    try{const data=await api.FileSearchFilters(query);
      if(!visible()||visit!==X.visit||seq!==X.filterSeq)return;
      X.groups=data.groups||[];refreshPanel();
    }catch(e){if(visible()&&seq===X.filterSeq)say("fail",errText(e));}
  },90);
}
function toggle(name){X[name]=!X[name];queryCard.querySelector(`[data-option="${name}"]`)?.setAttribute("aria-pressed",String(X[name]));if(X.text.trim())run();}
function request(){return {pattern:X.text,files:X.files,modsQuery:X.mods,case:X.case,regex:X.regex};}

function buildResults(){
  const head=el("div","fs-result-head");
  summary=el("span","fs-summary muted");summary.setAttribute("aria-live","polite");
  stopButton=button(T("stop"),"esc",stop,"quiet small fs-stop");stopButton.hidden=true;
  head.append(el("h2","",T("results")),summary,stopButton);
  notes=el("div","fs-notes muted");
  tree=el("div","fs-tree");tree.tabIndex=0;tree.setAttribute("role","tree");tree.setAttribute("aria-label",T("results"));
  tree.onkeydown=e=>{if(key(e))e.stopPropagation();};
  resultsCard.append(head,notes,tree);
}
export function applyDeltas(deltas){
  for(const delta of deltas || []){
    let mod=X.byID.get(delta.id);
    if(!mod){mod={id:delta.id,name:delta.name,copy:delta.copy,count:0,files:[]};X.byID.set(mod.id,mod);X.modsFound.push(mod);}
    const file={...delta.file,mod,id:delta.id+"\n"+delta.file.path};
    mod.files.push(file);mod.count+=file.count;
  }
}
function flatten(){
  const rows=[];
  for(const mod of X.modsFound){
    rows.push({type:"mod",mod});
    for(const file of mod.files){
      rows.push({type:"file",mod,file,line:0});
      if(X.expanded.has(file.id))for(const hit of file.lines || [])rows.push({type:"line",mod,file,line:hit.line,hit});
    }
  }
  return rows;
}
function many(file){return file.count>1 && file.lines?.some(l=>l.line>0);}
function drawTree(){
  const saved=treeRows?.snapshot(),selected=X.rows[X.cursor];treeRows?.destroy();treeRows=null;
  X.rows=flatten();
  if(selected?.file){const i=X.rows.findIndex(r=>r.file?.id===selected.file.id&&r.line===selected.line);if(i>=0)X.cursor=i;}
  X.cursor=Math.max(0,Math.min(X.rows.length-1,X.cursor));
  tree.replaceChildren();
  if(!X.rows.length){
    if(X.update?.done && !X.update.error){tree.append(el("p","fs-empty muted",T("not found")));
      if(X.update.hint){tree.append(el("p","fs-note muted",X.update.hint));if(X.at?.modsQuery.trim())tree.append(button(T("search every installed copy"),"",()=>{X.mods="";modsBar.setText("");refreshFilters();run();},"quiet small fs-widen"));}}
    return;
  }
  const sizes=new Float64Array(X.rows.length).fill(24);
  treeRows=virtualRows(tree,X.rows.length,renderTreeRow,{heights:sizes});
  if(saved)tree.scrollTop=saved.scrollTop;
  if(!X.selected){const first=X.rows.findIndex(r=>r.type==="file");if(first>=0)selectRow(first,false);}
  treeRows.select(X.cursor,false);
}
function renderTreeRow(i){
  const row=X.rows[i],node=el(row.type==="mod"?"div":"button","fs-tree-row fs-"+row.type);
  node.setAttribute("role","treeitem");node.setAttribute("aria-level",String(row.type==="mod"?1:row.type==="file"?2:3));
  if(row.type==="mod"){
    node.append(el("span","fs-mod-name",row.mod.name),el("span","fs-copy muted",row.mod.copy),el("span","fs-count muted",String(row.mod.count)));
    node.setAttribute("aria-expanded","true");return node;
  }
  node.tabIndex=-1;
  const selected=X.rows[X.cursor]===row;
  node.classList.toggle("selected",selected);node.setAttribute("aria-selected",String(selected));
  if(row.type==="file"){
    const expandable=many(row.file),expanded=X.expanded.has(row.file.id);
    const chevron=el("span","fs-chevron"+(expanded?" open":"")+(!expandable?" none":""),"›");chevron.setAttribute("aria-hidden","true");
    const path=el("span","fs-path"),at=row.file.rel.lastIndexOf("/");
    path.append(el("span","muted",row.file.rel.slice(0,at+1)),document.createTextNode(row.file.rel.slice(at+1)));
    node.append(chevron,path,el("span","fs-count muted",String(row.file.count)));
    if(expandable)node.setAttribute("aria-expanded",String(expanded));
    node.onclick=()=>{if(expandable){expanded?X.expanded.delete(row.file.id):X.expanded.add(row.file.id);}selectRow(i,false);drawTree();tree.focus({preventScroll:true});};
  }else{
    node.append(el("span","fs-line-number muted",String(row.line)));
    const text=el("span","fs-line-text");
    appendParts(text,row.hit.parts || [{text:row.hit.text}],false);node.append(text);
    node.onclick=()=>{selectRow(i,false);tree.focus({preventScroll:true});};
  }
  return node;
}
function drawStatus(){
  summary.textContent=X.update?.summary || "";stopButton.hidden=!X.running;
  notes.replaceChildren(...(X.update?.notes || []).map(text=>el("p","fs-note",text)));
  if(X.update?.error)notes.append(el("p","fs-note fail",X.update.error));
  hintsChanged();
}
export async function run(){
  X.text=textInput.value;X.files=filesBar.input.value;X.mods=modsBar.input.value;
  if(!validateKinds())return;
  if(!X.text.trim()){say("fail",T("type what to look for"));return;}
  const q=request(),seq=++X.seq,visit=X.visit;
  clearTimeout(X.timer);X.running=true;X.stopWanted=false;X.request=null;X.since=0;
  X.at=q;X.modsFound=[];X.byID.clear();X.rows=[];X.expanded.clear();X.cursor=0;X.selected=null;X.line=0;
  X.update={summary:"",done:false};++X.readSeq;drawTree();drawViewer();drawStatus();
  const current=()=>visible()&&visit===X.visit&&seq===X.seq;
  try{
    const n=await(begins=begins.catch(()=>{}).then(()=>api.BeginFileSearch()));
    if(!current()){api.StopFileSearch(n).catch(()=>{});return;}
    X.request=n;await api.StartFileSearch(q,n);
    if(X.stopWanted)await api.StopFileSearch(n);
    if(current())poll(seq,visit,n);
  }catch(e){if(current()){X.running=false;say("fail",errText(e));drawStatus();}}
}
async function poll(seq,visit,n){
  if(!visible()||seq!==X.seq||visit!==X.visit)return;
  try{
    const update=await api.FileSearchUpdate(n,X.since);
    if(!visible()||seq!==X.seq||visit!==X.visit)return;
    X.since=update.next;X.update=update;X.running=!update.done;
    if(update.deltas?.length){applyDeltas(update.deltas);drawTree();}
    else if(update.done && !X.rows.length)drawTree();
    drawStatus();
    if(!update.done)X.timer=setTimeout(()=>poll(seq,visit,n),300);
  }catch(e){if(visible()&&seq===X.seq){X.running=false;drawStatus();say("fail",errText(e));}}
}
function stop(){X.stopWanted=true;if(X.running && X.request!==null)api.StopFileSearch(X.request).catch(e=>say("fail",errText(e)));}
export function leave(){
  ++X.seq;++X.visit;++X.readSeq;++X.filterSeq;
  clearTimeout(X.timer);clearTimeout(X.filterTimer);
  if(X.request!==null)api.StopFileSearch(X.request).catch(()=>{});
  X.request=null;X.running=false;X.selected=null;fileText=null;X.update=null;X.modsFound=[];X.byID.clear();X.rows=[];X.expanded.clear();
  observer?.disconnect();observer=null;treeRows?.destroy();treeRows=null;codeRows?.destroy();codeRows=null;
  filesBar?.destroy();modsBar?.destroy();filesBar=modsBar=null;
}
function focusResults(){tree?.focus();if(X.rows.length && X.rows[X.cursor]?.type==="mod")selectRow(X.rows.findIndex(r=>r.type==="file"));}
export function focusInput(){textInput?.focus();textInput?.select();}
function selectRow(i,reveal=true){
  const row=X.rows[i];if(!row?.file)return;
  X.cursor=i;treeRows?.select(i,reveal);
  const changed=X.selected?.id!==row.file.id;
  X.selected=row.file;X.line=row.type==="line"?row.line:(row.file.lines?.[0]?.line||0);
  // Selection reads just this file; repeated line selections reuse its bounded text.
  if(changed)readFile(row.file);else markSelected();
}
let fileText=null;
async function readFile(file){
  const seq=++X.readSeq,visit=X.visit;fileText=null;drawViewer();
  try{
    const data=await api.ReadSearchFile(file.mod.id,file.path,X.at);
    if(!visible()||seq!==X.readSeq||visit!==X.visit)return;
    fileText=data;drawViewer();markSelected();
  }catch(e){if(visible()&&seq===X.readSeq)say("fail",errText(e));}
}
function appendParts(node,parts,syntax=true){
  for(const part of parts){
    if(part.hit){const mark=el("mark");mark.textContent=part.text;node.append(mark);}
    else if(syntax)for(const token of syntaxParts(part.text,fileText?.type?.includes("C#")?"cs":"xml")){const span=el("span",token.cls);span.textContent=token.text;node.append(span);}
    else node.append(document.createTextNode(part.text));
  }
}
function drawViewer(){
  codeRows?.destroy();codeRows=null;viewerCard.replaceChildren();
  if(!X.selected){fileText=null;viewerCard.append(el("p","fs-view-empty muted",T("select a file")));return;}
  const file=X.selected,head=el("div","fs-view-head"),top=el("div","fs-view-title"),actions=el("div","fs-view-actions");
  const name=file.path.split(/[\\/]/).pop();
  actions.append(button(T("open file"),"enter",()=>openFile(false),"small primary"),button(T("show in folder"),"ctrl+o",()=>openFile(true),"small"));
  top.append(el("h2","",name),actions);
  const where=el("div","fs-view-path");where.textContent=file.path;copyablePath(where,file.path);
  head.append(top,where);viewerCard.append(head);
  if(!fileText || fileText.binary)return;
  const body=el("div","fs-code"+(X.wrap?" wrap":""));body.tabIndex=0;body.setAttribute("role","region");body.setAttribute("aria-label",T("file contents"));
  viewerCard.append(body);
  codeRows=virtualRows(body,fileText.lines.length,i=>{
    const row=fileText.lines[i],node=el("div","fs-code-line"),text=el("span","fs-code-text");
    node.dataset.line=String(row.number);node.classList.toggle("hit",fileText.hits.includes(row.number));
    node.append(el("span","fs-code-number",String(row.number)));appendParts(text,row.parts);node.append(text);
    return node;
  },{heights:new Float64Array(fileText.lines.length).fill(20.4)});
  if(fileText.note)viewerCard.append(el("p","fs-view-note muted",fileText.note));
  const foot=el("div","fs-view-foot"),match=el("span","fs-match-position");
  if(fileText.hits.length){
    foot.append(match,button(T("next"),"n",()=>nextMatch(1),"quiet small"),button(T("previous"),"N",()=>nextMatch(-1),"quiet small"));
  }
  foot.append(el("span","fs-file-type muted",fileText.source?T("read-only: decompiled code from the source library"):fileText.type));
  const wrap=button(T("wrap lines"),"alt+z",toggleWrap,"quiet small fs-wrap");wrap.setAttribute("aria-pressed",String(X.wrap));foot.append(wrap);viewerCard.append(foot);
  markSelected();
}
function markSelected(){
  if(!fileText)return;
  const at=fileText.hits.indexOf(X.line),label=viewerCard.querySelector(".fs-match-position");
  if(label)label.textContent=Tf("match %d of %d",at<0?1:at+1,fileText.hits.length);
  const index=fileText.lines.findIndex(l=>l.number===X.line);
  if(index>=0)codeRows?.select(index);
  // Virtual rows use cur for the selected file line, with no extra count sentence.
}
function nextMatch(d){if(!fileText?.hits.length)return;const hits=fileText.hits,at=hits.indexOf(X.line);X.line=hits[(Math.max(0,at)+d+hits.length)%hits.length];markSelected();}
function toggleWrap(){X.wrap=!X.wrap;X.wrapTouched=true;applyWrap();api.SetSetting("ui.gui.search.wrap",String(X.wrap)).catch(e=>say("fail",errText(e)));}
function applyWrap(){viewerCard?.querySelector(".fs-code")?.classList.toggle("wrap",X.wrap);viewerCard?.querySelector(".fs-wrap")?.setAttribute("aria-pressed",String(X.wrap));}
function openFile(folder){const file=X.selected;if(!file)return;(folder?api.OpenSearchFolder(file.mod.id,file.path):api.OpenSearchFile(file.mod.id,file.path)).catch(e=>say("fail",errText(e)));}
function shortcuts(e){
  const ctrl=e.ctrlKey||e.metaKey,k=e.key.toLowerCase();
  if(ctrl&&!e.altKey&&k==="f"){consume(e);focusInput();return true;}
  if(e.altKey&&!ctrl){
    if(k==="c"||k==="r"){consume(e);toggle(k==="c"?"case":"regex");return true;}
    if(k==="z"){consume(e);toggleWrap();return true;}
  }
  return false;
}
export function key(e){
  if(shortcuts(e))return true;
  const k=e.key,ctrl=e.ctrlKey||e.metaKey;
  if(k==="Escape"&&X.running){consume(e);stop();return true;}
  if(e.target?.tagName==="INPUT"||e.altKey)return false;
  if((k==="Enter"||k===" ")&&e.target?.closest?.("button")&&!tree?.contains(e.target))return false;
  if(k==="\\"){consume(e);modsBar.toggle();return true;}
  if(k==="/"){consume(e);focusInput();return true;}
  if(ctrl&&k.toLowerCase()==="o"&&X.selected){consume(e);openFile(true);return true;}
  if(ctrl&&k.toLowerCase()==="c"&&X.selected){consume(e);copy(X.selected.path);return true;}
  if(k==="Enter"&&X.selected){consume(e);openFile(false);return true;}
  if((k==="n"||k==="N")&&fileText?.hits.length){consume(e);nextMatch(k==="n"?1:-1);return true;}
  const row=X.rows[X.cursor];
  if(!row)return false;
  if((k==="ArrowRight"||k==="ArrowLeft")&&row.file){
    consume(e);
    if(k==="ArrowRight"&&many(row.file))X.expanded.add(row.file.id);
    if(k==="ArrowLeft"){X.expanded.delete(row.file.id);const i=X.rows.findIndex(r=>r.type==="file"&&r.file.id===row.file.id);if(i>=0)X.cursor=i;}
    drawTree();return true;
  }
  let i=X.cursor,d=0;
  if(k==="ArrowDown"||k==="j")d=1;
  else if(k==="ArrowUp"||k==="k")d=-1;
  else if(k==="Home"){i=0;d=1;}
  else if(k==="End"){i=X.rows.length-1;d=-1;}
  else if(k==="PageDown")d=Math.max(1,Math.floor(tree.clientHeight/24));
  else if(k==="PageUp")d=-Math.max(1,Math.floor(tree.clientHeight/24));
  else return false;
  consume(e);
  if(k!=="Home"&&k!=="End")i=Math.max(0,Math.min(X.rows.length-1,i+d));
  while(X.rows[i]?.type==="mod"){const next=i+(d<0?-1:1);if(next<0||next>=X.rows.length)return true;i=next;}
  selectRow(i);return true;
}
export function hintList(){
  const list=[{key:"↑↓",label:T("move the cursor")}];
  if(X.selected)list.push({key:"enter",label:T("open file")},{key:"ctrl+o",label:T("show in folder")},{key:"ctrl+c",label:T("copy the path")});
  if(fileText?.hits.length)list.push({key:"n N",label:T("next / previous match")});
  if(X.running)list.push({key:"esc",label:T("stop")});
  list.push({key:"\\",label:T("filter")},{key:"?",label:T("all keys")});return list;
}
