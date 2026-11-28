// RimArcho's feedback relay (16 §2.14 Feedback; deploying it: README.md). A Cloudflare Worker
// that files a report sent from RimArcho's Settings > Feedback as an issue in the public
// repository, so a player without a GitHub account can report. The GitHub App's key lives
// only here, as a Worker secret; RimArcho carries nothing but this Worker's address.
//
// Abuse limits: one report a minute and a daily cap per sender (SendGate, a Durable Object per
// sender: exact), a daily cap for everyone (a KV counter, approximate), size limits, and
// @mentions defused so a report cannot ping people. The sender's IP is only hashed, with
// the secret and the day, for the counters; it never reaches the issue. Cloudflare's
// rate-limit binding did this until 1.0.6: it is approximate and per location, and let a
// second report through 13 s after the first (VM round 1.0.6).
//
// handle, SendGate and the helpers are exported for worker.test.mjs, which runs them in Node
// with a fake GitHub, KV and Durable Object namespace.

const API = "https://api.github.com";

// LIMITS are in JavaScript string length (UTF-16 units); GitHub takes 65,536 for a body.
export const LIMITS = { title: 120, body: 60000, contact: 200, request: 70000 };

export default {
  fetch(request, env) {
    return handle(request, env);
  },
};

// handle answers POST /report with {number, url, token, issues} of the new issue (or issues:
// a body {reports: [...], contact} files up to MAX_SPLIT problems, one issue each, in one
// request), or {error} with a code RimArcho turns into words: not_configured, client,
// bad_json, empty, too_long, slow_down, ip_daily, daily, github. GET /status/<n>?t=<token> is
// the sender's read of that issue (status); GET /requests lists the open feature requests and
// POST /plus/<n> adds a +1 to one (requests and plus, below); POST /timings appends a slow
// check's timings to the one issue TIMINGS_ISSUE (timings, below).
export async function handle(request, env, deps = {}) {
  const fetchFn = deps.fetch || fetch;
  const now = deps.now || Date.now;
  const path = new URL(request.url).pathname;
  if (path.startsWith("/status/")) return status(request, env, fetchFn, now, deps.random);
  if (path === "/requests") return requests(request, env, fetchFn, now);
  if (path.startsWith("/plus/")) return plus(request, env, fetchFn, now);
  if (path === "/timings") return timings(request, env, fetchFn, now);
  if (path !== "/report") return reply(404, { error: "not_found" });
  if (request.method !== "POST") return reply(405, { error: "method" });
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS || !env.GATE) {
    return reply(503, { error: "not_configured" });
  }
  // not a defence (anyone can say it), only keeps stray crawlers out of the counters
  if (!(request.headers.get("user-agent") || "").startsWith("RimArcho/")) return reply(403, { error: "client" });
  const raw = await request.text();
  if (raw.length > LIMITS.request) return reply(413, { error: "too_long" });
  let input;
  try {
    input = JSON.parse(raw);
  } catch {
    return reply(400, { error: "bad_json" });
  }
  const batch = Array.isArray(input?.reports);
  if (batch && (input.reports.length < 1 || input.reports.length > MAX_SPLIT)) return reply(400, { error: "bad_json" });
  const reports = [];
  for (const one of batch ? input.reports : [input]) {
    const report = clean(batch ? { ...one, contact: input.contact } : one);
    if (report.error) return reply(report.status, { error: report.error });
    reports.push(report);
  }
  const k = reports.length;

  const day = new Date(now()).toISOString().slice(0, 10);
  const who = await sender(request.headers.get("cf-connecting-ip") || "", day, env.GITHUB_APP_KEY);
  const gate = env.GATE.get(env.GATE.idFromName(who));
  // one minute for the whole send; each issue counts toward the day
  const ask = (op, n) => gate.fetch(`https://gate/${op}?now=${now()}&perSender=${limit(env.PER_IP_DAY, 5)}&n=${n}`, { method: "POST" });
  const claimed = await (await ask("claim", k)).json();
  if (claimed.error) return reply(429, { error: claimed.error });
  const cap = await count(env, day, k);
  if (cap) {
    await ask("release", k);
    return reply(429, { error: cap });
  }

  const issues = [];
  for (const report of reports) {
    let issue;
    try {
      issue = await createIssue(env, report, fetchFn, now);
    } catch (e) {
      // what was not filed goes back: the player may send it again at once, and it does not
      // use the daily caps
      const left = k - issues.length;
      if (issues.length) await ask("keep", left);
      else await ask("release", left);
      await uncount(env, day, left).catch(() => {});
      return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200), issues, ...(issues[0] || {}) });
    }
    if (report.contact) {
      // for the maintainer only: wrangler kv key get --binding COUNTS contact:<number> --remote
      await env.COUNTS.put("contact:" + issue.number, report.contact, { expirationTtl: 180 * 86400 }).catch(() => {});
    }
    // the reply token: only its keyed hash is kept, so KV alone cannot read anyone's replies
    const token = randomToken(deps.random);
    await env.COUNTS.put("reply:" + issue.number, await keyed(env.GITHUB_APP_KEY, "reply", issue.number + "\n" + token), {
      expirationTtl: 180 * 86400,
    }).catch(() => {});
    issues.push({ number: issue.number, url: issue.html_url, token });
  }
  return reply(200, { ...issues[0], issues });
}

