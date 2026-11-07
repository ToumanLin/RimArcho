// A temporary, scrollable version list. Its rows are fetched only while it is open.
import { S, el, api, T, errText, keyCaps } from "./state.js";
import * as pop from "./popups.js";

export function openVersionList(act, run, show) {
  const env = S.envSeq, environment = S.board?.env, screen = S.screenSeq, plan = S.shell?.plan?.id;
  let pane, body, title, all, panel;
  const sameContext = () => env === S.envSeq && environment === S.board?.env && screen === S.screenSeq && plan === S.shell?.plan?.id;
  const active = () => S.popup === pane && sameContext();
  function create() {
    pop.open("version-list", act.title, (box, p) => {
      pane = p;
      panel = box;
      box.classList.add("version-panel");
      box.id = "version-panel"; box.setAttribute("role", "dialog");
      box.setAttribute("aria-labelledby", "version-panel-title");
      if (p.opener) {
        p.opener.setAttribute("aria-controls", box.id); p.opener.setAttribute("aria-haspopup", "dialog");
        p.opener.setAttribute("aria-expanded", "true");
      }
      p.onclose = () => p.opener?.setAttribute("aria-expanded", "false");
      p.restoreFocus = () => {
        const opener = p.opener?.isConnected ? p.opener : document.querySelector('#screen[role="main"], #board:not([inert])');
        opener?.focus({ preventScroll: true, focusVisible: opener === p.opener && p.openerFocusVisible });
      };
      const head = el("div", "version-head");
      title = el("h2", "", act.title); title.id = "version-panel-title";
      all = el("button", "btn small");
      all.hidden = true;
      head.append(title, all);
      body = el("div", "body", T("loading…")); body.setAttribute("role", "list"); body.setAttribute("aria-labelledby", title.id);
      const foot = el("div", "btns"), close = el("button", "", T("close"));
      close.append(keyCaps("esc"));
      close.onclick = pop.close;
      foot.append(close);
      box.append(head, body, foot);
      close.focus();
      p.keys = (e) => {
        const buttons = [...box.querySelectorAll("button")].filter((b) => !b.disabled && !b.hidden);
        if (e.key === "Tab") {
          e.preventDefault();
          const i = buttons.indexOf(document.activeElement), d = e.shiftKey ? -1 : 1;
          const next = buttons[(i + d + buttons.length) % buttons.length];
          next?.focus(); next?.scrollIntoView({ block: "nearest" });
        } else if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          if (buttons.includes(document.activeElement)) document.activeElement.click();
        } else if (["PageDown", "PageUp", "ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
          e.preventDefault();
          if (e.key === "Home") body.scrollTop = 0;
          else if (e.key === "End") body.scrollTop = body.scrollHeight;
          else body.scrollTop += (e.key.endsWith("Down") ? 1 : -1) * (e.key.startsWith("Page") ? body.clientHeight * 0.9 : 40);
        }
        return true;
      };
    });
  }
  const button = (action) => {
    const b = el("button", "btn small", action.label);
    b.dataset.envWrite = action.kind;
    b.onclick = () => apply(action);
    return b;
  };
  async function load() {
    try {
      const data = await api.VersionWarnings(act.gameVersion, !!act.versionMuted);
      if (!active()) return;
      const scroll = body.scrollTop;
      title.textContent = data.title;
      body.replaceChildren();
      all.hidden = !data.groups.length;
      all.disabled = false;
      all.textContent = data.all.label;
      all.dataset.envWrite = data.all.kind;
      all.onclick = () => apply(data.all);
      for (const group of data.groups) {
        const heading = el("div", "version-group");
        heading.append(el("h3", "word-facts", group.title), button(group.action));
        body.append(heading);
        for (const entry of group.rows) {
          const row = el("div", "version-row"), name = el("button", "version-name", entry.name);
          name.onclick = () => { if (active()) { pop.close(); show(entry.id); } };
          name.id = "version-entry-" + entry.id;
          row.setAttribute("role", "listitem"); row.setAttribute("aria-labelledby", name.id);
          row.append(name, el("span", "muted", entry.supported), button(entry.action));
          body.append(row);
        }
      }
      if (!data.groups.length) body.append(el("p", "muted", T("no version warnings")));
      body.scrollTop = scroll;
    } catch (e) {
      if (active()) { body.textContent = errText(e); all.hidden = true; }
    }
  }
  async function execute(action) {
    const before = [...panel.querySelectorAll("button")], focus = before.indexOf(document.activeElement);
    for (const b of [all, ...body.querySelectorAll("button")]) b.disabled = true;
    await run({}, { ...action, confirm: "" });
    if (active()) {
      await load();
      if (!active()) return;
      const buttons = [...panel.querySelectorAll("button")].filter((b) => !b.disabled && !b.hidden);
      buttons[Math.max(0, Math.min(focus, buttons.length - 1))]?.focus({ preventScroll: true });
    }
  }
  function apply(action) {
    if (!active()) return;
    if (action.confirm) {
      const scroll = body.scrollTop;
      pop.confirm(action.label, [action.warn], action.label, () => {
        if (sameContext()) {
          create();
          return execute(action).then(() => { if (active()) body.scrollTop = scroll; });
        }
      }, action.kind);
    } else return execute(action);
  }
  create();
  return load();
}
