// Local review host. Product CSS, icons, shell and original home are served
// directly from the repository; /runtime.js substitutes browser-local fixtures.
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const root = __dirname;
const repo = path.resolve(root, '../../..');
const app = path.join(repo, 'internal/surface/gui/assets');
const port = Number(process.env.RIMARCHO_DRAFT_PORT || 8766);
const mime = {'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.svg':'image/svg+xml','.jpg':'image/jpeg','.png':'image/png','.md':'text/plain; charset=utf-8'};
// Keep the game's artwork local instead of adding it to the source repository.
const background = process.env.RIMARCHO_DRAFT_BACKGROUND || 'D:/SteamLibrary/steamapps/common/RimWorld/Data/Core/About/Preview.png';
http.createServer((req, res) => {
  let name;
  try { name = decodeURIComponent(new URL(req.url, 'http://127.0.0.1').pathname); } catch { res.writeHead(400); res.end(); return; }
  res.setHeader('Cache-Control', 'no-store');
  let file;
  if (name === '/background.png') file = background;
  else if (name === '/app-icon.svg') file = path.join(repo, 'docs/assets/rimarcho.svg');
  else {
    const base = name.startsWith('/app/') ? app : root;
    file = path.resolve(base, '.' + (name.startsWith('/app/') ? name.slice(4) : name === '/' ? '/index.html' : name));
    if (!file.startsWith(base + path.sep)) { res.writeHead(403); res.end(); return; }
  }
  fs.readFile(file, (err, bytes) => {
    if (err) { res.writeHead(404); res.end('Not found'); return; }
    res.setHeader('Content-Type', mime[path.extname(file)] || 'application/octet-stream');
    res.end(bytes);
  });
}).listen(port, '127.0.0.1', () => console.log(`RimArcho home drafts: http://127.0.0.1:${port}/?design=a`));
