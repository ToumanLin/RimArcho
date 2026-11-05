import { closeChoiceMenu } from "./choices.js";
// Session page navigation (16 §3.1 rule 11). Page reads report completion here;
// refreshes and stale replies never add another visit. No browser History API is used.
import { S, $, api } from "./state.js";
import { openEditor } from "./lists.js";
import * as pop from "./popups.js";
import * as scr from "./screens.js";
import * as shell from "./shell.js";
import * as homePage from "./home.js";
import * as fullPage from "./fulldetail.js";
import * as guide from "./guide.js";
import * as plans from "./plans.js";
import * as library from "./library.js";
import { exchange } from "./exchange.js";
import * as uninst from "./uninstall.js";
import * as feedbackPage from "./feedback.js";
import * as packsPage from "./packs.js";
import * as planManage from "./planmanage.js";
import * as tex from "./textures.js";
import * as tasksPage from "./tasks.js";
import * as fileSearch from "./filesearch.js";
import * as modSettings from "./modsettings.js";
import * as savesPage from "./saves.js";

let ui;
// The displayed shell can still name the previous plan while OpenPlan is in flight:
// clicks open in order, and a click on the displayed plan is a no-op only when no other
// plan is on its way (then returning to it must open it again).
let openingPlan = Promise.resolve(), requestedPlan = null;
export function initNavigation(u) { ui = u; }

// go opens a sidebar page; "plan" is the open plan's editor, "plan:<id>" a plan (opened
// only: the current list, the one launched, stays; 07 §1.3), "plan:new" the new-list form,
// "pack:<name>" a pack on the packs page.
export async function go(id, entry = null) {
  if (uninst.locked()) return; // an uninstall runs, or has run (16 §2.14)
  pageEntering();
  shell.navigated();
  if (entry) {
    S.buildFrom = entry.buildFrom;
    S.planExport = entry.planExport;
    if (id === "plans") planManage.restoreHistory(entry.listManage);
  } else if (id === "plans" && S.screen === "plans") {
    planManage.restoreHistory({ creating: false, selected: planManage.selected() });
  }
  if (id === "plan:new") return plans.newPlan();
  const plan = id.startsWith("plan:") ? +id.slice(5) : null;
  if (plan !== null) id = "plan";
  S.packFocus = id.startsWith("pack:") ? id.slice(5) : null;
  if (id !== "plans") planManage.abandon();
  if (id !== "build") S.buildFrom = null;
  if (id !== "plan-export") S.planExport = null;
  await openPage(id, plan, entry);
  // a page already shown is redrawn in place: its sub-entry and breadcrumb follow too
  ui.renderWhere();
}

export async function openPage(id, plan = null, entry = null) {
  if (S.packFocus) return packsPage.pack(S.packFocus);
  switch (id) {
    case "plan":
      return openEditor(async (read) => {
        if (plan !== null && !S.busy.env && plan !== (requestedPlan ?? S.shell?.plan?.id)) {
          requestedPlan = plan;
          const p = openingPlan = openingPlan.catch(() => {}).then(() => api.OpenPlan(plan));
          try { await p; } finally { if (openingPlan === p) requestedPlan = null; }
          if (!read.current()) return false;
          S.cols.forEach((c, k) => {
            c.query = ""; c.cursor = 0; c.sel.clear(); c.searching = false;
            const input = $(`.area[data-area="${k}"] .search input`);
            if (input) input.value = "";
          });
        }
        return ui.reload(null, read);
      }, plan !== null && plan !== S.shell?.plan?.id, () => { ui.render(); ui.detailFollow(true); });
    case "home": return homePage.home();
    case "needs": return scr.needs();
    case "packs": return packsPage.list();
    case "plans": return planManage.open();
    case "plan-import": return exchange();
    case "plan-export": return planManage.exportOf(entry?.planExport?.id || planManage.exportPlan(), entry?.planExport?.from || planManage.exportFrom());
    case "build": return packsPage.build();
    case "env": return scr.environment();
    case "changes": return scr.changes(S.screen === "changes" ? undefined : null);
    case "subscribe": return scr.openAdd();
    case "library": return library.open();
    case "log": return scr.gameLog(undefined, true);
    case "settings": return scr.settings();
    case "uninstall": return uninst.open();
    case "feedback": return feedbackPage.open();
    case "textures": return tex.open();
    case "tasks": return tasksPage.open();
    case "search": return fileSearch.open();
    case "mod-settings": return modSettings.open();
    case "saves": return savesPage.open();
    case "guide": return entry ? guide.resume() : guide.start(S.board && S.board.envs && S.board.envs.length ? "env" : "first");
  }
}

const visits = [];
let position = -1, revision = 0, replay = null, opening = false, pendingVisit = false;
const LIMIT = 50;

