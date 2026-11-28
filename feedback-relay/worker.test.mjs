// The feedback relay without Cloudflare or GitHub: node --test "feedback-relay/*.test.mjs".
// A fake GitHub answers the three calls, a Map stands in for KV, and the real SendGate class
// runs behind a fake Durable Object namespace with Map storage.
import test from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, createVerify } from "node:crypto";
import { handle, clean, cleanTimings, defuse, appJWT, pkcs8, forgetToken, LIMITS, SendGate } from "./worker.mjs";

const keys = generateKeyPairSync("rsa", {
  modulusLength: 2048,
  privateKeyEncoding: { type: "pkcs1", format: "pem" },
  publicKeyEncoding: { type: "spki", format: "pem" },
});

const NOW = Date.parse("2026-10-31T12:00:00Z");

function kv() {
  const m = new Map();
  return { m, get: async (k) => m.get(k) ?? null, put: async (k, v) => void m.set(k, v) };
}

// gates is a Durable Object namespace: one SendGate per name, each with its own storage.
function gates() {
  const objects = new Map();
  const storage = () => {
    const m = new Map();
    return {
      m,
      alarm: 0,
      get: async (k) => m.get(k),
      put: async (k, v) => void m.set(k, structuredClone(v)),
      setAlarm: async function (t) { this.alarm = t; },
      deleteAll: async () => m.clear(),
    };
  };
  return {
    objects,
    idFromName: (name) => name,
    get: (id) => {
      if (!objects.has(id)) objects.set(id, new SendGate({ storage: storage() }));
      const o = objects.get(id);
      return { fetch: (url, init) => o.fetch(new Request(url, init)) };
    },
  };
}

function env(over = {}) {
  return { APP_ID: "123456", GITHUB_APP_KEY: keys.privateKey, REPO: "ToumanLin/RimArcho", LABEL: "feedback", COUNTS: kv(), GATE: gates(), ...over };
}

function github({ labelFails = false, issueStatus = 201 } = {}) {
  const calls = [];
  let number = 40;
  const fn = async (url, init) => {
    const body = init.body ? JSON.parse(init.body) : null;
    calls.push({ url, method: init.method, auth: init.headers.authorization, body });
    const json = (status, data) => new Response(JSON.stringify(data), { status });
    if (url.endsWith("/repos/ToumanLin/RimArcho/installation")) return json(200, { id: 77 });
    if (url.endsWith("/app/installations/77/access_tokens")) return json(201, { token: "ghs_test", expires_at: new Date(NOW + 3600e3).toISOString() });
    if (url.endsWith("/repos/ToumanLin/RimArcho/issues")) {
      if (labelFails && body.labels) return json(422, { message: "Validation Failed" });
      if (issueStatus !== 201) return json(issueStatus, { message: "nope" });
      number++;
      return json(201, { number, html_url: `https://github.com/ToumanLin/RimArcho/issues/${number}` });
    }
    return json(404, {});
  };
  return { fn, calls };
}

function post(data, { ip = "203.0.113.9", agent = "RimArcho/0.9.0", method = "POST", path = "/report" } = {}) {
  return new Request("https://relay.example" + path, {
    method,
    headers: { "user-agent": agent, "cf-connecting-ip": ip, "content-type": "application/json" },
    body: method === "POST" ? (typeof data === "string" ? data : JSON.stringify(data)) : undefined,
  });
}

async function send(e, gh, data, opts = {}) {
  const at = opts.at ?? NOW;
  const res = await handle(post(data, opts), e, { fetch: gh.fn, now: () => at });
  return { status: res.status, data: await res.json() };
}

test("a report becomes an issue; the IP and the contact stay out of it", async () => {
  forgetToken();
  const e = env(), gh = github();
  const r = await send(e, gh, { title: "Crash on load", body: "It crashed @someone\n\n---\nRimArcho 0.9.0", contact: "me#1234" });
  assert.equal(r.status, 200);
  assert.equal(r.data.number, 41);
  assert.equal(r.data.url, "https://github.com/ToumanLin/RimArcho/issues/41");
  assert.match(r.data.token, /^[A-Za-z0-9_-]{32}$/);

  const issue = gh.calls.find((c) => c.url.endsWith("/issues"));
  assert.equal(issue.auth, "Bearer ghs_test");
  assert.equal(issue.body.title, "Crash on load");
  assert.deepEqual(issue.body.labels, ["feedback"]);
  assert.match(issue.body.body, /^It crashed @​someone\n\n---\nRimArcho 0\.9\.0\n\n---\n<sub>Sent from RimArcho/);
  assert.match(issue.body.body, /left a contact/);
  const sent = JSON.stringify(gh.calls);
  assert.ok(!sent.includes("203.0.113.9") && !sent.includes("me#1234"), "no IP or contact goes to GitHub");
  assert.ok(![...e.COUNTS.m.keys(), ...e.COUNTS.m.values()].some((s) => s.includes("203.0.113.9")), "KV holds only a hash of the IP");
  assert.equal(e.COUNTS.m.get("contact:41"), "me#1234");
  assert.ok(!sent.includes(r.data.token), "the reply token stays out of GitHub");
  assert.ok(![...e.COUNTS.m.values()].includes(r.data.token), "KV holds only the token's keyed hash");
  assert.ok(e.COUNTS.m.has("reply:41"));

  // the App's JWT signs the two token calls and verifies with the App's public key
  const [jwt] = gh.calls.filter((c) => !c.url.endsWith("/issues")).map((c) => c.auth.slice(7));
  const [h, p, s] = jwt.split(".");
  const v = createVerify("RSA-SHA256");
  v.update(h + "." + p);
  assert.ok(v.verify(keys.publicKey, Buffer.from(s, "base64url")));
  assert.deepEqual(JSON.parse(Buffer.from(p, "base64url")), { iat: NOW / 1000 - 60, exp: NOW / 1000 + 540, iss: 123456 });
});

