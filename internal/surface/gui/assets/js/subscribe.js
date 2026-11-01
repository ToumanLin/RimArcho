// Mod library › add a subscription (06 §2.9): one paste box. What is pasted is recognised (a Git repository,
// a LoversLab file page, another web page, Workshop links or IDs); the card below asks only
// the one decision that source needs and shows the --dry-run text before subscribe. Also the
// file choice a download-page update needs (pageUpdate, in the subscriptions and updates detail).
import { pathContent, editablePath } from "./fields.js";
import { el, api, T, Tf, errText, say } from "./state.js";
import * as pop from "./popups.js";

// button runs an action once at a time: disabled with "…" while it runs.
const button = (label, run, cls = "", enabled = () => true) => {
  const b = el("button", "btn " + cls, label);
  b.type = "button";
  b.onclick = async () => {
    b.disabled = true; b.classList.add("busy");
    try { await run(); } catch (e) { say("fail", errText(e)); }
    finally { b.disabled = !enabled(); b.classList.remove("busy"); }
  };
  return b;
};

// option is one choice of a card: a round mark, its name and a line under it.
const option = (name, desc, on, pick) => {
  const b = el("button", "opt" + (on ? " on" : ""));
  b.type = "button";
  b.append(el("span", "rd"), el("span", "nm word-facts", name));
  if (desc) b.append(el("span", "ds word-facts", desc));
  b.onclick = pick;
  return b;
};

