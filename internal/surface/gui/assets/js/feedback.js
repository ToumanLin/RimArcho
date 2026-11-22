// Feedback (16 §2.14): two panes, as All lists (16 §7.3 item 12). The left lists the report
// templates and My reports; the right card is the picked template's questions under a head
// with the problems, the buttons and Archo. The gap between them drags (16 §3.1 rule 10). A
// report is sent through the feedback relay (send, no GitHub account needed; offered only when
// RimArcho has one) or opened in the browser with the title and body filled in
// (view.FeedbackAttachments, ComposeProblem, SendFeedbackProblems). Each template ticks its
// own attachments; the rest stay behind "change". One report, one topic: "add another problem"
// keeps a second (and third) problem in the same draft, and send files each as its own issue.
// The whole text shows (preview) before anything leaves. A body too long for a link is copied
// and the link says to paste it.
import { copy } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { icon } from "./icons.js";
import { page, button, area, beginPageRead } from "./screens.js";
import { archoSpot, ARCHO_PAGE } from "./archo.js";
import { wireSeparator } from "./separator.js";
import { sync } from "./shell.js";

const MAX_SPLIT = 3; // feedback.MaxSplit
const PARTS = ["mods", "paths", "log", "ops", "problems"];

// The draft stays for the session: leaving the page does not lose what was typed (16 §3.1).
// probs: the problems of this report, cur the one shown; side: the template list's pick, or
// "mine" for My reports. sent: the issues send opened last, shown until the next draft. open:
// the preview's attachments unfolded by the player (by key), kept while typing.
const F = {
  probs: [], cur: 0, side: "", contact: "", view: "form", change: false,
  tpls: null, choices: null, similar: [], at: null, res: null, sent: null, sending: false, failed: false,
  open: new Set(), mine: null, fresh: new Set(), freshAt: new Set(), landed: 0,
};
let seq = 0, typing = 0, retry = 0, simSeq = 0, simTimer = 0;

const tpl = (kind) => F.tpls?.find(t => t.kind === kind) || F.tpls?.[0];
const prob = () => F.probs[F.cur];
// optsFor: the attachments a template ticks when it is chosen
function optsFor(kind) {
  const o = { mods: false, paths: false, log: false, ops: false, problems: false };
  for (const k of tpl(kind)?.attach || []) o[k] = true;
  return o;
}
const fresh = (kind) => ({ kind, title: "", answers: {}, opts: optsFor(kind) });
// wire is the problem as the API takes it
const wire = (p) => ({ kind: p.kind, title: p.title, answers: { ...p.answers }, opts: { ...p.opts } });
const isEmpty = (p) => !p.title.trim() && !Object.entries(p.answers).some(([k, v]) => k !== "files" && k !== "every" && String(v || "").trim());
const priority = (p) => p.kind === "bug" && p.answers.files === "yes";

