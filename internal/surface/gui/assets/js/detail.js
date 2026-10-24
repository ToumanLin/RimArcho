// The detail (06 §4): who the mod is, its state, one field list (fields.js), the
// description, the history. The description is built from Go's BBCode blocks with
// elements and textContent only: outside text is never inserted as HTML (06 §8.2).
import { S, $, el, esc, api, T, Tf, errText, say } from "./state.js";
import { icon, sourceIcon, statusIcon } from "./icons.js";
import { fieldList } from "./fields.js";

let seq = 0;
let fetchTimer = 0;

// show loads and draws the detail of a row; the Workshop page is fetched only after the
// cursor rested on the mod (06 §4.3 item 3).
export async function showDetail(it, staged) {
  const n = ++seq, gameRequest = S.envSeq;
  const current = () => n === seq && S.envSeq === gameRequest;
  clearTimeout(fetchTimer);
  const box = $("#detail");
  if (!it) { box.innerHTML = ""; S.detail = null; return; }
  if (it.missing) {
    S.detail = null;
    box.innerHTML = "";
    box.append(el("h1", "", it.name), el("p", "second", T("listed in ModsConfig.xml, but not installed")));
    return;
  }
  try {
    const d = await api.Detail(it.id, !!staged, false, false);
    if (!current()) return;
    draw(d);
    if (d.state === "not cached") {
      fetchTimer = setTimeout(async () => {
        if (!current()) return;
        const note = $("#detail .desc-note");
        if (note) note.textContent = T("fetching the Workshop page…");
        try {
          const f = await api.Detail(it.id, !!staged, true, false);
          if (current()) draw(f, true);
        } catch (e) { /* the note stays */ }
      }, 400);
    }
  } catch (e) {
    if (current()) { box.innerHTML = ""; box.append(el("p", "err", errText(e))); }
  }
}

function draw(d, keepScroll) {
  S.detail = d;
  renderDetail(d, $("#detail"), $(".detail .scroll"), keepScroll);
}

// renderDetail draws a detail into box (scrolled by sc). Mod 库存 (06 §2.9) draws its
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
  // a copy's details on Mod 库存 draw their own note, actions and field list (06 §2.9)
  if (!opts.page) {
    const facts = el("div", "facts");
    facts.innerHTML = sourceIcon(d.kind) + `<span>${esc(d.header.join(" · "))}</span>`;
    box.append(facts);
    // state
    const st = el("div", "state");
    st.innerHTML = (d.status ? statusIcon(d.status) : icon.ok) + `<span class="${d.status > 1 ? "" : "second"}">${esc(d.statusText)}</span>`;
    if (d.status > 1) {
      const b = el("button", "", T("show what"));
      b.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "needs" }));
      st.append(b);
    }
    box.append(st);
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
    // the field list (06 §4.1): a click on a value copies it
    box.append(fieldList(d.fields, {
      tags: () => document.dispatchEvent(new CustomEvent("verb", { detail: "tags" })),
      folder: () => api.OpenFolder(d.id).catch((e) => say("fail", errText(e))),
    }));
  }
  // description (06 §4.3 item 2)
  const desc = el("div", "desc");
  const head = (title, note) => {
    const h = el("div", "sub", title);
    if (note) { const s = el("span", "desc-note", "  ·  " + note); h.append(s); }
    desc.append(h);
  };
  let note = d.note;
  if (d.show === "about" || d.show === "both") {
    head(T("description · About.xml"), note);
    note = "";
    if (!d.about || !d.about.length) desc.append(el("p", "dim", T("(empty)")));
    else blocks(desc, d.about);
  }
  if (d.show === "workshop" || d.show === "both") {
    head(T("description · Workshop page"), note);
    blocks(desc, d.workshop || []);
  }
  box.append(desc);
  if (d.history && !opts.noHistory) {
    // the three newest here; the whole history of its Lineage on 变化 (06 §2.9)
    const all = el("div", "sub link", T("see all on the changes page"));
    all.onclick = () => document.dispatchEvent(new CustomEvent("changes-of", { detail: { id: d.id, name: d.name } }));
    box.append(el("div", "sub", T("history")), el("div", "history", d.history), all);
  }
  if (!opts.page) {
    const more = el("div", "sub link", T("technical information · enter"));
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
        e.onerror = () => { e.replaceWith(el("p", "dim", Tf("[image · %s]", host(b.URL)))); };
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

// scrollDetail moves the detail by a page or a line (PgUp / PgDn anywhere, 06 §3.3).
export function scrollDetail(dy, page) {
  const sc = $(".detail .scroll");
  sc.scrollTop += page ? dy * sc.clientHeight * 0.9 : dy * 40;
}