test("the installation token is reused", async () => {
  forgetToken();
  const e = env(), gh = github();
  assert.equal((await send(e, gh, { title: "a", body: "x" }, { ip: "198.51.100.1" })).status, 200);
  assert.equal((await send(e, gh, { title: "b", body: "y" }, { ip: "198.51.100.2" })).status, 200);
  assert.equal(gh.calls.filter((c) => c.url.includes("access_tokens")).length, 1);
});

test("one report a minute, a daily cap per sender and for everyone", async () => {
  forgetToken();
  const slow = { status: 429, data: { error: "slow_down" } };
  let e = env(), gh = github();
  assert.equal((await send(e, gh, { title: "1", body: "x" })).status, 200);
  // the 1.0.6 VM round's case: the same sender 13 s later
  assert.deepEqual(await send(e, gh, { title: "2", body: "x" }, { at: NOW + 13e3 }), slow);
  assert.deepEqual(await send(e, gh, { title: "3", body: "x" }, { at: NOW + 59999 }), slow);
  assert.equal((await send(e, gh, { title: "4", body: "x" }, { at: NOW + 60e3 })).status, 200, "a minute later");
  assert.equal((await send(e, gh, { title: "5", body: "x" }, { ip: "198.51.100.7", at: NOW + 61e3 })).status, 200, "another sender is not held up");
  assert.equal(gh.calls.filter((c) => c.url.endsWith("/issues")).length, 3, "refused reports file nothing");

  // per sender: PER_IP_DAY stops the third, a minute apart each
  e = env({ PER_IP_DAY: "2" });
  for (const i of [0, 1]) assert.equal((await send(e, gh, { title: String(i), body: "x" }, { at: NOW + i * 60e3 })).status, 200);
  assert.deepEqual(await send(e, gh, { title: "3", body: "x" }, { at: NOW + 120e3 }), { status: 429, data: { error: "ip_daily" } });
  assert.equal((await send(e, gh, { title: "4", body: "x" }, { ip: "198.51.100.7", at: NOW + 120e3 })).status, 200, "another sender still can");

  // everyone: PER_DAY across senders; a refused report does not use the sender's minute
  e = env({ PER_DAY: "2" });
  assert.equal((await send(e, gh, { title: "1", body: "x" }, { ip: "192.0.2.1" })).status, 200);
  assert.equal((await send(e, gh, { title: "2", body: "x" }, { ip: "192.0.2.2" })).status, 200);
  assert.deepEqual(await send(e, gh, { title: "3", body: "x" }, { ip: "192.0.2.3" }), { status: 429, data: { error: "daily" } });
  const [gate3] = [...e.GATE.objects.values()].slice(-1);
  assert.deepEqual(gate3.storage.m.get("s"), { last: 0, prev: 0, n: 0 });

  // the KV counter is per day; the gates clean up two days after their first report
  assert.deepEqual([...e.COUNTS.m.keys()].filter((k) => !k.startsWith("reply:")), ["all:2026-10-31"]);
  const [gate1] = e.GATE.objects.values();
  assert.equal(gate1.storage.alarm, NOW + 2 * 86400e3);
  await gate1.alarm();
  assert.equal(gate1.storage.m.size, 0);
});

test("a GitHub failure gives the sender's minute and the daily count back", async () => {
  forgetToken();
  const e = env();
  assert.equal((await send(e, github({ issueStatus: 500 }), { title: "1", body: "x" })).status, 502);
  assert.equal(e.COUNTS.m.get("all:2026-10-31"), "0", "nothing filed, nothing counted");
  assert.equal((await send(e, github(), { title: "1", body: "x" }, { at: NOW + 5e3 })).status, 200, "sent again at once");
  assert.deepEqual(await send(e, github(), { title: "2", body: "x" }, { at: NOW + 10e3 }), { status: 429, data: { error: "slow_down" } });
  assert.equal(e.COUNTS.m.get("all:2026-10-31"), "1");

  // PER_DAY 1: failures from other senders do not use up everyone's cap
  const one = env({ PER_DAY: "1" });
  for (const ip of ["192.0.2.1", "192.0.2.2"]) assert.equal((await send(one, github({ issueStatus: 500 }), { title: "1", body: "x" }, { ip })).status, 502);
  assert.equal((await send(one, github(), { title: "1", body: "x" }, { ip: "192.0.2.3" })).status, 200);
});

