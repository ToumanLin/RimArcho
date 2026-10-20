// Import and export of the mod list (10 §4): to and from RimSort, RimCrow, RimWorld's
// own .rml, share codes, Rentry and Workshop collections. An import is previewed first —
// what is installed, what is missing and where to get it, what the enabled list becomes —
// and applied as one undo step; nothing here writes ModsConfig.xml (that stays save).
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";

let ui = null;
export function initExchange(u) { ui = u; }

let info = null;  // ExchangeInfo(): dialogs, the typed paths' folder, the formats
let fmt = 0;      // the chosen export format
let pasted = "";  // what the paste box held (kept while the popup is reopened)

// exchange opens the popup on import (0) or export (1).
export async function exchange(tab = 0) {
  try { info = await api.ExchangeInfo(); } catch (e) { return say("fail", errText(e)); }
  pop.open("exchange", T("import / export"), (box, p) => {
    box.classList.add("xc");
    const head = el("div", "xc-head");
    const seg = el("div", "seg");
    const body = el("div", "xc-body");
    [T("import"), T("export")].forEach((name, i) => {
      const b = el("button", i === tab ? "on" : "", name);
      b.onclick = () => { [...seg.children].forEach((x, j) => x.classList.toggle("on", j === i)); draw(i); };
      seg.append(b);
    });
    head.append(el("span", "xc-title", T("mod list exchange")), seg);
    box.append(head, body);
    const draw = (i) => { body.innerHTML = ""; (i === 0 ? importSource : exportView)(body); };
    p.keys = () => true; // typing stays in the boxes; esc closes
    draw(tab);
  });
}

// ---- import, step 1: where the list comes from ----

function importSource(body) {
  body.append(el("div", "xc-note", T("RimSort, RimCrow and RimWorld lists, share codes, packageIds or workshop links. Nothing changes before you confirm.")));

  // a file: the window's open dialog, a typed path, or a dropped file
  const file = section(body, T("from a file"));
  const row = el("div", "xc-row");
  const path = el("input", "xc-input");
  path.placeholder = T("a file path: .xml .rml .json .txt, a save (.rws)");
  path.spellcheck = false;
  const read = el("button", "btn", T("read"));
  read.onclick = () => path.value.trim() && preview("file", path.value.trim());
  path.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); read.click(); } });
  if (info.dialogs) {
    const pick = el("button", "btn primary", T("choose a file…"));
    pick.onclick = async () => {
      let f = "";
      try { f = await api.PickImportFile(); } catch (e) { return say("fail", errText(e)); }
      if (f) { path.value = f; preview("file", f); }
    };
    row.append(pick);
  }
  row.append(path, read);
  file.append(row);
  dropZone(file);

  // pasted text: a list, a share code, a Rentry or collection link
  const paste = section(body, T("paste"));
  const ta = el("textarea", "xc-text");
  ta.spellcheck = false;
  ta.placeholder = T("a list from RimSort or RimCrow, a share code (RC-…), a Rentry link, or a Steam Workshop collection link");
  ta.value = pasted;
  ta.oninput = () => { pasted = ta.value; };
  const btns = el("div", "xc-row end");
  const clip = el("button", "btn", T("paste from the clipboard"));
  clip.onclick = async () => {
    let t = "";
    try { t = await api.Clipboard(); } catch { /* the page asks the browser */ }
    if (!t && navigator.clipboard) { try { t = await navigator.clipboard.readText(); } catch { /* not allowed */ } }
    if (!t) return say("", T("the clipboard has no text"));
    ta.value = pasted = t;
  };
  const go = el("button", "btn primary", T("preview"));
  go.onclick = () => ta.value.trim() && preview("text", ta.value);
  btns.append(el("span", "xc-hint", T("links are read from Steam or Rentry when you press preview")), clip, go);
  paste.append(ta, btns);
  setTimeout(() => (info.dialogs ? ta : path).focus(), 0);
}

// dropZone: a file dropped on the section is read here and previewed as text (a dropped
// file has no path in the page; a save's mod list is in its first lines).
function dropZone(sec) {
  sec.classList.add("drop");
  sec.append(el("div", "xc-hint", T("or drop a file here")));
  sec.addEventListener("dragover", (e) => { e.preventDefault(); sec.classList.add("over"); });
  sec.addEventListener("dragleave", () => sec.classList.remove("over"));
  sec.addEventListener("drop", async (e) => {
    e.preventDefault();
    sec.classList.remove("over");
    const f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
    if (!f) return;
    const blob = /\.rws$/i.test(f.name) ? f.slice(0, 4 << 20) : f;
    preview("text", await blob.text(), f.name);
  });
}

// ---- import, step 2: the preview ----

