// The detail (16 §4): who the mod is, its state, one field list (fields.js), the
// description, the history. The description is built from Go's BBCode blocks with
// elements and textContent only: outside text is never inserted as HTML (16 §8.2).
import { factText, S, $, el, esc, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { icon, sourceIcon, statusIcon } from "./icons.js";
import { fieldList } from "./fields.js";

import * as ops from "./ops.js";
import { versionNotice } from "./versionwarnings.js";

let seq = 0, detailTimer;

// A details pane changing Mods keeps the one shown until the next is ready, then swaps once
// (16 §4.1): drawing each part as it came moved everything below it, up to four times a
// move. The old Mod is inert at once (its actions cannot run) and dims if the wait is long.
// REVEAL_MS caps the wait: a slower part fills in place after the swap.
const REVEAL_MS = 250;
export function stale(box) { if (box) { box.dataset.stale = ""; box.inert = true; } }
export function unstale(box) { if (box) { delete box.dataset.stale; box.inert = false; } }

// Cursor bursts keep only the last detail read. Invalidate the previous response
// immediately so an abandoned row cannot refill the pane while this one waits.
export function followDetail(it, staged) {
 clearTimeout(detailTimer);
 ++seq;
 S.detail = null;
 if (!it || it.missing || !it.id) return showDetail(it, staged);
 stale($("#detail"));
 const game = S.envSeq;
 detailTimer = setTimeout(() => {
  if (game === S.envSeq && !S.screen) showDetail(it, staged);
 }, 75);
}


// showDetail draws the plan editor's details of a row (16 §4.1): its copies, name and
// picture, the detail, and the copy's note and actions, shown together once Go has answered
// (REVEAL_MS at most). A Workshop page not read yet is fetched straight away.
export async function showDetail(it, staged) {
 clearTimeout(detailTimer);
  const n = ++seq, gameRequest = S.envSeq;
  const current = () => n === seq && S.envSeq === gameRequest;
  const box = $("#detail"), sc = $(".detail .scroll");
  if (!it) { box.innerHTML = ""; delete box.dataset.ref; editorFrame = null; S.detail = null; unstale(box); return; }
  if (it.missing || !it.id) {
    S.detail = null;
    box.innerHTML = "";
    delete box.dataset.ref;
    editorFrame = null;
    unstale(box);
    const pkg = (it.ref || "").replace(/^pkg:/, "");
    box.append(el("h1", "", it.name), ...(pkg && pkg !== it.name ? [el("p", "second", pkg)] : []), el("p", "second", T(it.canUse ? "an installed copy can take its place: use it from the list's problems" : "listed in ModsConfig.xml, but not installed")));
    return;
  }
  // The frame must be this Mod's: after A→B→A the unshown frame is still B's (its header, its note closures).
  const same = box.dataset.ref === it.id && editorFrame && editorFrame.ref === it.id && editorFrame.box === box && box.firstChild;
  const f = same ? editorFrame : (editorFrame = detailFrame(box, sc, {
    fieldActs: (d) => ({
      tags: () => document.dispatchEvent(new CustomEvent("verb", { detail: "tags" })),
      folder: () => api.OpenFolder(d.id).catch((e) => say("fail", errText(e))),
      tagOrder: (list) => api.SetTagOrder(d.id, list).then((t) => { say("ok", t); document.dispatchEvent(new CustomEvent("tags-changed")); }, (e) => say("fail", errText(e))),
    }),
    run: (code, i) => document.dispatchEvent(new CustomEvent("detail-op", { detail: { code, id: i.id || it.id } })),
    noteSaved: async () => { const i = await api.LibraryCopy(it.id).catch(() => null); if (current() && i) f.inst(i); },
    tech: true,
    later: !same,
    host: box,
  }));
  f.ref = it.id;
  f.copies(editorCopies(it));
  if (!same) f.head(it.name, "", it.id);
  // the copy's note and actions come from Mod library's data, rebuilt after an edit: not while
  // edits are on their way (the reload after the last one draws the details again)
  const copy = S.editing ? null : api.LibraryCopy(it.id).then((i) => { if (current() && i) f.inst(i); }, () => {});
  const detail = api.Detail(it.id, !!staged, false, false).then((d) => {
    if (!current()) return null;
    S.detail = d;
    f.detail(d);
    return d;
  }, (e) => { if (current()) f.error(errText(e)); return null; });
  await f.settle([copy, detail]);
  if (!current()) return;
  box.dataset.ref = it.id;
  f.show();
  const d = await detail;
  if (!current() || d?.state !== "not cached") return;
  f.fetching();
  try {
    const d2 = await api.Detail(it.id, !!staged, true, false);
    if (current()) { S.detail = d2; f.detail(d2); }
  } catch { /* the note stays */ }
}
let editorFrame = null;

// refreshCopies draws the shown copies again from the lists, after a change made here
// before Go answers (use this copy: the "in the current list" mark moves at once).
export function refreshCopies() {
  const f = editorFrame;
  if (!f || f.box !== $("#detail") || $("#detail").dataset.ref !== f.ref) return;
  const it = S.cols.flatMap((c) => c.items).find((x) => x.id === f.ref);
  if (it) f.copies(editorCopies(it));
}

// editorCopies: the row's copies in the editor's lists, the one shown marked; the one the
// current list uses says so, the others offer to take its place (16 §4.1).
function editorCopies(it) {
  if (!it.lineage) return [];
  const copies = S.cols.flatMap((c) => c.items).filter((x) => x.id && x.lineage === it.lineage);
  if (copies.length < 2) return [];
  const active = new Set(S.cols[1].items.map((x) => x.id));
  return copies.map((x) => ({
    label: x.copyLabel || x.name, on: x.id === it.id, active: active.has(x.id), dim: !!x.dim,
    view: () => showDetail(x, S.cols[2].items.some((y) => y.id === x.id)),
    use: () => document.dispatchEvent(new CustomEvent("editor-use-copy", { detail: x.id })),
  }));
}

// detailFrame lays out a copy's details, the same on the plan editor and on Mod library (06
// §4.1, §2.9), top to bottom: the copies to choose from (the editor), the copy's note, its
// picture, name and authors, where it comes from and its version, the field list, the
// copy's actions, the description, the history. Each part is filled when its data comes:
//   head(name, authors, id)  what the row already knows
//   inst(row)                the copy's library row: its note and its ops
//   detail(d)                the Detail: header, fields, description, history
//   fields(list)             a field list of its own (Mod library's, with its extra fields)
// o: fieldActs(d, row), run(code, row), noteSaved(), noteLeave(), tech (the full detail's
// link), ownFields (the fields come through fields(), not with the Detail).
// With o.later the parts are built off screen and replace box's content only at show()
// (o.mount() instead, when given); settle(waits) waits for them and the picture, REVEAL_MS at
// most. show() also takes o.host out of its stale state.
export function detailFrame(box, sc, o = {}) {
  const parts = [];
  const part = (cls) => { const e = el("div", "md-" + cls); parts.push(e); return e; };
  const P = {
    copies: part("copies"), note: part("note"), head: part("head"), facts: part("facts"),
    version: part("version"), fields: part("fields"), acts: part("acts"), desc: part("desc"), hist: part("hist"), more: part("more"),
  };
  let shownID = "", row = null, d0 = null, picture = null, shown = false;
  const f = {
    box,
    show() {
      if (!shown) {
        shown = true;
        box.replaceChildren(...parts);
        if (o.mount) o.mount();
        if (sc) sc.scrollTop = 0;
      }
      unstale(o.host);
    },
    settle(waits) {
      if (shown) return Promise.resolve();
      return new Promise((done) => {
        const t = setTimeout(done, REVEAL_MS);
        Promise.allSettled([...waits, picture]).then(() => { clearTimeout(t); done(); });
      });
    },
    copies(list) {
      P.copies.innerHTML = "";
      if (!list || list.length < 2) return;
      const section = el("div", "editor-copies");
      section.append(el("div", "sub", T("copies")));
      for (const c of list) {
        const b = el("div", "editor-copy" + (c.on ? " on" : ""));
        b.append(el("span", "lbl", c.label));
        if (c.active) b.append(el("small", "", T("in the current list")));
        else {
          const use = el("button", "editor-use", T("use this copy"));
          use.disabled = c.dim;
          use.title = c.dim ? T("cannot be loaded") : T("use this copy in the list");
          use.onclick = (e) => { e.stopPropagation(); c.use(); };
          b.append(use);
        }
        b.title = T("view this copy");
        b.onclick = () => c.view();
        section.append(b);
      }
      P.copies.append(section);
    },
    head(name, authors, id) {
      if (id && id !== shownID) {
        shownID = id;
        P.head.innerHTML = "";
        const img = el("img", "thumb");
        img.alt = "";
        img.onerror = () => img.remove();
        img.src = "/preview/" + encodeURIComponent(id);
        // decoded before the swap, so the picture does not push the details down after it
        picture = shown ? null : img.decode().catch(() => {});
        P.head.append(img, el("h1", "", name || ""), el("div", "who"));
      } else if (!P.head.firstChild) P.head.append(el("h1", "", name || ""), el("div", "who"));
      const h = P.head.querySelector("h1"), w = P.head.querySelector(".who");
      if (h && name) h.textContent = name;
      if (w) { w.textContent = authors || ""; w.hidden = !authors; }
    },
    inst(r) {
      row = r;
      const editing = P.note.querySelector(".lib-note.editing");
      if (!editing || P.note.dataset.ref !== r.ref) {
        P.note.innerHTML = "";
        P.note.dataset.ref = r.ref;
        P.note.append(noteEl(r, { saved: o.noteSaved, leave: o.noteLeave }));
      }
      P.acts.innerHTML = "";
      const codes = ops.sorted((r.ops || []).filter((c) => { const x = ops.op(c); return x.detail || x.group === 4; }));
      if (!codes.length) return;
      const acts = el("div", "lib-acts");
      for (const c of codes) {
        const x = ops.op(c);
        const b = el("button", "btn" + (x.danger ? " danger" : ""), x.label);
        if (x.group !== 3) b.dataset.envWrite = c;
        if (x.key) b.append(keyCaps(x.key));
        b.onclick = () => o.run && o.run(c, r);
        acts.append(b);
      }
      P.acts.append(acts);
    },
    detail(d) {
      d0 = d;
      f.head(d.name, d.authors, d.id);
      P.facts.innerHTML = "";
      const facts = el("div", "facts");
      facts.innerHTML = sourceIcon(d.kind);
      facts.append(factText(el("span"), d.header));
      P.facts.append(facts);
      P.version.replaceChildren();
      if (d.version) P.version.append(versionNotice(d.version));
      if (!o.ownFields) f.fields(d.fields);
      P.desc.innerHTML = "";
      P.desc.append(description(d));
      P.hist.innerHTML = "";
      if (d.history) {
        const all = el("div", "sub link", T("see all in change history"));
        all.onclick = () => document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: d.id, name: d.name } }));
        P.hist.append(el("div", "sub", T("history")), el("div", "history word-facts", d.history), all);
      }
      P.more.innerHTML = "";
      if (o.tech) {
        const more = el("div", "sub link", T("technical information, enter"));
        more.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "full" }));
        P.more.append(more);
      }
    },
    fields(list) {
      P.fields.innerHTML = "";
      P.fields.append(fieldList(list, o.fieldActs ? o.fieldActs(d0 || { id: shownID }, row) : {}));
    },
    fetching() {
      const note = P.desc.querySelector(".desc-note");
      if (note) note.textContent = T("fetching the Workshop page…");
    },
    error(text) { P.desc.innerHTML = ""; P.desc.append(el("p", "err", text)); },
  };
  if (!o.later) f.show();
  return f;
}

