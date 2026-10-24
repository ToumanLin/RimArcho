// node scripts/docs-reader/build.cjs [path/to/markdown-it]
// Uses the workspace's existing markdown-it; the generated HTML has no dependencies.
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../..');
const docsDir = path.join(root, 'docs');
const outputDir = path.join(root, '.cache', 'docs-reader');
const outputFile = path.join(outputDir, 'rimarcho-docs-reader.html');
const candidates = [process.argv[2], 'markdown-it', path.resolve(root, '../RimModManager/frontend/node_modules/markdown-it')].filter(Boolean);
let MarkdownIt;
for (const name of candidates) {
  try { MarkdownIt = require(name); break; } catch (e) { if (e.code !== 'MODULE_NOT_FOUND') throw e; }
}
if (!MarkdownIt) throw new Error('请安装 markdown-it，或通过命令参数传入模块的绝对路径。');
const md = new MarkdownIt({ html: false, linkify: true, typographer: false });
function walk(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const full = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(full) : /\.md$/i.test(entry.name) ? [full] : [];
  });
}
const docs = walk(docsDir).map(full => {
  const source = fs.readFileSync(full, 'utf8');
  const relative = path.relative(docsDir, full).split(path.sep).join('/');
  const title = source.match(/^#\s+(.+)$/m)?.[1].trim() || path.basename(full, '.md');
  return { path: relative, title, source, html: md.render(source), modified: fs.statSync(full).mtime.toISOString() };
}).sort((a, b) => a.path.localeCompare(b.path, 'zh-CN', { numeric: true }));
const template = fs.readFileSync(path.join(__dirname, 'template.html'), 'utf8');
const data = JSON.stringify(docs).replace(/</g, '\\u003c').replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029');
const output = template.replace('/*__DOCUMENTS__*/[]', () => data);
fs.mkdirSync(outputDir, { recursive: true });
fs.writeFileSync(outputFile, output, 'utf8');
console.log(`已生成 .cache/docs-reader/rimarcho-docs-reader.html：${docs.length} 篇文档，${Buffer.byteLength(output).toLocaleString()} 字节。`);
