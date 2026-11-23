// The steps after a sort (16 §6.1): what the sort found that ordering cannot fix, one step at
// a time. A strip at the top shows every step as an icon and a count (done, skipped, now,
// later); below it the current step shows what it changes, Mod by Mod, with the place each
// lands in the list, before the player applies or skips it. Each list step is its own undo
// step: the latest applied one (the sort included) offers undo here. Words are on hover and
// in aria labels (16 §7 principle 9); the dialog reads the plan again after every step.
import { S, el, api, T, Tf, errText, say, keyCaps } from "./state.js";
import { icon } from "./icons.js";
import * as pop from "./popups.js";
import { getChoices } from "./credit.js";

let ui = null; // { edited, reload }
export function initSortSteps(u) { ui = u; }

const ORDER = ["sort", "use-installed", "replace", "enable", "subscribe"];

// What each kind looks like: its icon, its short name on the strip, the sentence that says
// what it does, and its button's verb. Counts are badges beside them, not words.
export const KINDS = {
  sort: { icon: "order", short: () => T("sort"), say: () => T("sorted") },
  "use-installed": {
    icon: "copy", short: () => T("use copies"),
    say: () => T("use the installed copies for entries with no folder"), go: () => T("use copies"),
  },
  replace: {
    icon: "swap", short: () => T("replace"),
    say: () => T("replace with the recommended mods"), go: () => T("replace"),
  },
  enable: {
    icon: "plus", short: () => T("enable"),
    say: () => T("enable the required mods, then sort"), go: () => T("enable"),
  },
  subscribe: {
    icon: "import", short: () => T("download"),
    say: () => T("get the missing mods from the Workshop"),
  },
};

// stripOf: the strip's chips in step order. done is the applied record (kind, n, text,
// undo), skipped the skipped kinds, steps what the plan still has; the current step is
// data.current. A kind can be done and still listed (a step that left something behind):
// done wins.
export function stripOf(flow) {
  const data = flow.data || { steps: [], current: -1 };
  const chips = new Map();
  if (flow.sort) chips.set("sort", { kind: "sort", state: flow.sort.state, n: 0, text: flow.sort.text });
  data.steps.forEach((s, i) => chips.set(s.kind, {
    kind: s.kind, n: s.items.length,
    state: i === data.current ? "now" : flow.skipped.has(s.kind) ? "skipped" : "later",
  }));
  for (const d of flow.done) chips.set(d.kind, { kind: d.kind, state: "done", n: d.n || 0, text: d.text });
  const last = flow.done[flow.done.length - 1];
  const out = ORDER.filter((k) => chips.has(k)).map((k) => chips.get(k));
  for (const c of out) c.undo = !!(last && last.kind === c.kind && last.undo && !flow.busy);
  return out;
}

// pastOf: the kinds the plan read skips to find the current step.
export const pastOf = (flow) => [...flow.done.map((d) => d.kind).filter((k) => k !== "sort"), ...flow.skipped];

const mark = (name) => { const s = el("span", "ss-ico"); s.innerHTML = icon[name] || ""; return s; };

// offerAfterSort opens the steps when the sort left something to do or to know; o is the
// sort's view.Outcome.
export async function offerAfterSort(o) {
  const flow = { sort: { state: "done", text: o.text, undo: !!o.undo }, done: [], skipped: new Set(), data: null, busy: false, error: "" };
  if (o.undo) flow.done.push({ kind: "sort", n: 0, text: o.text, undo: true });
  try { flow.data = await api.SortSteps([]); } catch (e) { return say("fail", errText(e)); }
  if (!flow.data.steps.length && !(flow.data.unfixable || []).length) return;
  let node = null;
  pop.open("sort-steps", T("after the sort"), (p, state) => {
    node = p;
    state.modal = true;
    p.classList.add("sort-steps");
    state.keys = (e) => keys(flow, node, e);
    draw(flow, node);
  });
}

async function refresh(flow, node) {
  try { flow.data = await api.SortSteps(pastOf(flow)); } catch (e) { flow.error = errText(e); }
  if (S.popup?.kind === "sort-steps") draw(flow, node);
}

function keys(flow, node, e) {
  if (flow.busy) return true;
  const k = e.key;
  if (k === "Enter") { e.preventDefault(); node.querySelector(".btns .go")?.click(); }
  else if (k === "s") node.querySelector(".btns .ss-skip")?.click();
  else if (k === "u") node.querySelector(".ss-undo")?.click();
  return true;
}