// description: the About.xml text and the Workshop page's (16 §4.3 item 2).
function description(d) {
  const desc = el("div", "desc");
  const head = (title, note) => {
    const h = el("div", "sub word-facts", title);
    if (note) h.append(el("span", "desc-note", note));
    desc.append(h);
  };
  let note = d.note;
  if (d.show === "about" || d.show === "both") {
    head(T("description / About.xml"), note);
    note = "";
    if (!d.about || !d.about.length) desc.append(el("p", "dim", T("(empty)")));
    else blocks(desc, d.about);
  }
  if (d.show === "workshop" || d.show === "both") {
    head(T("description / Workshop page"), note);
    blocks(desc, d.workshop || []);
  }
  return desc;
}

// noteEl: the copy's own note (16 §2.9). The player's text, else what RimArcho wrote (grey),
// else a place to write one; a click (or n) edits it, enter saves, shift+enter breaks the
// line, esc leaves it as it was. o.saved() after a save, o.leave() when the editing ends;
// o.save(text) saves another note (a tag's) in place of the copy's, o.placeholder its hint.
export function noteEl(i, o = {}) {
  const box = el("div", "lib-note" + (i.note ? "" : i.auto ? " auto" : " empty"));
  const show = () => {
    box.innerHTML = "";
    box.classList.remove("editing");
    box.append(el("div", "txt", i.note || i.auto || T("write a note")));
    if (!i.note && i.auto) box.append(el("div", "by", T("by RimArcho, click to edit")));
  };
  show();
  if (!i.id && !o.save) return box;
  box.title = T("click to edit") + T("; ") + "n";
  box.onclick = () => {
    if (box.classList.contains("editing")) return;
    box.classList.add("editing");
    box.innerHTML = "";
    const start = i.note || i.auto || "";
    const ta = el("textarea", "lib-note-in");
    ta.value = start;
    ta.placeholder = o.placeholder || T("a note on this copy…");
    ta.rows = Math.min(6, Math.max(2, start.split("\n").length + 1));
    box.append(ta, el("div", "by", T("enter saves, esc cancels")));
    ta.focus();
    ta.setSelectionRange(ta.value.length, ta.value.length);
    let done = false;
    const leave = () => { if (o.leave) o.leave(); };
    const save = async () => {
      if (done) return;
      done = true;
      const text = ta.value.trim();
      if (text === start.trim()) { show(); leave(); return; }
      try { say("ok", await (o.save ? o.save(text) : api.LibrarySetNote(i.ref, text))); i.note = text; } catch (e) { say("fail", errText(e)); }
      show();
      leave();
      if (o.saved) await o.saved(text);
    };
    ta.onkeydown = (e) => {
      e.stopPropagation();
      if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); save(); }
      else if (e.key === "Escape") { e.preventDefault(); done = true; show(); leave(); }
    };
    ta.onblur = save;
  };
  return box;
}

