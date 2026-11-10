// The colour picker (16 §7.5): a floating popover anchored to the control that opened it, with
// preset swatches, a hue/saturation wheel, a lightness slider, a hex field and a preview. A
// swatch picks at once; the wheel, slider and field only change the preview until Enter or
// the confirm button. Esc, a click outside, scrolling the page or resizing the window close
// it without a change. Focus returns to the anchor. One picker is open at a time.
import { el, T, Tf, keyCaps } from "./state.js";

let opened = null;

// closeColorPicker closes the open picker without a change; true when one was open.
export function closeColorPicker(restoreFocus = false) {
  if (!opened) return false;
  opened.close(restoreFocus);
  return true;
}

const hexRe = /^#[0-9a-f]{6}$/i;
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

function hexToHsl(hex) {
  const [r, g, b] = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255);
  const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min;
  let h = 0, s = 0;
  if (d) {
    s = d / (1 - Math.abs(2 * l - 1));
    h = max === r ? ((g - b) / d + 6) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
  }
  return { h: h * 60, s: s * 100, l: l * 100 };
}

function hslToRgb(h, s, l) {
  s /= 100; l /= 100;
  const k = n => (n + h / 30) % 12, a = s * Math.min(l, 1 - l);
  return [0, 8, 4].map(n => Math.round(255 * (l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1)))));
}
const hslToHex = (h, s, l) => "#" + hslToRgb(h, s, l).map(v => v.toString(16).padStart(2, "0")).join("");