function target() {
  let id = S.screen || (S.shell?.plan ? "plan:" + S.shell.plan.id : "plan");
  // Full details use their own Backspace return, rather than the page history.
  if (id === "full") return null;
  if (id === "packs" && S.packFocus) id = "pack:" + S.packFocus;
  return { id, env: S.board?.env, guide: id === "guide" ? guide.historySession() : null,
    buildFrom: id === "build" ? S.buildFrom : null,
    planExport: id === "plan-export" && S.planExport ? { ...S.planExport } : null,
    listManage: id === "plans" ? planManage.historyState() : null };
}
const same = (a, b) => a && b && a.id === b.id &&
  (a.id !== "plans" || !!a.listManage?.creating === !!b.listManage?.creating) &&
  (a.id !== "plan-export" || a.planExport?.id === b.planExport?.id);

// go() and direct entries to a different screen start visits. A same-screen read
// only replaces its data ticket: it still belongs to the pending visit or replay.
export function pageEntering(name) {
  closeChoiceMenu();
  if (arguments.length && name === S.screen) return;
  if (!opening) { revision++; replay = null; pendingVisit = true; }
}

export function pageEntered() {
  const entry = target();
  if (!entry || scr.pageLoading()) return;
  if (replay && same(entry, replay.entry)) { replay.done = true; return; }
  if (same(entry, visits[position])) { visits[position] = entry; pendingVisit = false; return; }
  if (!pendingVisit) return;
  recordVisit(entry);
}

function recordVisit(entry) {
  pendingVisit = false;
  revision++; replay = null;
  visits.splice(position + 1);
  visits.push(entry);
  if (visits.length > LIMIT) visits.shift();
  position = visits.length - 1;
}

function allowed(entry) {
  if (entry.env !== S.board?.env) return false; // never switch games to replay a visit
  if (entry.id === "guide") return entry.guide === guide.historySession();
  const plan = entry.id.startsWith("plan:") ? +entry.id.slice(5) : entry.planExport?.id;
  if (plan && S.busy.env && plan !== S.shell?.plan?.id) return false;
  return !plan || !!S.shell?.plans?.some(p => p.id === plan);
}

// Back closes only the temporary top layer; it never runs a page's data actions.
export async function travel(direction) {
  if (direction < 0 && closeChoiceMenu(true)) return;
  if (direction < 0) {
    if ($("#menu") && !$("#menu").hidden) { pop.hideMenu(); return; }
    if (S.popup) { pop.close(); return; }
  }
  if (uninst.locked()) return;
  if (direction < 0 && S.screen === "full") return fullPage.back();
  const current = target();
  // Back can arrive as soon as the destination's sidebar item lights up. Keep
  // that explicitly requested route available to Forward while its read is pending.
  if (direction < 0 && pendingVisit && current && !same(current, visits[position]) && scr.pendingPageRead()) recordVisit(current);
  const request = ++revision, previous = position;
  pendingVisit = false;
  // During replay position already names the pending page. A failed ordinary
  // entry has no visit: back returns to the last completed page.
  const start = direction < 0 && !same(target(), visits[position]) ? position : position + direction;
  let attempted = false;
  for (let i = start; i >= 0 && i < visits.length; i += direction) {
    const entry = visits[i];
    if (!allowed(entry)) continue;
    attempted = true;
    position = i;
    const attempt = replay = { entry, done: false };
    opening = true;
    let pending;
    try { pending = go(entry.id, entry); } finally { opening = false; }
    try { await pending; } catch { /* removed/unavailable page: try the next visit */ }
    // A refresh may have superseded go's read. Its completion owns this replay.
    while (revision === request && !attempt.done && scr.pendingPageRead()) await scr.pendingPageRead();
    if (revision !== request) return;
    if (attempt.done) { replay = null; return; }
  }
  position = previous; replay = null;
  // A failed read may have shown an error page. Restore the last completed visit.
  const entry = visits[position];
  if (attempted && entry && (!same(target(), entry) || scr.pageLoading()) && allowed(entry)) {
    const attempt = replay = { entry, done: false };
    opening = true;
    let pending;
    try { pending = go(entry.id, entry); } finally { opening = false; }
    try { await pending; } catch { /* keep the page's own error state */ }
    while (revision === request && !attempt.done && scr.pendingPageRead()) await scr.pendingPageRead();
    if (replay === attempt) replay = null;
  }
}

export function wireHistory() {
  let released = null;
  for (const type of ["mousedown", "mouseup", "auxclick"]) {
    window.addEventListener(type, (e) => {
      if (e.button !== 3 && e.button !== 4) return;
      // WebView2 must never turn these into navigation away from wails.localhost.
      e.preventDefault(); e.stopPropagation();
      if (type === "mousedown") { released = null; return; }
      if (type === "auxclick" && released === e.button) { released = null; return; }
      released = type === "mouseup" ? e.button : null;
      void travel(e.button === 3 ? -1 : 1);
    }, { capture: true, passive: false });
  }
  window.addEventListener("keydown", (e) => {
    const back = e.key === "BrowserBack" || e.altKey && e.key === "ArrowLeft";
    const forward = e.key === "BrowserForward" || e.altKey && e.key === "ArrowRight";
    if ((!back && !forward) || e.ctrlKey || e.metaKey || e.shiftKey) return;
    e.preventDefault(); e.stopPropagation();
    if (!e.isComposing) void travel(back ? -1 : 1);
  }, { capture: true });
}
