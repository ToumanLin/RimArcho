// Shared search/CLI panel (16 §3.5); closing preserves draft and the last output.
import { fieldError } from "./feedbacksemantics.js";
import { S, el, api, T, errText } from "./state.js";
import { copy } from "./fields.js";
let draft = "", result = null, paint = null, polling = false, afterRun = null;
async function poll() {
  if (polling) return;
  polling = true;
  try {
    const r = await api.CommandStatus();
    const accepted = r.sequence && r.sequence >= (result?.sequence || 0);
    const ended = accepted && result?.running && !r.running;
    if (accepted) result = r;
    paint?.();
    if (result?.running) setTimeout(() => { polling = false; poll(); }, 250);
    else { polling = false; if (ended) await afterRun?.(); }
  } catch { polling = false; }
}
export function buildCommandPanel(pop, p, { close, pick, reload, uninstall }) {
  const previous = document.activeElement, live = () => S.popup === p;
  pop.classList.add("command-panel");
  const head = el("div", "command-head"), output = el("pre", "command-output"), list = el("div", "opts"), row = el("div", "in"), input = el("input"), foot = el("div", "command-foot");
  const copyButton = el("button", "btn", T("copy")), dismiss = el("button", "btn", "×");
  const title = el("span", "", T("command palette")); title.id = "command-panel-title";
  head.append(title, copyButton, dismiss);
  list.id = "command-results"; list.setAttribute("role", "listbox"); list.setAttribute("aria-labelledby", title.id);
  input.setAttribute("role", "combobox"); input.setAttribute("aria-controls", list.id);
  input.setAttribute("aria-autocomplete", "list"); input.setAttribute("aria-expanded", "false");
  dismiss.setAttribute("aria-label", T("close")); dismiss.onclick = close;
  output.tabIndex = 0; input.spellcheck = false; input.value = draft;
  input.placeholder = T("a command, a mod, a game"); input.setAttribute("aria-label", input.placeholder);
  foot.setAttribute("role", "status"); row.append(el("span", "pr", "❯"), input);
  pop.append(head, output, list, row, foot);
  let options = [], selected = 0, sequence = 0, historyAt = -1, historyDraft = "", completion = null, error = "";
  fieldError(input, foot, () => !!error || !!(result && !result.running && result.code));
  const paintResult = () => {
    if (!live()) return;
    output.hidden = !result?.output; output.textContent = result?.output || ""; copyButton.hidden = !result?.output;
    foot.classList.toggle("err", !!error || !!(result && !result.running && result.code));
    const text = error || (result?.running ? T("running…") : result ? `exit ${result.code}` : "Tab " + T("complete") + "  ↑↓ " + T("history") + "  Ctrl+N/P  Enter  Esc");
    foot.setAttribute("role", foot.classList.contains("err") ? "alert" : "status");
    foot.setAttribute("aria-busy", String(!!result?.running));
    if (text !== foot.textContent) foot.textContent = text;
  };
  copyButton.onclick = () => copy(result?.output || "", copyButton);
  const run = async () => {
    error = "";
    try {
      const r = await api.RunCLI(input.value);
      // uninstall --yes: the progress page takes over (16 §2.14).
      if (r.uninstall) { draft = ""; close(); await uninstall?.(); return; }
      result = r; draft = input.value; historyAt = -1; paintResult(); afterRun = reload; poll();
    }
    catch (e) { error = errText(e); paintResult(); }
  };
  const applyCompletion = c => {
    const chars = Array.from(completion.original);
    let insert = c.insert;
    if (!c.open && !/^\s/.test(chars.slice(completion.to).join(""))) insert += " ";
    input.value = chars.slice(0, completion.from).join("") + insert + chars.slice(completion.to).join("");
    const cursor = chars.slice(0, completion.from).join("").length + insert.length;
    input.setSelectionRange(cursor, cursor); draft = input.value;
  };
  const choose = o => {
    if (o.cli) return run();
    if (o.completion) { applyCompletion(o.completion); completion = null; options = []; draw(); refresh(); input.focus(); return; }
    if (o.id === "cli") { input.value = "archo "; changed(); input.focus(); return; }
    close(); pick(o);
  };
  const draw = () => {
    list.replaceChildren();
    options.forEach((o, i) => {
      const item = el("button", "opt" + (i === selected ? " on" : "")); item.type = "button";
      item.id = "command-result-" + i;
      item.setAttribute("role", "option"); item.setAttribute("aria-selected", String(i === selected));
      item.setAttribute("aria-posinset", String(i + 1)); item.setAttribute("aria-setsize", String(options.length));
      item.append(el("span", "", o.name));
      if (o.desc) {
        const desc = el("span", "d", o.desc);
        if (o.desc === o.name) desc.setAttribute("aria-hidden", "true");
        item.append(desc);
      }
      item.onclick = () => choose(o); item.onmousemove = () => { if (selected !== i) { selected = i; draw(); } }; list.append(item);
    });
    const active = list.children[selected];
    input.setAttribute("aria-expanded", String(options.length > 0));
    if (active) input.setAttribute("aria-activedescendant", active.id); else input.removeAttribute("aria-activedescendant");
    active?.scrollIntoView({ block: "nearest" });
  };
  const refresh = async () => {
    const n = ++sequence, text = input.value;
    try {
      const found = await api.Palette(text);
      if (!live() || n !== sequence || text !== input.value) return;
      options = (found || []).map(o => ({ ...o, desc: o.kind === 0 ? o.alias : o.desc }));
      if (text.trim()) {
        const c = { cli: true, name: "archo " + text.replace(/^archo\s+/, ""), desc: T("run") };
        if (/\s/.test(text.trim()) || !options.length) options.unshift(c); else options.push(c);
      }
      selected = 0; draw();
    } catch (e) { if (live() && n === sequence) { error = errText(e); paintResult(); } }
  };
  const changed = () => { draft = input.value; completion = null; historyAt = -1; error = ""; paintResult(); refresh(); };
  input.oninput = changed;
  const complete = async back => {
    if (completion) { selected = (selected + (back ? -1 : 1) + options.length) % options.length; applyCompletion(options[selected].completion); draw(); return; }
    const n = ++sequence, original = input.value, c = S.cols[S.focus];
    const current = c?.items[c.view[c.cursor]]?.id || "", ids = c ? c.items.filter(x => c.sel.has(x.id)).map(x => x.id) : [];
    try {
      const r = await api.CommandComplete(original, Array.from(original.slice(0, input.selectionStart)).length, current, ids);
      if (!live() || n !== sequence || input.value !== original || !r.candidates.length) return;
      completion = { original, from: r.from, to: r.to };
      options = r.candidates.map(x => ({ name: x.label, desc: x.note, completion: x })); selected = back ? options.length - 1 : 0;
      applyCompletion(options[selected].completion); draw();
      if (options.length === 1) { completion = null; options = []; draw(); refresh(); }
    } catch (e) { error = errText(e); paintResult(); }
  };
  p.keys = e => {
    if (e.isComposing) return false;
    if (e.key === "Tab" && document.activeElement === input && (!e.shiftKey || completion)) { e.preventDefault(); complete(e.shiftKey); return true; }
    if (["ArrowUp", "ArrowDown"].includes(e.key) && document.activeElement === input) {
      e.preventDefault(); const n = ++sequence;
      api.CommandHistory(historyAt, e.key === "ArrowUp" ? -1 : 1, input.value, historyDraft).then(r => {
        if (!live() || n !== sequence) return;
        historyAt = r.at; historyDraft = r.draft; input.value = draft = r.text; completion = null; refresh();
      }).catch(e => { error = errText(e); paintResult(); }); return true;
    }
    if (e.ctrlKey && ["n", "p"].includes(e.key.toLowerCase())) { e.preventDefault(); selected = Math.max(0, Math.min(options.length - 1, selected + (e.key.toLowerCase() === "n" ? 1 : -1))); draw(); return true; }
    if (e.ctrlKey && e.key.toLowerCase() === "u") { e.preventDefault(); input.value = ""; changed(); return true; }
    if (e.key === "Enter" && document.activeElement === input) {
      e.preventDefault(); if (completion) { completion = null; options = []; draw(); refresh(); } else if (e.ctrlKey || !options.length) run(); else choose(options[selected]); return true;
    }
    return true; // native controls keep their default keys; the page beneath does not.
  };
  p.onclose = () => { draft = input.value; sequence++; paint = null; if (previous?.isConnected) previous.focus(); };
  paint = paintResult; paintResult(); refresh(); poll();
  setTimeout(() => { if (live()) { input.focus(); input.setSelectionRange(input.value.length, input.value.length); } }, 0);
}
