// G5 (06 §11): from any state of any page, esc brings the page back to its base state in at
// most 3 presses: the same page, nothing selected, no popup, menu or overlay, no open
// filter (filters applied with the bar closed are the page's own state; a further esc
// clears them). Run in the dev harness's page (guidev serves this file at /dev/esccheck.js):
//
//	const r = await (await import("/dev/esccheck.js")).run(); r
//
// Each scenario opens a page, builds a state with the keyboard and the mouse as a player
// would, then presses esc (a keydown on the focused element, as the browser sends it)
// until the page is at its base, failing after 3. It changes no data: only overlays,
// selections, filters and focus.
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];

let S, lib;

function key(k, o = {}) {
  const t = document.activeElement && document.activeElement !== document.body ? document.activeElement : document;
  const code = o.code || (k.length === 1 && /[a-z]/i.test(k) ? "Key" + k.toUpperCase() : k);
  t.dispatchEvent(new KeyboardEvent("keydown", { key: k, code, bubbles: true, cancelable: true, shiftKey: !!o.shift, ctrlKey: !!o.ctrl }));
}

async function go(id) {
  if (id === "settings") $("#nav-settings").click();
  else if (id === "guide" || id === "uninstall") {
    // pages under another: 环境's "add an environment", 设置's "uninstall RimArcho"
    await go(id === "guide" ? "env" : "settings");
    const re = id === "guide" ? /添加环境|add an environment/i : /卸载|uninstall/i;
    const b = [...document.querySelectorAll("#acts button, #screen button")].find((x) => re.test(x.textContent));
    if (!b) throw new Error("no way to " + id);
    b.click();
  }
  else {
    // a sub-entry (全部 / 变化) shows once its parent is open
    let b = $(`#nav [data-id="${id}"]`);
    if (!b && id === "changes") { $(`#nav [data-id="library"]`).click(); await wait(600); b = $(`#nav [data-id="changes"]`); }
    b.click();
  }
  // a page shows once its data is in: wait for it (up to 20 s: the first read of a real Player.log parses it)
  const want = { plan: null, guide: "guide", uninstall: "uninstall" }[id] !== undefined ? { plan: null, guide: "guide", uninstall: "uninstall" }[id] : id;
  for (let i = 0; i < 200 && S.screen !== want; i++) await wait(100);
  if (S.screen !== want) throw new Error(`${id} did not open (still ${S.screen || "plan"}; status: ${document.querySelector("#status").textContent.slice(0, 60)})`);
  await wait(300);
  if (document.activeElement && document.activeElement.blur) document.activeElement.blur();
}

// what is not base on the page shown, in words ("" when it is at its base)
function notBase(screen) {
  const out = [];
  if ((S.screen || "plan") !== screen) out.push("page " + (S.screen || "plan"));
  if (S.popup) out.push("popup " + S.popup.kind);
  if (!$("#menu").hidden) out.push("menu");
  if ($$(".fbar.open").some((b) => b.offsetParent)) out.push("open filter");
  const lw = lib.where();
  if (S.screen === "library" && / › /.test(lw)) out.push("details over the page");
  if (S.screen === "library" && $(".lib-detail.focus")) out.push("focus in the details");
  if (S.screen === "library" && /selected|已选/.test(lw)) out.push("library selection");
  if (!S.screen && S.cols.some((c) => c.sel.size)) out.push("selection");
  if (!S.screen && S.cols.some((c) => c.query)) out.push("staging search");
  const a = document.activeElement;
  if (a && a.tagName === "INPUT" && (a.closest(".fbar") || a.closest("#popup"))) out.push("typing");
  return out.join(", ");
}

const rowMenu = async (sel) => {
  const r = $(sel);
  if (!r) throw new Error("no row " + sel);
  const b = r.getBoundingClientRect();
  r.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.left + 30, clientY: b.top + 8 }));
  await wait(300);
};

