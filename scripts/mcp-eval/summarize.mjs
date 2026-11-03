// Summarises a run's Pi transcript (--mode json): tool calls by name, turns, tokens, final text.
// usage: node summarize.mjs <resultDir>
import fs from 'fs'; import path from 'path';
const dir = path.resolve(process.argv[2]);
const ev = fs.readFileSync(path.join(dir, 'transcript.jsonl'), 'utf8').split(/\r?\n/).filter(Boolean).map(l => { try { return JSON.parse(l); } catch { return null; } }).filter(Boolean);
const tools = {}; let turns = 0, usage = {input: 0, output: 0, cacheRead: 0, total: 0}, last = '';
for (const e of ev) {
  if (e.type === 'tool_execution_start') { const n = e.toolName || e.name || e.tool?.name || '?'; tools[n] = (tools[n] || 0) + 1; }
  if (e.type === 'turn_end') { turns++; const u = e.message?.usage; if (u) { usage.input += u.input || 0; usage.output += u.output || 0; usage.cacheRead += u.cacheRead || 0; usage.total += u.totalTokens || 0; } }
  if (e.type === 'message_end' && e.message?.role === 'assistant') { const t = (e.message.content || []).filter(c => c.type === 'text').map(c => c.text).join('\n'); if (t) last = t; }
}
const run = JSON.parse(fs.readFileSync(path.join(dir, 'run.json'), 'utf8').replace(/^﻿/, ''));
const s = {run: path.basename(dir), seconds: run.seconds, finished: run.finished, turns, toolCalls: Object.values(tools).reduce((a, b) => a + b, 0), tools, usage, finalText: last.slice(0, 600)};
fs.writeFileSync(path.join(dir, 'summary.json'), JSON.stringify(s, null, 2)); console.log(JSON.stringify(s, null, 2));
