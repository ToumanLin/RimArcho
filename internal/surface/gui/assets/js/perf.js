// Opt-in diagnostics for the real-window runner; disabled for normal windows.
const enabled = typeof location !== "undefined" && new URLSearchParams(location.search).has("perf");
export const trace = enabled ? (window.__f4 = {
  origin: performance.timeOrigin, calls: [], phases: [], longTasks: [], gaps: [],
  entryTypes: PerformanceObserver.supportedEntryTypes,
}) : null;
if (trace) {
  new PerformanceObserver(list => trace.longTasks.push(...list.getEntries().map(e => ({ at: e.startTime, ms: e.duration }))))
    .observe({ type: "longtask", buffered: true });
  let last = performance.now();
  setInterval(() => {
    const now = performance.now();
    if (now - last > 100) trace.gaps.push({ at: last, ms: now - last });
    last = now;
  }, 25);
  trace.phases.push({ name: "js-module", at: performance.now(), ms: 0 });
}
export function phase(name) {
  if (!trace) return () => {};
  const at = performance.now();
  return () => trace.phases.push({ name, at, ms: performance.now() - at });
}
export function call(method, fn) {
  if (!trace) return fn();
  return measuredCall(method, fn);
}
async function measuredCall(method, fn) {
  const at = performance.now();
  try {
    const value = await fn(), end = performance.now(), json = JSON.stringify(value);
    trace.calls.push({ method, at, ms: end - at, bytes: new TextEncoder().encode(json || "").length, sizeMs: performance.now() - end });
    return value;
  } catch (e) {
    trace.calls.push({ method, at, ms: performance.now() - at, error: String(e) });
    throw e;
  }
}