async function preview(kind, src, name) {
  const body = $(".xc-body");
  if (!body) return;
  say("busy", kind === "text" && /^\s*\S+\s*$/.test(src) && /steamcommunity|rentry|^\s*\d{6,}\s*$/.test(src) ? T("reading the link…") : T("reading the list…"));
  let p;
  try { p = await api.ImportPreview(kind, src); } catch (e) { return say("fail", errText(e)); }
  say("", "");
  if (name) p.source = name;
  body.innerHTML = "";

  const top = el("div", "xc-sum");
  top.append(el("div", "xc-big", p.summary));
  const facts = [p.source];
  if (p.name) facts.push(Tf("list: %s", p.name));
  if (p.game) facts.push("RimWorld " + p.game);
  top.append(el("div", "xc-note", facts.filter(Boolean).join(" · ")));
  if (p.gameNote) top.append(el("div", "xc-note warn", p.gameNote));
  body.append(top);

  // the two ways to apply it, each with what it does
  const ways = el("div", "xc-ways");
  ways.append(
    way(T("replace the enabled list"), p.replace, true, () => apply(false)),
    way(T("add the ones you lack"), p.merge, false, () => apply(true)),
  );
  body.append(ways);
  body.append(el("div", "xc-hint", T("one step: u undoes it; the game's ModsConfig.xml changes only when you save")));

  if (p.missing.length) {
    const sec = section(body, Tf("not installed: %d", p.missing.length));
    const list = el("div", "xc-list");
    for (const r of p.missing) {
      const d = el("div", "xc-mod");
      const n = el("div", "n");
      n.append(el("span", "", r.name));
      if (r.package && r.package !== r.name) n.append(el("span", "dim", r.package));
      d.append(n);
      if (r.instead) {
        const i = el("div", "xc-instead", Tf("replaced by %s", r.instead));
        if (r.insteadUrl) i.append(link(T("Workshop page"), r.insteadUrl));
        d.append(i);
      }
      if (r.url) d.append(link(T("subscribe"), r.url, true));
      else d.append(el("span", "dim", T("not on the Workshop list")));
      list.append(d);
    }
    sec.append(list);
  }
  if (p.installed.length) {
    const det = el("details", "xc-sec");
    det.append(el("summary", "", Tf("installed: %d (%d enabled now)", p.installed.length, p.installed.filter((r) => r.enabled).length)));
    const list = el("div", "xc-list");
    p.installed.forEach((r, i) => {
      const d = el("div", "xc-mod");
      d.append(el("span", "pos", String(i + 1)), el("span", r.enabled ? "" : "accent", r.name));
      if (!r.enabled) d.append(el("span", "dim", T("not enabled now")));
      list.append(d);
    });
    det.append(list);
    body.append(det);
  }
  const back = el("div", "xc-row end");
  const b = el("button", "btn", T("back"));
  b.onclick = () => { body.innerHTML = ""; importSource(body); };
  back.append(b);
  body.append(back);
}

function way(title, e, primary, run) {
  const d = el("div", "xc-way");
  d.append(el("div", "t", title), el("div", "xc-note", e.text));
  const b = el("button", "btn" + (primary ? " primary" : ""), title);
  b.disabled = e.same;
  b.onclick = run;
  d.append(b);
  return d;
}

function link(text, url, button) {
  const a = el(button ? "button" : "a", button ? "btn small" : "xc-link", text);
  a.title = url;
  a.onclick = (e) => { e.preventDefault(); api.OpenURL(url).catch((x) => say("fail", errText(x))); };
  return a;
}

async function apply(merge) {
  let t;
  try { t = await api.ImportApply(merge); } catch (e) { return say("fail", errText(e)); }
  pop.close();
  for (const c of S.cols) c.sel.clear();
  await ui.reload();
  say("ok", t, true);
}

// ---- export ----

function exportView(body) {
  const fs = info.formats;
  const sec = section(body, T("format"));
  const list = el("div", "xc-formats");
  const draw = () => {
    list.innerHTML = "";
    fs.forEach((f, i) => {
      const b = el("button", "xc-fmt" + (i === fmt ? " on" : ""));
      b.append(el("span", "t", f.name), el("span", "xc-note", f.desc));
      b.onclick = () => { fmt = i; draw(); dest(); };
      list.append(b);
    });
  };
  sec.append(list);
  const to = section(body, T("destination"));
  const out = el("div", "xc-out");
  body.append(out);
  const dest = () => {
    const f = fs[fmt];
    to.querySelectorAll(":scope > :not(.xc-sec-t)").forEach((x) => x.remove());
    const row = el("div", "xc-row");
    if (info.dialogs) {
      const save = el("button", "btn primary", T("save as a file…"));
      save.onclick = async () => {
        let path = "";
        try { path = await api.PickExportFile(f.id); } catch (e) { return say("fail", errText(e)); }
        if (path) write(f, path, true);
      };
      row.append(save);
    }
    const path = el("input", "xc-input");
    path.spellcheck = false;
    path.value = (info.dir ? info.dir + (info.dir.includes("/") ? "/" : "\\") : "") + f.file;
    const w = el("button", "btn" + (info.dialogs ? "" : " primary"), T("write"));
    w.onclick = () => path.value.trim() && write(f, path.value.trim(), false);
    path.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); w.click(); } });
    row.append(path, w);
    to.append(row);
    if (f.text) {
      const r2 = el("div", "xc-row");
      const c = el("button", "btn", T("copy to the clipboard"));
      c.onclick = () => copy(f);
      r2.append(c, el("span", "xc-hint", T("to paste in a chat or in another manager")));
      to.append(r2);
    }
  };
  const write = async (f, path, overwrite) => {
    try {
      const t = await api.ExportSave(f.id, path, overwrite);
      out.textContent = t;
      out.className = "xc-out ok";
      say("ok", t);
    } catch (e) { out.textContent = errText(e); out.className = "xc-out err"; }
  };
  const copy = async (f) => {
    let c;
    try { c = await api.ExportCopy(f.id); } catch (e) { out.textContent = errText(e); out.className = "xc-out err"; return; }
    let ok = c.copied;
    if (!ok && navigator.clipboard) { try { await navigator.clipboard.writeText(c.text); ok = true; } catch { /* shown below */ } }
    out.innerHTML = "";
    out.className = "xc-out " + (ok ? "ok" : "");
    out.append(el("div", "", ok ? c.line : T("copy it from here:")));
    const ta = el("textarea", "xc-text small");
    ta.readOnly = true;
    ta.value = c.text;
    out.append(ta);
    if (!ok) ta.select();
    say(ok ? "ok" : "", ok ? c.line : T("copy it from here:"));
  };
  draw();
  dest();
}

function section(body, title) {
  const s = el("div", "xc-sec");
  s.append(el("div", "xc-sec-t", title));
  body.append(s);
  return s;
}
