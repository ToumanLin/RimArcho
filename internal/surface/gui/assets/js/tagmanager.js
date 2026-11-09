// The Tag manager (16 §2.9): a tag's details, shown where a copy's would be when the cursor
// is on a tag's header row: Mod library's Tags view, and the editor's Disabled column grouped
// by tag (16 §2.5). Its note, name, how many Mods have it, its colour, similar tags, its
// actions (rename, merge into, delete), then every tag to go to. Tag changes are player
// data, outside the list's undo (16 §3.8), so merge and delete confirm first.
//
// The page that shows it gives a context:
//   tags()      API.TagList's rows, in the groups' order
//   picked      a Set of lower-cased tags picked with ctrl+click in "all tags"
//   done(key)   reads the page again after a change, then puts the cursor on key's group
//               (null: none)
//   goTo(key)   puts the cursor on key's group
//   redraw()    draws the panel again (the picked tags changed)
//   leave()     gives the keyboard back to the rows
//   editing()   a field in the panel takes the keyboard (rename)
import { el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { noteEl } from "./detail.js";
import * as pop from "./popups.js";
import { tagCSS, TAG_PRESETS } from "./tagcolor.js";

// tagPanel is the panel for group g ({key, tag, none, rows, info?}), or for no tag at all.
export function tagPanel(g, ctx) {
  const info = tagInfo(g);
  const d = el("div", "mod-detail tag-detail");
  if (info) tagHead(d, g, info, ctx);
  else if (g) d.append(el("h1", "", T("no tag")), el("div", "facts", Tf("%d mods", g.rows.length)), el("p", "tm-hint", T("mods without tags: select them and press g to tag them")));
  else d.append(el("p", "tm-hint", T("no tags yet: select mods and press g to tag them")));
  allTags(d, g ? g.key : null, ctx);
  return d;
}

// tagInfo: group g's row of API.TagList (a tag no Mod list has read yet: its name alone).
export function tagInfo(g) {
  return g && !g.none ? g.info || { tag: g.tag, n: g.rows.length, note: "", pinned: false } : null;
}

// act runs a tag change, says how it went and reads the page again.
async function act(ctx, p, key) {
  ctx.leave(); // first, so the redraw is not held back by a field being typed in
  try { say("ok", await p); } catch (e) { say("fail", errText(e)); }
  ctx.picked.clear();
  await ctx.done(key);
}

// tagHead: the tag's note, name, facts, fields and actions.
function tagHead(d, g, info, ctx) {
  d.append(noteEl({ id: info.tag, note: info.note || "" }, {
    placeholder: T("what this tag is for (shown in its group row)"),
    save: (text) => api.SetTagNote(info.tag, text),
    saved: () => ctx.done(g.key),
    leave: () => ctx.leave(),
  }));
  // the name, and keep on top (first in the tag bar and in this view) on its right
  const row = el("div", "tm-titlerow");
  const h = el("h1", "tm-name", info.tag);
  h.title = T("rename") + T("; ") + "F2";
  h.ondblclick = () => startRename(d, g, ctx);
  const pin = el("button", "fact", info.pinned ? T("stop keeping on top") : T("keep on top"));
  pin.onclick = () => act(ctx, api.PinTag(info.tag, !info.pinned), g.key);
  row.append(h, pin);
  const facts = el("div", "facts");
  facts.append(el("span", "", Tf("%d mods", info.n)));
  if (info.n !== g.rows.length) facts.append(el("span", "", Tf("%d shown", g.rows.length)));
  if (info.pinned) facts.append(el("span", "", T("on top")));
  d.append(row, facts, colorField(info, g, ctx));
  // similar tags, drawn as fields.js draws a copy's field: a label, the value, its action
  const like = similarTags(info.tag, ctx.tags());
  if (like.length) {
    const r = el("div", "frow");
    const v = el("span", "v");
    v.append(...like.map((t) => el("span", "chip", t)));
    const b = el("button", "fact", T("merge in"));
    b.onclick = () => confirmMerge(like, info.tag, ctx);
    r.append(el("span", "k", T("similar")), v, b);
    const fields = el("div", "fields");
    fields.append(r);
    d.append(fields);
  }
  const acts = el("div", "lib-acts");
  const btn = (label, key, run, cls = "") => {
    const b = el("button", "btn" + cls, label);
    b.append(keyCaps(key));
    b.onclick = run;
    acts.append(b);
  };
  btn(T("rename"), "F2", () => startRename(d, g, ctx));
  btn(T("merge into…"), "M", () => pickMerge(info.tag, ctx));
  btn(T("delete tag"), "Del", () => confirmDelete([info.tag], ctx), " danger");
  d.append(acts);
}

// colorField: the tag's colour: none, the presets, or a colour of the player's own from the
// colour wheel. It marks the Mods whose first coloured tag this is.
function colorField(info, g, ctx) {
  const fields = el("div", "fields");
  const r = el("div", "frow tag-color");
  const v = el("span", "v tm-swatches");
  const cur = info.color || "";
  const set = (c) => act(ctx, api.SetTagColor(info.tag, c), g.key);
  const sw = (value, label, css, cls = "") => {
    const b = el("button", "tm-swatch" + cls + (value === cur || (value === "#" && cur.startsWith("#")) ? " on" : ""));
    b.type = "button";
    b.title = label;
    b.setAttribute("aria-label", label);
    b.setAttribute("aria-pressed", String(b.classList.contains("on")));
    if (css) b.style.setProperty("--sw", css);
    v.append(b);
    return b;
  };
  sw("", T("no colour"), "", " none").onclick = () => set("");
  for (const name of TAG_PRESETS) sw(name, T(name), tagCSS(name)).onclick = () => set(name);
  const own = sw("#", cur.startsWith("#") ? cur : T("a colour of your own…"), cur.startsWith("#") ? cur : "", " own");
  own.onclick = () => pickOwnColor(own, cur, set);
  r.append(el("span", "k", T("colour")), v);
  fields.append(r);
  const hint = el("div", "tm-hint", T("marks the rows of Mods whose first coloured tag this is; drag a Mod's tags in its details to change which"));
  const box = el("div", "tm-colorbox");
  box.append(fields, hint);
  return box;
}

// pickOwnColor opens the colour wheel (colorpicker.js); without it, the system's picker.
async function pickOwnColor(anchor, cur, set) {
  try {
    const m = await import("./colorpicker.js");
    return m.openColorPicker(anchor, { value: cur, presets: TAG_PRESETS.map((name) => ({ name, color: getComputedStyle(document.documentElement).getPropertyValue("--tag-" + name).trim(), label: T(name) })), onPick: (c) => set(c) });
  } catch { /* not built in: the system picker */ }
  const input = document.createElement("input");
  input.type = "color";
  input.value = cur.startsWith("#") ? cur : "#3a7bd5";
  input.onchange = () => set(input.value);
  input.click();
}

// allTags: every tag with Mods, as chips in the groups' order; a click goes to its group,
// ctrl+click picks several (the cursor's tag first, as a file list's selection would), and
// a bar under them merges, deletes or clears them. Tags kept on top that no Mod has are
// named after them.
function allTags(d, curKey, ctx) {
  const all = ctx.tags(), picked = ctx.picked;
  const used = all.filter((t) => t.n), idle = all.filter((t) => !t.n);
  if (!used.length && !idle.length) return;
  for (const k of picked) if (!used.some((t) => t.tag.toLowerCase() === k)) picked.delete(k);
  const sub = el("div", "sub", T("all tags"));
  sub.append(el("span", "cnt", String(used.length)));
  const cloud = el("div", "tm-cloud");
  for (const t of used) {
    const k = t.tag.toLowerCase();
    const c = el("button", "tm-chip" + (k === curKey ? " cur" : "") + (picked.has(k) ? " picked" : ""));
    if (tagCSS(t.color)) { const m = el("span", "tm-mark"); m.style.background = tagCSS(t.color); m.setAttribute("aria-hidden", "true"); c.append(m); }
    c.append(el("span", "", t.tag), el("span", "n", String(t.n)));
    c.title = (t.note ? t.tag + "\n" + t.note : t.tag) + "\n" + T("ctrl+click picks several");
    c.setAttribute("aria-pressed", String(picked.has(k)));
    c.onclick = (e) => {
      if (e.ctrlKey || e.metaKey) {
        if (!picked.size && curKey && curKey !== k && used.some((u) => u.tag.toLowerCase() === curKey)) picked.add(curKey);
        if (picked.has(k)) picked.delete(k); else picked.add(k);
        return ctx.redraw();
      }
      picked.clear();
      ctx.goTo(k);
    };
    cloud.append(c);
  }
  d.append(sub, cloud);
  if (picked.size) {
    const tags = used.filter((t) => picked.has(t.tag.toLowerCase())).map((t) => t.tag);
    const b = el("div", "tm-picked");
    b.append(el("span", "cnt", Tf("%d tags picked", tags.length)));
    const btn = (label, run, cls = "") => { const x = el("button", "btn" + cls, label); x.onclick = run; b.append(x); };
    btn(T("merge into…"), () => pop.tagTarget(Tf("merge %s into", tags.join(", ")), used, (into) => confirmMerge(tags, into, ctx)));
    btn(T("delete tags"), () => confirmDelete(tags, ctx), " danger");
    btn(T("clear the selection"), () => { picked.clear(); ctx.redraw(); }, " quiet");
    d.append(b);
  }
  if (idle.length) d.append(el("p", "tm-idle", Tf("on top, but no mod has them: %s", idle.map((t) => t.tag).join(", "))));
}

// similarTags: tags spelled almost the same (case, spaces, dashes, a plural s), or one a
// longer form of the other: candidates for a merge.
export function similarTags(tag, all) {
  const norm = (s) => s.toLowerCase().replace(/[\s_\-./]+/g, "").replace(/s$/, "");
  const n = norm(tag);
  return all.filter((t) => {
    if (!t.n || t.tag.toLowerCase() === tag.toLowerCase()) return false;
    const m = norm(t.tag);
    if (m === n) return true;
    const [a, b] = m.length < n.length ? [m, n] : [n, m];
    return a.length >= 4 && b.startsWith(a);
  }).map((t) => t.tag);
}

// startRename turns the tag's name in box into a field: enter renames, esc (or leaving it)
// does not.
export function startRename(box, g, ctx) {
  const h = box && box.querySelector(".tag-detail .tm-name, .tm-name");
  if (!h || !g || g.none) return;
  const from = h.textContent;
  const inp = el("input", "tm-title");
  inp.value = from;
  inp.spellcheck = false;
  inp.setAttribute("aria-label", T("rename"));
  let over = false;
  const end = () => { if (over) return; over = true; inp.replaceWith(h); };
  inp.onkeydown = (e) => {
    if (e.key === "Enter") { e.preventDefault(); e.stopPropagation(); const to = inp.value; end(); renameTag(from, to, ctx); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); end(); ctx.leave(); }
  };
  inp.onblur = end;
  h.replaceWith(inp);
  ctx.editing();
  inp.focus();
  inp.select();
}