test("refusals: not configured, other clients, wrong method, bad or empty or long reports", async () => {
  forgetToken();
  const gh = github();
  assert.deepEqual(await send(env({ GITHUB_APP_KEY: "" }), gh, { title: "a" }), { status: 503, data: { error: "not_configured" } });
  assert.deepEqual(await send(env(), gh, { title: "a" }, { agent: "curl/8" }), { status: 403, data: { error: "client" } });
  assert.equal((await handle(post(null, { method: "GET" }), env(), { fetch: gh.fn, now: () => NOW })).status, 405);
  assert.equal((await handle(post({}, { path: "/" }), env(), { fetch: gh.fn, now: () => NOW })).status, 404);
  assert.deepEqual(await send(env(), gh, "{nope"), { status: 400, data: { error: "bad_json" } });
  assert.deepEqual(await send(env(), gh, { title: " ", body: "\n" }), { status: 400, data: { error: "empty" } });
  assert.deepEqual(await send(env(), gh, { title: "a", body: "x".repeat(LIMITS.body + 1) }), { status: 413, data: { error: "too_long" } });
  assert.deepEqual(await send(env(), gh, { title: "a", contact: "c".repeat(LIMITS.contact + 1) }), { status: 413, data: { error: "too_long" } });
  assert.equal(gh.calls.length, 0, "nothing reached GitHub");
});

test("a label GitHub refuses is dropped; a GitHub failure is 502", async () => {
  forgetToken();
  let gh = github({ labelFails: true });
  const r = await send(env(), gh, { title: "a", body: "x" });
  assert.equal(r.status, 200);
  const tries = gh.calls.filter((c) => c.url.endsWith("/issues"));
  assert.equal(tries.length, 2);
  assert.equal(tries[1].body.labels, undefined);

  gh = github({ issueStatus: 500 });
  const f = await send(env(), gh, { title: "a", body: "x" }, { ip: "198.51.100.99" });
  assert.equal(f.status, 502);
  assert.equal(f.data.error, "github");
});

test("clean: a title from the body, long titles cut, line breaks folded", () => {
  assert.equal(clean({ body: "---\nRimArcho 0.9.0" }).title, "RimArcho 0.9.0");
  assert.equal(clean({ body: "Mods vanish\nafter saving" }).title, "Mods vanish");
  assert.equal(clean({ title: "a\nb", body: "" }).title, "a b");
  const long = clean({ title: "L".repeat(300) }).title;
  assert.equal(Array.from(long).length, LIMITS.title);
  assert.ok(long.endsWith("…"));
});

test("defuse: mentions and cross-repository references stop notifying", () => {
  assert.equal(defuse("@alice and (@bob) mail a@b.com `@code`"), "@​alice and (@​bob) mail a@b.com `@code`");
  assert.equal(defuse("see other/repo#12 and #3"), "see other/repo#​12 and #3");
});

test("the downloaded PKCS#1 key, a PKCS#8 key and a one-line key all sign", async () => {
  const p8 = generateKeyPairSync("rsa", { modulusLength: 2048, privateKeyEncoding: { type: "pkcs8", format: "pem" }, publicKeyEncoding: { type: "spki", format: "pem" } });
  for (const [pem, pub] of [[keys.privateKey, keys.publicKey], [p8.privateKey, p8.publicKey], [keys.privateKey.replace(/\n/g, "\\n"), keys.publicKey]]) {
    const jwt = await appJWT("Iv23liTEST", pem, NOW);
    const [h, p, s] = jwt.split(".");
    const v = createVerify("RSA-SHA256");
    v.update(h + "." + p);
    assert.ok(v.verify(pub, Buffer.from(s, "base64url")));
    assert.equal(JSON.parse(Buffer.from(p, "base64url")).iss, "Iv23liTEST");
  }
  assert.equal(pkcs8(keys.privateKey)[0], 0x30);
});

// statusGitHub adds the issue and its comments to the fake GitHub.
function statusGitHub({ state = "open", comments = [], issueStatus = 200 } = {}) {
  const base = github();
  const fn = async (url, init) => {
    if (/\/issues\/41$/.test(url)) {
      base.calls.push({ url, method: init.method });
      return new Response(JSON.stringify({ number: 41, state, closed_at: state === "closed" ? "2026-11-01T00:00:00Z" : null }), { status: issueStatus });
    }
    if (/\/issues\/41\/comments/.test(url)) {
      base.calls.push({ url, method: init.method });
      return new Response(JSON.stringify(comments), { status: 200 });
    }
    return base.fn(url, init);
  };
  return { fn, calls: base.calls };
}

async function ask(e, gh, n, t, { ip = "203.0.113.9", agent = "RimArcho/1.1.0", at = NOW } = {}) {
  const req = new Request(`https://relay.example/status/${n}?t=${encodeURIComponent(t)}`, { headers: { "user-agent": agent, "cf-connecting-ip": ip } });
  const res = await handle(req, e, { fetch: gh.fn, now: () => at });
  return { status: res.status, data: await res.json() };
}

