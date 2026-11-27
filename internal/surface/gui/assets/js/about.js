// Settings › About's Author and Changelog sections (16 §2.14). The changelog is the built-in
// CHANGELOG.md (view.Changelog): the running version open, the next few folded, the rest
// behind "older versions"; a fold asks for its text (API.ChangelogText) the first time it
// opens. Its Markdown is drawn as plain DOM from text nodes, never as HTML.
import { el, api, T } from "./state.js";

// authorCard: the name with the GitHub handle after it, the study line, the work looked for
// (in the accent), and the links as Feedback's community pills. pill(label, run) is
// screens.js's button, passed in so this module does not import it.
export function authorCard(a, { pill, open }) {
  const box = el("div", "about-author");
  const text = el("div", "aa-text");
  const name = el("div", "aa-name");
  name.append(span("aa-n", a.name));
  if (a.handle) name.append(span("muted aa-h", a.handle));
  text.append(name);
  for (const l of a.lines || []) text.append(span("sub aa-line", l, "div"));
  if (a.seeking) text.append(span("aa-seek", a.seeking, "div"));
  const links = el("div", "fb-com-links aa-links");
  for (const l of a.links || []) {
    const b = pill(l.label, "", () => open(l.url), "community-link");
    b.title = l.url;
    links.append(b);
  }
  box.append(text, links);
  return box;
}

// changelogView: one fold per version, the first c.shown drawn now and the others when
// "older versions" opens.
export function changelogView(c, { open }) {
  const box = el("div", "cl");
  const vs = c.versions || [];
  for (const v of vs.slice(0, c.shown)) box.append(versionFold(v, c, open));
  const rest = vs.slice(c.shown);
  if (rest.length) {
    const older = fold("cl-older", (body) => { for (const v of rest) body.append(versionFold(v, c, open)); });
    older.head.append(span("cl-v", T("older versions")), span("muted num", String(rest.length)));
    box.append(older.box);
  }
  return box;
}

function versionFold(v, c, open) {
  const f = fold("cl-ver", async (body) => {
    const text = v.text ?? await api.ChangelogText(v.key).catch(() => "");
    const md = renderMarkdown(text || "", { source: c.source, open, leads: true });
    body.append(md);
    // measured once it has a width: About may be hidden behind another group when drawn
    const ro = new ResizeObserver(() => { if (md.clientWidth) { ro.disconnect(); wrapWhenCut(md); } });
    ro.observe(md);
  }, v.current);
  f.box.dataset.version = v.key;
  const title = span("cl-v" + (v.current ? " current" : ""), v.title);
  if (v.current) title.title = T("the version you are running");
  f.head.append(title);
  if (v.note) f.head.append(span("muted num cl-note", v.note));
  return f.box;
}

// fold is a heading with Settings › Folders' chevron (folderfolds.js) over a body that fill
// draws the first time it opens; a click on the heading or enter on the chevron toggles it.
function fold(cls, fill, startOpen = false) {
  const box = el("div", "folder-fold " + cls);
  const head = el("div", "folder-fold-head cl-head " + cls + "-head");
  const toggle = el("button", "folder-fold-toggle");
  toggle.type = "button";
  const chev = el("span", "fold-chev", "›");
  chev.setAttribute("aria-hidden", "true");
  toggle.append(chev);
  head.append(toggle);
  const body = el("div", "cl-body");
  let filled = null;
  const show = (on) => {
    box.classList.toggle("open", on);
    body.hidden = !on;
    toggle.setAttribute("aria-expanded", String(on));
    toggle.title = T(on ? "hide" : "show");
    if (on && !filled) filled = Promise.resolve(fill(body)).catch(() => {});
  };
  show(startOpen);
  toggle.onclick = (e) => { e.stopPropagation(); show(body.hidden); };
  head.onclick = (e) => { if (!e.target.closest("a, .cl-body")) toggle.click(); };
  // the heading's text names the toggle
  queueMicrotask(() => { toggle.setAttribute("aria-label", head.textContent.replace("›", "").trim()); });
  box.append(head, body);
  return { box, head, body };
}

function span(cls, text, tag = "span") {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  n.append(document.createTextNode(text ?? ""));
  return n;
}

