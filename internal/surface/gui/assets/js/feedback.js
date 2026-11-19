// Settings > Feedback (16 §2.14): a GitHub issue written here, sent through the feedback relay
// (send, no GitHub account needed; offered only when RimArcho has one) or opened in the
// browser with the title and body filled in (view.FeedbackAttachments, ComposeFeedback,
// SendFeedback). RimArcho, the game and the system always go with it; the enabled mods, the
// paths and the game log's errors only when ticked. The whole text shows before anything
// leaves. A body too long for a link is copied and the link says to paste it.
import { copy } from "./fields.js";
import { S, $, el, api, T, Tf, errText, say } from "./state.js";
import { page, button, area, beginPageRead } from "./screens.js";

// The draft stays for the session: leaving the page does not lose what was typed (16 §3.1).
// sent: the issue send opened last, shown until the next draft. open: the preview's
// attachments unfolded by the player (by key), kept while typing.
const F = { title: "", text: "", contact: "", opts: { mods: false, paths: false, log: false }, at: null, res: null, sent: null, sending: false, open: new Set(), mine: null, fresh: new Set() };
let seq = 0, typing = 0, retry = 0;

export async function open() {
  const read = beginPageRead("feedback");
  try {
    const [, mine] = await Promise.all([attach(), api.FeedbackReports()]);
    F.mine = mine;
  } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  draw();
  read.finish();
  $("#screen .fb-title")?.focus({ preventScroll: true });
  checkReplies();
}

// checkReplies asks the relay for replies once the page shows (16 §2.14 My reports). A
// failure is quiet: the last answer stays, with one muted line.
async function checkReplies() {
  let mine;
  try { mine = await api.FeedbackCheck(); } catch { return; }
  if (S.screen !== "feedback") return;
  F.mine = mine;
  // the replies new on this visit stay marked while the page is open; the dot goes
  for (const r of mine.reports) if (r.unseen) F.fresh.add(r.number);
  drawMine();
  if (mine.unseen) api.FeedbackSeen().catch(() => {});
}

// attach reads the attachments for the ticked options, then the body; the newest request wins.
async function attach() {
  const n = ++seq;
  const at = await api.FeedbackAttach({ ...F.opts });
  if (n !== seq) return false;
  F.at = at;
  F.res = await api.FeedbackCompose(F.title, F.text);
  clearTimeout(retry);
  // the game log is still being analysed: ask again while the page is open
  if (at.pending) retry = setTimeout(() => { if (S.screen === "feedback") attach().then(ok => ok !== false && refresh()).catch(() => {}); }, 1000);
  return true;
}

async function compose() {
  const n = seq;
  const res = await api.FeedbackCompose(F.title, F.text);
  if (n === seq && S.screen === "feedback") { F.res = res; refresh(); }
}

function draw() {
  const sc = $("#screen");
  const top = sc.scrollTop;
  sc.innerHTML = "";
  const w = page("feedback", "");
  sc.append(w);

  const lab1 = el("div", "label section", T("content"));
  const form = el("section", "card settings fb-form");
  const title = el("input", "field fb-title");
  title.value = F.title; title.spellcheck = false;
  title.setAttribute("aria-label", T("title"));
  const text = el("textarea", "field fb-text");
  text.value = F.text;
  text.setAttribute("aria-label", T("what happened"));
  text.placeholder = T("what happened, and what you expected; steps to make it happen again help");
  const typed = () => { F.title = title.value; F.text = text.value; F.sent = null; clearTimeout(typing); typing = setTimeout(() => compose().catch(e => say("fail", errText(e))), 150); };
  title.oninput = typed; text.oninput = typed;
  const l1 = el("label", "fb-field"); l1.append(el("span", "", T("title")), title);
  const l2 = el("label", "fb-field"); l2.append(el("span", "", T("what happened")), text);
  form.append(l1, l2);
  // the contact goes to the relay only, never into the issue; GitHub has the account instead
  if (F.at?.send) {
    const contact = el("input", "field fb-title");
    contact.value = F.contact; contact.spellcheck = false;
    contact.placeholder = T("Discord name or email, so the maintainer can ask you more");
    contact.oninput = () => { F.contact = contact.value; };
    const l3 = el("label", "fb-field"); l3.append(el("span", "", T("contact (optional, not public)")), contact);
    form.append(l3);
  }
  w.append(lab1, form);
  area(form, lab1, () => title.focus());

  const lab2 = el("div", "label section", T("attach"));
  const card = el("section", "card settings");
  card.append(partRow({ label: T("basic information"), fixed: true }));
  for (const p of F.at?.parts || []) card.append(partRow(p));
  w.append(lab2, card);
  area(card, lab2);

  const lab3 = el("div", "label section", T("preview"));
  lab3.append(el("span", "fb-public", T("public on GitHub")));
  const prev = el("section", "card settings");
  prev.append(el("div", "fb-prev"));
  w.append(lab3, prev);
  area(prev, lab3);

  w.append(el("div", "fb-foot"));
  // My reports: numbered once here; drawMine fills it again after each check
  const mine = el("div", "fb-mine-wrap");
  const labM = el("div", "label section", T("my reports"));
  const cardM = el("section", "card settings fb-mine");
  mine.append(labM, cardM);
  w.append(mine);
  area(cardM, labM);
  drawMine();
  refresh();
  sc.scrollTop = top;
}

