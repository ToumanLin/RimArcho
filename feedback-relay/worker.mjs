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

// handle answers POST /report with {number, url} of the new issue, or {error} with a code
// RimArcho turns into words: not_configured, client, bad_json, empty, too_long, slow_down,
// ip_daily, daily, github.
export async function handle(request, env, deps = {}) {
  const fetchFn = deps.fetch || fetch;
  const now = deps.now || Date.now;
  if (new URL(request.url).pathname !== "/report") return reply(404, { error: "not_found" });
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
  const report = clean(input);
  if (report.error) return reply(report.status, { error: report.error });

  const day = new Date(now()).toISOString().slice(0, 10);
  const who = await sender(request.headers.get("cf-connecting-ip") || "", day, env.GITHUB_APP_KEY);
  const gate = env.GATE.get(env.GATE.idFromName(who));
  const ask = (op) => gate.fetch(`https://gate/${op}?now=${now()}&perSender=${limit(env.PER_IP_DAY, 5)}`, { method: "POST" });
  const claimed = await (await ask("claim")).json();
  if (claimed.error) return reply(429, { error: claimed.error });
  const cap = await count(env, day);
  if (cap) {
    await ask("release");
    return reply(429, { error: cap });
  }

  let issue;
  try {
    issue = await createIssue(env, report, fetchFn, now);
  } catch (e) {
    await ask("release"); // nothing was filed: the player may send again at once
    return reply(502, { error: "github", detail: String(e?.message || e).slice(0, 200) });
  }
  if (report.contact) {
    // for the maintainer only: wrangler kv key get --binding COUNTS contact:<number> --remote
    await env.COUNTS.put("contact:" + issue.number, report.contact, { expirationTtl: 180 * 86400 }).catch(() => {});
  }
  return reply(200, { number: issue.number, url: issue.html_url });
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
  return { title: defuse(title), body: defuse(body), contact };
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
export async function count(env, day) {
  const perDay = limit(env.PER_DAY, 30);
  const all = `all:${day}`;
  const total = parseInt((await env.COUNTS.get(all)) || "0", 10) || 0;
  if (total >= perDay) return "daily";
  await env.COUNTS.put(all, String(total + 1), { expirationTtl: 2 * 86400 }).catch(() => {});
  return "";
}

const MINUTE = 60000;

// SendGate is one sender's limits for one day (its name is the day's sender hash): the time of
// the last report and how many were sent. A Durable Object handles one request at a time per
// sender and reads back its own writes, so a resend within the minute is always refused.
// POST /claim?now=&perSender= answers {} or {error: "slow_down" | "ip_daily"} and records the
// report; POST /release gives the last claim back (no issue was filed). The object deletes its
// storage two days after its first report.
export class SendGate {
  constructor(state) {
    this.storage = state.storage;
  }

  async fetch(request) {
    const u = new URL(request.url);
    const now = Number(u.searchParams.get("now")) || Date.now();
    const s = (await this.storage.get("s")) || { last: 0, prev: 0, n: 0 };
    if (u.pathname === "/release") {
      if (s.n > 0) await this.storage.put("s", { last: s.prev, prev: s.prev, n: s.n - 1 });
      return Response.json({});
    }
    if (s.last && now - s.last < MINUTE) return Response.json({ error: "slow_down" });
    if (s.n >= limit(u.searchParams.get("perSender"), 5)) return Response.json({ error: "ip_daily" });
    if (!s.n) await this.storage.setAlarm(now + 2 * 86400e3);
    await this.storage.put("s", { last: now, prev: s.last, n: s.n + 1 });
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

// createIssue files the report with the App's installation token. A label the App may not
// set (or that does not exist) is dropped rather than losing the report.
export async function createIssue(env, report, fetchFn, now) {
  const token = await installationToken(env, fetchFn, now);
  let body = report.body + "\n\n---\n<sub>Sent from RimArcho's Feedback page, without a GitHub account.";
  if (report.contact) body += " The sender left a contact for the maintainer.";
  body += "</sub>\n";
  const labels = String(env.LABEL || "").split(",").map((s) => s.trim()).filter(Boolean);
  const issue = { title: report.title, body };
  let res = await github(fetchFn, "POST", `/repos/${env.REPO}/issues`, token, labels.length ? { ...issue, labels } : issue);
  if (res.status === 422 && labels.length) res = await github(fetchFn, "POST", `/repos/${env.REPO}/issues`, token, issue);
  if (!res.ok) throw new Error(`creating the issue: ${res.status} ${(await res.text()).slice(0, 120)}`);
  return res.json();
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
