// Groups L and M: one Alma run, headless (no window): start alma-server with a fresh data folder,
// add the magpie gateway and model, Archo as MCP server, a project at C:\Eval\work, then send the
// task over the thread WebSocket and record every event until the thread stops generating.
//   node alma-run.mjs --alma C:\Eval\alma --task C:\Eval\work\TASK.md --out C:\Eval\out --minutes 15
//     --base-url <gateway /v1> --api-key <key> [--instructions] [--full] [--probe]
// --full (groups J and K): also the tool model Alma uses for titles, tool preselection, memory and
// query rewriting (it picks one by itself only from official model names, so on a gateway it has
// none), and memory with the local embedding model multilingual-e5-small (the gateway has no
// embeddings; the model files are pushed into C:\Eval\alma-data\embedding-models beforehand).
// --instructions (group M): Alma never reads an MCP server's instructions and keeps MCP tools out of
// the model's tool list until ToolSearch finds them or the model calls one by name. This puts
// Archo's instructions in the project card, which Alma adds to the system prompt (as group P does
// for Pi). --probe sets everything up, dumps the tool list and settings, and stops before the task.
// ALMA_HEADLESS=1 approves every tool call (Pi and dsh run without approvals too).
import fs from 'node:fs'; import path from 'node:path'; import { spawn } from 'node:child_process';

const arg = (k, d) => { const i = process.argv.indexOf('--' + k); return i > 0 ? process.argv[i + 1] : d; };
const alma = arg('alma', 'C:\\Eval\\alma'), out = arg('out', 'C:\\Eval\\out'), minutes = +arg('minutes', '15');
const probe = process.argv.includes('--probe');
const model = 'apikeyfan/deepseek-flash', port = 23111, base = `http://127.0.0.1:${port}`;
fs.mkdirSync(out, { recursive: true });
const t0 = Date.now();
const log = fs.createWriteStream(path.join(out, 'transcript.jsonl'));
const note = (o) => log.write(JSON.stringify({ t: Date.now() - t0, ...o }) + '\n');

