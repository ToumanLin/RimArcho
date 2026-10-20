// The detail (06 §6, 08 §7): who the mod is, its state, facts, dependencies, the
// description, the history. The description is built from Go's BBCode blocks with
// elements and textContent only: outside text is never inserted as HTML (08 §3).
import { S, $, el, esc, api, T, Tf, errText, say } from "./state.js";
import { icon, sourceIcon, statusIcon } from "./icons.js";

let seq = 0;
let fetchTimer = 0;

// show loads and draws the detail of a row; the Workshop page is fetched only after the
// cursor rested on the mod (06 §6.4 item 3).
export async function showDetail(it, staged) {
  const n = ++seq;
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
    if (n !== seq) return;
    draw(d);
    if (d.state === "not cached") {
      fetchTimer = setTimeout(async () => {
        if (n !== seq) return;
        const note = $("#detail .desc-note");
        if (note) note.textContent = T("fetching the Workshop page…");
        try {
          const f = await api.Detail(it.id, !!staged, true, false);
          if (n === seq) draw(f, true);
        } catch (e) { /* the note stays */ }
      }, 400);
    }
  } catch (e) {
    if (n === seq) { box.innerHTML = ""; box.append(el("p", "err", errText(e))); }
  }
}

function draw(d, keepScroll) {
  S.detail = d;
  const sc = $(".detail .scroll");
  const top = keepScroll ? sc.scrollTop : 0;
  const box = $("#detail");
  box.innerHTML = "";
  const img = el("img", "thumb");
  img.alt = "";
  img.onerror = () => img.remove();
  img.src = "/preview/" + encodeURIComponent(d.id);
  box.append(img, el("h1", "", d.name));
  if (d.authors) box.append(el("div", "who", d.authors));
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
  // facts
  const kv = el("table", "kv");
  const row = (label, node) => {
    const tr = el("tr");
    tr.append(el("td", "", label));
    const td = el("td");
    td.append(node);
    tr.append(td);
    kv.append(tr);
  };
  for (const f of d.facts) {
    const v = el("span");
    switch (f.kind) {
      case "workshop": {
        v.append(f.value + "  ");
        const a = el("span", "link", T("open the page"));
        a.onclick = () => api.OpenURL(d.url).catch((e) => say("fail", errText(e)));
        v.append(a);
        break;
      }
      case "tags":
        if (!f.list || !f.list.length) v.append(el("span", "dim", T("none")));
        for (const t of f.list || []) v.append(el("span", "chip", t));
        {
          const a = el("span", "link", " " + T("edit"));
          a.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "tags" }));
          v.append(a);
        }
        break;
      case "alias":
        v.append(f.value, el("span", "dim", `  (${f.extra})`));
        break;
      default:
        v.textContent = f.value || "—";
    }
    row(f.label, v);
  }
  if (d.deps && d.deps.length) {
    const v = el("div");
    for (const x of d.deps) {
      const line = el("div");
      const ic = { enabled: icon.ok, disabled: icon.warn, missing: icon.err }[x.state];
      line.innerHTML = ic + " " + esc(x.name) + (x.state === "disabled" ? `<span class="dim">${esc(T(" (not enabled)"))}</span>` : x.state === "missing" ? `<span class="dim">${esc(T(" (not installed)"))}</span>` : "");
      v.append(line);
    }
    row(T("requires"), v);
  }
  box.append(kv);
  // description (06 §6.4 item 2)
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
  if (d.history) {
    box.append(el("div", "sub", T("history")), el("div", "history", d.history));
  }
  const more = el("div", "sub link", T("technical information · enter"));
  more.onclick = () => document.dispatchEvent(new CustomEvent("verb", { detail: "full" }));
  box.append(more);
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

// scrollDetail moves the detail by a page or a line (PgUp / PgDn anywhere, 06 §13.4).
export function scrollDetail(dy, page) {
  const sc = $(".detail .scroll");
  sc.scrollTop += page ? dy * sc.clientHeight * 0.9 : dy * 40;
}