export async function open() {
  const read = beginPageRead("feedback");
  try {
    if (!F.tpls) F.tpls = await api.FeedbackTemplates();
    if (!F.probs.length) { F.probs = [fresh(F.tpls[0].kind)]; F.cur = 0; }
    const [, mine] = await Promise.all([attach(), api.FeedbackReports()]);
    F.mine = mine;
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  // came from a Can it run item's or a Mod's flag: that report, filled in (16 §2.4, §2.9)
  const pre = S.feedbackPrefill;
  S.feedbackPrefill = null;
  const filled = pre && prefill(pre);
  if (filled) await attach().catch(() => {});
  if (!read.current()) return;
  // came for an unseen reply (the sidebar's dot): land on My reports, not on the form
  const hit = filled ? null : F.mine?.reports.find(r => r.unseen);
  if (F.mine) for (const r of F.mine.reports) if (r.unseen) F.fresh.add(r.number);
  F.landed = hit ? hit.number : 0;
  F.side = hit ? "mine" : prob().kind;
  draw();
  read.finish();
  if (hit) $(`#screen .fb-rep[data-report="${hit.number}"]`)?.scrollIntoView({ block: "center" });
  else (filled ? openField() : firstField())?.focus({ preventScroll: true });
  checkReplies();
  loadChoices();
}

// prefill starts the problem a flag asked for: kind with its answers, in place of an empty
// problem or as another one; a problem already about the same item is shown instead. False:
// no room (three problems), said in the status line.
function prefill({ kind, answers }) {
  if (!tpl(kind) || tpl(kind).kind !== kind) return false;
  const same = F.probs.findIndex(p => p.kind === kind && Object.entries(answers).every(([k, v]) => p.answers[k] === v));
  if (same >= 0) { F.cur = same; }
  else {
    const p = { ...fresh(kind), answers: { ...answers } };
    if (isEmpty(prob())) F.probs[F.cur] = p;
    else if (F.probs.length < MAX_SPLIT) { F.probs.push(p); F.cur = F.probs.length - 1; }
    else { say("warn", T("send or remove a problem first")); return false; }
  }
  F.sent = null; F.failed = false; F.view = "form"; F.change = false;
  return true;
}

// openField is the first question the flag left for the player (why, what it should be).
const openField = () => [...document.querySelectorAll("#screen .fb-main .fb-q .field")].find(f => !f.value.trim()) || firstField();

const firstField = () => $("#screen .fb-main .fb-q .field, #screen .fb-main .fb-title");

// checkReplies asks the relay for replies once the page shows (16 §2.14 My reports). A
// failure is quiet: the last answer stays, with one muted line.
async function checkReplies() {
  let mine;
  try { mine = await api.FeedbackCheck(); } catch { return; }
  if (S.screen !== "feedback") return;
  F.mine = mine;
  // the replies new on this visit stay marked while the page is open; the dot goes
  for (const r of mine.reports) if (r.unseen) F.fresh.add(r.number);
  if (!F.landed) { const n = mine.reports.find(r => r.unseen); if (n) F.landed = n.number; }
  drawSide();
  if (F.side === "mine") drawMain();
  if (mine.unseen) api.FeedbackSeen().catch(() => {});
}

// loadChoices reads the diagnoses and Mods "which diagnosis" and "which Mod" offer.
async function loadChoices() {
  try { F.choices = await api.FeedbackChoices(); } catch { return; }
  if (S.screen !== "feedback") return;
  fillLists();
}

// attach reads the attachments for the problem shown, then the body; the newest request wins.
async function attach() {
  const n = ++seq;
  const at = await api.FeedbackAttach(wire(prob()));
  if (n !== seq) return false;
  F.at = at;
  F.res = await api.FeedbackCompose(wire(prob()));
  clearTimeout(retry);
  // the game log is still being analysed: ask again while the page is open
  if (at.pending) retry = setTimeout(() => { if (S.screen === "feedback") attach().then(ok => ok !== false && refresh()).catch(() => {}); }, 1000);
  return true;
}

async function compose() {
  const n = seq;
  const res = await api.FeedbackCompose(wire(prob()));
  if (n === seq && S.screen === "feedback") { F.res = res; refresh(); }
}

// reattach after a change that moves the attachments (a template, a box, the diagnosis or Mod)
const reattach = () => attach().then(ok => { if (ok !== false && S.screen === "feedback") refresh(); }).catch(e => say("fail", errText(e)));

// Archo stands at the right end of the right card's head (16 §7.1 "Placements"), at the pages'
// size: thinking while you write, pondering while it sends, a salute once sent, tears if
// sending failed, a wink at a new reply.
const spot = () => archoSpot("feedback", { size: ARCHO_PAGE, calm: ["neutral", "ponder", "think", "wink", "smirk"] });
function fbFace() {
  if (F.sending) return "ponder";
  if (F.failed) return "cry";
  if (F.sent) return "salute";
  if (F.probs.some(p => !isEmpty(p))) return "think";
  return F.landed ? "wink" : "neutral";
}
// only the empty form is calm: her idle faces follow there, never over a state
const face = () => { const e = fbFace(); spot().stand($("#screen .fb-bar .fig"), e, e === "neutral"); };

// crumb: the breadcrumb is "feedback / <template>" or "feedback / my reports" (16 §2.2)
function crumb() {
  const c = F.side === "mine" ? T("my reports") : tpl(F.side)?.label || "";
  if (S.feedbackCrumb === c) return;
  S.feedbackCrumb = c;
  sync();
}

function draw() {
  const sc = $("#screen");
  sc.innerHTML = "";
  const w = page("feedback fill fb-page", "");
  const grid = el("div", "fb-lay");
  const side = el("div", "fb-side");
  const card = el("section", "card settings xc-do fb-kinds");
  side.append(card);
  const main = el("div", "fb-main");
  // the gap between the two cards drags; the left starts at All lists' 38% (16 §7.3 item 12)
  const gap = el("div", "split fb-split");
  grid.append(side, gap, main);
  const setSide = (v) => { if (v) grid.style.setProperty("--fb-side", v + "px"); else grid.style.removeProperty("--fb-side"); };
  const range = () => [220, Math.max(220, grid.clientWidth - 420)];
  wireSeparator(gap, {
    key: "ui.gui.feedback.side", range,
    get: () => side.getBoundingClientRect().width,
    set: setSide, reset: () => setSide(0),
    restore: (v) => setSide(+v ? Math.max(range()[0], +v) : 0),
  });
  w.append(grid);
  sc.append(w);
  area(card, null);
  drawSide();
  drawMain();
}

// drawSide is the left: the templates (the one shown marked, with how many problems use each)
// and My reports with its reply count.
function drawSide() {
  const card = $("#screen .fb-kinds");
  if (!card) return;
  card.innerHTML = "";
  for (const t of F.tpls || []) {
    const r = kindRow(t.kind, t.label, t.lead, F.side === t.kind);
    if (t.kind === "bug" && F.probs.some(priority)) r.classList.add("prio");
    card.append(r);
  }
  const m = F.mine;
  if (m && (F.at?.send || m.reports.length)) {
    const n = m.reports.length;
    const unseen = m.reports.filter(r => r.unseen || F.fresh.has(r.number)).length;
    const r = kindRow("mine", T("my reports"), n ? Tf("%d sent", n) : T("none sent yet"), F.side === "mine");
    r.classList.add("fb-mine-row");
    if (unseen) { const d = el("span", "fb-rep-dot"); d.title = T("new reply"); r.querySelector(".name").prepend(d); }
    card.append(r);
  }
}

function kindRow(kind, name, lead, on) {
  const r = el("button", "xc-dorow fb-kind" + (on ? " on" : ""));
  r.type = "button";
  r.dataset.kind = kind;
  const l = el("div", "l"), t = el("div", "name");
  t.append(el("span", "nt", name));
  l.append(t, el("div", "sub", lead));
  r.append(l);
  r.onclick = () => pick(kind);
  return r;
}

// pick shows a template for the problem shown (or My reports). A problem already written in
// another template keeps its title; its answers stay for when it comes back.
function pick(kind) {
  if (kind === "mine") { F.side = "mine"; drawSide(); drawMain(); return; }
  F.side = kind;
  const p = prob();
  if (p.kind !== kind) { p.kind = kind; p.opts = optsFor(kind); F.change = false; }
  F.view = "form";
  F.sent = null; F.failed = false;
  drawSide();
  drawMain();
  firstField()?.focus({ preventScroll: true });
  reattach();
  similarSoon();
}

// drawMain is the right: one card whose head (.fb-bar: the problems, the buttons, Archo) stands
// over its body, the problem shown as a form or as its preview, or My reports. The head is
// inside the card, so the two cards' tops are one line (16 §7.3 item 12).
function drawMain() {
  const main = $("#screen .fb-main");
  if (!main) return;
  main.innerHTML = "";
  crumb();
  const card = el("section", "card fb-card");
  main.append(card);
  if (F.side === "mine") {
    const head = el("div", "fb-bar fb-mine-bar");
    const note = el("div", "fb-mine-note");
    head.append(note, el("div", "fig"));
    card.append(head, el("div", "settings fb-body fb-mine"));
    area(card, null);
    drawMine();
    face();
    return;
  }
  const bar = el("div", "fb-bar");
  bar.append(el("div", "fb-probs"), el("div", "fb-acts"), el("div", "fig"));
  const body = el("div", "settings fb-body fb-form");
  card.append(bar, body);
  area(card, null, () => firstField()?.focus());
  if (F.view === "preview") drawPreview(body); else drawForm(body);
  body.append(el("div", "fb-similar"));
  refresh();
}

// drawForm: the template's hint, the title, its questions, the contact, what goes with it
function drawForm(card) {
  const p = prob(), t = tpl(p.kind);
  if (t.note) card.append(el("div", "fb-note", t.note));
  const title = el("input", "field fb-title");
  title.value = p.title; title.spellcheck = false;
  title.setAttribute("aria-label", T("title"));
  title.placeholder = T("optional; the first answer's first line otherwise");
  title.oninput = () => { p.title = title.value; typed(); };
  card.append(field(T("title"), title));
  for (const q of t.questions) card.append(question(p, q));
  // the contact goes to the relay only, never into the issue; GitHub has the account instead
  if (F.at?.send) {
    const contact = el("input", "field fb-title fb-contact");
    contact.value = F.contact; contact.spellcheck = false;
    contact.placeholder = T("Discord name or email, so the maintainer can ask you more");
    contact.oninput = () => { F.contact = contact.value; };
    card.append(field(T("contact (optional, not public)"), contact));
  }
  card.append(el("div", "fb-att"));
  const lists = el("div", "fb-lists");
  lists.hidden = true;
  card.append(lists);
  fillLists();
}

function field(label, input, cls = "") {
  const l = el("label", "fb-field " + cls);
  l.append(typeof label === "string" ? el("span", "", label) : label, input);
  return l;
}

// question is one of the template's questions as its form asks: lines, a line, a choice of
// pills, a box, or a line with the current diagnoses or Mods to pick from.
function question(p, q) {
  // the label, and a muted "(optional)" beside it
  const label = () => { const l = el("span", "", q.label); if (q.optional) l.append(el("span", "fb-opt", T("(optional)"))); return l; };
  const set = (v) => { if (v) p.answers[q.key] = v; else delete p.answers[q.key]; };
  if (q.form === "check") {
    const box = el("label", "fb-check fb-q fb-files");
    const c = el("input");
    c.type = "checkbox"; c.dataset.q = q.key;
    c.checked = p.answers[q.key] === "yes";
    c.onchange = () => { set(c.checked ? "yes" : ""); F.sent = null; drawSide(); reattach(); typed(); };
    box.append(c, el("span", "name", q.label));
    return box;
  }
  if (q.form === "choice") {
    const wrap = el("div", "fb-field fb-q");
    wrap.append(label());
    const pills = el("div", "fb-pills");
    pills.setAttribute("role", "radiogroup");
    pills.setAttribute("aria-label", q.label);
    for (const c of q.choices) {
      const b = button(c, "", () => { set(p.answers[q.key] === c ? "" : c); for (const x of pills.children) x.classList.toggle("on", x.textContent === p.answers[q.key]); typed(); }, "fb-pill" + (p.answers[q.key] === c ? " on" : ""));
      b.setAttribute("role", "radio");
      pills.append(b);
    }
    wrap.append(pills);
    return wrap;
  }
  const multi = q.form === "text";
  const f = el(multi ? "textarea" : "input", "field " + (multi ? "fb-text" : "fb-title"));
  f.dataset.q = q.key;
  f.value = p.answers[q.key] || "";
  f.spellcheck = false;
  if (q.hint) f.placeholder = q.hint;
  f.setAttribute("aria-label", q.label);
  if (q.form === "finding" || q.form === "mod") f.setAttribute("list", "fb-list-" + q.form);
  f.oninput = () => {
    set(f.value.trim() ? f.value : "");
    typed();
    // the diagnosis or Mod named goes with the report: read again a moment after typing stops
    if (q.form === "finding" || q.form === "mod") { clearTimeout(f._t); f._t = setTimeout(reattach, 400); }
    if (p.kind === "feature") similarSoon();
  };
  return field(label(), f, "fb-q");
}

// fillLists puts the diagnoses and Mods in the datalists the two kinds of line use.
function fillLists() {
  const box = $("#screen .fb-lists");
  if (!box || !F.choices) return;
  box.innerHTML = "";
  for (const [id, rows] of [["finding", F.choices.findings], ["mod", F.choices.mods]]) {
    const dl = el("datalist");
    dl.id = "fb-list-" + id;
    for (const r of rows || []) { const o = el("option"); o.value = r.id; o.label = r.text; dl.append(o); }
    box.append(dl);
  }
}

function typed() {
  F.sent = null; F.failed = false;
  face();
  drawProbs();
  clearTimeout(typing);
  typing = setTimeout(() => compose().catch(e => say("fail", errText(e))), 150);
}

// drawAtt is what goes with the report: chips for the basics and each attachment, and
// "change" for the boxes of the optional ones (16 §7 principle 9: the detail behind a click).
function drawAtt() {
  const box = $("#screen .fb-att");
  if (!box) return;
  box.innerHTML = "";
  const row = el("div", "fb-chips");
  row.append(el("span", "fb-att-l", T("attached")));
  row.append(chip(T("basic information"), (F.at?.basic || []).join("\n")));
  for (const s of F.at?.sections || []) row.append(chip(s.heading, s.lines.slice(0, 12).join("\n"), s.key === "ops" && priority(prob())));
  if (prob().opts.log && F.at?.pending) row.append(el("span", "fb-chip wait", T("the game log is still being analysed…")));
  // files lost with nothing in the operation log: say so rather than leave the chip out
  const ops = F.at?.parts?.find(x => x.key === "ops");
  if (priority(prob()) && ops?.none) row.append(el("span", "fb-chip wait", ops.label + ": " + ops.note));
  const ch = button(F.change ? T("done") : T("change"), "", () => { F.change = !F.change; drawAtt(); }, "link fb-change");
  ch.setAttribute("aria-expanded", String(F.change));
  row.append(ch);
  box.append(row);
  if (!F.change) return;
  const parts = el("div", "fb-parts");
  for (const part of F.at?.parts || []) parts.append(partRow(part));
  box.append(parts);
}

function chip(label, tip, hot = false) {
  const c = el("span", "fb-chip" + (hot ? " hot" : ""), label);
  if (tip) c.title = tip;
  return c;
}

function partRow(part) {
  const p = prob();
  const box = el("label", "fb-check fb-part");
  box.dataset.part = part.key;
  const c = el("input");
  c.type = "checkbox";
  const forced = part.key === "ops" && priority(p);
  c.checked = forced || !!p.opts[part.key];
  c.disabled = !!part.none || forced;
  box.append(c, el("span", "name", part.label), el("span", "sub", part.note || ""));
  c.onchange = () => { p.opts[part.key] = c.checked; reattach(); };
  return box;
}

// drawProbs is the bar's left: the problems of this report, the one shown marked, each with
// its template's name; "add another problem" while there is room.
function drawProbs() {
  const box = $("#screen .fb-probs");
  if (!box) return;
  box.innerHTML = "";
  if (F.probs.length > 1) {
    F.probs.forEach((p, i) => {
      const t = tpl(p.kind);
      const c = el("span", "fb-prob" + (i === F.cur ? " on" : "") + (priority(p) ? " prio" : ""));
      const b = button(`${i + 1}  ${p.title.trim() || t.label}`, "", () => show(i), "fb-prob-b");
      b.title = t.label;
      const x = button("", "", () => drop(i), "icon fb-prob-x");
      x.innerHTML = icon.close || "×";
      x.title = T("remove this problem");
      x.setAttribute("aria-label", T("remove this problem"));
      c.append(b, x);
      box.append(c);
    });
  }
  if (F.probs.length < MAX_SPLIT) {
    const add = button("+ " + T("add another problem"), "", addProblem, "link fb-add");
    add.title = T("one report, one topic: each problem is sent as its own report");
    box.append(add);
  }
}

function show(i) {
  F.cur = i;
  F.side = prob().kind;
  F.view = "form";
  F.change = false;
  drawSide(); drawMain();
  reattach();
  similarSoon();
}

function addProblem() {
  if (F.probs.length >= MAX_SPLIT) return;
  F.probs.push(fresh(prob().kind));
  show(F.probs.length - 1);
  firstField()?.focus();
}

function drop(i) {
  F.probs.splice(i, 1);
  if (!F.probs.length) F.probs = [fresh(F.tpls[0].kind)];
  show(Math.min(F.cur, F.probs.length - 1));
}

// similarSoon reads the open feature requests like the one being written (title and what the
// player wants), a moment after typing stops; only for a feature request.
function similarSoon() {
  clearTimeout(simTimer);
  const p = prob();
  if (p.kind !== "feature") { F.similar = []; drawSimilar(); return; }
  simTimer = setTimeout(async () => {
    const n = ++simSeq, text = (p.title + " " + (p.answers.want || "")).trim();
    if (!text) { F.similar = []; drawSimilar(); return; }
    let rs = [];
    try { rs = await api.FeedbackSimilar(text); } catch { /* quiet: no list */ }
    if (n !== simSeq || S.screen !== "feedback") return;
    F.similar = rs || [];
    drawSimilar();
  }, 400);
}

// drawSimilar lists them under the form, inside its card: each title opens GitHub; +1 adds this player's vote
// through the relay instead of a new report.
function drawSimilar() {
  const box = $("#screen .fb-similar");
  if (!box) return;
  box.innerHTML = "";
  if (prob()?.kind !== "feature" || !F.similar.length || F.view !== "form") return;
  const head = el("div", "label section", T("similar requests"));
  head.append(el("span", "fb-public", T("a +1 counts more than a new report")));
  const card = el("div", "settings fb-sim");
  for (const r of F.similar) {
    const row = el("div", "fb-sim-row");
    const t = button(r.title, "", () => api.OpenURL(r.url).catch(e => say("fail", errText(e))), "link fb-sim-t");
    t.title = T("open on GitHub");
    row.append(t, el("span", "fb-rep-num num", "#" + r.number));
    const plus = button(r.given ? `+${r.plus} ✓` : `+1  ${r.plus}`, "", async () => {
      plus.disabled = true;
      try { r.plus = await api.FeedbackPlus(r.number); r.given = true; say("ok", Tf("+1 for #%d", r.number)); }
      catch (e) { say("fail", errText(e)); }
      drawSimilar();
    }, "fb-plus" + (r.given ? " given" : ""));
    plus.disabled = !!r.given;
    plus.title = r.given ? T("you gave this request a +1") : T("+1: you want this too");
    row.append(plus);
    card.append(row);
  }
  box.append(head, card);
}

// drawPreview is the report as the player reads it: the title, the answers, the basic lines,
// then each attachment folded under its heading. No Markdown or HTML here; that is only in
// the body sent and in copy (F.res.body). The TUI shows view.FeedbackPreview.
function drawPreview(card) {
  card.classList.add("fb-prev-card");
  const head = el("div", "fb-prev-head");
  head.append(el("span", "fb-p-head", T("preview")), el("span", "fb-public", T("public on GitHub")));
  card.append(head, el("div", "fb-prev"));
}

function fillPreview() {
  const box = $("#screen .fb-prev");
  if (!box) return;
  box.innerHTML = "";
  const p = prob();
  const title = p.title.trim() || firstLine(p);
  if (title) box.append(el("div", "fb-p-title", title));
  const t = tpl(p.kind);
  if (priority(p)) box.append(el("div", "fb-p-prio", t.questions.find(q => q.key === "files")?.label || ""));
  for (const q of t.questions) {
    const v = String(p.answers[q.key] || "").trim();
    if (!v || q.form === "check") continue;
    const d = el("div", "fb-p-ans");
    if (t.kind !== "other") d.append(el("div", "fb-p-head", q.label));
    d.append(el("div", "fb-p-text", v));
    box.append(d);
  }
  const basic = el("div", "fb-p-part");
  basic.append(el("div", "fb-p-head", T("basic information")), el("div", "fb-p-lines", (F.at?.basic || []).join("\n")));
  box.append(basic);
  for (const s of F.at?.sections || []) {
    const d = el("details", "fb-p-part");
    d.open = F.open.has(s.key);
    d.ontoggle = () => { if (d.open) F.open.add(s.key); else F.open.delete(s.key); };
    d.append(el("summary", "fb-p-head", s.heading), el("div", "fb-p-lines", s.lines.join("\n")));
    box.append(d);
  }
}

function firstLine(p) {
  for (const q of tpl(p.kind).questions) {
    if (q.form === "check" || q.form === "choice") continue;
    const v = String(p.answers[q.key] || "").trim();
    if (v) return v.split("\n")[0].trim().slice(0, 60);
  }
  return "";
}

// refresh shows the newest attachments, preview, buttons and face without touching what is
// being typed.
function refresh() {
  if (F.side === "mine" || !F.res) return;
  drawProbs();
  drawAtt();
  fillPreview();
  drawSimilar();
  face();
  const acts = $("#screen .fb-acts");
  if (!acts) return;
  acts.innerHTML = "";
  const p = prob();
  const pv = button(F.view === "preview" ? T("edit") : T("preview"), "", () => { F.view = F.view === "preview" ? "form" : "preview"; drawMain(); }, "fb-tab-preview");
  const copyBtn = button(T("copy the text"), "", () => copy(F.res.body, copyBtn));
  const gh = button(F.res.full ? T("open GitHub") : T("copy and open GitHub"), "", openIssue, F.at?.send ? "" : "primary");
  if (F.probs.length > 1) gh.title = T("opens the problem shown; GitHub takes one at a time");
  const m = el("div", "fb-status");
  if (!F.at?.send) {
    m.className = "fb-status" + (F.res.full ? "" : " warn");
    m.textContent = F.res.full ? T("opens GitHub in the browser; nothing is sent until you submit it there")
      : T("too long for a link: the link carries the title and the basics, and the full text is copied for you to paste");
    acts.append(pv, copyBtn, gh, m);
    return;
  }
  // send first; GitHub stays as the other way for players with an account
  if (F.sent?.length) {
    for (const s of F.sent) m.append(button(Tf("sent as #%d", s.number) + " ↗", "", () => api.OpenURL(s.url).catch(e => say("fail", errText(e))), "link"));
  } else if (priority(p)) m.append(el("span", "fb-prio-note", T("files changed or lost: sent as a priority report, with the recent operations")));
  const n = F.probs.length;
  const send = button(F.sending ? T("sending…") : n > 1 ? Tf("send %d reports", n) : T("send"), "", sendReport, "primary fb-send");
  send.disabled = F.sending || F.probs.some(isEmpty);
  if (n > 1 && F.probs.some(isEmpty)) send.title = T("every problem needs a title or an answer");
  acts.append(pv, copyBtn, gh, send, m);
}

async function sendReport() {
  if (F.sending) return;
  F.sending = true;
  F.failed = false;
  refresh();
  const ps = F.probs.map(wire);
  let sent = [];
  try {
    sent = await api.FeedbackSend(ps, F.contact);
    F.sent = sent;
    F.probs = [fresh(F.probs[0].kind)];
    F.cur = 0; F.view = "form";
    say("ok", sent.length > 1 ? Tf("sent as %s", sent.map(s => "#" + s.number).join(", ")) : Tf("sent as #%d", sent[0].number));
  } catch (e) {
    F.failed = true;
    say("fail", errText(e));
    // the problems filed before a failure leave the draft; the rest stay to send again
    if (sent?.length) { F.probs = F.probs.slice(sent.length); F.cur = 0; }
  } finally {
    F.sending = false;
    try { F.mine = await api.FeedbackReports(); } catch { /* the list stays as it was */ }
    if (S.screen === "feedback") {
      try { await attach(); } catch { /* the bar says why */ }
      F.side = prob().kind;
      drawSide(); drawMain();
    }
  }
}

async function openIssue() {
  const res = F.res;
  if (!res) return;
  try {
    if (!res.full) await copy(res.body, $("#screen .fb-acts .btn.primary"));
    await api.OpenURL(res.url);
    say("ok", res.full ? T("opened GitHub in the browser") : T("opened GitHub; paste the copied text into the issue"));
  } catch (e) { say("fail", errText(e)); }
}

// ---- My reports: the reports sent from here, their state and the developer's replies as
// plain text, with a follow-up for each. Only when the relay is used.

function drawMine() {
  const card = $("#screen .fb-mine");
  if (!card) return;
  card.innerHTML = "";
  const m = F.mine;
  // at the top: what replies are, and why there may be none just now
  const note = $("#screen .fb-mine-note");
  if (note && m) {
    note.innerHTML = "";
    note.append(el("span", "", T("the developer may reply here; not guaranteed, and public on GitHub")));
    const why = m.off ? T("checking for replies is off (Settings › network)") : m.failed ? T("could not check for replies just now; RimArcho asks again next time") : "";
    if (why) note.append(el("span", "fb-mine-why", why));
  }
  if (!m || !m.reports.length) { card.append(el("div", "fb-mine-empty", T("no reports sent from this computer yet"))); return; }
  for (const r of m.reports) card.append(reportRow(r));
}

// reportRow is one report: its title and state on one line, the date and reply count under
// it, then the developer's replies as plain text, then a quiet follow-up.
function reportRow(r) {
  const isNew = F.fresh.has(r.number) || r.unseen;
  const row = el("div", "fb-rep" + (r.number === F.landed ? " landed" : ""));
  row.dataset.report = String(r.number);
  const head = el("div", "fb-rep-head");
  if (isNew) { const d = el("span", "fb-rep-dot"); d.title = T("new reply"); head.append(d); }
  head.append(el("span", "fb-rep-title", r.title || "#" + r.number), el("span", "fb-rep-num num", "#" + r.number));
  const state = r.gone ? T("no longer on GitHub") : r.state === "closed" ? T("closed") : T("still open");
  const pill = el("span", "fb-rep-state" + (r.state === "closed" && !r.gone ? " closed" : ""), state);
  if (r.state === "closed" && r.closedAt) pill.title = Tf("closed %s", day(r.closedAt));
  head.append(pill);
  if (r.url && !r.gone) {
    const gh = button("", "", () => api.OpenURL(r.url).catch(e => say("fail", errText(e))), "icon fb-rep-open");
    gh.innerHTML = icon.ext;
    gh.title = T("open on GitHub");
    gh.setAttribute("aria-label", T("open on GitHub"));
    head.append(gh);
  }
  row.append(head);
  const facts = el("div", "fb-rep-facts word-facts");
  facts.append(el("span", "", Tf("sent %s", day(r.sentAt))));
  facts.append(el("span", isNew ? "fb-rep-newtext" : "", r.replies.length ? (isNew ? T("new reply") : Tf("replies: %d", r.replies.length)) : T("no reply yet")));
  row.append(facts);
  if (r.replies.length) {
    const thread = el("div", "fb-thread");
    for (const p of r.replies) {
      const fr = F.freshAt.has(r.number + "|" + p.created_at) || (isNew && p.created_at > (r.lastSeen || ""));
      if (fr) F.freshAt.add(r.number + "|" + p.created_at);
      const q = el("div", "fb-reply" + (fr ? " new" : ""));
      q.append(whoLine(p), el("div", "fb-reply-text", p.body));
      thread.append(q);
    }
    row.append(thread);
  }
  if (!r.gone) row.append(button(Tf("follow up on #%d", r.number), "", () => followUp(r), "link fb-rep-follow"));
  return row;
}

// followUp starts a new problem about #N (other / help) through the normal send path; #N in
// the title links the two on GitHub.
async function followUp(r) {
  let title;
  try { title = await api.FeedbackFollowUp(r.number, r.title || ""); } catch { title = "#" + r.number; }
  const p = prob();
  if (isEmpty(p)) { F.probs[F.cur] = { ...fresh("other"), title }; }
  else if (F.probs.length < MAX_SPLIT) { F.probs.push({ ...fresh("other"), title }); F.cur = F.probs.length - 1; }
  else { say("warn", T("send or remove a problem first")); return; }
  F.sent = null;
  F.side = "other"; F.view = "form";
  drawSide(); drawMain();
  reattach();
  const t = $("#screen .fb-main [data-q=\"text\"]");
  t?.focus({ preventScroll: false });
}

const day = (iso) => String(iso || "").slice(0, 10);

// whoLine is "developer" and the reply's date, spaced like other facts (16 §7.3).
function whoLine(p) {
  const w = el("div", "fb-reply-who word-facts");
  w.append(el("span", "", T("developer")), el("span", "", day(p.created_at)));
  return w;
}

export { PARTS };
