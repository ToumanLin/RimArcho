// Local review host for the Archo motion drafts.
// /app/*   the product's assets (style.css, js/icons.js) straight from the repository
// /archo/* the Archo runtime and its asset package, kept outside the repository until the
//          plan is approved (RIMARCHO_ARCHO_RUNTIME, default: the archo-portrait checkout)
// /*       this folder
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const root = __dirname;
const repo = path.resolve(root, '../../..');
const app = path.join(repo, 'internal/surface/gui/assets');
const archo = path.resolve(process.env.RIMARCHO_ARCHO_RUNTIME || 'D:/Game/RimModManager_Upgrade/archo-portrait/anim/runtime');
const port = Number(process.env.RIMARCHO_DRAFT_PORT || 8767);
const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml', '.png': 'image/png', '.webp': 'image/webp', '.json': 'application/json', '.md': 'text/plain; charset=utf-8' };
http.createServer((req, res) => {
  let name;
  try { name = decodeURIComponent(new URL(req.url, 'http://127.0.0.1').pathname); } catch { res.writeHead(400); res.end(); return; }
  let base = root, rel = name === '/' ? '/index.html' : name;
  if (name.startsWith('/app/')) { base = app; rel = name.slice(4); }
  else if (name.startsWith('/archo/')) { base = archo; rel = name.slice(6); }
  const file = path.resolve(base, '.' + rel);
  if (!file.startsWith(base + path.sep)) { res.writeHead(403); res.end(); return; }
  fs.readFile(file, (err, bytes) => {
    if (err) { res.writeHead(404); res.end('Not found'); return; }
    // images may be cached so repeated waits show the warm case; scripts are always fresh
    res.setHeader('Cache-Control', /\.(webp|png)$/.test(file) ? 'max-age=3600' : 'no-store');
    res.setHeader('Content-Type', mime[path.extname(file)] || 'application/octet-stream');
    res.end(bytes);
  });
}).listen(port, '127.0.0.1', () => console.log(`Archo motion drafts: http://127.0.0.1:${port}/`));
