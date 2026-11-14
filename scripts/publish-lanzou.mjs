// Uploads a RimArcho Windows release ZIP to the Lanzou mirror folder (17 §5, maintainer only).
// It drives the maintainer's signed-in Lanzou console in ego lite, using the page's own
// upload box, so no cookie or password ever leaves the browser. Run through ego-browser,
// passing inputs in the module URL's query (ego-browser does not pass arguments or env):
//
//   ego-browser nodejs -e "await import('file:///D:/path/RimArcho/scripts/publish-lanzou.mjs?zip=D:/path/RimArcho/dist/archo-1.1.0-windows-amd64.zip')"
//
// Query parameters: zip (absolute path, required), folder (Lanzou folder name, default
// RimArcho), dry=1 (sign-in, folder and duplicate checks only; nothing is uploaded).
// Exit codes: 0 uploaded or already present, 1 failed, 2 the console needs a sign-in
// (the browser is handed to the maintainer; sign in, then run the same command again).
// Only the ZIP is uploaded: no .sha256, no Linux tarball (17 §5.1 Assets).

const params = new URL(import.meta.url).searchParams;
const zip = params.get("zip") || "";
const folderName = params.get("folder") || "RimArcho";
const dry = params.get("dry") === "1";
const CONSOLE = "https://pc.woozooo.com/mydisk.php";
const NAME = /^archo-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-windows-amd64\.zip$/;

function fail(message, code = 1) {
  console.log("FAILED: " + message);
  process.exitCode = code;
}

const fs = await import("node:fs/promises");
const path = await import("node:path");
const name = path.basename(zip.replaceAll("\\", "/"));

async function main() {
  if (!path.isAbsolute(zip)) return fail("zip must be an absolute path");
  if (!NAME.test(name)) return fail(`${name} is not a stable Windows release ZIP (archo-X.Y.Z-windows-amd64.zip)`);
  const info = await fs.stat(zip).catch(() => null);
  if (!info || !info.isFile()) return fail(`${zip} does not exist`);
  if (info.size > 100 * 1024 * 1024) return fail("Lanzou accepts at most 100 MB per file on this account");

  const task = await taskSpace(`RimArcho Lanzou publish ${name}`);
  const page = task.page("p1");
  await page.goto(CONSOLE, { timeout: 30000 });
  if (!(await page.url()).startsWith(CONSOLE)) {
    await task.handOff();
    return fail(`the Lanzou console needs a sign-in: sign in in ego lite (task space ${task.spaceId}), then run this again`, 2);
  }
  // The file manager is an iframe on the same host; open it directly so its scripts run here.
  await page.waitForSelector("iframe[src*='mydisk.php']", { timeout: 15000 });
  const manager = await page.evaluate(() => document.querySelector("iframe[src*='mydisk.php']").src);
  await page.goto(manager, { timeout: 30000 });

  // doupload.php task 47 lists folders, task 5 lists files (read-only); uid and vei come from the page.
  const api = (task, fields) => page.evaluate(async ({ task, fields }) => {
    const uid = (location.search.match(/[?&]u=(\d+)/) || [])[1];
    const vei = ([...document.scripts].map((s) => s.textContent).join("\n").match(/'vei'\s*:\s*'([^']+)'/) || [])[1];
    if (!uid || !vei) return { zt: 0, info: "console page format changed (no uid or vei)" };
    const body = new URLSearchParams({ task: String(task), vei, ...fields });
    const r = await fetch("/doupload.php?uid=" + uid, { method: "POST", body });
    return r.json();
  }, { task, fields });

  const folders = await api(47, { folder_id: "-1" });
  const folder = (folders.text || []).find((f) => f.name === folderName);
  if (folders.zt != 1 || !folder) return fail(`no Lanzou folder named ${folderName} (${folders.info || folders.zt})`);
  const listFiles = async () => {
    const out = [];
    for (let pg = 1; pg <= 16; pg++) {
      const r = await api(5, { folder_id: folder.fol_id, pg: String(pg) });
      if (r.zt != 1) {
        if (pg > 1) break; // past the last page
        throw new Error("file listing failed: " + (r.info || r.zt));
      }
      const rows = r.text || [];
      out.push(...rows);
      if (rows.length === 0) break;
    }
    return out;
  };
  const before = await listFiles();
  if (before.some((f) => f.name === name)) {
    console.log(`OK: ${name} is already in ${folderName}; nothing uploaded`);
    await task.finish({ keep: [] });
    return;
  }
  if (dry) {
    console.log(`DRY RUN: signed in; folder ${folderName} (${folder.fol_id}) has ${before.length} files; ${name} would be uploaded`);
    await task.finish({ keep: [] });
    return;
  }

  // Same steps as a click: select the folder, open the upload box, pick the file, start.
  await page.evaluate((id) => folder(Number(id)), folder.fol_id);
  await page.waitForFunction((id) => document.getElementById("folder_id_bibao")?.value === String(id), folder.fol_id, { timeout: 15000 });
  await page.evaluate(() => f_upc());
  await page.waitForSelector("input[type=file][name=file]", { state: "attached", timeout: 15000 });
  await page.setInputFiles("input[type=file][name=file] >> nth=0", [zip.replaceAll("\\", "/")]);
  await page.waitForSelector("text=开始上传", { timeout: 15000 });
  await page.click("text=开始上传", { label: "start Lanzou upload" });

  const deadline = Date.now() + 10 * 60 * 1000;
  let uploaded = null;
  while (!uploaded && Date.now() < deadline) {
    await page.waitForTimeout(3000);
    uploaded = (await listFiles()).find((f) => f.name === name) || null;
  }
  if (!uploaded) {
    await task.handOff();
    return fail(`${name} did not appear in ${folderName} within 10 minutes; the browser is left open (task space ${task.spaceId})`);
  }
  console.log(`OK: uploaded ${name} (${uploaded.size}) to ${folderName}; file id ${uploaded.id}`);
  await task.finish({ keep: [] });
}

try {
  await main();
} catch (e) {
  fail(e && e.message ? e.message : String(e));
}