// parseMarkdown reads the subset CHANGELOG.md uses: "### " headings, "- " bullets (nested by
// indent, continued by indented lines), paragraphs, and inline **bold**, `code`, *emphasis*
// and [links](url). Anything else stays text.
export function parseMarkdown(text) {
  const blocks = [];
  let para = null, stack = [];
  for (const line of String(text).replace(/\r\n/g, "\n").split("\n")) {
    if (!line.trim()) { para = null; stack = []; continue; }
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) { blocks.push({ t: "h", level: h[1].length, raw: h[2].trim() }); para = null; stack = []; continue; }
    const b = /^(\s*)[-*+]\s+(.*)$/.exec(line);
    if (b) {
      const indent = b[1].length, item = { raw: b[2].trim(), sub: null };
      while (stack.length && stack.at(-1).indent > indent) stack.pop();
      const top = stack.at(-1);
      if (top && top.indent === indent) top.list.items.push(item);
      else {
        const list = { t: "ul", items: [item] };
        const parent = top?.list.items.at(-1);
        if (parent) parent.sub = list; else blocks.push(list);
        stack.push({ indent, list });
      }
      para = null;
      continue;
    }
    if (stack.length && /^\s/.test(line)) { const it = stack.at(-1).list.items.at(-1); it.raw += " " + line.trim(); continue; }
    stack = [];
    if (para) para.raw += " " + line.trim();
    else blocks.push(para = { t: "p", raw: line.trim() });
  }
  const conv = (l) => ({ t: "ul", items: l.items.map(i => ({ inl: inline(i.raw), sub: i.sub && conv(i.sub) })) });
  return blocks.map(b => b.t === "ul" ? conv(b) : { ...b, inl: inline(b.raw), raw: undefined });
}

// inline splits a line into text, bold, emphasis, code and link runs.
export function inline(s) {
  const out = [];
  let buf = "", i = 0;
  const flush = () => { if (buf) out.push({ t: "text", text: buf }); buf = ""; };
  while (i < s.length) {
    const c = s[i];
    if (c === "`") {
      const j = s.indexOf("`", i + 1);
      if (j > i) { flush(); out.push({ t: "code", text: s.slice(i + 1, j) }); i = j + 1; continue; }
    }
    if (s.startsWith("**", i)) {
      const j = s.indexOf("**", i + 2);
      if (j > i + 2) { flush(); out.push({ t: "b", kids: inline(s.slice(i + 2, j)) }); i = j + 2; continue; }
    }
    if (c === "[") {
      const m = /^\[([^\]]+)\]\(([^)\s]+)\)/.exec(s.slice(i));
      if (m) { flush(); out.push({ t: "link", url: m[2], kids: inline(m[1]) }); i += m[0].length; continue; }
    }
    if (c === "*" && s[i + 1] && s[i + 1] !== " " && s[i + 1] !== "*" && !/\w/.test(s[i - 1] || "")) {
      const j = s.indexOf("*", i + 1);
      if (j > i + 1 && s[j - 1] !== " ") { flush(); out.push({ t: "em", kids: inline(s.slice(i + 1, j)) }); i = j + 1; continue; }
    }
    buf += c; i++;
  }
  flush();
  return out;
}