test("status: the sender reads the state and only the owner's comments", async () => {
  forgetToken();
  const comments = [
    { body: "Thanks, fixed in 1.1.1", created_at: "2026-11-02T00:00:00Z", author_association: "OWNER", user: { login: "x" } },
    { body: "me too", created_at: "2026-11-02T01:00:00Z", author_association: "NONE" },
    { body: "<b>collab</b>", created_at: "2026-11-02T02:00:00Z", author_association: "COLLABORATOR" },
  ];
  const e = env(), gh = statusGitHub({ state: "closed", comments });
  const { data } = await send(e, gh, { title: "a", body: "x" });
  const r = await ask(e, gh, 41, data.token);
  assert.equal(r.status, 200);
  assert.deepEqual(r.data, { state: "closed", closed_at: "2026-11-01T00:00:00Z", replies: [{ body: "Thanks, fixed in 1.1.1", created_at: "2026-11-02T00:00:00Z" }] });
});

test("status: a wrong token is 404 like a missing issue, and asks nothing of GitHub", async () => {
  forgetToken();
  const e = env(), gh = statusGitHub();
  const { data } = await send(e, gh, { title: "a", body: "x" });
  const before = gh.calls.length;
  const wrong = { status: 404, data: { error: "not_found" } };
  assert.deepEqual(await ask(e, gh, 41, "A".repeat(32)), wrong);
  assert.deepEqual(await ask(e, gh, 42, data.token), wrong, "another issue's number");
  assert.deepEqual(await ask(e, gh, 41, ""), wrong);
  assert.deepEqual(await ask(e, gh, "abc", data.token), wrong);
  assert.equal(gh.calls.length, before);
  assert.deepEqual(await ask(e, gh, 41, data.token, { agent: "curl/8" }), { status: 403, data: { error: "client" } });
  // GitHub lost the issue (deleted): 404 too
  const gone = statusGitHub({ issueStatus: 404 });
  assert.deepEqual(await ask(e, gone, 41, data.token), wrong);
});

test("status: a per-sender hourly limit, looser than reports", async () => {
  forgetToken();
  const e = env({ STATUS_PER_HOUR: "3" }), gh = statusGitHub();
  const { data } = await send(e, gh, { title: "a", body: "x" });
  for (let i = 0; i < 3; i++) assert.equal((await ask(e, gh, 41, data.token)).status, 200);
  assert.deepEqual(await ask(e, gh, 41, data.token), { status: 429, data: { error: "slow_down" } });
  assert.equal((await ask(e, gh, 41, data.token, { ip: "198.51.100.5" })).status, 200, "another sender");
  assert.equal((await ask(e, gh, 41, data.token, { at: NOW + 3600e3 })).status, 200, "the next hour");
});

// a GitHub with open feature requests, comments and labels that may not exist
function hub({ missing = [], failAfter = Infinity } = {}) {
  const calls = [];
  let number = 40, comment = 900, made = 0;
  const issues = [
    { number: 7, title: "Sort the list by size", html_url: "https://github.com/ToumanLin/RimArcho/issues/7", reactions: { "+1": 3 } },
    { number: 8, title: "A pull request", html_url: "x", pull_request: {} },
    { number: 9, title: "Export as a collection", html_url: "https://github.com/ToumanLin/RimArcho/issues/9", reactions: {} },
  ];
  const fn = async (url, init) => {
    const body = init.body ? JSON.parse(init.body) : null;
    calls.push({ url, method: init.method, body });
    const json = (status, data) => new Response(JSON.stringify(data), { status });
    if (url.endsWith("/installation")) return json(200, { id: 77 });
    if (url.endsWith("/access_tokens")) return json(201, { token: "ghs_test", expires_at: new Date(NOW + 3600e3).toISOString() });
    if (url.endsWith("/repos/ToumanLin/RimArcho/issues") && init.method === "POST") {
      if ((body.labels || []).some((l) => missing.includes(l))) return json(422, { message: "Validation Failed" });
      if (made >= failAfter) return json(500, {});
      made++;
      number++;
      return json(201, { number, html_url: `https://github.com/ToumanLin/RimArcho/issues/${number}` });
    }
    if (url.includes("/issues?state=open&labels=enhancement")) return json(200, issues);
    if (/\/issues\/\d+\/comments$/.test(url) && init.method === "POST") return json(201, { id: ++comment });
    if (/\/issues\/comments\/\d+$/.test(url) && init.method === "PATCH") return json(200, {});
    return json(404, {});
  };
  return { fn, calls };
}

const created = (gh) => gh.calls.filter((c) => c.method === "POST" && c.url.endsWith("/issues")).map((c) => c.body.labels);