// openColorPicker(anchor, { value, presets, onPick, onClose, label }):
// value is a preset name, #rrggbb or "" for none; presets are { name, color, label? };
// onPick(value) gets a preset name or #rrggbb; onClose() runs however the picker closes.
// Opening it again from the same anchor closes it (the anchor toggles it); returns null then.
export function openColorPicker(anchor, { value = "", presets = [], onPick, onClose, label = T("colour") } = {}) {
  if (opened?.anchor === anchor) { closeColorPicker(true); return null; }
  closeColorPicker();
  const preset = presets.find(p => p.name === value);
  const before = preset?.color || (hexRe.test(value) ? value.toLowerCase() : "");
  const start = before || presets[0]?.color || "#3a7bd5";
  let { h, s, l } = hexToHsl(start), hex = start.toLowerCase();

  const pop = el("div", "color-pop");
  pop.setAttribute("role", "dialog");
  pop.setAttribute("aria-label", label);
  const controller = new AbortController();
  const listen = (target, name, fn, capture = false) => target.addEventListener(name, fn, { capture, signal: controller.signal });

  // presets
  if (presets.length) {
    const row = el("div", "cp-presets");
    row.setAttribute("role", "group");
    row.setAttribute("aria-label", T("preset colours"));
    for (const p of presets) {
      const name = p.label || p.name;
      const b = el("button", "cp-swatch" + (p.name === value ? " on" : ""), p.name === value ? "✓" : "");
      b.type = "button";
      b.title = name;
      b.setAttribute("aria-label", name);
      b.setAttribute("aria-pressed", String(p.name === value));
      b.style.setProperty("--swatch", p.color);
      b.style.setProperty("--swatch-ink", p.ink || "#ffffff");
      b.onclick = () => finish(p.name);
      row.append(b);
    }
    pop.append(row);
  }

  // wheel: hue is the angle (0° red at the top, clockwise), saturation the distance from the centre
  const wheel = el("div", "cp-wheel");
  wheel.tabIndex = 0;
  wheel.setAttribute("role", "slider");
  wheel.setAttribute("aria-roledescription", T("colour wheel"));
  wheel.setAttribute("aria-label", T("hue and saturation"));
  wheel.setAttribute("aria-valuemin", "0");
  wheel.setAttribute("aria-valuemax", "359");
  const canvas = el("canvas");
  canvas.setAttribute("aria-hidden", "true");
  const thumb = el("span", "cp-thumb");
  thumb.setAttribute("aria-hidden", "true");
  wheel.append(canvas, thumb);

  const light = el("input", "cp-light");
  light.type = "range";
  light.min = "0"; light.max = "100"; light.step = "1";
  light.setAttribute("aria-label", T("lightness"));

  const field = el("input", "cp-hex");
  field.type = "text";
  field.spellcheck = false;
  field.autocomplete = "off";
  field.setAttribute("aria-label", T("hex colour"));

  const preview = el("span", "cp-preview");
  preview.setAttribute("role", "img");
  const was = el("span", "cp-was"), now = el("span", "cp-now");
  if (before) was.style.background = before; else was.classList.add("none");
  preview.append(was, now);

  const go = el("button", "btn small primary cp-go", T("use colour"));
  go.type = "button";
  go.append(keyCaps("enter"));
  go.onclick = () => finish(hex);

  const bottom = el("div", "cp-row");
  bottom.append(preview, field);
  pop.append(wheel, light, bottom, go);

  let size = 0, drawn = "";
  function drawWheel() {
    const dpr = devicePixelRatio || 1, px = Math.round(size * dpr), key = px + ":" + Math.round(l);
    if (!px || key === drawn) return;
    drawn = key;
    canvas.width = canvas.height = px;
    const ctx = canvas.getContext("2d"), img = ctx.createImageData(px, px), R = px / 2;
    for (let y = 0; y < px; y++) {
      for (let x = 0; x < px; x++) {
        const dx = x + 0.5 - R, dy = y + 0.5 - R, dist = Math.hypot(dx, dy), alpha = clamp(R - dist + 0.5, 0, 1);
        if (!alpha) continue;
        const hue = (Math.atan2(dx, -dy) * 180 / Math.PI + 360) % 360;
        const [r, g, b] = hslToRgb(hue, Math.min(1, dist / R) * 100, l);
        const i = (y * px + x) * 4;
        img.data[i] = r; img.data[i + 1] = g; img.data[i + 2] = b; img.data[i + 3] = alpha * 255;
      }
    }
    ctx.putImageData(img, 0, 0);
  }

  function paint(fromField = false) {
    hex = hslToHex(h, s, l);
    drawWheel();
    const r = size / 2, a = h * Math.PI / 180, d = s / 100 * r;
    thumb.style.left = r + Math.sin(a) * d + "px";
    thumb.style.top = r - Math.cos(a) * d + "px";
    thumb.style.background = hex;
    wheel.setAttribute("aria-valuenow", String(Math.round(h) % 360));
    wheel.setAttribute("aria-valuetext", Tf("hue %d°, saturation %d", Math.round(h) % 360, Math.round(s)) + "%");
    light.value = String(Math.round(l));
    light.setAttribute("aria-valuetext", Math.round(l) + "%");
    light.style.setProperty("--cp-hue", hslToHex(h, s, 50));
    now.style.background = hex;
    preview.setAttribute("aria-label", Tf("new colour %s", hex));
    if (!fromField) { field.value = hex; field.removeAttribute("aria-invalid"); }
  }

  function fromPoint(e) {
    const b = wheel.getBoundingClientRect(), r = b.width / 2;
    const dx = e.clientX - b.left - r, dy = e.clientY - b.top - r;
    h = (Math.atan2(dx, -dy) * 180 / Math.PI + 360) % 360;
    s = clamp(Math.hypot(dx, dy) / r * 100, 0, 100);
    paint();
  }
  wheel.onpointerdown = e => {
    if (e.button !== 0) return;
    e.preventDefault();
    wheel.focus({ preventScroll: true, focusVisible: false });
    wheel.setPointerCapture(e.pointerId);
    fromPoint(e);
  };
  wheel.onpointermove = e => { if (wheel.hasPointerCapture(e.pointerId)) fromPoint(e); };
  // Arrows nudge the wheel's own value, as a native slider does (16 §3.1 rule 1).
  wheel.onkeydown = e => {
    const step = e.shiftKey ? 10 : 2;
    const move = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] }[e.key];
    if (!move) return;
    e.preventDefault();
    h = (h + move[0] + 360) % 360;
    s = clamp(s + move[1], 0, 100);
    paint();
  };
  light.oninput = () => { l = +light.value; paint(); };
  field.oninput = () => {
    const v = field.value.trim().replace(/^([0-9a-f]{6})$/i, "#$1");
    const ok = hexRe.test(v);
    if (ok) ({ h, s, l } = hexToHsl(v.toLowerCase()));
    field.setAttribute("aria-invalid", String(!ok));
    if (ok) paint(true);
  };
  field.onblur = () => paint();

  // Clicking the popover's own background keeps the focus where it is.
  pop.onmousedown = e => { if (!e.target.closest("button, input, .cp-wheel")) e.preventDefault(); };
  pop.addEventListener("keydown", e => {
    if (e.isComposing) return;
    e.stopPropagation();
    if (e.key === "Escape") { e.preventDefault(); close(true); return; }
    if (e.key === "Tab") {
      const controls = [...pop.querySelectorAll("button, input, [tabindex='0']")];
      const i = controls.indexOf(document.activeElement);
      e.preventDefault();
      controls[(i + (e.shiftKey ? controls.length - 1 : 1)) % controls.length]?.focus();
      return;
    }
    // Enter on a swatch or the confirm button activates it; elsewhere it confirms the wheel's colour.
    if (e.key === "Enter" && !e.target.closest("button")) {
      e.preventDefault();
      if (e.target === field && field.getAttribute("aria-invalid") === "true") return;
      finish(hex);
    }
  });

  let closed = false;
  function close(restoreFocus) {
    if (closed) return;
    closed = true;
    controller.abort(); observer.disconnect();
    const inside = pop.contains(document.activeElement);
    pop.remove();
    if (opened?.close === close) opened = null;
    anchor.setAttribute?.("aria-expanded", "false");
    if ((restoreFocus || inside) && anchor.isConnected) anchor.focus({ preventScroll: true });
    onClose?.();
  }
  function finish(v) {
    close(true);
    onPick?.(v);
  }

  document.body.append(pop);
  size = wheel.getBoundingClientRect().width;
  paint();
  // below the anchor, else above; inside the window
  const a = anchor.getBoundingClientRect(), m = pop.getBoundingClientRect(), gap = 6, edge = 8;
  const up = a.bottom + gap + m.height > innerHeight - edge && a.top - gap - m.height >= edge;
  pop.style.left = clamp(a.left, edge, innerWidth - m.width - edge) + "px";
  pop.style.top = clamp(up ? a.top - gap - m.height : a.bottom + gap, edge, Math.max(edge, innerHeight - m.height - edge)) + "px";
  anchor.setAttribute?.("aria-haspopup", "dialog");
  anchor.setAttribute?.("aria-expanded", "true");

  listen(document, "pointerdown", e => { if (!pop.contains(e.target) && !anchor.contains(e.target)) close(false); }, true);
  listen(document, "scroll", e => { if (!pop.contains(e.target)) close(false); }, true);
  listen(window, "resize", () => close(false));
  const observer = new MutationObserver(() => { if (!anchor.isConnected || anchor.closest("[inert]")) close(false); });
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["inert", "hidden"] });
  opened = { close, anchor };
  wheel.focus({ preventScroll: true });
  return { close: () => close(false) };
}
