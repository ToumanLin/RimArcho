// The Go service (internal/surface/gui.API) through the Wails runtime (06 §8.2): no
// generated bindings, no build step.
import { call as perfCall } from "./perf.js";
import { Call, Events } from "/wails/runtime.js";

const SVC = "github.com/ToumanLin/RimArcho/internal/surface/gui.API.";

// Environment jobs acknowledge immediately and deliver their result by event (06 §8.4).
let request = Date.now() * 1000; // remains newer when the WebView reloads (06 §3.1)
const pending = new Map();
let actionGuard = () => "";
export function setEnvironmentGuard(fn) { actionGuard = fn; }
const writesGame = new Set([
  "CompleteDependencies", "ReplaceRecommended", "UseInstalled",
  "Run", "RunMoving", "RunCLI", "Toggle", "Stage", "Unstage", "StageAll", "RestoreAll",
  "MoveBy", "MoveShown", "Place", "Swap", "Undo", "Redo", "HistoryGo", "RunOp", "DoAction",
  "CreatePlan", "ImportApplyTo", "UsePlan", "OpenPlan", "NewPlan", "RenamePlan", "RemovePlan", "ImportApply", "ImportAsPlan", "ImportAsPlanPreview", "ImportFollow",
  "PlanSync", "PlanFollow", "PlanFollowMode", "PlanUnfollow", "PlanVersionsSet", "SetModNote", "SetTag",
  "LibraryDo", "LibrarySetNote", "Subscribe", "SubscriptionFrom", "CheckSubscriptionSource", "WorkshopUpdate", "Scan", "Analyze", "RemoveEnv", "ApplySetting", "AddMonitor",
  "PackLaunch", "PackBuild", "PackRemove", "PackRemoveUnfinished", "TexturesRun", "TexturesClean", "CreditAnswer", "CompleteDependencies", "ReplaceRecommended",
]);
on("environment", (result) => {
  const p = pending.get(result.request);
  if (!p) return;
  pending.delete(result.request);
  if (result.error && !result.stale) p.reject(new Error(result.error));
  else p.resolve(p.method === "GuideAdd" ? result.names || [] : result);
});

export const api = new Proxy({}, {
  get: (_, method) => (...args) => perfCall(method, () => {
    const reason = writesGame.has(method) && actionGuard();
    if (reason) return Promise.reject(new Error(reason));
    if (!["UseEnv", "HomeApply", "GuideAdd", "Workspace"].includes(method)) return Call.ByName(SVC + method, ...args);
    const id = ++request;
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject, method });
      Call.ByName(SVC + method, id, ...args).then((ok) => {
        if (!ok) { pending.delete(id); reject(new Error(T("the window is closing"))); }
      }, (error) => { pending.delete(id); reject(error); });
    });
  }),
});

// on subscribes to a Go event; the handler gets its data.
export function on(name, fn) {
  Events.On(name, (ev) => fn(ev && "data" in ev ? ev.data : ev));
}

// The interface text: English keys, the Chinese table from Go (view.Table).
let table = {};
export function setText(t) { table = t || {}; }
export function T(s) { return table[s] || s; }
export function Tf(f, ...a) {
  let i = 0;
  return T(f).replace(/%[sdv]/g, () => String(a[i++]));
}