test("a report's kind becomes a label, files lost adds priority; missing labels fall back step by step", async () => {
  forgetToken();
  let gh = hub();
  assert.equal((await send(env(), gh, { title: "a", body: "x", kind: "bug", priority: true })).status, 200);
  assert.deepEqual(created(gh), [["feedback", "bug", "priority"]]);
  gh = hub();
  await send(env(), gh, { title: "a", body: "x", kind: "feature", priority: true }, { ip: "192.0.2.50" });
  assert.deepEqual(created(gh), [["feedback", "enhancement"]], "priority is for bugs only");
  gh = hub();
  await send(env(), gh, { title: "a", body: "x", kind: "made-up" }, { ip: "192.0.2.51" });
  assert.deepEqual(created(gh), [["feedback"]], "an unknown kind adds nothing");
  gh = hub();
  await send(env(), gh, { title: "a", body: "x", kind: "l10n", priority: true }, { ip: "192.0.2.58" });
  assert.deepEqual(created(gh), [["feedback", "localization"]], "a translation fix is labelled localization");
  // localization does not exist yet: LABEL still goes on
  gh = hub({ missing: ["localization"] });
  await send(env(), gh, { title: "a", body: "x", kind: "l10n" }, { ip: "192.0.2.59" });
  assert.deepEqual(created(gh), [["feedback", "localization"], ["feedback"]]);

  // diagnosis does not exist yet: LABEL still goes on
  gh = hub({ missing: ["diagnosis", "priority"] });
  assert.equal((await send(env(), gh, { title: "a", body: "x", kind: "diagnosis" }, { ip: "192.0.2.52" })).status, 200);
  assert.deepEqual(created(gh), [["feedback", "diagnosis"], ["feedback"]]);
  // a bug keeps GitHub's own label when priority is missing
  gh = hub({ missing: ["priority"] });
  assert.equal((await send(env(), gh, { title: "a", body: "x", kind: "bug", priority: true }, { ip: "192.0.2.54" })).status, 200);
  assert.deepEqual(created(gh), [["feedback", "bug", "priority"], ["feedback", "bug"]]);
  gh = hub({ missing: ["feedback", "data"] });
  assert.equal((await send(env(), gh, { title: "a", body: "x", kind: "data" }, { ip: "192.0.2.53" })).status, 200);
  assert.deepEqual(created(gh), [["feedback", "data"], ["feedback"], undefined]);
});

test("a split send files each problem as its own issue: one minute, each counts for the day", async () => {
  forgetToken();
  const e = env({ PER_IP_DAY: "3" }), gh = hub();
  const r = await send(e, gh, { reports: [{ title: "one", body: "x", kind: "bug" }, { title: "two", body: "y", kind: "feature" }], contact: "me#1" });
  assert.equal(r.status, 200);
  assert.deepEqual(r.data.issues.map((i) => i.number), [41, 42]);
  assert.equal(r.data.number, 41, "the first one also at the top, for older RimArcho");
  assert.ok(r.data.issues.every((i) => /^[A-Za-z0-9_-]{32}$/.test(i.token)) && r.data.issues[0].token !== r.data.issues[1].token);
  assert.deepEqual(created(gh), [["feedback", "bug"], ["feedback", "enhancement"]]);
  assert.equal(e.COUNTS.m.get("contact:41"), "me#1");
  assert.equal(e.COUNTS.m.get("contact:42"), "me#1");
  assert.equal(e.COUNTS.m.get("all:2026-10-31"), "2");
  // two of three used: a split of two more is over the sender's day, one more is not
  assert.deepEqual(await send(e, gh, { reports: [{ title: "a", body: "x" }, { title: "b", body: "x" }] }, { at: NOW + 61e3 }), { status: 429, data: { error: "ip_daily" } });
  assert.equal((await send(e, gh, { title: "c", body: "x" }, { at: NOW + 62e3 })).status, 200);

  // too many, none, or one empty: refused whole
  for (const reports of [[1, 2, 3, 4].map((i) => ({ title: String(i), body: "x" })), []]) {
    assert.deepEqual(await send(env(), hub(), { reports }, { ip: "192.0.2.60" }), { status: 400, data: { error: "bad_json" } });
  }
  assert.deepEqual(await send(env(), hub(), { reports: [{ title: "a", body: "x" }, { title: "", body: "" }] }, { ip: "192.0.2.61" }), { status: 400, data: { error: "empty" } });
});

test("a split send GitHub fails partway returns what was filed and gives the rest back", async () => {
  forgetToken();
  const e = env({ PER_IP_DAY: "3" });
  const r = await send(e, hub({ failAfter: 1 }), { reports: [{ title: "one", body: "x" }, { title: "two", body: "y" }] });
  assert.equal(r.status, 502);
  assert.equal(r.data.error, "github");
  assert.deepEqual(r.data.issues.map((i) => i.number), [41]);
  assert.equal(e.COUNTS.m.get("all:2026-10-31"), "1");
  const [gate] = e.GATE.objects.values();
  assert.equal(gate.storage.m.get("s").n, 1, "only the filed one counts for the sender");
  assert.deepEqual(await send(e, hub(), { title: "two", body: "y" }, { at: NOW + 5e3 }), { status: 429, data: { error: "slow_down" } }, "something was filed: the minute stays");
});