// renderDetail draws a detail into box (scrolled by sc). Mod library (16 §2.9) draws its
// copies' details with it; opts.page leaves out what acts on the plan's selection (the tag
// editor, the full detail).
export function renderDetail(d, box, sc, keepScroll, opts = {}) {
  const top = keepScroll ? sc.scrollTop : 0;
  box.innerHTML = "";
  const img = el("img", "thumb");
  img.alt = "";
  img.onerror = () => img.remove();
  img.src = "/preview/" + encodeURIComponent(d.id);
  box.append(img, el("h1", "", d.name));
  if (d.authors) box.append(el("div", "who", d.authors));
  if (d.version) box.append(versionNotice(d.version));
  // a copy's details on Mod library draw their own note, actions and field list (16 §2.9)
  if (!opts.page) {
    const facts = el("div", "facts");
    facts.innerHTML = sourceIcon(d.kind);
    facts.append(factText(el("span"), d.header));
    box.append(facts);
    const own = S.cols.flatMap((c)=>c.items).find((it)=>it.id===d.id);
    const lineage=own?.lineage;
    const copies=lineage ? S.cols.flatMap((c)=>c.items).filter((it)=>it.id && it.lineage===lineage) : [];
    if(copies.length>1) {
      const section=el("div","editor-copies");section.append(el("div","sub",T("copies")));
      const active=new Set(S.cols[1].items.map((it)=>it.id));
      for(const it of copies) {
        const b=el("button","editor-copy"+(it.id===d.id?" on":""));
        b.append(el("span","",it.copyLabel || it.name));
        if(active.has(it.id))b.append(el("small","",T("in the current list")));
        b.title=T("view this copy");b.onclick=()=>showDetail(it,S.cols[2].items.some(x=>x.id===it.id));
        section.append(b);
      }
      if(!active.has(d.id)) {
        const use=el("button","editor-full-link",T("use this copy in the list"));
        use.disabled=!!own?.dim;use.title=own?.dim ? d.statusText : "";
        use.onclick=()=>document.dispatchEvent(new CustomEvent("editor-use-copy",{detail:d.id}));
        section.append(use);
      }
      box.append(section);
    }
    if (d.canSeparate) {
      const b = el("button", "btn", T("do not list as a copy"));
      b.dataset.envWrite = "separate";
      b.title = T("list this copy as an independent mod; package ID conflicts still apply");
      b.onclick = () => {
        b.disabled = true;
        document.dispatchEvent(new CustomEvent("separate-mod", { detail: d.id }));
      };
      box.append(b);
    }
    // the field list (16 §4.1): a click on a value copies it
    box.append(fieldList(d.fields, {
      tags: () => document.dispatchEvent(new CustomEvent("verb", { detail: "tags" })),
      folder: () => api.OpenFolder(d.id).catch((e) => say("fail", errText(e))),
    }));
  }
  // description (16 §4.3 item 2)
  const desc = el("div", "desc");
  const head = (title, note) => {
    const h = el("div", "sub word-facts", title);
    if (note) { const s = el("span", "desc-note", note); h.append(s); }
    desc.append(h);
  };
  let note = d.note;
  if (d.show === "about" || d.show === "both") {
    head(T("description / About.xml"), note);
    note = "";
    if (!d.about || !d.about.length) desc.append(el("p", "dim", T("(empty)")));
    else blocks(desc, d.about);
  }
  if (d.show === "workshop" || d.show === "both") {
    head(T("description / Workshop page"), note);
    blocks(desc, d.workshop || []);
  }
  box.append(desc);
  if (d.history && !opts.noHistory) {
    // the three newest here; the whole history of its Lineage on Updates (16 §2.9)
    const all = el("div", "sub link", T("see all in change history"));
    all.onclick = () => document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: d.id, name: d.name } }));
    box.append(el("div", "sub", T("history")), el("div", "history word-facts", d.history), all);
  }
  if (!opts.page) {
    const more = el("div", "sub link", T("technical information, enter"));
    more.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "full" }));
    box.append(more);
  }
  sc.scrollTop = top;
}