const server = spawn(path.join(alma, 'alma-server', 'runtime', 'alma-server.exe'),
  [path.join(alma, 'alma-server', 'lib', 'server.mjs'), 'serve', '--port', String(port), '--data-dir', 'C:\\Eval\\alma-data'], {
    env: { ...process.env, ALMA_HEADLESS: '1', ALMA_TOOL_APPROVAL: 'auto',
      // test_run starts the game and takes 1 to 3 minutes; Archo calls get 15 minutes, as in dsh
      ALMA_TOOL_TIMEOUT_MS: '900000', ALMA_TOOL_TIMEOUT_MAX_MS: '900000' },
    cwd: path.join(alma, 'alma-server'), stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
const slog = fs.createWriteStream(path.join(out, 'stderr.txt'));
server.stdout.pipe(slog); server.stderr.pipe(slog);

async function api(method, p, body) {
  const r = await fetch(base + p, { method, headers: { 'content-type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  const text = await r.text();
  let j; try { j = JSON.parse(text); } catch { j = text; }
  if (!r.ok) throw new Error(`${method} ${p}: ${r.status} ${text.slice(0, 300)}`);
  return j;
}
// the instructions this archo build sends at initialize, read from the server itself
function archoInstructions() {
  return new Promise((res, rej) => {
    const p = spawn('C:\\Eval\\tools\\archo.exe', ['mcp'], { stdio: ['pipe', 'pipe', 'ignore'], windowsHide: true });
    let buf = '';
    p.stdout.on('data', (d) => {
      buf += d; const nl = buf.indexOf('\n'); if (nl < 0) return;
      p.kill();
      try { const ins = JSON.parse(buf.slice(0, nl)).result?.instructions; ins ? res(ins) : rej(new Error('no instructions: ' + buf.slice(0, 200))); } catch (e) { rej(e); }
    });
    p.on('error', rej);
    p.stdin.write('{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"eval","version":"1"}}}\n');
  });
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const save = (name, v) => fs.writeFileSync(path.join(out, name), typeof v === 'string' ? v : JSON.stringify(v, null, 2));

let finished = false, threadId = null;
try {
  for (let i = 0; ; i++) {
    try { await api('GET', '/api/settings'); break; } catch (e) { if (i > 90) throw new Error('alma-server did not start: ' + e.message); await sleep(1000); }
  }
  const prov = await api('POST', '/api/providers', { name: 'magpie', type: 'custom', apiFormat: 'openai-chat', apiKey: arg('api-key'), baseURL: arg('base-url'), models: [model] });
  const settings = await api('GET', '/api/settings');
  settings.chat = { ...(settings.chat || {}), defaultModel: `${prov.id}:${model}` };
  if (process.argv.includes('--full')) {
    settings.toolModel = { model: `${prov.id}:${model}` };
    settings.memory = { ...(settings.memory || {}), enabled: true, autoSummarize: true, autoRetrieve: true, embeddingModel: '__local__:multilingual-e5-small' };
  }
  await api('PUT', '/api/settings', settings);
  await api('POST', '/api/mcp-servers', { name: 'archo', config: { command: 'C:\\Eval\\tools\\archo.exe', args: ['mcp'] } });
  let mcp;
  for (let i = 0; i < 60; i++) { mcp = await api('GET', '/api/mcp-client/status'); if (mcp?.servers?.some((s) => s.name === 'archo' && s.initialized)) break; await sleep(1000); }
  const ws0 = await api('POST', '/api/workspaces', { name: 'eval', path: 'C:\\Eval\\work' });
  const wsId = ws0.id || ws0.workspace?.id;
  if (process.argv.includes('--instructions')) {
    const ins = await archoInstructions();
    save('archo-instructions.md', `# Instructions from the MCP server archo\n\n${ins}\n`);
    // the card's fields hold 2000 characters each: brief is the instructions, focus says how Alma names the tools
    if (ins.length > 2000) note({ type: 'driver_error', error: `instructions cut to 2000 of ${ins.length} characters` });
    await api('PUT', `/api/workspaces/${wsId}/project-card`, { brief: ins,
      focus: 'The text above is the instructions of the MCP server archo. Its tools are named archo__<tool> here (archo__analyze_log, archo__get_mod, ...); call one by that name and it loads.' });
  }
  // the same reasoning effort as Pi and dsh
  try { await api('PUT', `/api/workspaces/${wsId}/project-config`, { defaultReasoningEffort: 'high' }); } catch (e) { note({ type: 'driver_error', error: 'effort: ' + e }); }
  const thread = await api('POST', '/api/threads', { title: 'eval', workspaceId: wsId });
  threadId = thread.id || thread.thread?.id;
  note({ type: 'setup', provider: prov.id, workspace: wsId, thread: threadId, mcp });
  // what this Alma offers the model, saved with every run
  {
    save('probe-settings.json', await api('GET', '/api/settings'));
    save('probe-providers.json', await api('GET', '/api/providers'));
    save('probe-mcp-tools.json', await api('GET', '/api/mcp-client/tools'));
    try { save('probe-tools.json', await api('GET', '/api/tools')); } catch (e) { save('probe-tools.json', String(e)); }
    save('probe-thread.json', await api('GET', `/api/threads/${threadId}`));
  }
  if (!probe) {
    const task = fs.readFileSync(arg('task'), 'utf8').replace(/^\uFEFF/, '');
    const ws = new WebSocket(base.replace('http', 'ws') + '/ws/threads');
    await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
    let generating = false, idleSince = 0;
    ws.onmessage = (m) => {
      let msg; try { msg = JSON.parse(String(m.data)); } catch { return; }
      // message_delta is a token stream: keep it out of the log (message_added has the whole message)
      if (msg.type !== 'message_delta') note(msg);
      if (msg.type === 'thread_generating' && msg.data?.id === threadId) { generating = !!msg.data.isGenerating; if (!generating) idleSince = Date.now(); }
    };
    ws.send(JSON.stringify({ type: 'generate_response', data: { threadId, model: `${prov.id}:${model}`,
      userMessage: { role: 'user', parts: [{ type: 'text', text: `${task}\n\nDo the task described above.` }] } } }));
    const deadline = t0 + minutes * 60_000;
    let started = false;
    while (Date.now() < deadline) {
      await sleep(2000);
      if (generating) started = true;
      // done: it stopped generating and stayed idle for 20 s (auto-continue may start it again)
      if (started && !generating && Date.now() - idleSince > 20_000) { finished = true; break; }
    }
    if (!finished) { try { ws.send(JSON.stringify({ type: 'stop_generation', data: { threadId } })); } catch {} await sleep(3000); }
    ws.close();
  }
} catch (e) {
  note({ type: 'driver_error', error: String(e?.stack || e) });
} finally {
  if (threadId) {
    try { save('messages.json', await api('GET', `/api/threads/${threadId}/messages?limit=1000`)); } catch (e) { note({ type: 'driver_error', error: 'messages: ' + e }); }
    try { save('traces.json', await api('GET', `/api/threads/${threadId}/traces`)); } catch (e) { note({ type: 'driver_error', error: 'traces: ' + e }); }
  }
  // --full: what the tool model and memory did (usage includes the tool model's calls)
  if (process.argv.includes('--full')) {
    try { save('memories.json', await api('GET', '/api/memories')); } catch (e) { note({ type: 'driver_error', error: 'memories: ' + e }); }
    try { save('usage.json', await api('GET', '/api/usage/stats')); } catch (e) { note({ type: 'driver_error', error: 'usage: ' + e }); }
  }
  // agent.ps1 writes run.json (the host waits for it) after this process exits
  save('alma-run.json', { finished, seconds: Math.round((Date.now() - t0) / 1000) });
  log.end(); server.kill();
  setTimeout(() => process.exit(0), 2000);
}