test("requests: the open feature requests with their +1s, no pull requests, kept ten minutes", async () => {
  forgetToken();
  const e = env(), gh = hub();
  const get = () => handle(post(null, { method: "GET", path: "/requests" }), e, { fetch: gh.fn, now: () => NOW }).then(async (r) => ({ status: r.status, data: await r.json() }));
  const r = await get();
  assert.equal(r.status, 200);
  assert.deepEqual(r.data.requests, [
    { number: 7, title: "Sort the list by size", url: "https://github.com/ToumanLin/RimArcho/issues/7", plus: 3 },
    { number: 9, title: "Export as a collection", url: "https://github.com/ToumanLin/RimArcho/issues/9", plus: 0 },
  ]);
  await get();
  assert.equal(gh.calls.filter((c) => c.url.includes("labels=enhancement")).length, 1, "the second read comes from KV");
  const other = await handle(post(null, { method: "GET", path: "/requests", agent: "curl/8" }), e, { fetch: gh.fn, now: () => NOW });
  assert.equal(other.status, 403);
});

test("plus: a +1 for an open request, once a day per sender, written in one App comment", async () => {
  forgetToken();
  const e = env(), gh = hub();
  const plus = (n, opts = {}) => handle(post({}, { path: "/plus/" + n, ...opts }), e, { fetch: gh.fn, now: () => NOW }).then(async (r) => ({ status: r.status, data: await r.json() }));
  assert.deepEqual(await plus(7), { status: 200, data: { plus: 4 } });
  assert.deepEqual(await plus(7), { status: 200, data: { plus: 4 } }, "the same sender again: counted once");
  assert.deepEqual(await plus(7, { ip: "192.0.2.70" }), { status: 200, data: { plus: 5 } });
  assert.equal(e.COUNTS.m.get("plus:7"), "2");
  assert.equal(gh.calls.filter((c) => c.url.endsWith("/issues/7/comments")).length, 1);
  const patches = gh.calls.filter((c) => c.method === "PATCH");
  assert.equal(patches.length, 1);
  assert.match(patches[0].body.body, /^\+2 from RimArcho players/);
  assert.ok(!JSON.stringify(gh.calls).includes("192.0.2.70"), "no IP reaches GitHub");
  assert.deepEqual(await plus(8), { status: 404, data: { error: "not_found" } }, "a pull request is not a request");
  assert.deepEqual(await plus(12345), { status: 404, data: { error: "not_found" } });
  // the list read afterwards includes the relayed +1s
  const r = await handle(post(null, { method: "GET", path: "/requests" }), e, { fetch: gh.fn, now: () => NOW });
  assert.equal((await r.json()).requests[0].plus, 5);

  // PLUS_PER_DAY a day per sender
  const many = env();
  await many.COUNTS.put("plusday:" + "seed", "0");
  const gh2 = hub();
  // 20 distinct +1s use the day up; issues are only 7 and 9, so count them by forging the counter
  const r1 = await handle(post({}, { path: "/plus/7", ip: "192.0.2.80" }), many, { fetch: gh2.fn, now: () => NOW });
  assert.equal(r1.status, 200);
  const key = [...many.COUNTS.m.keys()].find((k) => k.startsWith("plusday:") && k !== "plusday:seed");
  await many.COUNTS.put(key, "20");
  const r2 = await handle(post({}, { path: "/plus/9", ip: "192.0.2.80" }), many, { fetch: gh2.fn, now: () => NOW });
  assert.deepEqual({ status: r2.status, data: await r2.json() }, { status: 429, data: { error: "slow_down" } });
});

// commentHub is a fake GitHub that takes comments on any issue.
function commentHub(status = 201) {
  const calls = [];
  const fn = async (url, init) => {
    const body = init.body ? JSON.parse(init.body) : null;
    calls.push({ url, method: init.method, body });
    const json = (s, data) => new Response(JSON.stringify(data), { status: s });
    if (url.endsWith("/repos/ToumanLin/RimArcho/installation")) return json(200, { id: 77 });
    if (url.endsWith("/app/installations/77/access_tokens")) return json(201, { token: "ghs_test", expires_at: new Date(NOW + 3600e3).toISOString() });
    if (/\/issues\/\d+\/comments$/.test(url) && init.method === "POST") return json(status, { id: 900 });
    return json(404, {});
  };
  return { fn, calls };
}

const TIMINGS = {
  version: "1.1.4", os: "Windows 11 Pro 25H2 (26200.6899)", game: "1.6", active_mods: 312, total_mods: 1354, automatic: false, wall_ms: 48200,
  phases: [{ name: "read", ms: 900 }, { name: "defs", ms: 20100 }, { name: "patches", ms: 19000 }, { name: "harmony", ms: 4100 }, { name: "other", ms: 4100 }],
  cpu_threads: 8, ram_gb: 16, disk: "hdd",
};

async function sendTimings(e, gh, data, opts = {}) {
  const res = await handle(post(data, { path: "/timings", ...opts }), e, { fetch: gh.fn, now: () => opts.at ?? NOW });
  return { status: res.status, data: await res.json() };
}