// MAX_SPLIT is how many problems one send may carry (RimArcho's "add another problem").
export const MAX_SPLIT = 3;

// KIND_LABELS turn a report's template into a label; anything else gets none. A ticked "files
// were changed or lost" adds PRIORITY_LABEL. GitHub's own labels (bug, enhancement, question)
// exist in every repository; the others are created once by hand (README step 6).
export const KIND_LABELS = { feature: "enhancement", bug: "bug", diagnosis: "diagnosis", data: "data", l10n: "localization", other: "question" };
export const PRIORITY_LABEL = "priority";
const GITHUB_LABELS = new Set(["bug", "enhancement", "question"]);
const REQUEST_LABEL = KIND_LABELS.feature;

const STATUS_PER_HOUR = 120;

// status answers GET /status/<n>?t=<token> with {state, closed_at, replies:[{body,
// created_at}]}: only comments by the repository owner, so a stranger's comment never reaches
// the player. A wrong token is 404 like a missing issue. Each sender may ask STATUS_PER_HOUR
// times an hour (a KV counter: approximate, which is enough for a read).
async function status(request, env, fetchFn, now, random) {
  if (request.method !== "GET") return reply(405, { error: "method" });
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS) return reply(503, { error: "not_configured" });
  if (!(request.headers.get("user-agent") || "").startsWith("RimArcho/")) return reply(403, { error: "client" });
  const u = new URL(request.url);
  const n = u.pathname.slice("/status/".length);
  const t = u.searchParams.get("t") || "";
  if (!/^[1-9]\d{0,9}$/.test(n) || !/^[A-Za-z0-9_-]{16,64}$/.test(t)) return reply(404, { error: "not_found" });

  if (!(await readsLeft(request, env, now))) return reply(429, { error: "slow_down" });

  const want = await env.COUNTS.get("reply:" + n);
  if (!want || !same(want, await keyed(env.GITHUB_APP_KEY, "reply", n + "\n" + t))) return reply(404, { error: "not_found" });
  try {
    const gh = await installationToken(env, fetchFn, now);
    let res = await github(fetchFn, "GET", `/repos/${env.REPO}/issues/${n}`, gh);
    if (res.status === 404 || res.status === 410) return reply(404, { error: "not_found" });
    if (!res.ok) throw new Error(`reading the issue: ${res.status}`);
    const issue = await res.json();
    res = await github(fetchFn, "GET", `/repos/${env.REPO}/issues/${n}/comments?per_page=100`, gh);
    if (!res.ok) throw new Error(`reading the comments: ${res.status}`);
    const replies = (await res.json())
      .filter((c) => c && c.author_association === "OWNER")
      .map((c) => ({ body: String(c.body || ""), created_at: c.created_at }));
    return reply(200, { state: issue.state === "closed" ? "closed" : "open", closed_at: issue.closed_at || null, replies });
  } catch (e) {
    return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200) });
  }
}

// randomToken is 24 random bytes, base64url: the sender's only key to its issue's replies.
export function randomToken(random) {
  const b = new Uint8Array(24);
  (random || ((a) => crypto.getRandomValues(a)))(b);
  return b64url(b);
}