function draw(flow, node) {
  const data = flow.data;
  node.replaceChildren();
  node.append(el("div", "ttl", T("after the sort")));
  node.append(strip(flow, node));
  const body = el("div", "body");
  const step = data.current >= 0 ? data.steps[data.current] : null;
  body.append(step ? card(step) : summary(flow));
  if (flow.error) body.append(el("div", "ss-err", flow.error));
  if ((data.unfixable || []).length) body.append(unfixable(data.unfixable));
  node.append(body);
  node.append(buttons(flow, node, step));
  node.setAttribute("aria-busy", flow.busy ? "true" : "false");
}

// strip: one chip per step: state mark, the kind's icon, its count; hover says it in words.
function strip(flow, node) {
  const row = el("div", "ss-strip");
  row.setAttribute("role", "list");
  for (const c of stripOf(flow)) {
    const k = KINDS[c.kind];
    const chip = el("span", "ss-chip " + c.state);
    chip.setAttribute("role", "listitem");
    chip.dataset.kind = c.kind;
    const what = (c.text || k.say()) + (c.n && !c.text ? " (" + c.n + ")" : "");
    const where = { done: T("done"), skipped: T("skipped"), now: T("now"), later: T("next"), undone: T("undone") }[c.state];
    chip.title = where + ": " + what;
    chip.setAttribute("aria-label", chip.title);
    chip.append(mark(c.state === "done" ? "ok" : c.state === "undone" ? "undo" : k.icon));
    chip.append(el("span", "ss-name", k.short()));
    if (c.n) chip.append(el("b", "ss-n", String(c.n)));
    if (c.state === "skipped") {
      // a skipped step can be taken up again
      chip.tabIndex = 0;
      chip.onclick = () => { if (flow.busy) return; flow.skipped.delete(c.kind); refresh(flow, node); };
    }
    if (c.undo) {
      const u = el("button", "ss-undo");
      u.append(mark("undo"), keyCaps("u"));
      u.title = Tf("undo: %s", what);
      u.setAttribute("aria-label", u.title);
      u.onclick = () => undo(flow, node);
      chip.append(u);
    }
    row.append(chip);
  }
  return row;
}

// card: the current step: its sentence, its counts, then every Mod it changes.
function card(step) {
  const k = KINDS[step.kind], n = step.items.length;
  const box = el("div", "ss-card");
  box.dataset.kind = step.kind;
  const head = el("div", "ss-head");
  head.append(mark(k.icon), el("span", "", k.say()));
  box.append(head);
  const sum = el("div", "ss-sum");
  const pill = (cls, glyph, text, title) => { const p = el("span", "ss-pill " + cls); p.append(mark(glyph), el("span", "", text)); p.title = title; p.setAttribute("aria-label", title); sum.append(p); };
  if (step.kind === "enable") pill("add", "plus", "+" + n, Tf("%d mods enabled", n));
  if (step.kind === "use-installed") pill("copy", "copy", String(n), Tf("%d entries use an installed copy", n));
  if (step.kind === "replace") pill("swap", "swap", String(n), Tf("%d mods replaced, each at its own place", n));
  if (step.kind === "subscribe") pill("get", "import", String(n), Tf("%d mods to download", n));
  if (step.moved) pill("moved", "order", String(step.moved), Tf("%d other mods change place", step.moved));
  if (!step.preview && step.kind !== "subscribe") pill("later", "hint", T("no preview"), T("the preview could not be read"));
  box.append(sum);
  const list = el("ol", "ss-rows");
  for (const it of step.items) list.append(row(it));
  box.append(list);
  if (step.error) box.append(el("div", "ss-err", step.error));
  return box;
}

// row: one Mod: what happens to it (icon), its name, where it lands, and why (muted).
function row(it) {
  const r = el("li", "ss-row " + it.change);
  const glyph = { copy: "copy", replace: "swap", add: "plus", get: "import", none: "hint" }[it.change] || "hint";
  r.append(mark(glyph));
  const names = el("span", "ss-names");
  if (it.old) { names.append(el("s", "ss-old", it.old), el("span", "ss-arrow", "→")); }
  names.append(el("span", "ss-mod", it.name));
  r.append(names);
  if (it.at) { const at = el("span", "ss-at", "#" + it.at); at.title = Tf("position %d in the list", it.at); r.append(at); }
  if (it.for?.length) { const f = el("span", "ss-for", Tf("for %s", it.for.join(T(", ")))); f.title = Tf("required by %s", it.for.join(T(", "))); r.append(f); }
  if (it.why) r.append(el("span", "ss-for", it.why));
  const words = [it.old ? Tf("%s replaces %s", it.name, it.old) : it.name, it.at ? Tf("position %d", it.at) : "", it.for?.length ? Tf("required by %s", it.for.join(T(", "))) : "", it.why || ""].filter(Boolean).join(", ");
  r.setAttribute("aria-label", words);
  return r;
}

