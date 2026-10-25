import { el, api, T, Tf, say, errText } from "./state.js";
import { subscribeForm, updateFromForm } from "./subscribe.js";
import * as pop from "./popups.js";

let panel = null, panelAt = "sources", host, pendingBox, ui;
let rows = [], cursor = 0, selected = new Set();
export function resetSubscriptions() { panel = null; selected.clear(); cursor = 0; }

const action = (label, run, key = "") => {
  const b = el("button", "btn", label);
  b.type = "button";
  if (key) b.append(el("kbd", "", key));
  b.onclick = async () => {
    b.disabled = true;
    try { await run(); } catch (e) { say("fail", errText(e)); }
    finally { b.disabled = false; }
  };
  return b;
};

function closePanel() {
  panel?.remove(); panel = null;
  host?.querySelector("button")?.focus();
  ui?.renderHints();
}

export function toggleSubscribe() {
  if (panel?.isConnected) return closePanel();
  const form = subscribeForm(closePanel);
  panelAt = "sources";
  panel = form.el;
  host.append(panel);
  form.focus();
  ui.renderHints();
}

export function drawSubscriptions(scroll, data, area, controls) {
  ui = controls;
  rows = data.pending || [];
  selected = new Set([...selected].filter(ref => rows.some(r => r.ref === ref)));
  cursor = Math.min(cursor, Math.max(0, rows.length - 1));
  const section = (key, label) => {
    const box = el("section", "card pad subscription-section");
    box.dataset.section = key;
    box.tabIndex = -1;
    const head = el("h2", "label", T(label));
    box.append(head); scroll.append(box); area(box, head, () => box.focus());
    return box;
  };
  host = section("sources", "sources");
  host.append(action(T("subscribe…"), toggleSubscribe, "+"));
  for (const source of data.sources || []) {
    const row = el("div", "subscription-source");
    row.dataset.source = source.key;
    row.append(el("span", "word-facts", source.name + "  " + Tf("%d mods", source.count)),
      el("span", "muted word-facts", source.checked + "  " + source.result));
    if (source.account) row.append(action(source.account + " → " + T("accounts"), async () => {
      await ui.go("settings");
      const heading = document.querySelector('[data-setting-action^="account "]')?.closest("[data-area]");
      heading?.scrollIntoView({ block: "start" });
      heading?.querySelector("button, input")?.focus({ preventScroll: true });
    }));
    row.append(action(T("check for updates"), async () => say("ok", await api.CheckSubscriptionSource(source.key))));
    host.append(row);
  }
  // A background completion must not discard text the player is entering.
  if (panel && panelAt === "sources") host.append(panel);
  pendingBox = section("pending", "pending updates");
  pendingBox.onkeydown = e => { if (!e.target.closest(".subscribe-form") && subscriptionKey(e)) e.stopPropagation(); };
  const bulk = action(T("update all"), () => updateRows(selected.size ? rows.filter(r => selected.has(r.ref)) : rows), "U");
  bulk.disabled = !rows.length;
  pendingBox.append(bulk, action(T("clear the selection"), () => { selected.clear(); mark(); }, "esc"));
  if (!rows.length) pendingBox.append(el("p", "muted", T("no known updates; check a source above")));
  rows.forEach((r, i) => {
    const row = el("div", "subscription-pending");
    row.dataset.ref = r.ref; row.tabIndex = -1;
    const check = el("input"); check.type = "checkbox";
    check.setAttribute("aria-label", T("select") + " " + r.name);
    check.checked = selected.has(r.ref);
    check.onchange = () => { cursor = i; check.checked ? selected.add(r.ref) : selected.delete(r.ref); mark(); };
    row.addEventListener("focusin", () => { cursor = i; mark(); });
    row.onclick = e => { if (e.target.closest("button, input")) return; cursor = i; row.focus(); mark(); };
    row.append(check, el("span", "word-facts", r.name + "  " + r.source + "  " + r.installed + " → " + r.available + "  " + T("used in") + ": " + (r.usedIn || []).join(", ")),
      action(T("update subscription"), () => updateRows([r]), "u"),
      action(T("view changes"), () => ui.history(r), "C"));
    pendingBox.append(row);
  });
  if (panel && panelAt === "pending") pendingBox.prepend(panel);
  const historyBox = section("history", "change history");
  historyBox.append(controls.rescan());
  mark();
  return historyBox;
}

