import * as runtime from "/wails/runtime.js";
import { $, el, T } from "./state.js";

const glyph = body => `<svg class="i" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
const maximize = glyph('<rect x="3" y="3" width="10" height="10"/>');
const restore = glyph('<path d="M5.5 3H13v7.5M3 5.5h7.5V13H3z"/>');
let controls;
let layout = "side", environmentRequest;

export function initWindowChrome() {
  // The development browser's runtime shim has no Window. Other platforms keep
  // their native frame (gui.go), so none of them gets desktop caption buttons.
  if (controls || !runtime.Window) return;
  const os = globalThis._wails?.environment?.OS;
  // The last run's answer (localStorage) puts the buttons in at once: added after the
  // first drawing, they took their room from the breadcrumb (16 §8.2).
  let known = "";
  try { known = localStorage.getItem("window.os") || ""; } catch { /* storage can be off */ }
  if (!os && known === "windows") { createControls(); return; }
  // Wails may publish environment after Init has already returned. Ask its
  // runtime once, without delaying the page, then use the latest nav layout.
  if (!os && runtime.System?.Environment) {
    environmentRequest ||= runtime.System.Environment().then(info => {
      if (info.OS === "windows") createControls();
    }).catch(error => console.error("Window environment:", error));
    return;
  }
  if (os === "windows") createControls();
}

function createControls() {
  if (controls) return;
  try { localStorage.setItem("window.os", "windows"); } catch { /* storage can be off */ }
  const win = runtime.Window;
  controls = el("div", "window-controls");
  controls.setAttribute("role", "group");
  controls.setAttribute("aria-label", T("window controls"));
  controls.addEventListener("keydown", e => {
    if (["Enter", " ", "Tab"].includes(e.key)) e.stopPropagation();
  });
  const act = fn => () => Promise.resolve().then(fn).catch(error => console.error("Window controls:", error));
  const button = (name, image, run) => {
    const b = el("button", "navtoggle window-control");
    b.type = "button";
    b.title = name;
    b.setAttribute("aria-label", name);
    b.innerHTML = image;
    b.onclick = act(run);
    controls.append(b);
    return b;
  };
  button(T("minimize window"), glyph('<path d="M3 8h10"/>'), () => win.Minimise());
  const max = button(T("maximize window"), maximize, () => win.ToggleMaximise());
  max.classList.add("window-maximize");
  // Close emits WindowClosing, the same cancellable path as Alt+F4 / taskbar Close.
  button(T("close window"), glyph('<path d="M3.5 3.5l9 9M12.5 3.5l-9 9"/>'), () => win.Close());
  $("#app").classList.add("window-chrome");
  placeWindowControls(layout);

  let sequence = 0;
  const sync = async () => {
    const request = ++sequence;
    const maximized = await win.IsMaximised();
    if (request !== sequence) return;
    max.innerHTML = maximized ? restore : maximize;
    max.title = maximized ? T("restore window") : T("maximize window");
    max.setAttribute("aria-label", max.title);
  };
  for (const event of ["common:WindowMaximise", "common:WindowUnMaximise", "common:WindowRestore"]) {
    runtime.Events.On(event, act(sync));
  }
  act(sync)();
  // Wails owns dragging and resizing; only a double-click on a drag surface is ours.
  $("#app").addEventListener("dblclick", e => {
    if (getComputedStyle(e.target).getPropertyValue("--wails-draggable").trim() !== "drag") return;
    e.preventDefault();
    act(() => win.ToggleMaximise())();
  });
}

export function placeWindowControls(value) {
  layout = value;
  if (controls) $(layout === "top" ? "#side" : "#top").append(controls);
}