// summary: nothing left: what was done and skipped, one row each.
function summary(flow) {
  const box = el("div", "ss-card ss-done");
  const head = el("div", "ss-head");
  head.append(mark("ok"), el("span", "", T("all steps done")));
  box.append(head);
  const list = el("ol", "ss-rows");
  for (const d of flow.done) {
    const r = el("li", "ss-row done");
    r.append(mark(KINDS[d.kind].icon), el("span", "ss-mod", d.text || KINDS[d.kind].say()));
    list.append(r);
  }
  for (const k of ORDER.filter((x) => flow.skipped.has(x))) {
    const r = el("li", "ss-row skipped");
    r.append(mark(KINDS[k].icon), el("span", "ss-mod", Tf("skipped: %s", KINDS[k].short())));
    list.append(r);
  }
  box.append(list);
  return box;
}

// unfixable: what no step fixes here, folded under a count.
function unfixable(items) {
  const box = el("details", "ss-unfix");
  const sum = el("summary", "");
  sum.append(mark("hint"), el("span", "", Tf("%d cannot be fixed here", items.length)));
  box.append(sum);
  const list = el("ol", "ss-rows");
  for (const it of items) list.append(row(it));
  box.append(list);
  return box;
}

function buttons(flow, node, step) {
  const btns = el("div", "btns");
  const close = el("button", "", step ? T("stop here") : T("close"));
  close.append(keyCaps("esc"));
  close.onclick = () => pop.close();
  btns.append(close);
  if (!step) { close.className = "go"; close.replaceChildren(T("close"), keyCaps("enter")); return btns; }
  const skip = el("button", "ss-skip", T("skip"));
  skip.append(keyCaps("s"));
  skip.onclick = () => { if (flow.busy) return; flow.skipped.add(step.kind); flow.error = ""; refresh(flow, node); };
  btns.append(skip);
  const n = step.items.length;
  if (step.kind === "subscribe") {
    const [steam, cmd] = getChoices();
    const via = (label, title, i) => {
      const b = el("button", i === 0 ? "go" : "", label);
      b.append(el("b", "ss-n", String(n)));
      if (i === 0) b.append(keyCaps("enter"));
      b.title = title;
      b.dataset.envWrite = "apply";
      b.onclick = () => subscribe(flow, node, step, i === 1);
      btns.append(b);
    };
    via(T("get with SteamCMD"), cmd, 1);
    via(T("get with Steam"), steam, 0);
    return btns;
  }
  const go = el("button", "go", KINDS[step.kind].go());
  go.append(el("b", "ss-n", String(n)), keyCaps("enter"));
  go.dataset.envWrite = "apply";
  go.disabled = !!step.error;
  go.onclick = () => apply(flow, node, step);
  btns.append(go);
  return btns;
}

async function apply(flow, node, step) {
  if (flow.busy) return;
  flow.busy = true; flow.error = "";
  draw(flow, node);
  const r = await ui.edited(() => api.ApplySortStep(step.kind), true);
  flow.busy = false;
  if (r) flow.done.push({ kind: step.kind, n: step.items.length, text: r.text, undo: !!r.undo });
  else flow.error = T("this step did not apply; the list is unchanged");
  await refresh(flow, node);
}

async function subscribe(flow, node, step, steamcmd) {
  if (flow.busy) return;
  flow.busy = true; flow.error = "";
  draw(flow, node);
  try {
    const text = await api.Subscribe({ kind: "workshop", url: step.workshopIds.join(" "), steamcmd, batch: true });
    say("ok", text);
    flow.done.push({ kind: step.kind, n: step.items.length, text, undo: false });
  } catch (e) { flow.error = errText(e); }
  flow.busy = false;
  await refresh(flow, node);
}

// undo takes back the latest applied step, the sort included; that step comes back.
async function undo(flow, node) {
  const last = flow.done[flow.done.length - 1];
  if (flow.busy || !last || !last.undo) return;
  flow.busy = true;
  draw(flow, node);
  const r = await ui.edited(() => api.Undo(), false);
  flow.busy = false;
  if (r !== null) {
    flow.done.pop();
    if (last.kind === "sort") flow.sort.state = "undone";
  }
  await refresh(flow, node);
}