function mark() {
  pendingBox?.querySelectorAll(".subscription-pending").forEach((node, i) => {
    node.classList.toggle("cur", i === cursor);
    node.classList.toggle("selected", selected.has(rows[i].ref));
    node.querySelector("input").checked = selected.has(rows[i].ref);
  });
  ui?.renderHints();
}

async function updateRows(items) {
  if (!items.length) return;
  // Download-page versions need the player's file choice, using the existing panel.
  const page = items.find(r => r.kind === "page");
  if (page) {
    closePanel();
    panelAt = "pending";
    panel = updateFromForm(pendingBox, page.ref, closePanel, () => { updateRows(items.filter(r => r !== page)).catch(e => say("fail", errText(e))); });
    return;
  }
  const previews = [];
  for (const r of items) previews.push(r.kind === "workshop" ? await api.WorkshopUpdatePreview(r.ref) : await api.LibraryPreview("subscription-update", [r.ref]));
  pop.confirm(T("update subscription"), previews.flatMap((p, i) => [items[i].name, ...(p.lines || [])]), T("update subscription"), previews.every(p => p.apply) ? async () => {
    try {
      for (const r of items) say("ok", r.kind === "workshop" ? await api.WorkshopUpdate(r.ref) : await api.LibraryDo("subscription-update", [r.ref]));
    } catch (e) { say("fail", errText(e)); }
  } : null);
}

export function subscriptionKey(e) {
  const k = e.key, ctrl = e.ctrlKey || e.metaKey;
  if (k === "Escape") {
    if (panel?.isConnected) { e.preventDefault(); closePanel(); return true; }
    if (selected.size) { e.preventDefault(); selected.clear(); mark(); return true; }
  }
  if (k === "+" && !ctrl) { e.preventDefault(); toggleSubscribe(); return true; }
  if (k === "U" && !ctrl) { e.preventDefault(); updateRows(selected.size ? rows.filter(r => selected.has(r.ref)) : rows).catch(e => say("fail", errText(e))); return true; }
  if (!pendingBox?.contains(document.activeElement)) return false;
  if (ctrl && k.toLowerCase() === "a") { e.preventDefault(); rows.forEach(r => selected.add(r.ref)); mark(); return true; }
  if (ctrl && k === " " && rows[cursor]) {
    e.preventDefault(); const ref = rows[cursor].ref; selected.has(ref) ? selected.delete(ref) : selected.add(ref); mark(); return true;
  }
  if (["Home", "End", "PageUp", "PageDown"].includes(k) && rows.length) {
    e.preventDefault(); const previous = cursor;
    cursor = k === "Home" ? 0 : k === "End" ? rows.length - 1 : Math.max(0, Math.min(rows.length - 1, cursor + (k === "PageDown" ? 10 : -10)));
    if (e.shiftKey) for (let i = Math.min(previous, cursor); i <= Math.max(previous, cursor); i++) selected.add(rows[i].ref);
    pendingBox.querySelectorAll(".subscription-pending")[cursor]?.focus(); mark(); return true;
  }
  if (k === "ArrowDown" || k === "ArrowUp") {
    e.preventDefault(); const previous = cursor;
    cursor = Math.max(0, Math.min(rows.length - 1, cursor + (k === "ArrowDown" ? 1 : -1)));
    if (e.shiftKey) for (let i = Math.min(previous, cursor); i <= Math.max(previous, cursor); i++) if (rows[i]) selected.add(rows[i].ref);
    pendingBox.querySelectorAll(".subscription-pending")[cursor]?.focus(); mark(); return true;
  }
  if (k === "u" && rows[cursor]) { e.preventDefault(); updateRows([rows[cursor]]).catch(e => say("fail", errText(e))); return true; }
  if (k === "C" && rows[cursor]) { e.preventDefault(); ui.history(rows[cursor]); return true; }
  return false;
}

export const subscriptionHints = () => [{ key: "1/2/3", label: T("sources, updates, history") }, { key: "+", label: T("subscribe…") }, { key: "U", label: T("update all") }, ...(selected.size ? [{ key: "esc", label: Tf("selected %d", selected.size) }] : [])];