const SCENARIOS = [
  ["启动 · help", "home", "home", async () => { key("?"); }],
  ["能不能跑 · help", "needs", "needs", async () => { key("?"); }],
  ["能不能跑 · an item's ⋯ menu (mouse)", "needs", "needs", async () => { const m = $(".need .more"); if (!m) throw new Error("no ⋯"); m.click(); }],
  ["能不能跑 · palette", "needs", "needs", async () => { key("k", { ctrl: true }); }],
  ["游戏日志 · help", "log", "log", async () => { key("?"); }],
  ["游戏日志 · palette", "log", "log", async () => { key("k", { ctrl: true }); }],
  ["Mod 库存 › 全部 · filter open, focus in it", "library", "library", async () => { key("\\"); await wait(300); }],
  ["Mod 库存 › 全部 · filter open, focus on the rows", "library", "library", async () => { key("\\"); await wait(300); $(".lib-rows, .lib .rows").focus(); }],
  ["Mod 库存 › 全部 · search typed", "library", "library", async () => { key("/"); await wait(100); const i = document.activeElement; i.value = "a"; i.dispatchEvent(new Event("input")); await wait(300); }],
  ["Mod 库存 › 全部 · selection + details over the page", "library", "library", async () => { key("ArrowDown", { shift: true }); key("ArrowDown", { shift: true }); key("Enter"); await wait(200); }],
  ["Mod 库存 › 全部 · selection + open filter + details", "library", "library", async () => { key("ArrowDown", { shift: true }); key("\\"); await wait(300); $(".lib-rows, .lib .rows").focus(); key("Enter"); await wait(200); }],
  ["Mod 库存 › 全部 · search applied + selection + details + filter open, focus in it", "library", "library", async () => {
    key("/"); await wait(100); const i = document.activeElement; i.value = "a"; i.dispatchEvent(new Event("input")); await wait(300); key("Enter"); i.blur();
    $(".lib-rows").focus(); key("ArrowDown", { shift: true }); key("Enter"); await wait(300); key("\\"); await wait(300);
  }],
  ["Mod 库存 › 全部 · row menu (menu key)", "library", "library", async () => { key("ContextMenu"); await wait(300); }],
  ["Mod 库存 › 全部 · row menu (right click)", "library", "library", async () => { await rowMenu(".lib .lib-row"); }],
  ["Mod 库存 › 全部 · selection + filters set, bar open, focus on the rows + details", "library", "library", async () => {
    key("ArrowDown", { shift: true }); key("\\"); await wait(300); key("Enter"); await wait(300); $(".lib-rows").focus(); key("Enter"); await wait(300);
  }],
  ["Mod 库存 › 全部 · help", "library", "library", async () => { key("?"); }],
  ["Mod 库存 › 变化 · help", "changes", "changes", async () => { key("?"); }],
  ["方案 · selection + open filter + palette", "plan", "plan", async () => { key("ArrowDown", { shift: true }); key("\\"); await wait(300); key("k", { ctrl: true }); }],
  ["方案 · filter open, focus on the rows", "plan", "plan", async () => { key("\\"); await wait(300); $('.area[data-area="1"] .rows').focus(); document.activeElement.blur(); }],
  ["方案 · selection + search in staging", "plan", "plan", async () => { key("ArrowDown", { shift: true }); }],
  ["方案 · row menu (right click)", "plan", "plan", async () => { await rowMenu('.area[data-area="1"] .row'); }],
  ["方案 · full detail", "plan", "plan", async () => { key("Enter"); await wait(500); }],
  ["整合包 · help", "packs", "packs", async () => { key("?"); }],
  ["环境 · help", "env", "env", async () => { key("?"); }],
  ["设置 · help", "settings", "settings", async () => { key("?"); }],
  ["设置 › 卸载 · help", "uninstall", "uninstall", async () => { key("?"); }],
  ["环境 › 添加环境 · download a version (SteamCMD) + help", "guide", "guide", async () => {
    const seg = [...document.querySelectorAll("#screen .seg button")].find((b) => /下载|download/i.test(b.textContent));
    if (!seg) throw new Error("no download segment");
    seg.click(); await wait(1500); key("?");
  }],
];

export async function run(only) {
  ({ S } = await import("/js/state.js"));
  lib = await import("/js/library.js");
  const results = [];
  for (const [name, page, screen, setup] of SCENARIOS) {
    if (only && !name.includes(only)) continue;
    const r = { name, presses: 0, ok: false, built: "", left: "" };
    try {
      await go(page);
      // start from the base: the previous scenario may have left something
      for (let i = 0; i < 4 && notBase(screen); i++) { key("Escape"); await wait(200); }
      await setup();
      await wait(300);
      r.built = notBase(screen);
      for (;;) {
        const nb = notBase(screen);
        if (!nb) { r.ok = true; break; }
        if (r.presses === 3) { r.left = nb; break; }
        key("Escape");
        r.presses++;
        await wait(250);
      }
    } catch (e) {
      r.left = "error: " + e.message;
    }
    results.push(r);
  }
  return results;
}