// blocks renders BBCode blocks (Para, Heading, Item, Table, Image).
export function blocks(root, bs) {
  for (const b of bs) {
    let e;
    switch (b.Kind) {
      case 0: // paragraph; an empty one is a break
        e = el("p", b.Spans && b.Spans.length ? "" : "gap");
        spans(e, b.Spans);
        break;
      case 1:
        e = el("h3");
        spans(e, b.Spans);
        break;
      case 2:
        e = el("div", "item");
        e.style.paddingLeft = Math.max(0, (b.Level || 1) - 1) * 16 + "px";
        e.append(el("span", "dim", b.Marker || "•"));
        { const t = el("span"); spans(t, b.Spans); e.append(t); }
        break;
      case 3:
        e = el("table");
        for (const r of b.Rows || []) {
          const tr = el("tr");
          for (const c of r) tr.append(el("td", "", c));
          e.append(tr);
        }
        break;
      case 4:
        e = el("img");
        e.loading = "lazy";
        e.alt = "";
        e.onerror = () => { e.replaceWith(el("p", "dim", Tf("[image, %s]", host(b.URL)))); };
        e.src = "/img?u=" + encodeURIComponent(b.URL);
        break;
      default:
        continue;
    }
    if (b.Quote) e.classList.add("quote");
    root.append(e);
  }
}

function spans(parent, ss) {
  for (const s of ss || []) {
    let n = document.createTextNode(s.Text);
    if (s.Bold) { const b = el("b"); b.append(n); n = b; }
    if (s.Strike) { const x = el("s"); x.append(n); n = x; }
    if (s.Link) {
      const a = el("span", "link");
      a.title = s.Link;
      a.append(n);
      a.onclick = () => api.OpenURL(s.Link).catch((e) => say("fail", errText(e)));
      n = a;
    }
    parent.append(n);
  }
}

function host(u) {
  try { return new URL(u).host; } catch { return u; }
}

// scrollDetail moves the detail by a page or a line (PgUp / PgDn anywhere, 16 §3.3).
export function scrollDetail(dy, page) {
  const sc = $(".detail .scroll");
  sc.scrollTop += page ? dy * sc.clientHeight * 0.9 : dy * 40;
}
