// Summarises a run's transcript: tool calls by name, turns (model calls), tokens, final text.
// Pi (--mode json), dsh (headless --json: tool_call, status step_end with usage, final) or Alma
// (alma-run.mjs: messages.json holds the thread, traces.json the token counts).
// usage: node summarize.mjs <resultDir>
import fs from 'fs'; import path from 'path';
const dir = path.resolve(process.argv[2]);
const ev = fs.readFileSync(path.join(dir, 'transcript.jsonl'), 'utf8').split(/\r?\n/).filter(Boolean).map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(Boolean);
const tools = {}; let turns = 0, usage = {input: 0, output: 0, cacheRead: 0, total: 0}, last = '';
const dsh = ev.some(e => e.type === 'session' && e.sessionId);
const almaThread = path.join(dir, 'messages.json');
const alma = fs.existsSync(almaThread);
if (alma) {
  const m = JSON.parse(fs.readFileSync(almaThread, 'utf8'));
  for (const x of Object.values(m).flat()) {
    const msg = x?.message || x;
    if (msg?.role !== 'assistant') continue;
    for (const p of msg.parts || []) {
      if (p.type === 'step-start') turns++;
      if (p.type?.startsWith('tool-') || p.type === 'dynamic-tool') { const t = p.toolName || p.type.slice(5); tools[t] = (tools[t] || 0) + 1; }
      // run_script runs JS that calls tools with almaTool(name, args): count those calls too
      if (p.type === 'tool-run_script') for (const [, t] of String(p.input?.code || '').matchAll(/almaTool\(\s*["'`]([\w-]+)/g)) tools[t] = (tools[t] || 0) + 1;
      if (p.type === 'text' && p.text) last = p.text;
    }
  }
  try { for (const t of JSON.parse(fs.readFileSync(path.join(dir, 'traces.json'), 'utf8')).traces || []) { usage.input += t.inputTokens || 0; usage.output += t.outputTokens || 0; usage.cacheRead += t.cachedInputTokens || 0; usage.total += t.totalTokens || 0; } } catch {}
}
for (const e of alma ? [] : ev) {
  if (dsh) {
    if (e.type === 'tool_call') tools[e.tool || '?'] = (tools[e.tool || '?'] || 0) + 1;
    if (e.type === 'status' && e.phase === 'step_end') { turns++; const u = e.usage || {}; usage.input += u.inputTokens || 0; usage.output += u.outputTokens || 0; usage.cacheRead += u.cacheReadTokens || 0; usage.total += u.totalTokens || 0; }
    if (e.type === 'text' && e.text) last = e.text;
    if (e.type === 'final' && e.text) last = e.text;
    continue;
  }
  if (e.type === 'tool_execution_start') { const n = e.toolName || e.name || e.tool?.name || '?'; tools[n] = (tools[n] || 0) + 1; }
  if (e.type === 'turn_end') { turns++; const u = e.message?.usage; if (u) { usage.input += u.input || 0; usage.output += u.output || 0; usage.cacheRead += u.cacheRead || 0; usage.total += u.totalTokens || 0; } }
  if (e.type === 'message_end' && e.message?.role === 'assistant') { const t = (e.message.content || []).filter(c => c.type === 'text').map(c => c.text).join('\n'); if (t) last = t; }
}
const run = JSON.parse(fs.readFileSync(path.join(dir, 'run.json'), 'utf8').replace(/^﻿/, ''));
const s = {run: path.basename(dir), harness: alma ? 'alma' : dsh ? 'dsh' : 'pi', seconds: run.seconds, finished: run.finished, turns, toolCalls: Object.values(tools).reduce((a, b) => a + b, 0), tools, usage, finalText: last.slice(0, 600)};
fs.writeFileSync(path.join(dir, 'summary.json'), JSON.stringify(s, null, 2)); console.log(JSON.stringify(s, null, 2));