// drawMine is My reports: the reports sent from here, their state and the developer's
// replies as plain text, with a follow-up for each. Only when the relay is used.
function drawMine() {
  const wrap = $("#screen .fb-mine-wrap"), card = $("#screen .fb-mine");
  if (!wrap || !card) return;
  card.innerHTML = "";
  const m = F.mine;
  wrap.hidden = !m || (!F.at?.send && !m.reports.length);
  if (wrap.hidden) return;
  const note = el("div", "set fb-mine-note");
  const nl = el("div", "l");
  nl.append(el("div", "sub setting-note", T("the developer may reply to a report here; there is no promise of a reply, and replies are public on GitHub")));
  if (m.off) nl.append(el("div", "sub setting-note", T("checking for replies is off (Settings › network)")));
  else if (m.failed) nl.append(el("div", "sub setting-note", T("could not check for replies just now; RimArcho asks again next time")));
  note.append(nl);
  card.append(note);
  if (!m.reports.length) {
    const row = el("div", "set");
    const l = el("div", "l");
    l.append(el("span", "muted", T("no reports sent from this computer yet")));
    row.append(l);
    card.append(row);
  }
  for (const r of m.reports) card.append(reportRow(r));
}

function reportRow(r) {
  const row = el("div", "set fb-report");
  row.dataset.report = String(r.number);
  const l = el("div", "l");
  const head = el("div", "fb-r-head");
  const name = button("#" + r.number + " " + (r.title || ""), "", () => r.url && api.OpenURL(r.url).catch(e => say("fail", errText(e))), "link");
  name.classList.add("fb-r-name");
  if (r.url) name.title = r.url;
  head.append(name);
  if (F.fresh.has(r.number) || r.unseen) head.append(el("span", "fb-r-new", T("new reply")));
  l.append(head);
  const facts = el("div", "sub setting-note word-facts");
  facts.append(el("span", "", Tf("sent %s", day(r.sentAt))));
  if (r.gone) facts.append(el("span", "", T("no longer on GitHub")));
  else if (r.state === "closed") facts.append(el("span", "", r.closedAt ? Tf("closed %s", day(r.closedAt)) : T("closed")));
  else if (r.state === "open") facts.append(el("span", "", T("still open")));
  facts.append(el("span", "", r.replies.length ? Tf("replies: %d", r.replies.length) : T("no reply yet")));
  l.append(facts);
  for (const p of r.replies) {
    const q = el("div", "fb-reply");
    q.append(el("div", "fb-reply-when", Tf("developer, %s", day(p.created_at))), el("div", "fb-reply-text", p.body));
    l.append(q);
  }
  row.append(l);
  const right = el("div", "r");
  right.append(button(T("send a follow-up"), "", () => followUp(r)));
  row.append(right);
  return row;
}

// followUp starts a new report about #N through the normal send path; #N in the title links
// the two on GitHub.
async function followUp(r) {
  try { F.title = await api.FeedbackFollowUp(r.number, r.title || ""); } catch { F.title = "#" + r.number; }
  F.sent = null;
  draw();
  try { F.res = await api.FeedbackCompose(F.title, F.text); refresh(); } catch (e) { say("fail", errText(e)); }
  const t = $("#screen .fb-text");
  t?.focus({ preventScroll: false });
  t?.scrollIntoView({ block: "center" });
}

