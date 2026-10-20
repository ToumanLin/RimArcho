// The Go service (internal/surface/gui.API) through the Wails runtime (08 §3): no
// generated bindings, no build step.
import { Call, Events } from "/wails/runtime.js";

const SVC = "github.com/ToumanLin/RimArcho/internal/surface/gui.API.";

export const api = new Proxy({}, {
  get: (_, method) => (...args) => Call.ByName(SVC + method, ...args),
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
