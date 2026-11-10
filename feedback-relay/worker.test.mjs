// The feedback relay without Cloudflare or GitHub: node --test "feedback-relay/*.test.mjs".
// A fake GitHub answers the three calls, a Map stands in for KV, and the real SendGate class
// runs behind a fake Durable Object namespace with Map storage.
import test from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, createVerify } from "node:crypto";
import { handle, clean, defuse, appJWT, pkcs8, forgetToken, LIMITS, SendGate } from "./worker.mjs";

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
  assert.deepEqual(r.data, { number: 41, url: "https://github.com/ToumanLin/RimArcho/issues/41" });

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
  assert.deepEqual([...e.COUNTS.m.keys()], ["all:2026-10-31"]);
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