// editTagNote opens the tag's note in box, as a click on it does (n).
export function editTagNote(box, ctx) {
  const n = box && box.querySelector(".tag-detail .lib-note");
  if (!n) return;
  ctx.editing();
  n.click();
}

export function pickMerge(tag, ctx) {
  const others = ctx.tags().filter((t) => t.n && t.tag.toLowerCase() !== tag.toLowerCase());
  pop.tagTarget(Tf("merge %s into", tag), others, (into) => confirmMerge([tag], into, ctx));
}

export async function confirmMerge(from, into, ctx) {
  let p;
  try { p = await api.TagPreview(from, into); } catch (e) { return say("fail", errText(e)); }
  const lines = [
    p.from.join(", ") + "  →  " + p.into,
    p.both ? Tf("%d mods change to %s (%d already had it)", p.lineages, p.into, p.both) : Tf("%d mods change to %s", p.lineages, p.into),
    T("their notes are joined; tag changes cannot be undone"),
  ];
  pop.confirm(T("merge tags"), lines, T("merge"), () => act(ctx, api.MergeTags(p.from, p.into), p.into.toLowerCase()), "");
}

export async function confirmDelete(tags, ctx) {
  let p;
  try { p = await api.TagPreview(tags, ""); } catch (e) { return say("fail", errText(e)); }
  const lines = [
    Tf("remove %s from %d mods", p.from.join(", "), p.lineages),
    T("mod files and lists are not touched; tag changes cannot be undone"),
  ];
  pop.confirm(tags.length > 1 ? T("delete tags") : T("delete tag"), lines, T("delete"), () => act(ctx, api.DeleteTags(p.from), null), "");
}

function renameTag(from, to, ctx) {
  to = to.trim();
  if (!to || to === from) return ctx.leave();
  const other = ctx.tags().find((t) => t.tag.toLowerCase() === to.toLowerCase() && t.tag.toLowerCase() !== from.toLowerCase());
  if (other) return confirmMerge([from], other.tag, ctx); // the name is taken: that is a merge
  act(ctx, api.RenameTag(from, to), to.toLowerCase());
}
