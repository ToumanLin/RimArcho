// Calls archo's test_run the way Pi does, without a model: Pi's own MCP client, a 60 s
// request timeout, progress requested. Run as the standard user after run-case.ps1 -StageOnly.
//   node C:\Eval\probe-testrun.mjs [map]
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

// The VM image installs Pi with npm under C:\Tools\npm (the pi command there).
const agent = 'C:\\Tools\\npm\\node_modules\\@earendil-works\\pi-coding-agent';
const version = JSON.parse(fs.readFileSync(path.join(agent, 'package.json'), 'utf8')).version;
const lib = path.join(agent, 'node_modules', '@earendil-works', 'pi-mcp', 'dist', 'index.js');
const { McpClient, StdioTransport } = await import(pathToFileURL(lib).href);

const out = 'C:\\Eval\\out\\probe.txt';
const t0 = Date.now();
const log = (...parts) => {
  const line = `${String(Math.round((Date.now() - t0) / 1000)).padStart(4)}s ${parts.join(' ')}`;
  console.log(line);
  fs.appendFileSync(out, line + '\n');
};

log(`pi-mcp ${version}`);
const client = new McpClient({ name: 'pi', version, requestTimeoutMs: 60_000 });
await client.connect(new StdioTransport({ command: 'C:\\Eval\\tools\\archo.exe', args: ['mcp'] }));
const args = { env: 'main', ...(process.argv[2] === 'map' ? { map: true } : {}) };
try {
  const res = await client.callTool('test_run', args, {
    timeoutMs: 60_000,
    onProgress: (p) => log('progress', p.progress, p.message ?? ''),
  });
  log('result, isError', res.isError);
  log(res.content.map((c) => c.text ?? '').join(''));
} catch (e) {
  log('error', e.message);
}
await client.close();
