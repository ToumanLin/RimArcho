import { el, api, errText, say } from "./state.js";
import { scales, accentPalette, isHex } from "./accent.js";

// The last custom accent, kept so the custom swatch still shows it after a named accent is
// chosen (16 §2.14).
const CUSTOM_KEY = "ui.gui.accent.custom";

function paintSwatch(b, value) {
  const palette = accentPalette(value);
  if (!palette) return;
  b.style.setProperty("--swatch-light", palette.light[8]);
  b.style.setProperty("--swatch-dark", palette.dark[10]);
  b.style.setProperty("--swatch-ink-light", palette.fg);
  b.style.setProperty("--swatch-ink-dark", palette.dark[0]);
}

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
  // A custom swatch ends the row when the item offers one (it.custom, its label).
  let custom = null, customColour = isHex(it.value) ? it.value : "";
  function mark() {
    buttons.forEach((b, i) => {
      const on = it.choices[i] === selected;
      b.classList.toggle("on", on);
      b.setAttribute("aria-pressed", String(on));
      if (swatches) b.textContent = on ? "✓" : "";
      if (on) name.textContent = it.choiceLabels?.[i] || selected;
    });
    if (!custom) return;
    const on = isHex(selected);
    custom.classList.toggle("on", on);
    custom.classList.toggle("has-colour", !!customColour);
    custom.setAttribute("aria-pressed", String(on));
    custom.textContent = on ? "✓" : customColour ? "" : "+";
    const label = customColour ? it.custom + " " + customColour : it.custom;
    custom.title = label;
    custom.setAttribute("aria-label", label);
    if (customColour) paintSwatch(custom, customColour);
    if (on) name.textContent = selected;
  }
  it.choices.forEach((value, i) => {
    const label = it.choiceLabels?.[i] || value;
    const b = el("button", swatches ? "accent-swatch" : "", swatches ? "" : label);
    b.type = "button";
    b.title = label;
    b.setAttribute("aria-label", label);
    if (swatches) paintSwatch(b, value);
    b.onkeydown = e => {
      const delta = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
      if (delta) {
        e.preventDefault(); e.stopPropagation();
        buttons[(i + delta + buttons.length) % buttons.length].focus();
      } else if (e.key === "Enter" || e.key === " ") e.stopPropagation();
    };
    b.onclick = () => apply(value);
    buttons.push(b);
    options.append(b);
  });
  function apply(value) {
    return saving = saving.then(async () => {
      if (value === selected) return;
      options.setAttribute("aria-busy", "true");
      try {
        await api.ApplySetting(it.argv, value);
        await refresh();
        selected = value;
        if (isHex(value)) {
          customColour = value;
          Promise.resolve(api.SetSetting?.(CUSTOM_KEY, value)).catch(() => {});
        }
        mark();
      } catch (e) { say("fail", errText(e)); }
      finally { options.setAttribute("aria-busy", "false"); }
    });
  }
  if (swatches && it.custom) {
    custom = el("button", "accent-swatch accent-custom");
    custom.type = "button";
    const at = buttons.length;
    custom.onkeydown = buttons[0] ? e => {
      const delta = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
      if (delta) {
        e.preventDefault(); e.stopPropagation();
        buttons[(at + delta + buttons.length) % buttons.length].focus();
      } else if (e.key === "Enter" || e.key === " ") e.stopPropagation();
    } : null;
    // The named accents are the picker's presets, in the current theme's colours.
    custom.onclick = () => {
      const dark = document.documentElement.dataset.scheme === "dark";
      const presets = it.choices.filter(v => scales[v]).map(v => {
        const p = accentPalette(v);
        return { name: v, label: it.choiceLabels?.[it.choices.indexOf(v)] || v, color: dark ? p.dark[10] : p.light[8], ink: dark ? p.dark[0] : p.fg };
      });
      import("./colorpicker.js").then(({ openColorPicker }) => openColorPicker(custom, {
        value: isHex(selected) ? selected : customColour || selected, presets, label: it.label, onPick: v => apply(v) }));
    };
    buttons.push(custom);
    options.append(custom);
    if (!customColour) Promise.resolve(api.Setting?.(CUSTOM_KEY)).then(v => {
      if (isHex(v) && !customColour) { customColour = v.toLowerCase(); mark(); }
    }).catch(() => {});
  }
  mark();
  wrap.append(options);
  if (swatches) wrap.append(name);
  return wrap;
}