test("timings: a slow check's timings become a comment on the one fixed issue", async () => {
  forgetToken();
  const e = env({ TIMINGS_ISSUE: "50" }), gh = commentHub();
  const r = await sendTimings(e, gh, { ...TIMINGS, mods: ["Combat Extended"], path: "C:\Users\me\Mods", steam_id: "7656119" });
  assert.deepEqual(r, { status: 200, data: { ok: true } });
  const c = gh.calls.filter((x) => x.url.endsWith("/repos/ToumanLin/RimArcho/issues/50/comments"));
  assert.equal(c.length, 1);
  const body = c[0].body.body;
  assert.match(body, /^\*\*48\.2 s\*\*: 312\/1354 Mods, game 1\.6, RimArcho 1\.1\.4; slowest: defs 20\.1 s, patches 19\.0 s\n/);
  assert.ok(body.includes('"wall_ms": 48200') && body.includes('"disk": "hdd"'));
  assert.ok(body.includes('"phases": {"read":900,"defs":20100,"patches":19000,"harmony":4100,"other":4100}'), "1.1.5's phase array becomes one object");
  for (const s of ["Combat Extended", "Users", "7656119", "203.0.113.9"]) assert.ok(!JSON.stringify(gh.calls).includes(s), s + " stays out of GitHub");
  assert.equal(gh.calls.filter((x) => x.url.endsWith("/issues")).length, 0, "no new issue");
});

// TIMINGS_DETAIL is 1.1.6's shape: phases as one object, the run, the slowest operations
// and DLLs by packageId and Workshop id, the CPU.
const TIMINGS_DETAIL = {
  version: "1.1.6", os: "Windows 11 Pro 25H2 (26200.6899)", game: "1.7", active_mods: 572, total_mods: 573, automatic: false, priority: "normal", wall_ms: 75400,
  phases: { spawn: 90, boot: 140, open: 60, read: 33, defs: 1000, patches: 43800, harmony: 2900, cache: 2, save: 507, refs: 11, exit: 40, lock: 26000, ui: 20, other: 800 },
  run: { cold: true, first: true, list: "changed", mods_read: 572, mods_cached: 1, workers: 8, gc_ms: 9000, peak_heap_mb: 290 },
  patches: {
    ops: 7495, nodes: 2100000, too_slow: 2, parse_ms: 3000, sim_ms: 38000, facts_ms: 2800,
    slowest: [
      { ms: 12100.4, pkg: "some.mod", ws: "2009463077", class: "PatchOperationReplace", xpath: 'Defs/ThingDef[@Name="BaseX"]/comps', file: "1.6/Patches/A.xml" },
      { ms: 900, pkg: "other.mod", class: "XmlExtensions.PatchOperationSafeAdd", xpath: "Defs/*", file: "Patches/65b030573044/B.xml", too_slow: true },
    ],
  },
  harmony: { dlls: 340, cached: 300, patches: 4100, methods: 3600, slowest: [{ ms: 800, pkg: "some.mod", ws: "2009463077", file: "1.6/Assemblies/X.dll" }] },
  cpu: "AMD Ryzen 7 5800H with Radeon Graphics", cpu_threads: 16, ram_gb: 32, disk: "ssd",
};

test("timings: 1.1.6's detail is kept, bounded, and named in the summary line", async () => {
  forgetToken();
  const e = env({ TIMINGS_ISSUE: "50" }), gh = commentHub();
  assert.deepEqual(await sendTimings(e, gh, TIMINGS_DETAIL), { status: 200, data: { ok: true } });
  const body = gh.calls.find((x) => x.url.endsWith("/issues/50/comments")).body.body;
  assert.match(body, /^\*\*75\.4 s\*\* \(cold\): 572\/573 Mods, game 1\.7, RimArcho 1\.1\.6; slowest: patches 43\.8 s \(slowest: some\.mod 12\.1 s PatchOperationReplace\), lock 26\.0 s\n/);
  assert.deepEqual(JSON.parse(body.split("```json\n")[1].split("\n```")[0]), JSON.parse(JSON.stringify(cleanTimings(TIMINGS_DETAIL))));
  assert.ok(body.includes('\n      {"ms":12100.4,"pkg":"some.mod","ws":"2009463077","class":"PatchOperationReplace","xpath":"Defs/ThingDef[@Name=\\"BaseX\\"]/comps","file":"1.6/Patches/A.xml"},\n'), "one operation to a line");
  const t = cleanTimings(TIMINGS_DETAIL);
  assert.equal(t.cpu, "AMD Ryzen 7 5800H with Radeon Graphics");
  assert.deepEqual(Object.keys(t), ["version", "os", "game", "active_mods", "total_mods", "automatic", "priority", "wall_ms", "phases", "run", "patches", "harmony", "cpu", "cpu_threads", "ram_gb", "disk"]);
  assert.equal(t.patches.slowest[1].file, "Patches/65b030573044/B.xml");
  assert.equal(t.patches.slowest[1].too_slow, true);
});

