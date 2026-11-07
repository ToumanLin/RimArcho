// RimArcho's feedback relay (16 §2.14 Feedback; deploying it: README.md). A Cloudflare Worker
// that files a report sent from RimArcho's Settings > Feedback as an issue in the public
// repository, so a player without a GitHub account can report. The GitHub App's key lives
// only here, as a Worker secret; RimArcho carries nothing but this Worker's address.
//
// Abuse limits: one report a minute per sender (the PER_MINUTE rate-limit binding), a daily
// cap per sender and a daily cap for everyone (KV counters, approximate), size limits, and
// @mentions defused so a report cannot ping people. The sender's IP is only hashed, with
// the secret and the day, for the counters; it never reaches the issue.
//
// handle and the helpers are exported for worker.test.mjs, which runs them in Node with a
// fake GitHub, KV and rate limiter.

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
  if (!env.APP_ID || !env.GITHUB_APP_KEY || !env.REPO || !env.COUNTS || !env.PER_MINUTE) {
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
  const { success } = await env.PER_MINUTE.limit({ key: who });
  if (!success) return reply(429, { error: "slow_down" });
  const cap = await count(env, who, day);
  if (cap) return reply(429, { error: cap });

  let issue;
  try {
    issue = await createIssue(env, report, fetchFn, now);
  } catch (e) {
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

// count adds a report to today's counters, or names the cap it would pass ("ip_daily",
// "daily"). KV is eventually consistent, so a burst can pass a cap by a few; PER_MINUTE
// stops bursts from one sender.
export async function count(env, who, day) {
  const perSender = limit(env.PER_IP_DAY, 5);
  const perDay = limit(env.PER_DAY, 30);
  const mine = `ip:${who}:${day}`;
  const all = `all:${day}`;
  const [a, b] = await Promise.all([env.COUNTS.get(mine), env.COUNTS.get(all)]);
  const n = parseInt(a || "0", 10) || 0;
  const total = parseInt(b || "0", 10) || 0;
  if (total >= perDay) return "daily";
  if (n >= perSender) return "ip_daily";
  const ttl = { expirationTtl: 2 * 86400 };
  await Promise.all([env.COUNTS.put(mine, String(n + 1), ttl), env.COUNTS.put(all, String(total + 1), ttl)]).catch(() => {});
  return "";
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