const day = (iso) => String(iso || "").slice(0, 10);

function partRow(p) {
  const row = el("div", "set");
  row.dataset.part = p.key || "basic";
  const left = el("div", "l");
  const box = el("label", "fb-check");
  const c = el("input");
  c.type = "checkbox";
  c.checked = p.fixed || F.opts[p.key];
  c.disabled = !!(p.fixed || p.none);
  box.append(c, el("span", "name", p.label));
  const note = el("div", "sub setting-note");
  // the basic facts side by side, spaced like other facts (16 §7.3 rule 5)
  if (p.fixed) { note.classList.add("word-facts"); for (const f of F.at?.basic || []) note.append(el("span", "", f)); }
  else note.textContent = p.note || "";
  left.append(box, note);
  row.append(left);
  if (p.fixed) { const r = el("div", "r"); r.append(el("span", "muted", T("always attached"))); row.append(r); }
  if (!p.fixed) c.onchange = async () => {
    F.opts[p.key] = c.checked;
    try { if (await attach() && S.screen === "feedback") refresh(); } catch (e) { say("fail", errText(e)); }
  };
  return row;
}

// preview is the report as the player reads it: the title, what happened, the basic lines,
// then each attachment folded under its heading. No Markdown or HTML here; that is only in
// the body sent and in copy all (F.res.body). The TUI shows view.FeedbackPreview.
function preview(box) {
  box.innerHTML = "";
  if (F.title.trim()) box.append(el("div", "fb-p-title", F.title.trim()));
  if (F.text.trim()) box.append(el("div", "fb-p-text", F.text.trim()));
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

// refresh shows the newest preview, notes and link without touching what is being typed.
function refresh() {
  const sc = $("#screen");
  const pre = sc.querySelector(".fb-prev"), foot = sc.querySelector(".fb-foot");
  if (!pre || !foot || !F.res) return;
  for (const p of F.at?.parts || []) {
    const n = sc.querySelector(`[data-part="${p.key}"] .setting-note`);
    if (n) n.textContent = p.key === "log" && F.opts.log && F.at.pending ? T("the game log is still being analysed…") : p.note;
  }
  preview(pre);
  foot.innerHTML = "";
  const copyBtn = button(T("copy the text"), "", () => copy(F.res.body, copyBtn));
  const gh = button(F.res.full ? T("open GitHub") : T("copy and open GitHub"), "", openIssue, F.at?.send ? "" : "primary");
  if (!F.at?.send) {
    const m = el("span", F.res.full ? "m" : "m warn", F.res.full ? T("opens GitHub in the browser; nothing is sent until you submit it there")
      : T("too long for a link: the link carries the title and the basics, and the full text is copied for you to paste"));
    foot.append(m, copyBtn, gh);
    return;
  }
  // send first; GitHub stays as the other way for players with an account
  const m = el("span", "m");
  if (F.sent) {
    const url = F.sent.url;
    m.append(button(Tf("sent as #%d", F.sent.number) + " ↗", "", () => api.OpenURL(url).catch(e => say("fail", errText(e))), "link"));
  }
  const send = button(F.sending ? T("sending…") : T("send"), "", sendReport, "primary");
  send.disabled = F.sending || (!F.title.trim() && !F.text.trim());
  foot.append(m, copyBtn, gh, send);
}

async function sendReport() {
  if (F.sending) return;
  F.sending = true;
  refresh();
  try {
    F.sent = await api.FeedbackSend(F.title, F.text, F.contact);
    F.title = F.text = "";
    try { F.mine = await api.FeedbackReports(); } catch { /* the list stays as it was */ }
    say("ok", Tf("sent as #%d", F.sent.number));
    F.res = await api.FeedbackCompose("", "");
    if (S.screen === "feedback") { F.sending = false; draw(); }
  } catch (e) { say("fail", errText(e)); }
  finally { F.sending = false; if (S.screen === "feedback") refresh(); }
}

async function openIssue() {
  const res = F.res;
  if (!res) return;
  try {
    if (!res.full) await copy(res.body, $("#screen .fb-foot .btn.primary"));
    await api.OpenURL(res.url);
    say("ok", res.full ? T("opened GitHub in the browser") : T("opened GitHub; paste the copied text into the issue"));
  } catch (e) { say("fail", errText(e)); }
}
