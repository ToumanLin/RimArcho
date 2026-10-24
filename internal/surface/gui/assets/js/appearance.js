import { el, api, errText, say } from "./state.js";
import { scales } from "./accent.js";

// Arrows move focus only; native buttons apply on Enter/space without rebuilding the page.
export function appearanceControl(it, refresh) {
  const swatches = it.edit === "swatches";
  const wrap = el("div", "appearance-control");
  const options = el("div", swatches ? "accent-swatches" : "bg-seg theme-segments");
  options.setAttribute("role", "group");
  options.setAttribute("aria-label", it.label);
  const name = el("span", "accent-name");
  name.setAttribute("aria-live", "polite");
  const buttons = [];
  let selected = it.value;
  let saving = Promise.resolve();
  function mark() {
    buttons.forEach((b, i) => {
      const on = it.choices[i] === selected;
      b.classList.toggle("on", on);
      b.setAttribute("aria-pressed", String(on));
      if (swatches) b.textContent = on ? "✓" : "";
      if (on) name.textContent = it.choiceLabels?.[i] || selected;
    });
  }
  it.choices.forEach((value, i) => {
    const label = it.choiceLabels?.[i] || value;
    const b = el("button", swatches ? "accent-swatch" : "", swatches ? "" : label);
    b.type = "button";
    b.title = label;
    b.setAttribute("aria-label", label);
    if (swatches && scales[value]) {
      const palette = scales[value];
      b.style.setProperty("--swatch-light", palette.light[8]);
      b.style.setProperty("--swatch-dark", palette.dark[10]);
      b.style.setProperty("--swatch-ink-light", value === "amber" ? palette.light[11] : "#ffffff");
      b.style.setProperty("--swatch-ink-dark", palette.dark[0]);
    }
    b.onkeydown = e => {
      const delta = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
      if (delta) {
        e.preventDefault(); e.stopPropagation();
        buttons[(i + delta + buttons.length) % buttons.length].focus();
      } else if (e.key === "Enter" || e.key === " ") e.stopPropagation();
    };
    b.onclick = () => saving = saving.then(async () => {
      if (value === selected) return;
      options.setAttribute("aria-busy", "true");
      try {
        await api.ApplySetting(it.argv, value);
        await refresh();
        selected = value;
        mark();
      } catch (e) { say("fail", errText(e)); }
      finally { options.setAttribute("aria-busy", "false"); }
    });
    buttons.push(b);
    options.append(b);
  });
  mark();
  wrap.append(options);
  if (swatches) wrap.append(name);
  return wrap;
}