test("timings: bounds on every new field; paths and accounts never pass", () => {
  const long = "x".repeat(1000);
  const t = cleanTimings({
    ...TIMINGS_DETAIL,
    phases: Object.fromEntries([...Array(30)].map((_, i) => ["p" + String.fromCharCode(97 + (i % 26)) + "x".repeat(i >> 4), i])),
    run: { ...TIMINGS_DETAIL.run, list: long, workers: -1, gc_ms: 1e12, user: "me", steam_id: "7656119" },
    patches: {
      ops: 1.5, nodes: 3, too_slow: "2",
      slowest: [...Array(15)].map((_, i) => ({ ms: 15 - i, pkg: "p" + i, ws: "12a", class: "Bad Class!", xpath: "Defs/" + long, file: "Patches/" + i + ".xml" }))
        .concat([{ ms: 1, pkg: "x", file: "C:\\Users\\me\\Mods\\A.xml", xpath: 'Defs/X[@a="D:\\Games"]' }]),
    },
    harmony: { dlls: 1, slowest: [
      { ms: 5, pkg: "a", file: "../../Users/me/x.dll" }, { ms: 4, pkg: "b", file: "/home/me/x.dll" }, { ms: 3, pkg: "c", file: "\\\\server\\x.dll" },
      { ms: 2, pkg: "Bad Pkg!", file: "x.dll" }, { ms: 1, pkg: "d", file: "Assemblies\\D.dll" }, { ms: 0.5, pkg: "e", file: "E.dll" }, { ms: 0.1, pkg: "f", file: "F.dll" },
    ] },
    mods: ["Combat Extended"], path: "C:\\Users\\me",
  });
  assert.equal(Object.keys(t.phases).length, 20, "at most 20 phases");
  assert.equal(t.run.list.length, 80);
  assert.equal(t.run.workers, undefined);
  assert.equal(t.run.gc_ms, undefined);
  assert.equal(t.run.user, undefined);
  assert.equal(t.patches.ops, undefined);
  assert.equal(t.patches.too_slow, undefined);
  assert.equal(t.patches.slowest.length, 10, "at most 10 operations");
  assert.equal(t.patches.slowest[0].ws, undefined);
  assert.equal(t.patches.slowest[0].class, undefined);
  assert.equal([...t.patches.slowest[0].xpath].length, 300);
  assert.deepEqual(t.harmony.slowest.map((d) => [d.pkg, d.file]), [["a", undefined], ["b", undefined], ["c", undefined], ["d", "Assemblies/D.dll"], ["e", "E.dll"]], "at most 5 DLLs; paths outside the Mod dropped");
  const s = JSON.stringify(t);
  for (const banned of ["Users", "me\\", "7656119", "Combat Extended", ":\\", "home/"]) assert.ok(!s.includes(banned), banned);
});

test("timings: no issue set is 404, which RimArcho takes quietly; bad input is refused", async () => {
  forgetToken();
  const gh = commentHub();
  assert.deepEqual(await sendTimings(env(), gh, TIMINGS), { status: 404, data: { error: "not_found" } });
  assert.equal(gh.calls.length, 0);
  const e = env({ TIMINGS_ISSUE: "50" });
  assert.equal((await sendTimings(e, gh, TIMINGS, { agent: "curl/8" })).status, 403);
  assert.deepEqual(await sendTimings(e, gh, "{"), { status: 400, data: { error: "bad_json" } });
  assert.deepEqual(await sendTimings(e, gh, { version: "1" }), { status: 400, data: { error: "empty" } });
  assert.deepEqual(await sendTimings(e, gh, { ...TIMINGS, phases: [{ name: "Mod Name!", ms: 1 }] }), { status: 400, data: { error: "empty" } });
  assert.equal((await sendTimings(e, gh, "x".repeat(17000))).status, 413);
  assert.equal(gh.calls.length, 0);
});

test("timings: one a minute and a daily cap per sender, apart from reports; a cap for everyone", async () => {
  forgetToken();
  const slow = { status: 429, data: { error: "slow_down" } };
  let e = env({ TIMINGS_ISSUE: "50" }), gh = commentHub();
  assert.equal((await sendTimings(e, gh, TIMINGS)).status, 200);
  assert.deepEqual(await sendTimings(e, gh, TIMINGS, { at: NOW + 30e3 }), slow);
  assert.equal((await sendTimings(e, gh, TIMINGS, { at: NOW + 60e3 })).status, 200);
  assert.deepEqual(await sendTimings(e, gh, TIMINGS, { at: NOW + 120e3 }), { status: 429, data: { error: "ip_daily" } }, "TIMINGS_PER_IP_DAY is 2");
  // a report from the same sender right after is not held up by the timings
  assert.equal((await send(e, github(), { title: "a", body: "x" }, { at: NOW + 121e3 })).status, 200);

  e = env({ TIMINGS_ISSUE: "50", TIMINGS_PER_DAY: "1" });
  assert.equal((await sendTimings(e, gh, TIMINGS, { ip: "192.0.2.1" })).status, 200);
  assert.deepEqual(await sendTimings(e, gh, TIMINGS, { ip: "192.0.2.2" }), { status: 429, data: { error: "daily" } });
  assert.equal(e.COUNTS.m.get("all:2026-10-31"), undefined, "timings never count toward reports");

  // GitHub refuses: nothing is used up
  e = env({ TIMINGS_ISSUE: "50" });
  const down = commentHub(500);
  assert.equal((await sendTimings(e, down, TIMINGS)).status, 502);
  assert.equal((await sendTimings(e, gh, TIMINGS, { at: NOW + 1e3 })).status, 200, "the minute was given back");
});