// linkURL: a web link as it is; a link into the repository (docs/…) on GitHub; anything
// else (an anchor, another scheme) is no link.
export function linkURL(url, source) {
  if (/^https?:\/\//i.test(url)) return url;
  if (/^[a-z][a-z0-9+.-]*:/i.test(url) || url.startsWith("#") || url.startsWith("/") || !source) return "";
  return source + url.replace(/^\.\//, "");
}

// renderMarkdown draws parseMarkdown's blocks with createElement and text nodes only.
// With leads, each bullet is one line, its lead (bulletLead) as plain text, with a "more"
// that shows it whole underneath.
export function renderMarkdown(text, { source = "", open = () => {}, leads = false } = {}) {
  const box = el("div", "cl-md");
  const runs = (parent, list) => {
    for (const r of list) {
      if (r.t === "text") parent.append(document.createTextNode(r.text));
      else if (r.t === "code") parent.append(span("", r.text, "code"));
      else if (r.t === "b" || r.t === "em") { const n = document.createElement(r.t === "b" ? "strong" : "em"); runs(n, r.kids); parent.append(n); }
      else if (r.t === "link") {
        const url = linkURL(r.url, source);
        if (!url) { runs(parent, r.kids); continue; }
        const a = document.createElement("a");
        a.href = url;
        a.title = url;
        a.onclick = (e) => { e.preventDefault(); e.stopPropagation(); open(url); };
        runs(a, r.kids);
        parent.append(a);
      }
    }
  };
  const list = (l) => {
    const ul = document.createElement("ul");
    for (const it of l.items) {
      const li = document.createElement("li");
      if (!leads) {
        runs(li, it.inl);
        if (it.sub) li.append(list(it.sub));
        ul.append(li);
        continue;
      }
      // one line of primary text: the lead as plain words (no code, bold or links), cut by
      // CSS when it is long; "more" shows the whole bullet under it, in secondary text
      const lead = bulletLead(it.inl);
      const row = document.createElement("div");
      row.className = "cl-row";
      const short = document.createElement("span");
      short.className = "cl-lead";
      short.append(document.createTextNode(plainText(lead || it.inl)));
      row.append(short);
      li.append(row);
      if (!lead && !it.sub) {
        // one sentence: whole in its hover; once drawn, a line cut short gets a "more" that
        // lets it wrap (wrapWhenCut)
        short.title = plainText(it.inl);
        short.dataset.whole = "1";
      } else {
        const full = document.createElement("div");
        full.className = "cl-full";
        runs(full, it.inl);
        if (it.sub) full.append(list(it.sub));
        row.append(moreToggle((on) => { full.hidden = !on; }));
        li.append(full);
      }
      ul.append(li);
    }
    return ul;
  };
  for (const b of parseMarkdown(text)) {
    if (b.t === "ul") { box.append(list(b)); continue; }
    const n = document.createElement(b.t === "h" ? "h4" : "p");
    if (b.t === "h") n.className = "cl-h";
    runs(n, b.inl);
    box.append(n);
  }
  return box;
}

// moreToggle is the muted "more" / "hide" after a lead; set(on) shows or hides the rest.
function moreToggle(set) {
  const more = document.createElement("button");
  more.type = "button";
  more.className = "cl-more";
  const flip = (on) => {
    set(on);
    more.replaceChildren(document.createTextNode(T(on ? "hide" : "more")));
    more.setAttribute("aria-expanded", String(on));
  };
  flip(false);
  more.onclick = (e) => { e.stopPropagation(); flip(more.getAttribute("aria-expanded") !== "true"); };
  return more;
}

// wrapWhenCut gives each one-sentence lead in box that its line cuts short a "more" that lets
// it wrap; it reads the layout once, when the fold first has a width.
export function wrapWhenCut(box) {
  for (const s of box.querySelectorAll(".cl-lead[data-whole]")) {
    if (s.scrollWidth <= s.clientWidth) continue;
    delete s.dataset.whole;
    s.parentNode.append(moreToggle((on) => s.classList.toggle("wrap", on)));
  }
}

// bulletLead is a bullet's lead: the bold phrase it starts with, else its first sentence, cut
// at the first ". " or "; " outside code and quotes. null when the lead is the whole bullet.
export function bulletLead(inl) {
  if (inl[0]?.t === "b") return inl.length > 1 && !(inl.length === 2 && inl[1].t === "text" && !inl[1].text.trim()) ? [inl[0]] : null;
  const out = [];
  let quotes = 0;
  for (const [i, r] of inl.entries()) {
    if (r.t === "text") {
      let m = null;
      for (const c of r.text.matchAll(/[.;] |"/g)) {
        if (c[0] === '"') quotes++;
        else if (quotes % 2 === 0) { m = c; break; }
      }
      if (m) {
        const cut = r.text.slice(0, m.index + (r.text[m.index] === "." ? 1 : 0));
        if (cut.trim()) out.push({ t: "text", text: cut });
        const rest = r.text.slice(m.index + 1).trim() || inl.slice(i + 1).length;
        return rest ? out : null;
      }
    }
    out.push(r);
  }
  return null;
}

// plainText is runs' words without their marks: code, bold and links read as text.
export function plainText(inl) {
  return (inl || []).map(r => r.text ?? plainText(r.kids)).join("");
}