const GIT_HOSTS = ["github.com", "gitlab.com", "codeberg.org", "gitgud.io", "bitbucket.org", "gitee.com"];
export const loversLabPage = value => /^https:\/\/(www\.)?loverslab\.com\/files\/file\/[^?#]+$/.test(value);

// detect says what a pasted text is: "git", "loverslab", "page", "workshop", "bad", or "" for
// nothing. Repositories are recognised on known Git hosts (owner/name, nothing more) or by
// a .git ending; any other https page is a download page that is read first.
export function detect(text) {
  const t = text.trim();
  if (!t) return "";
  if (/^\d{5,}(\s+\d{5,})*$/.test(t) || /^https:\/\/steamcommunity\.com\/(sharedfiles|workshop)\/filedetails\/\?(.*&)?id=\d+/.test(t)) return "workshop";
  if (/\s/.test(t)) return "bad";
  if (loversLabPage(t)) return "loverslab";
  let u;
  try { u = new URL(t); } catch { return "bad"; }
  if (u.protocol !== "https:" && u.protocol !== "http:") return "bad";
  const parts = u.pathname.split("/").filter(Boolean);
  if (u.pathname.endsWith(".git") || (GIT_HOSTS.includes(u.hostname.replace(/^www\./, "")) && parts.length === 2)) return "git";
  return u.protocol === "https:" ? "page" : "bad";
}

const KIND = { git: "Git repository", loverslab: "LoversLab", page: "web page", workshop: "Workshop item" };

// LoversLab files: the site's statements stay text; every selection and agreement is the
// player's click, and opaque IDs bind the choices (no CSRF or signed URL reaches the page).
// ref: the subscription an update replaces ("" for a new one); done after installing.
export function loversLabFiles(pageURL, ref, done) {
  const root = el("div", "subscribe-ll");
  const current = () => root.isConnected !== false;
  const info = el("div", "muted word-facts"), choices = el("div", "subscribe-files");
  const show = p => {
    info.textContent = p?.reason || p?.agreement || (p?.files?.length ? T("Choose a file. Site agreements are shown before downloading.") : T("no files on this page"));
    choices.replaceChildren();
    for (const file of p?.files || []) choices.append(button([T(file.name), file.size, file.date].filter(Boolean).join("  "), async () => {
      const follow = async () => {
        const result = await api.LoverFollow(pageURL, file.id, !!p.agreement);
        if (!current()) return;
        if (result.ready) {
          const preview = await api.LoverInstallPreview(pageURL, result.ready, ref);
          if (!current()) return;
          pop.confirm(preview.title, preview.lines, ref ? T("update subscription") : T("subscribe"), preview.apply ? async () => {
            try { say("ok", await api.LoverInstall(pageURL, result.ready, ref)); done(); } catch (e) { say("fail", errText(e)); }
          } : null);
        } else show(result.page);
      };
      if (p.agreement) pop.confirm(T("site download agreement"), [p.agreement], T("I agree, continue"), follow);
      else await follow();
    }, "line"));
    if (p?.reason) choices.append(button(T("open the page"), () => api.OpenURL(pageURL)));
  };
  const read = async () => { info.textContent = T("reading the files…"); const p = await api.LoverFiles(pageURL); if (current()) show(p); };
  root.append(info, choices);
  read().catch(e => { info.textContent = errText(e); });
  return root;
}

// importPicker chooses files downloaded in the browser: a zip or a folder.
export function importPicker(onChange = () => {}) {
  const row = el("div", "subscribe-pick");
  const shown = el("span", "muted word-facts", T("nothing chosen yet"));
  let value = "";
  const choose = async (folder) => {
    const v = folder ? await api.PickFolder(value) : await api.PickSubscriptionZip();
    if (v) { value = v; pathContent(shown, v, [v]); editablePath(shown, () => choose(folder).catch(e => say("fail", errText(e))), !folder); onChange(v); }
  };
  row.append(button(T("choose zip…"), () => choose(false), "line"), button(T("choose folder…"), () => choose(true)), shown);
  return { el: row, value: () => value };
}

// pageUpdate is the subscriptions and updates detail's file choice for a download-page subscription: the
// LoversLab files with the session, or files downloaded in the browser (previewed, then
// confirmed: it replaces the installed one).
export function pageUpdate(ref, done) {
  const root = el("div", "subscribe-update");
  const files = importPicker();
  const fromFiles = button(T("update from these files"), async () => {
    const from = files.value();
    if (!from) return say("fail", T("choose downloaded files first"));
    const p = await api.SubscriptionFromPreview(ref, from);
    pop.confirm(p.title, p.lines, T("update subscription"), p.apply ? async () => {
      try { say("ok", await api.SubscriptionFrom(ref, from)); done(); } catch (e) { say("fail", errText(e)); }
    } : null);
  }, "line");
  const manual = el("div", "subscribe-manual");
  manual.append(el("div", "muted", T("or files downloaded in the browser")), files.el, fromFiles);
  root.append(manual);
  api.SubscriptionFilePage(ref).then(pageURL => {
    if (pageURL && root.isConnected !== false) root.insertBefore(loversLabFiles(pageURL, ref, done), manual);
  }).catch(() => {});
  return root;
}

// addForm is the add a subscription page's body. ctx: data (the subscriptions and updates page data, for the account
// lines and the recent subscriptions), back() (esc on an empty box), done() after queuing.
export function addForm(ctx = {}) {
  const root = el("div", "subscribe-add");
  const box = el("label", "subscribe-omni");
  const field = el("input");
  field.type = "text"; field.spellcheck = false;
  field.placeholder = T("paste a GitHub repository, a LoversLab file page, a Workshop link or ID");
  box.append(field);
  const result = el("div", "subscribe-result");
  const below = el("div", "subscribe-below");
  root.append(box, result, below);
  let seq = 0, timer = 0, shown = "";

  const account = (key) => ((ctx.data?.sources || []).find(s => s.key === key) || {}).account || "";
  const tile = (title, what, shape, status) => {
    const t = el("button", "subscribe-tile");
    t.type = "button";
    t.append(el("span", "t", T(title)), el("span", "w word-facts", T(what)), el("code", "", shape));
    if (status) t.append(el("span", "st word-facts", status));
    t.onclick = () => { field.placeholder = shape; field.focus(); };
    return t;
  };
  const tiles = el("div", "subscribe-tiles");
  tiles.append(
    tile("Git repository", "A mod its author publishes on GitHub, or the version in development. Updates download only what changed.", "https://github.com/<author>/<repository>", T("no account needed")),
    tile("LoversLab", "A mod published only on LoversLab. Paste its file page and choose a file; RimArcho tells you when a new version is out.", "https://www.loverslab.com/files/file/…", account("page")),
    tile("Workshop item", "Subscribe with Steam; without the Steam client, download with SteamCMD.", "https://steamcommunity.com/sharedfiles/filedetails/?id=…", account("steamcmd")));
  const local = el("div", "subscribe-local");
  local.append(el("span", "word-facts", T("Local mods need no subscription: copy them into the Mods folder and the next scan finds them.")),
    button(T("open the Mods folder"), async () => { await api.OpenModsFolder(); say("ok", T("opened the folder")); }, "link"));
  below.append(tiles, local);
  const recent = ctx.data?.recent || [];
  if (recent.length) {
    const list = el("div", "subscribe-recent");
    list.append(el("div", "subscribe-sub", T("recently added")));
    for (const r of recent) {
      const row = el("div", "r");
      row.append(el("b", "", r.name), el("span", "muted", r.source), el("span", "faint", r.added));
      list.append(row);
    }
    below.append(list);
  }

  // card: what was recognised, its one decision, the dry run and subscribe.
  const recognise = () => {
    clearTimeout(timer);
    const my = ++seq, text = field.value.trim(), kind = detect(text);
    shown = text;
    result.replaceChildren();
    below.hidden = !!kind;
    if (!kind) return;
    if (kind === "bad") {
      result.append(el("div", "subscribe-card subscribe-bad", T("Not recognised. Paste a GitHub or GitLab repository, a LoversLab file page, a web page with a download, Workshop links or IDs.")));
      return;
    }
    const live = () => my === seq;
    const card = el("section", "subscribe-card");
    const head = el("div", "subscribe-head");
    head.append(el("span", "pill", T(KIND[kind])), el("span", "word-facts", text));
    const choices = el("div", "subscribe-choices");
    const preview = el("pre", "subscribe-preview word-facts");
    const foot = el("div", "subscribe-foot");
    let ready = "", args = () => null, own = false;
    const sub = button(T("subscribe"), async () => {
      const req = args();
      if (!req || ready !== JSON.stringify(req)) return;
      say("ok", await api.Subscribe(req));
      field.value = ""; recognise(); ctx.done?.();
    }, "primary", () => !!ready && ready === JSON.stringify(args()));
    sub.disabled = true;
    sub.append(el("kbd", "", "enter"));
    const cancel = button(T("cancel"), () => { field.value = ""; recognise(); field.focus(); });
    cancel.append(el("kbd", "", "esc"));
    foot.append(cancel, sub);
    // refresh: the dry run of the current choice; a reply for an older choice is dropped.
    const refresh = async () => {
      ready = ""; sub.disabled = true; preview.replaceChildren(); preview.hidden = true;
      sub.hidden = own;
      const req = args();
      if (!req || own) return;
      const key = JSON.stringify(req);
      let p;
      try { p = await api.SubscribePreview(req); } catch (e) { if (live()) { preview.hidden = false; preview.textContent = errText(e); } return; }
      if (!live() || key !== JSON.stringify(args())) return;
      preview.hidden = false;
      pathContent(preview, (p.lines || []).join("\n"), [req.from].filter(Boolean));
      ready = p.apply ? key : "";
      sub.disabled = !ready;
    };
    card.append(head, choices, preview, foot);
    result.append(card);

    if (kind === "git") {
      let type = "default", ref = "";
      const name = el("input");
      name.type = "text"; name.spellcheck = false;
      name.oninput = () => { ref = name.value.trim(); refresh(); };
      const draw = () => {
        choices.replaceChildren(el("div", "subscribe-sub", T("track")));
        for (const [k, label, desc] of [["default", "the default branch", "follows what the repository shows first"], ["branch", "a branch", ""], ["tag", "a tag", ""], ["commit", "one commit", "never updates"]]) {
          choices.append(option(T(label), desc ? T(desc) : "", type === k, () => { type = k; name.value = ""; ref = ""; draw(); refresh(); if (k !== "default") name.focus(); }));
        }
        if (type !== "default") { name.placeholder = T(type === "commit" ? "full commit SHA" : type + " name"); choices.append(name); }
      };
      args = () => (type === "default" ? { kind: "git", url: text, refType: "branch", ref: "" } : ref ? { kind: "git", url: text, refType: type, ref } : null);
      draw(); refresh();
      return;
    }
    if (kind === "workshop") {
      let steamcmd = false;
      const draw = () => {
        choices.replaceChildren();
        choices.append(option(T("subscribe with Steam"), T("Steam downloads it and keeps it updated"), !steamcmd, () => { steamcmd = false; draw(); refresh(); }),
          option(T("download with SteamCMD"), [T("without the Steam client; RimArcho checks for updates"), account("steamcmd")].filter(Boolean).join("  "), steamcmd, () => { steamcmd = true; draw(); refresh(); }));
      };
      args = () => ({ kind: "workshop", url: text, steamcmd });
      draw(); refresh();
      return;
    }
    // a web page (LoversLab or another): read it, then Git, LoversLab files or downloaded files
    let mode = "", main = "", ll = null;
    const files = importPicker(() => refresh());
    const info = el("div", "subscribe-info muted", T("reading the page…"));
    const draw = () => {
      choices.replaceChildren(info);
      const modes = [];
      if (main) modes.push(["git", Tf("subscribe with Git: %s", main), T("updates come from the repository; no file to choose")]);
      if (kind === "loverslab") modes.push(["loverslab", T("download from LoversLab"), account("page")]);
      modes.push(["files", T("use downloaded files"), T("a zip or folder downloaded in the browser; this page is kept for update checks")]);
      if (!mode) mode = modes[0][0];
      for (const [k, label, desc] of modes) choices.append(option(label, desc, mode === k, () => { mode = k; draw(); refresh(); }));
      own = mode === "loverslab";
      if (mode === "files") choices.append(files.el);
      if (mode === "loverslab") choices.append(ll ||= loversLabFiles(text, "", () => { field.value = ""; recognise(); ctx.done?.(); }));
    };
    args = () => mode === "git" ? { kind: "page", url: text, git: true } : mode === "files" && files.value() ? { kind: "page", url: text, from: files.value() } : null;
    api.SubscriptionPage(text).then(p => {
      if (!live()) return;
      const date = p.DateModified && !p.DateModified.startsWith("0001-") ? p.DateModified.slice(0, 10) : "";
      info.textContent = [p.Name, p.Version, date].filter(Boolean).join("  ") || text;
      main = p.Main || ((p.Repos || []).length === 1 ? p.Repos[0] : "");
    }, e => {
      if (!live()) return;
      info.textContent = kind === "loverslab" ? T("The public page could not be read; use the LoversLab session or downloaded files.") : errText(e);
    }).finally(() => { if (live()) { mode = ""; draw(); refresh(); } });
    draw(); refresh();
  };

  field.oninput = () => { clearTimeout(timer); timer = setTimeout(recognise, 250); };
  field.onkeydown = (e) => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    if (field.value.trim() !== shown) return recognise();
    result.querySelector(".subscribe-foot .btn.primary")?.onclick();
  };
  root.onkeydown = (e) => {
    if (e.key !== "Escape") return;
    e.preventDefault(); e.stopPropagation();
    if (field.value) { field.value = ""; recognise(); field.focus(); } else ctx.back?.();
  };
  return {
    el: root,
    focus: () => field.focus(),
    // setText fills the box (a paste on subscriptions and updates) and recognises it at once.
    setText: (t) => { field.value = t || ""; recognise(); },
  };
}

// updateFromForm opens pageUpdate in place at the top of host (Mod library and the details, where a
// download-page copy's update needs the new files); esc or close closes it.
export function updateFromForm(host, ref, close, onApplied = () => {}) {
  host.querySelector(".subscribe-form")?.remove();
  const root = el("section", "subscribe-form");
  const head = el("div", "subscribe-formhead");
  head.append(el("span", "word-facts", T("choose the new files for this subscription")), button(T("close"), close));
  root.append(head, pageUpdate(ref, () => { close(); onApplied(); }));
  root.onkeydown = (e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); } };
  host.prepend(root);
  root.querySelector("button")?.focus();
  return root;
}
