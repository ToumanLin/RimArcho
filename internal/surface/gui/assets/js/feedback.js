// 设置 › 反馈 (06 §2.14): a GitHub issue written here and opened in the browser with the
// title and body filled in (view.FeedbackAttachments, view.ComposeFeedback). RimArcho, the game
// and the system always go with it; the enabled mods, the paths and the game log's errors
// only when ticked. The whole text shows before anything leaves; nothing is sent from here.
// A body too long for a link is copied and the link says to paste it.
import { copy } from "./fields.js";
import { S, $, el, api, T, errText, say } from "./state.js";
import { page, button, area, beginPageRead } from "./screens.js";

// The draft stays for the session: leaving the page does not lose what was typed (06 §3.1).
const F = { title: "", text: "", opts: { mods: false, paths: false, log: false }, at: null, res: null };
let seq = 0, typing = 0, retry = 0;

export async function open() {
  const read = beginPageRead("feedback");
  try { await attach(); } catch (e) { read.fail(e); return; }
  if (!read.current()) return;
  draw();
  read.finish();
  $("#screen .fb-title")?.focus({ preventScroll: true });
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
  const typed = () => { F.title = title.value; F.text = text.value; clearTimeout(typing); typing = setTimeout(() => compose().catch(e => say("fail", errText(e))), 150); };
  title.oninput = typed; text.oninput = typed;
  const l1 = el("label", "fb-field"); l1.append(el("span", "", T("title")), title);
  const l2 = el("label", "fb-field"); l2.append(el("span", "", T("what happened")), text);
  form.append(l1, l2);
  w.append(lab1, form);
  area(form, lab1, () => title.focus());

  const lab2 = el("div", "label section", T("attach"));
  const card = el("section", "card settings");
  card.append(partRow({ label: T("basic information"), fixed: true }));
  for (const p of F.at?.parts || []) card.append(partRow(p));
  w.append(lab2, card);
  area(card, lab2);

  const lab3 = el("div", "label section", T("preview"));
  const prev = el("section", "card settings");
  prev.append(el("pre", "fb-prev"));
  w.append(lab3, prev);
  area(prev, lab3);

  w.append(el("div", "fb-foot"));
  refresh();
  sc.scrollTop = top;
}

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
  // the basic facts side by side, spaced like other facts (06 §7.3 rule 5)
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

// refresh shows the newest body, notes and link without touching what is being typed.
function refresh() {
  const sc = $("#screen");
  const pre = sc.querySelector(".fb-prev"), foot = sc.querySelector(".fb-foot");
  if (!pre || !foot || !F.res) return;
  for (const p of F.at?.parts || []) {
    const n = sc.querySelector(`[data-part="${p.key}"] .setting-note`);
    if (n) n.textContent = p.key === "log" && F.opts.log && F.at.pending ? T("the game log is still being analysed…") : p.note;
  }
  pre.textContent = F.res.body;
  foot.innerHTML = "";
  const m = el("span", F.res.full ? "m" : "m warn", F.res.full ? T("opens GitHub in the browser; nothing is sent until you submit it there")
    : T("too long for a link: the link carries the title and the basics, and the full text is copied for you to paste"));
  const copyBtn = button(T("copy the text"), "", () => copy(F.res.body, copyBtn));
  const go = button(F.res.full ? T("open GitHub") : T("copy and open GitHub"), "", openIssue, "primary");
  foot.append(m, copyBtn, go);
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
