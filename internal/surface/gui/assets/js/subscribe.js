import { pathContent } from "./fields.js";
import { el, api, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";

const button = (label, run, enabled = () => true) => {
  const b = el("button", "btn", label);
  b.type = "button";
  b.onclick = async () => {
    b.disabled = true;
    try { await run(); } catch (e) { say("fail", errText(e)); }
    finally { b.disabled = !enabled(); }
  };
  return b;
};

const input = (label, type = "text") => {
  const box = el("label", "subscribe-field");
  const field = el("input");
  field.type = type;
  box.append(el("span", "dim", T(label)), field);
  return { box, field };
};

// File paths remain editable in the development harness, as with the import page.
export function importPicker(onChange = () => {}) {
  const row = el("div", "subscribe-row");
  const path = input("downloaded zip or folder");
  const choose = async (folder) => {
    const value = folder ? await api.PickFolder(path.field.value) : await api.PickSubscriptionZip();
    if (value) { path.field.value = value; onChange(value); }
  };
  path.field.oninput = () => onChange(path.field.value);
  row.append(path.box, button(T("choose zip…"), () => choose(false)), button(T("choose folder…"), () => choose(true)));
  return { el: row, value: () => path.field.value.trim(), focus: () => path.field.focus() };
}

// Forms expand above the table; only the irreversible replacement needs confirmation.
export function subscribeForm(close) {
  const root = el("section", "subscribe-form");
  const head = el("div", "subscribe-row");
  const type = el("select");
  for (const [value, label] of [["git", "Git repository"], ["page", "download page"], ["workshop", "Workshop item"]]) {
    const o = el("option", "", T(label)); o.value = value; type.append(o);
  }
  head.append(el("span", "dim", T("subscription type")), type, button(T("close"), close));
  const url = input("URL or Workshop ID");
  const track = el("div", "subscribe-row");
  const refType = el("select");
  for (const value of ["branch", "tag", "commit"]) { const o = el("option", "", T(value)); o.value = value; refType.append(o); }
  const ref = input("tracking name (blank: default branch)");
  track.append(el("span", "dim", T("tracking")), refType, ref.box);
  const steam = input("use SteamCMD (only without the Steam client)", "checkbox");
  const page = el("div", "subscribe-page");
  const info = el("div", "word-facts");
  const choices = el("div", "subscribe-row");
  const files = importPicker(() => { useGit = false; changed(); });
  let useGit = false, resolved = "";
  const read = button(T("read page"), async () => {
    const pageURL = url.field.value.trim();
    const p = await api.SubscriptionPage(pageURL);
    if (type.value !== "page" || url.field.value.trim() !== pageURL) return;
    const date = p.DateModified && !p.DateModified.startsWith("0001-") ? p.DateModified : "";
    info.replaceChildren(el("div", "", p.Name), el("div", "dim", [p.Version, date].filter(Boolean).join(" @ ")));
    for (const repo of p.Repos || []) info.append(el("div", "word-facts", repo));
    resolved = p.Main || ((p.Repos || []).length === 1 ? p.Repos[0] : "");
    useGit = !!resolved;
    choices.replaceChildren();
    if (resolved) choices.append(button(Tf("subscribe with Git: %s", resolved), () => { useGit = true; changed(); }));
    choices.append(button(T("open download page"), () => api.OpenURL(url.field.value.trim())));
    changed();
  });
  page.append(read, info, choices, files.el);
  const preview = el("pre", "subscribe-preview word-facts");
  const args = () => ({ kind: type.value, url: url.field.value.trim(), refType: refType.value, ref: ref.field.value.trim(),
    git: type.value === "page" && useGit, from: type.value === "page" && !useGit ? files.value() : "", steamcmd: steam.field.checked });
  const actions = el("div", "subscribe-row");
  let ready = "", request = null;
  const canApply = () => !!ready && ready === JSON.stringify(args()) && !(request.kind === "page" && !request.git && !request.from);
  const previewButton = button(T("preview"), async () => {
    const pending = args();
    const p = await api.SubscribePreview(pending);
    if (JSON.stringify(pending) !== JSON.stringify(args())) return;
    request = pending;
    pathContent(preview, (p.lines || []).join("\n"), [pending.from]);
    ready = p.apply ? JSON.stringify(request) : "";
    apply.disabled = !canApply();
  });
  const apply = button(T("subscribe"), async () => {
    if (!canApply()) return;
    say("ok", await api.Subscribe(request)); close();
  }, canApply);
  apply.disabled = true;
  actions.append(previewButton, apply);
  const changed = () => {
    ready = ""; apply.disabled = true; preview.replaceChildren();
    track.hidden = type.value !== "git"; steam.box.hidden = type.value !== "workshop"; page.hidden = type.value !== "page";
  };
  url.field.addEventListener("input", () => { useGit = false; resolved = ""; info.replaceChildren(); choices.replaceChildren(); });
  root.addEventListener("input", changed);
  type.onchange = changed;
  root.onkeydown = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); } };
  root.append(head, url.box, track, steam.box, page, preview, actions);
  changed();
  return { el: root, focus: () => url.field.focus() };
}

export function updateFromForm(host, ref, close) {
  host.querySelector(".subscribe-form")?.remove();
  const root = el("section", "subscribe-form");
  const files = importPicker();
  const actions = el("div", "subscribe-row");
  actions.append(button(T("preview"), async () => {
    const from = files.value();
    if (!from) return say("fail", T("choose downloaded files first"));
    const p = await api.SubscriptionFromPreview(ref, from);
    pop.confirm(p.title, p.lines, T("update subscription"), p.apply ? async () => {
      try { say("ok", await api.SubscriptionFrom(ref, from)); close(); } catch (e) { say("fail", errText(e)); }
    } : null);
  }), button(T("close"), close));
  root.append(el("div", "", T("choose downloaded files first")), files.el, actions);
  root.onkeydown = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); } };
  host.prepend(root);
  files.focus();
  return root;
}
