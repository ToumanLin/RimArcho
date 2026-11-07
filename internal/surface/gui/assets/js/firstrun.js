import { el, $, api, T, Tf, errText, say } from "./state.js";
import { button, chooseLocalPath } from "./screens.js";
import { editablePath, copyablePath } from "./fields.js";
import * as pop from "./popups.js";
import { bootDone } from "./archo.js";

const bytes = n => n < 0 ? T("free space unknown") : n >= 1073741824 ? (n / 1073741824).toFixed(1) + " GB" : (n / 1048576).toFixed(1) + " MB";

// Closing either prompt cancels it; no setting or release file changes until submit.
function folderDialog(title, sentence, info, inspect, submit, label, decline) {
  return new Promise(resolve => {
    let finished = false;
    pop.open("ask", T(title), (dialog, p) => {
      dialog.append(el("div", "ttl", T(title)));
      const body = el("div", "body");
      body.append(el("div", "", T(sentence)));
      const row = el("div", "typed"), address = el("span");
      let busy = false, sequence = 0;
      p.canClose = () => !busy;
      const space = el("div", "muted folder-space"), warning = el("div", "warn"), replace = el("div"), error = el("div", "err");
      const update = () => {
        address.textContent = info.path;
        editablePath(copyablePath(address, info.path), choose);
        space.textContent = "";
        if (info.workshop) space.append(el("span", "", Tf("space needed: %s", bytes(info.bytes))));
        space.append(el("span", "", Tf("free: %s", bytes(info.free))));
        warning.textContent = info.low ? T("Less than 10 GB is free on this drive.") : "";
        replace.textContent = info.replace ? T("The RimArcho already in this folder will be replaced.") : "";
        error.textContent = info.error || "";
        yes.disabled = busy || !!info.error;
      };
      const choose = () => chooseLocalPath(info.path, async path => {
        const seq = ++sequence;
        busy = true; yes.disabled = true;
        try { const next = await inspect(path); if (seq === sequence) info = next; }
        catch (e) { if (seq === sequence) info.error = errText(e); }
        finally { if (seq === sequence) { busy = false; update(); } }
      });
      row.append(address, button(T("change…"), "", choose));
      body.append(row, space, warning, replace, error); dialog.append(body);
      const buttons = el("div", "btns");
      const no = button(T(decline), "esc", () => pop.close());
      const yes = button(T(label), "enter", async () => {
        if (busy || info.error) return;
        busy = true; yes.disabled = no.disabled = true;
        try {
          await submit(info.path);
          finished = true; busy = false; pop.close(); resolve(true);
        } catch (e) { error.textContent = T(errText(e)); }
        finally { busy = false; yes.disabled = no.disabled = false; }
      }, "go");
      buttons.append(no, yes); dialog.append(buttons);
      p.onclose = () => { if (!finished) resolve(false); };
      p.keys = e => { if (e.key === "Enter" && !e.target.closest("button")) { e.preventDefault(); yes.click(); } };
      update();
    });
  });
}

export async function workshopStart() {
  try { await api.FinishWorkshopRelocation(); } catch (e) { say("warn", errText(e)); }
  const info = await api.WorkshopFolder("");
  if (!info.workshop) return true;
  bootDone(); // the dialog needs the window
  const moved = await folderDialog("install RimArcho", "Steam rewrites this Workshop folder when the item updates, so RimArcho needs a folder of its own. The files extracted here are removed after the move.", info,
    path => api.WorkshopFolder(path), path => api.RelocateWorkshop(path), "install and restart", "use here this time");
  if (moved) return false;
  const warning = el("div", "muted", T("Steam can replace or delete this folder; self-update is unavailable this session."));
  warning.id = "workshop-warning";
  $("#top").after(warning);
  return true;
}