// keyed is HMAC-SHA-256 of msg with a key derived from the App's secret and a purpose.
export async function keyed(secret, purpose, msg) {
  const enc = new TextEncoder();
  const k = await crypto.subtle.importKey("raw", enc.encode(purpose + "\n" + secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  return b64url(new Uint8Array(await crypto.subtle.sign("HMAC", k, enc.encode(msg))));
}

function same(a, b) {
  if (a.length !== b.length) return false;
  let d = 0;
  for (let i = 0; i < a.length; i++) d |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return d === 0;
}

function reply(status, data) {
  return new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json; charset=utf-8" } });
}

// clean checks and tidies a report: {title, body, contact} as strings within LIMITS, a title
// from the body's first line when there is none, mentions defused.
export function clean(input) {
  if (!input || typeof input !== "object") return { status: 400, error: "bad_json" };
  const str = (v) => (typeof v === "string" ? v : "");
  let title = str(input.title).replace(/\s+/g, " ").trim();
  const body = str(input.body).replace(/\r\n?/g, "\n").trim();
  const contact = str(input.contact).replace(/\s+/g, " ").trim();
  if (!title && !body) return { status: 400, error: "empty" };
  if (body.length > LIMITS.body || contact.length > LIMITS.contact) return { status: 413, error: "too_long" };
  if (!title) title = body.split("\n").map((l) => l.trim()).find((l) => l && l !== "---" && !l.startsWith("<")) || "Feedback";
  const cps = Array.from(title);
  if (cps.length > LIMITS.title) title = cps.slice(0, LIMITS.title - 1).join("") + "…";
  const kind = Object.hasOwn(KIND_LABELS, input.kind) ? input.kind : "";
  return { title: defuse(title), body: defuse(body), contact, kind, priority: input.priority === true && kind === "bug" };
}

// defuse keeps a report from notifying anyone: @name and owner/repo#1 get a zero-width
// space after @ and #, so GitHub shows them but does not link or notify.
export function defuse(s) {
  return s.replace(/(^|[^\w`])@(?=[A-Za-z0-9])/g, "$1@​").replace(/([\w.-]+\/[\w.-]+)#(?=\d)/g, "$1#​");
}

// sender is the IP's hash with the secret and the day: the counters can tell senders apart
// for a day, and nobody (not even with the KV contents) can get the IP back.
export async function sender(ip, day, secret) {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(secret + "\n" + day + "\n" + ip));
  return [...new Uint8Array(d)].slice(0, 16).map((b) => b.toString(16).padStart(2, "0")).join("");
}

// count adds a report to today's counter for everyone, or says "daily" when it is full. KV
// is eventually consistent, so a burst from many senders can pass the cap by a few; each
// sender's own limits are exact (SendGate).
export async function count(env, day, n = 1, what = "all", perDay = limit(env.PER_DAY, 30)) {
  const all = `${what}:${day}`;
  const total = parseInt((await env.COUNTS.get(all)) || "0", 10) || 0;
  if (total + n > perDay) return "daily";
  await env.COUNTS.put(all, String(total + n), { expirationTtl: 2 * 86400 }).catch(() => {});
  return "";
}

// uncount gives n reports back to today's counter for everyone when they were not filed.
export async function uncount(env, day, n = 1, what = "all") {
  const all = `${what}:${day}`;
  const total = parseInt((await env.COUNTS.get(all)) || "0", 10) || 0;
  if (total > 0) await env.COUNTS.put(all, String(Math.max(0, total - n)), { expirationTtl: 2 * 86400 });
}

const MINUTE = 60000;

// SendGate is one sender's limits for one day (its name is the day's sender hash): the time of
// the last report and how many were sent. A Durable Object handles one request at a time per
// sender and reads back its own writes, so a resend within the minute is always refused.
// POST /claim?now=&perSender=&n= answers {} or {error: "slow_down" | "ip_daily"} and records
// n reports (a split send); POST /release?n= gives the last claim back (no issue was filed);
// POST /keep?n= gives back only n of its reports and keeps the minute (some were filed). The
// object deletes its storage two days after its first report.
export class SendGate {
  constructor(state) {
    this.storage = state.storage;
  }

  async fetch(request) {
    const u = new URL(request.url);
    const now = Number(u.searchParams.get("now")) || Date.now();
    const s = (await this.storage.get("s")) || { last: 0, prev: 0, n: 0 };
    const k = Math.max(1, limit(u.searchParams.get("n"), 1));
    if (u.pathname === "/release") {
      if (s.n > 0) await this.storage.put("s", { last: s.prev, prev: s.prev, n: Math.max(0, s.n - k) });
      return Response.json({});
    }
    if (u.pathname === "/keep") {
      if (s.n > 0) await this.storage.put("s", { ...s, n: Math.max(0, s.n - k) });
      return Response.json({});
    }
    if (s.last && now - s.last < MINUTE) return Response.json({ error: "slow_down" });
    if (s.n + k > limit(u.searchParams.get("perSender"), 5)) return Response.json({ error: "ip_daily" });
    if (!s.n) await this.storage.setAlarm(now + 2 * 86400e3);
    await this.storage.put("s", { last: now, prev: s.last, n: s.n + k });
    return Response.json({});
  }

  async alarm() {
    await this.storage.deleteAll();
  }
}

function limit(v, dflt) {
  const n = parseInt(v ?? "", 10);
  return Number.isFinite(n) && n >= 0 ? n : dflt;
}

// createIssue files the report with the App's installation token, labelled with LABEL, its
// kind (KIND_LABELS) and PRIORITY_LABEL when files were changed or lost. GitHub refuses the
// whole issue over a label that does not exist: then it tries LABEL with GitHub's own labels,
// then none, rather than losing the report.
export async function createIssue(env, report, fetchFn, now) {
  const token = await installationToken(env, fetchFn, now);
  let body = report.body + "\n\n---\n<sub>Sent from RimArcho's Feedback page, without a GitHub account.";
  if (report.contact) body += " The sender left a contact for the maintainer.";
  body += "</sub>\n";
  const base = String(env.LABEL || "").split(",").map((s) => s.trim()).filter(Boolean);
  const all = [...base];
  if (report.kind) all.push(KIND_LABELS[report.kind]);
  if (report.priority) all.push(PRIORITY_LABEL);
  const tries = [all, all.filter((l) => base.includes(l) || GITHUB_LABELS.has(l)), []];
  const issue = { title: report.title, body };
  let res;
  for (let i = 0; i < tries.length; i++) {
    const labels = tries[i];
    if (i > 0 && (labels.length === tries[i - 1].length || (!labels.length && !tries[i - 1].length))) continue;
    res = await github(fetchFn, "POST", `/repos/${env.REPO}/issues`, token, labels.length ? { ...issue, labels } : issue);
    if (res.status !== 422 || !labels.length) break;
  }
  if (!res.ok) throw new Error(`creating the issue: ${res.status} ${(await res.text()).slice(0, 120)}`);
  return res.json();
}

const REQUESTS_TTL = 600; // seconds the open feature requests stand in KV
const PLUS_PER_DAY = 20; // +1s a day from one sender

// listRequests is the open issues labelled REQUEST_LABEL (no pull requests), newest 100, each
// with its +1s: GitHub's reactions plus the ones sent through here. Kept REQUESTS_TTL in KV,
// so a page of typing players asks GitHub once.
async function listRequests(env, fetchFn, now, fresh = false) {
  if (!fresh) {
    const hit = await env.COUNTS.get("cache:requests");
    if (hit) return JSON.parse(hit);
  }
  const gh = await installationToken(env, fetchFn, now);
  const res = await github(fetchFn, "GET", `/repos/${env.REPO}/issues?state=open&labels=${REQUEST_LABEL}&per_page=100`, gh);
  if (!res.ok) throw new Error(`listing the requests: ${res.status}`);
  const out = [];
  for (const i of await res.json()) {
    if (!i || i.pull_request || !Number.isInteger(i.number)) continue;
    const relayed = parseInt((await env.COUNTS.get("plus:" + i.number)) || "0", 10) || 0;
    out.push({ number: i.number, title: String(i.title || ""), url: String(i.html_url || ""), plus: (i.reactions?.["+1"] || 0) + relayed });
  }
  await env.COUNTS.put("cache:requests", JSON.stringify(out), { expirationTtl: REQUESTS_TTL }).catch(() => {});
  return out;
}

// readsLeft counts one read by the sender this hour (status and requests share it) and says
// whether it is within STATUS_PER_HOUR.
async function readsLeft(request, env, now) {
  const hour = new Date(now()).toISOString().slice(0, 13);
  const who = await sender(request.headers.get("cf-connecting-ip") || "", hour, env.GITHUB_APP_KEY);
  const key = `status:${who}`;
  const asked = parseInt((await env.COUNTS.get(key)) || "0", 10) || 0;
  if (asked >= limit(env.STATUS_PER_HOUR, STATUS_PER_HOUR)) return false;
  await env.COUNTS.put(key, String(asked + 1), { expirationTtl: 2 * 3600 }).catch(() => {});
  return true;
}

// requests answers GET /requests with {requests: [{number, title, url, plus}]}.
async function requests(request, env, fetchFn, now) {
  if (request.method !== "GET") return reply(405, { error: "method" });
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS) return reply(503, { error: "not_configured" });
  if (!(request.headers.get("user-agent") || "").startsWith("RimArcho/")) return reply(403, { error: "client" });
  if (!(await readsLeft(request, env, now))) return reply(429, { error: "slow_down" });
  try {
    return reply(200, { requests: await listRequests(env, fetchFn, now) });
  } catch (e) {
    return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200) });
  }
}

// plus answers POST /plus/<n> with {plus}: a +1 from a player without a GitHub account for an
// open feature request. One per sender per request per day (the sender's hash changes daily,
// so RimArcho also remembers what it gave), PLUS_PER_DAY a day per sender. The count lives in
// KV (plus:<n>, no expiry) and in one comment by the App on the issue, edited as it grows.
async function plus(request, env, fetchFn, now) {
  if (request.method !== "POST") return reply(405, { error: "method" });
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS) return reply(503, { error: "not_configured" });
  if (!(request.headers.get("user-agent") || "").startsWith("RimArcho/")) return reply(403, { error: "client" });
  const n = new URL(request.url).pathname.slice("/plus/".length);
  if (!/^[1-9]\d{0,9}$/.test(n)) return reply(404, { error: "not_found" });
  try {
    let list = await listRequests(env, fetchFn, now);
    let r = list.find((x) => String(x.number) === n);
    if (!r) {
      list = await listRequests(env, fetchFn, now, true);
      r = list.find((x) => String(x.number) === n);
    }
    if (!r) return reply(404, { error: "not_found" });
    const day = new Date(now()).toISOString().slice(0, 10);
    const who = await sender(request.headers.get("cf-connecting-ip") || "", day, env.GITHUB_APP_KEY);
    if (await env.COUNTS.get(`plusby:${who}:${n}`)) return reply(200, { plus: r.plus });
    const today = parseInt((await env.COUNTS.get(`plusday:${who}`)) || "0", 10) || 0;
    if (today >= PLUS_PER_DAY) return reply(429, { error: "slow_down" });
    await env.COUNTS.put(`plusday:${who}`, String(today + 1), { expirationTtl: 2 * 86400 }).catch(() => {});
    await env.COUNTS.put(`plusby:${who}:${n}`, "1", { expirationTtl: 2 * 86400 }).catch(() => {});
    const relayed = (parseInt((await env.COUNTS.get("plus:" + n)) || "0", 10) || 0) + 1;
    await env.COUNTS.put("plus:" + n, String(relayed));
    r.plus++;
    await env.COUNTS.put("cache:requests", JSON.stringify(list), { expirationTtl: REQUESTS_TTL }).catch(() => {});
    await plusComment(env, fetchFn, now, n, relayed).catch(() => {});
    return reply(200, { plus: r.plus });
  } catch (e) {
    return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200) });
  }
}

// plusComment writes the relayed count on the issue: one comment by the App, edited after.
async function plusComment(env, fetchFn, now, n, count) {
  const gh = await installationToken(env, fetchFn, now);
  const body = `+${count} from RimArcho players without a GitHub account (RimArcho's Feedback page).`;
  const id = await env.COUNTS.get("plusc:" + n);
  if (id) {
    const res = await github(fetchFn, "PATCH", `/repos/${env.REPO}/issues/comments/${id}`, gh, { body });
    if (res.ok) return;
  }
  const res = await github(fetchFn, "POST", `/repos/${env.REPO}/issues/${n}/comments`, gh, { body });
  if (res.ok) await env.COUNTS.put("plusc:" + n, String((await res.json()).id));
}

// TIMINGS_FIELDS is everything a slow check's timings may carry at the top level (RimArcho's
// feedback.Timings, 16 §2.14 "Slow checks"): anything else is dropped, so no player, path or
// account can reach the issue even from a changed client. Strings are short and plain;
// numbers are bounded. phases, run, patches and harmony are cleaned on their own below.
const TIMINGS_FIELDS = {
  version: "str",
  os: "str",
  game: "str",
  active_mods: "int",
  total_mods: "int",
  automatic: "bool",
  priority: "str",
  wall_ms: "int",
  cpu: "str",
  cpu_threads: "int",
  ram_gb: "int",
  disk: "str",
};
const TIMINGS_RUN = { cold: "bool", first: "bool", list: "str", mods_read: "int", mods_cached: "int", workers: "int", gc_ms: "int", peak_heap_mb: "int" };
const TIMINGS_PATCHES = { ops: "int", nodes: "int", too_slow: "int", parse_ms: "int", sim_ms: "int", facts_ms: "int" };
const TIMINGS_HARMONY = { dlls: "int", cached: "int", patches: "int", methods: "int" };
const TIMINGS_OP = { ms: "ms", pkg: "pkg", ws: "ws", class: "class", xpath: "xpath", file: "file", too_slow: "bool" };
const TIMINGS_DLL = { ms: "ms", pkg: "pkg", ws: "ws", file: "file", cached: "bool" };
const TIMINGS_PHASE = /^[a-z]{1,16}$/;
const TIMINGS_MAX_PHASES = 20;
const TIMINGS_SLOWEST_OPS = 10;
const TIMINGS_SLOWEST_DLLS = 5;
const TIMINGS_PER_IP_DAY = 2; // timings a day from one sender
const TIMINGS_PER_DAY = 100; // timings a day from everyone
const TIMINGS_MAX_BYTES = 16000; // ten XPaths of 300 characters fit

// a path on the sender's computer: a drive, a share, a user folder
const LOCAL_PATH = /(^|[^a-z])[a-z]:[\\/]|\\\\|(^|[\\/])(users|home|documents and settings)[\\/]/i;

// cleanValue cleans one value of a kind; undefined drops it.
function cleanValue(kind, v) {
  switch (kind) {
    case "str":
      return typeof v === "string" && v.trim() ? v.replace(/[^\w .,()+\-]/g, "").trim().slice(0, 80) || undefined : undefined;
    case "int":
      return Number.isInteger(v) && v >= 0 && v < 1e9 ? v : undefined;
    case "ms":
      return typeof v === "number" && Number.isFinite(v) && v >= 0 && v < 1e9 ? Math.round(v * 10) / 10 : undefined;
    case "bool":
      return typeof v === "boolean" ? v : undefined;
    case "pkg": // a packageId
      return typeof v === "string" && /^[\w.\-]{1,100}$/.test(v) ? v : undefined;
    case "ws": // a Workshop id
      return typeof v === "string" && /^\d{1,20}$/.test(v) ? v : undefined;
    case "class": // PatchOperationAdd, or a Mod's own class (namespace and generic arity allowed)
      return typeof v === "string" && /^[\w.+`\-]{1,120}$/.test(v) ? v : undefined;
    case "xpath": {
      if (typeof v !== "string" || LOCAL_PATH.test(v)) return undefined;
      const s = [...v.replace(/[\u0000-\u001f\u007f`]/g, "")].slice(0, 300).join("");
      return s || undefined;
    }
    case "file": { // relative to the Mod's folder
      if (typeof v !== "string") return undefined;
      const s = v.replace(/\\/g, "/");
      if (LOCAL_PATH.test(v) || s.startsWith("/") || s.split("/").includes("..")) return undefined;
      const f = [...s.replace(/[^\p{L}\p{N} ._\/()\-+\[\]&',]/gu, "")].slice(0, 200).join("");
      return f || undefined;
    }
  }
  return undefined;
}

// cleanObject keeps the fields of shape in input that clean, or null when none does.
function cleanObject(input, shape) {
  if (!input || typeof input !== "object" || Array.isArray(input)) return null;
  const out = {};
  for (const [k, kind] of Object.entries(shape)) {
    const v = cleanValue(kind, input[k]);
    if (v !== undefined) out[k] = v;
  }
  return Object.keys(out).length ? out : null;
}

// cleanPhases reads {name: ms} (1.1.6 and later) or [{name, ms}] (1.1.5) into {name: ms}.
function cleanPhases(input) {
  const pairs = Array.isArray(input)
    ? input.map((p) => [p?.name, p?.ms])
    : input && typeof input === "object"
      ? Object.entries(input)
      : [];
  const out = {};
  for (const [name, ms] of pairs) {
    if (Object.keys(out).length >= TIMINGS_MAX_PHASES) break;
    if (typeof name === "string" && TIMINGS_PHASE.test(name) && !(name in out) && cleanValue("int", ms) !== undefined) out[name] = ms;
  }
  return Object.keys(out).length ? out : null;
}

// cleanSlowest cleans up to n entries of an array, dropping ones without a packageId.
function cleanSlowest(input, shape, n) {
  if (!Array.isArray(input)) return undefined;
  const out = input.slice(0, 4 * n).map((x) => cleanObject(x, shape)).filter((x) => x && x.pkg && x.ms !== undefined).slice(0, n);
  return out.length ? out : undefined;
}

// cleanTimings keeps the fields above; null when the request is not timings at all (no
// wall time, or no phases). The phases come out as one object in either case.
export function cleanTimings(input) {
  if (!input || typeof input !== "object" || Array.isArray(input)) return null;
  const out = cleanObject(input, TIMINGS_FIELDS) || {};
  const phases = cleanPhases(input.phases);
  if (!phases || !out.wall_ms) return null;
  out.phases = phases;
  const run = cleanObject(input.run, TIMINGS_RUN);
  if (run) out.run = run;
  const patches = cleanObject(input.patches, TIMINGS_PATCHES) || {};
  const ops = cleanSlowest(input.patches?.slowest, TIMINGS_OP, TIMINGS_SLOWEST_OPS);
  if (ops) patches.slowest = ops;
  if (Object.keys(patches).length) out.patches = patches;
  const harmony = cleanObject(input.harmony, TIMINGS_HARMONY) || {};
  const dlls = cleanSlowest(input.harmony?.slowest, TIMINGS_DLL, TIMINGS_SLOWEST_DLLS);
  if (dlls) harmony.slowest = dlls;
  if (Object.keys(harmony).length) out.harmony = harmony;
  // the top level in RimArcho's order
  const order = [...Object.keys(TIMINGS_FIELDS).slice(0, 8), "phases", "run", "patches", "harmony", ...Object.keys(TIMINGS_FIELDS).slice(8)];
  return Object.fromEntries(order.filter((k) => k in out).map((k) => [k, out[k]]));
}

// compactJSON lays t out as RimArcho's preview does: one field to a line, phases on one
// line, each slow operation on one line.
export function compactJSON(t) {
  const field = (k, v, pad) => {
    if (k !== "phases" && v && typeof v === "object" && !Array.isArray(v)) {
      return `${pad}${JSON.stringify(k)}: {\n${Object.entries(v).map(([k2, v2]) => field(k2, v2, pad + "  ")).join(",\n")}\n${pad}}`;
    }
    if (Array.isArray(v)) return `${pad}${JSON.stringify(k)}: [\n${v.map((x) => pad + "  " + JSON.stringify(x)).join(",\n")}\n${pad}]`;
    return `${pad}${JSON.stringify(k)}: ${JSON.stringify(v)}`;
  };
  return `{\n${Object.entries(t).map(([k, v]) => field(k, v, "  ")).join(",\n")}\n}`;
}

// culprit is a phase and its seconds, with the slowest operation or DLL inside it when the
// timings name one: "patches 43.8 s (slowest: some.mod 12.1 s PatchOperationReplace)".
function culprit(t, name, ms) {
  const s = (ms) => (ms / 1000).toFixed(1) + " s";
  let line = `${name} ${s(ms)}`;
  const top = name === "patches" ? t.patches?.slowest?.[0] : name === "harmony" ? t.harmony?.slowest?.[0] : null;
  if (top) line += ` (slowest: ${top.pkg} ${s(top.ms)}${top.class ? " " + top.class : top.file ? " " + top.file.split("/").pop() : ""})`;
  return line;
}

// timingsComment is the comment: one line a maintainer can scan, naming the two slowest
// phases, then the JSON as it came (cleaned).
export function timingsComment(t) {
  const s = (ms) => (ms / 1000).toFixed(1) + " s";
  const top = Object.entries(t.phases).sort((a, b) => b[1] - a[1]).slice(0, 2).map(([n, ms]) => culprit(t, n, ms)).join(", ");
  const how = [t.automatic ? "automatic" : "", t.run ? (t.run.cold ? "cold" : "warm") : ""].filter(Boolean).join(", ");
  const head = `**${s(t.wall_ms)}**${how ? ` (${how})` : ""}: ${t.active_mods ?? "?"}/${t.total_mods ?? "?"} Mods, game ${t.game || "?"}, RimArcho ${t.version || "?"}; slowest: ${top}`;
  return defuse(head) + "\n\n```json\n" + compactJSON(t) + "\n```\n\n<sub>Sent from RimArcho after a slow check.</sub>\n";
}

// timings answers POST /timings with {ok: true} after appending the timings as a comment on
// issue TIMINGS_ISSUE; 404 {error: "not_found"} when no issue is set (RimArcho says nothing
// then). The limits are the report's: one a minute and TIMINGS_PER_IP_DAY a day per sender
// (a SendGate of its own, so timings never use up a sender's reports) and TIMINGS_PER_DAY a
// day for everyone (a KV counter).
async function timings(request, env, fetchFn, now) {
  const issue = String(env.TIMINGS_ISSUE || "").trim();
  if (!/^[1-9]\d{0,9}$/.test(issue)) return reply(404, { error: "not_found" });
  if (request.method !== "POST") return reply(405, { error: "method" });
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS || !env.GATE) return reply(503, { error: "not_configured" });
  if (!(request.headers.get("user-agent") || "").startsWith("RimArcho/")) return reply(403, { error: "client" });
  const raw = await request.text();
  if (raw.length > TIMINGS_MAX_BYTES) return reply(413, { error: "too_long" });
  let t;
  try {
    t = cleanTimings(JSON.parse(raw));
  } catch {
    return reply(400, { error: "bad_json" });
  }
  if (!t) return reply(400, { error: "empty" });

  const day = new Date(now()).toISOString().slice(0, 10);
  const who = await sender(request.headers.get("cf-connecting-ip") || "", day, env.GITHUB_APP_KEY);
  const gate = env.GATE.get(env.GATE.idFromName("timings:" + who));
  const ask = (op) => gate.fetch(`https://gate/${op}?now=${now()}&perSender=${limit(env.TIMINGS_PER_IP_DAY, TIMINGS_PER_IP_DAY)}&n=1`, { method: "POST" });
  const claimed = await (await ask("claim")).json();
  if (claimed.error) return reply(429, { error: claimed.error });
  const cap = await count(env, day, 1, "timings", limit(env.TIMINGS_PER_DAY, TIMINGS_PER_DAY));
  if (cap) {
    await ask("release");
    return reply(429, { error: cap });
  }
  try {
    const gh = await installationToken(env, fetchFn, now);
    const res = await github(fetchFn, "POST", `/repos/${env.REPO}/issues/${issue}/comments`, gh, { body: timingsComment(t) });
    if (!res.ok) throw new Error(`commenting on #${issue}: ${res.status}`);
  } catch (e) {
    await ask("release");
    await uncount(env, day, 1, "timings").catch(() => {});
    return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200) });
  }
  return reply(200, { ok: true });
}

let cached = null; // {repo, token, until}: an installation token lasts an hour

export function forgetToken() {
  cached = null;
}

// installationToken is the App's token for the repository's installation, reused while it
// has more than a minute left.
export async function installationToken(env, fetchFn, now) {
  if (cached && cached.repo === env.REPO && cached.until > now() + 60000) return cached.token;
  const jwt = await appJWT(env.APP_ID, env.GITHUB_APP_KEY, now());
  let res = await github(fetchFn, "GET", `/repos/${env.REPO}/installation`, jwt);
  if (!res.ok) throw new Error(`finding the App's installation on ${env.REPO}: ${res.status}`);
  const { id } = await res.json();
  res = await github(fetchFn, "POST", `/app/installations/${id}/access_tokens`, jwt);
  if (!res.ok) throw new Error(`getting an installation token: ${res.status}`);
  const t = await res.json();
  cached = { repo: env.REPO, token: t.token, until: Date.parse(t.expires_at) || now() + 50 * 60000 };
  return t.token;
}

function github(fetchFn, method, path, token, body) {
  const headers = {
    accept: "application/vnd.github+json",
    authorization: `Bearer ${token}`,
    "user-agent": "rimarcho-feedback-relay",
    "x-github-api-version": "2022-11-28",
  };
  if (body) headers["content-type"] = "application/json";
  return fetchFn(API + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
}

// appJWT is the GitHub App's own token (RS256, ten minutes at most): iat a minute back for
// clock drift. appId is the App ID (a number) or its client ID.
export async function appJWT(appId, pem, nowMs) {
  const iat = Math.floor(nowMs / 1000) - 60;
  const iss = /^\d+$/.test(String(appId)) ? Number(appId) : String(appId);
  const part = (o) => b64url(new TextEncoder().encode(JSON.stringify(o)));
  const data = part({ alg: "RS256", typ: "JWT" }) + "." + part({ iat, exp: iat + 600, iss });
  const key = await crypto.subtle.importKey("pkcs8", pkcs8(pem), { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["sign"]);
  const sig = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", key, new TextEncoder().encode(data));
  return data + "." + b64url(new Uint8Array(sig));
}

// pkcs8 reads the private key GitHub hands out ("BEGIN RSA PRIVATE KEY", PKCS#1) or a
// PKCS#8 one, as the DER WebCrypto imports; PKCS#1 is wrapped in a PrivateKeyInfo, so the
// downloaded .pem works as it is. Literal "\n" (a key pasted on one line) are newlines.
export function pkcs8(pem) {
  const text = String(pem).replace(/\\n/g, "\n");
  const der = Uint8Array.from(atob(text.replace(/-----[^-]+-----/g, "").replace(/\s+/g, "")), (c) => c.charCodeAt(0));
  if (!/BEGIN RSA PRIVATE KEY/.test(text)) return der;
  const algorithm = [0x30, 0x0d, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01, 0x05, 0x00]; // rsaEncryption, NULL
  const inner = [0x02, 0x01, 0x00, ...algorithm, 0x04, ...derLength(der.length), ...der];
  return new Uint8Array([0x30, ...derLength(inner.length), ...inner]);
}

function derLength(n) {
  if (n < 0x80) return [n];
  const bytes = [];
  for (; n > 0; n >>= 8) bytes.unshift(n & 0xff);
  return [0x80 | bytes.length, ...bytes];
}

function b64url(bytes) {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
