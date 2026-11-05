// Specification references (`NN §x.y`) and links into docs/specs.
//
//   node scripts/docs-refs/refs.cjs check                 every reference and spec link resolves
//   node scripts/docs-refs/refs.cjs remap <map.json> [files…]
//                                                         rewrite references in code and non-spec docs
//                                                         from an older numbering (default: all tracked files)
//
// The map lists, per old spec number, [old section prefix, new spec, new section prefix]
// rows; the longest matching prefix wins and an empty new prefix means the whole spec.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const cp = require('node:child_process');

const root = path.resolve(__dirname, '../..');
const SPEC_DIR = 'docs/specs';
const REF = /(?:(?<![\w/.\-])(0\d|1\d|00a) )?§(\d+(?:\.\d+)*)/gu;
const CONT_GAP = /^\s*(?:,|;|\/|–|—|\u3001|\uff0c|&|and|or|, and|, or)\s*$/u;
// A bare § right after a document name (`x.md`, decisions-corpus, research …) points into
// that document, not into a spec.
const DOCREF = /(?:\.md`?\)?|[\w.-]*\/[\w./-]+|[A-Za-z0-9]+(?:[-_][A-Za-z0-9]+)+`?|\bresearch|\bcorpus|\bdesign|\bdraft|\bguide)\s*(?:\/\/\s*)?$/u;
// Text that ends the sentence a ref was named in.
const SENTENCE_END = /[.!?]\s+[A-Z(]|\n\s*\n|\||\n\s*[-*] |\n#/u;
const SKIP_FILE = /(^|\/)testdata\/|\.(png|ico|svg|jpg|jpeg|gif|webp|dll|exe|zip|zst|tar|woff2?|ttf|otf|syso|rws|db|pdb|bin)$/i;

function tracked() {
  const out = cp.execFileSync('git', ['ls-files', '-z'], { cwd: root, maxBuffer: 1 << 28 }).toString('utf8');
  return out.split('\0').filter(f => f && !SKIP_FILE.test(f) && fs.existsSync(path.join(root, f)));
}

// The spec a docs file belongs to: docs/specs/NN-*.md, docs/specs/NN-*/…, docs/00-*.md, docs/00a-*.md.
function specOfFile(rel) {
  rel = rel.split(path.sep).join('/');
  let m = rel.match(/^docs\/specs\/(\d\d)-[^/]+(?:\/|$)/);
  if (m) return m[1];
  m = rel.match(/^docs\/(00a?)-[^/]+\.md$/);
  return m ? m[1] : null;
}

function slug(text) {
  return text.trim().toLowerCase().replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, '').replace(/ /g, '-');
}

// Numbered headings outside code fences: {level, num, title, text, line}.
function headings(text) {
  const out = [];
  let fence = false;
  text.split('\n').forEach((raw, i) => {
    const l = raw.replace(/\r$/, '');
    if (/^\s*(```|~~~)/.test(l)) fence = !fence;
    if (fence) return;
    const m = l.match(/^(#{1,6}) (.*)$/);
    if (!m) return;
    const n = m[2].match(/^(\d+(?:\.\d+)*)\.?\s+(.*)$/);
    out.push({ level: m[1].length, num: n ? n[1] : null, title: n ? n[2] : m[2], text: m[2], line: i, slug: slug(m[2]) });
  });
  return out;
}

// Every spec's sections: spec -> { main: file, sections: Map(num -> {file, slug}) }.
function specIndex() {
  const idx = {};
  const add = (spec, file, main) => {
    const e = idx[spec] || (idx[spec] = { main: null, sections: new Map() });
    if (main) e.main = file;
    for (const h of headings(fs.readFileSync(path.join(root, file), 'utf8'))) {
      if (h.num && !e.sections.has(h.num)) e.sections.set(h.num, { file, slug: h.slug });
    }
  };
  for (const f of fs.readdirSync(path.join(root, 'docs'))) {
    const s = specOfFile('docs/' + f);
    if (s) add(s, 'docs/' + f, true);
  }
  for (const f of fs.readdirSync(path.join(root, SPEC_DIR))) {
    const s = specOfFile(`${SPEC_DIR}/${f}`);
    if (!s) continue;
    const full = path.join(root, SPEC_DIR, f);
    if (fs.statSync(full).isDirectory()) {
      for (const g of fs.readdirSync(full).filter(g => g.endsWith('.md')).sort()) add(s, `${SPEC_DIR}/${f}/${g}`, g === 'README.md');
    } else add(s, `${SPEC_DIR}/${f}`, true);
  }
  return idx;
}

// Words that may stand before a bare § in code when it continues the comment's last spec.
const BARE_OK = /(?:^|[\s(])(?:\(|see|See|in|In|per|and|or|also|of|by|under|from|as|with|after|before|,|;|:)\s*$/u;

// Scan references. A ref without a spec number continues the previous ref when only a
// separator stands between them; otherwise it is bare.
function scan(text) {
  const refs = [];
  let prev = null;
  REF.lastIndex = 0;
  for (let m; (m = REF.exec(text));) {
    const r = { start: m.index, end: m.index + m[0].length, spec: m[1] || null, num: m[2], kind: m[1] ? 'explicit' : 'bare', range: null };
    if (!r.spec && prev && prev.end <= r.start && CONT_GAP.test(text.slice(prev.end, r.start))) {
      r.kind = prev.kind === 'docref' ? 'docref' : 'cont';
      r.spec = prev.spec;
    } else if (!r.spec && DOCREF.test(text.slice(Math.max(0, r.start - 80), r.start))) {
      r.kind = 'docref';
    } else if (!r.spec) {
      // "00 (§2, §4)": sections of the spec right before the parenthesis
      const p = text.slice(Math.max(0, r.start - 12), r.start).match(/(?<![\w/.\-])(0\d|1\d|00a) \($/);
      if (p) { r.kind = 'cont'; r.spec = p[1]; }
    }
    // The spec named last in the same sentence, for bare refs that continue it.
    const prevSpec = prev && (prev.kind === 'docref' ? null : prev.spec || prev.sentence);
    if (r.kind === 'bare' && prevSpec && !SENTENCE_END.test(text.slice(prev.end, r.start))) r.sentence = prevSpec;
    const rest = text.slice(r.end).match(/^–(\d+(?:\.\d+)*)(?![\d.]*\s*§)/u);
    if (rest) { r.range = rest[1]; r.end += rest[0].length; }
    refs.push(r);
    prev = r;
  }
  return refs;
}

// The spec a bare ref inside spec `own` points into: own, unless only the spec named earlier
// in the same sentence has that section.
function resolveBare(r, own, exists) {
  const p = r.sentence;
  if (!p || p === own) return { spec: own };
  const inP = exists(p, r.num), inOwn = exists(own, r.num);
  if (inP && inOwn) return { spec: own, note: 'ambiguous' };
  if (inP) return { spec: p };
  return { spec: own, note: inOwn ? null : 'missing' };
}

// The full section a range ends at ("§6.7.3–4" ends at 6.7.4).
function rangeEnd(r) {
  if (!r.range) return null;
  const sp = r.num.split('.'), ep = r.range.split('.');
  return [...sp.slice(0, Math.max(0, sp.length - ep.length)), ...ep].join('.');
}

function mapRef(map, spec, num) {
  const rows = map[spec];
  if (!rows) return null;
  const parts = num.split('.');
  let best = null;
  for (const [from, to, prefix] of rows) {
    const fp = from === '' ? [] : from.split('.');
    if (fp.length > parts.length || fp.some((p, i) => p !== parts[i])) continue;
    if (!best || fp.length > best.fp.length) best = { fp, to, prefix };
  }
  if (!best) return null;
  const sec = [...(best.prefix === '' ? [] : best.prefix.split('.')), ...parts.slice(best.fp.length)].join('.');
  return { spec: best.to, sec };
}

// Rewrite references. opts: map; mode 'spec' (bare refs belong to opts.fromSpec and the text
// now lives in opts.toSpec), 'doc' (bare refs belong to the document itself: left alone) or
// 'code' (bare refs continue the last spec named nearby, when a plain word precedes them).
function rewrite(text, opts) {
  const { map } = opts;
  const report = [];
  let out = '', pos = 0, lastOld = null, lastNew = opts.mode === 'spec' ? opts.toSpec : null, lastEnd = -1;
  const fmt = (t, withSpec) => (t.sec === '' ? t.spec : (withSpec ? `${t.spec} §${t.sec}` : `§${t.sec}`));
  for (const r of scan(text)) {
    // a continuation of a bare ref continues the spec that ref resolved to
    let oldSpec = r.kind === 'cont' && !r.spec ? lastOld : r.spec;
    if (r.kind === 'docref') continue;
    if (r.kind === 'cont' && !oldSpec) continue;
    if (r.kind === 'bare') {
      if (opts.mode === 'spec') {
        const b = resolveBare(r, opts.fromSpec, opts.exists);
        oldSpec = b.spec;
        if (b.note) report.push({ kind: 'bare-' + b.note, at: r.start, ref: `§${r.num}`, spec: oldSpec });
      } else if (opts.mode === 'code' && lastOld && r.start - lastEnd < 400 && BARE_OK.test(text.slice(Math.max(0, r.start - 40), r.start).replace(/\s*(\/\/|#|\*)\s*/g, ' '))) {
        oldSpec = lastOld;
        report.push({ kind: 'bare-code', at: r.start, ref: `§${r.num}`, spec: oldSpec });
      } else {
        if (opts.mode === 'code') report.push({ kind: 'bare-skip', at: r.start, ref: `§${r.num}` });
        lastOld = null;
        continue;
      }
    }
    // A bare ref in a spec is read against the spec it now sits in; one inferred in code is
    // always written with its spec number.
    if (r.kind === 'bare') lastNew = opts.mode === 'spec' ? opts.toSpec : null;
    const t = mapRef(map, oldSpec, r.num);
    if (!map[oldSpec]) { // a spec this map does not renumber (00, 00a)
      lastOld = oldSpec; lastEnd = r.end;
      if (r.kind === 'explicit') lastNew = oldSpec;
      continue;
    }
    if (!t) { report.push({ kind: 'unmapped', at: r.start, ref: `${oldSpec} §${r.num}` }); lastOld = oldSpec; lastEnd = r.end; continue; }
    const explicit = r.kind === 'explicit' || t.spec !== lastNew || t.sec === '';
    let s = fmt(t, explicit);
    if (r.range) {
      // "§6.7.3–4": a shorter end keeps the start's leading parts
      const sp = r.num.split('.'), ep = r.range.split('.');
      const full = [...sp.slice(0, Math.max(0, sp.length - ep.length)), ...ep].join('.');
      const e = mapRef(map, oldSpec, full);
      if (e && e.spec === t.spec && e.sec !== '') s += `–${e.sec.split('.').slice(-ep.length).join('.')}`;
      else { s += `–${r.range}`; report.push({ kind: 'range', at: r.start, ref: `${oldSpec} §${r.num}–${r.range}` }); }
    }
    out += text.slice(pos, r.start) + s;
    pos = r.end;
    lastOld = oldSpec; lastNew = t.spec; lastEnd = r.end;
  }
  return { text: out + text.slice(pos), report };
}

function lineOf(text, at) { return text.slice(0, at).split('\n').length; }

// check: every explicit ref names an existing section; every bare ref in a spec resolves in
// that spec; every link into docs/specs points at an existing file and heading.
function check() {
  const idx = specIndex();
  const problems = [];
  const has = (spec, num) => idx[spec] && (num === '' || idx[spec].sections.has(num));
  for (const f of tracked()) {
    const text = fs.readFileSync(path.join(root, f), 'utf8');
    const own = specOfFile(f);
    let prev = null;
    for (const r of scan(text)) {
      if (r.kind === 'docref' || (r.kind === 'bare' && (!own || /^00a?$/.test(own)))) { prev = null; continue; }
      const spec = r.kind === 'bare' ? resolveBare(r, own, has).spec : (r.spec || prev);
      prev = spec;
      if (!spec) continue;
      for (const num of [r.num, rangeEnd(r)].filter(Boolean)) {
        if (!has(spec, num)) problems.push(`${f}:${lineOf(text, r.start)}: ${spec} §${num} does not exist`);
      }
    }
    if (!f.endsWith('.md')) continue;
    for (const m of text.matchAll(/\]\(([^)\s#]*)(#[^)\s]*)?\)/g)) {
      const target = m[1] ? path.posix.normalize(path.posix.join(path.posix.dirname(f), m[1])) : f;
      if (/^[a-z]+:/i.test(m[1]) || !target.startsWith('docs/')) continue;
      if (!fs.existsSync(path.join(root, target))) { problems.push(`${f}:${lineOf(text, m.index)}: link to missing ${target}`); continue; }
      if (m[2] && target.endsWith('.md') && m[2].length > 1) {
        const want = decodeURIComponent(m[2].slice(1));
        const hs = headings(fs.readFileSync(path.join(root, target), 'utf8'));
        if (!hs.some(h => h.slug === want)) problems.push(`${f}:${lineOf(text, m.index)}: no heading #${want} in ${target}`);
      }
    }
  }
  return problems;
}

// Refresh anchors of links into docs/specs whose heading text changed: the section number at
// the start of the anchor (digits without dots) finds the heading in the target file.
function fixAnchors(files) {
  let changed = 0;
  for (const f of files.filter(f => f.endsWith('.md'))) {
    const full = path.join(root, f);
    const text = fs.readFileSync(full, 'utf8');
    const next = text.replace(/\]\(([^)\s#]*)#([^)\s]+)\)/g, (all, p, anchor) => {
      const target = p ? path.posix.normalize(path.posix.join(path.posix.dirname(f), p)) : f;
      if (!target.startsWith(SPEC_DIR + '/') && !/^docs\/00a?-/.test(target)) return all;
      if (!fs.existsSync(path.join(root, target))) return all;
      const hs = headings(fs.readFileSync(path.join(root, target), 'utf8'));
      const want = decodeURIComponent(anchor);
      if (hs.some(h => h.slug === want)) return all;
      const digits = (want.match(/^(\d+)-/) || [])[1];
      const cands = digits ? hs.filter(h => h.num && h.num.replace(/\./g, '') === digits) : [];
      if (cands.length !== 1) return all;
      return `](${p}#${cands[0].slug})`;
    });
    if (next !== text) { fs.writeFileSync(full, next); changed++; }
  }
  return changed;
}

module.exports = { root, tracked, specOfFile, slug, headings, specIndex, scan, mapRef, resolveBare, rewrite, check, fixAnchors, lineOf };

if (require.main === module) {
  const [cmd, ...args] = process.argv.slice(2);
  if (cmd === 'check') {
    const p = check();
    for (const l of p) console.log(l);
    console.log(p.length ? `${p.length} problems` : 'all references resolve');
    process.exit(p.length ? 1 : 0);
  } else if (cmd === 'fix-anchors') {
    console.log(`${fixAnchors(tracked())} files updated`);
  } else if (cmd === 'remap' && args[0]) {
    const map = JSON.parse(fs.readFileSync(args[0], 'utf8')).map;
    const files = args.length > 1 ? args.slice(1) : tracked().filter(f => !f.startsWith(SPEC_DIR + '/'));
    let n = 0;
    for (const f of files) {
      const full = path.join(root, f);
      const text = fs.readFileSync(full, 'utf8');
      const mode = specOfFile(f) && !/^docs\/00a?-/.test(f) ? null : (f.endsWith('.md') ? 'doc' : 'code');
      if (!mode) { console.log(`skip ${f}: specs are already in the new layout`); continue; }
      const r = rewrite(text, { map, mode });
      for (const x of r.report) console.log(`${f}:${lineOf(text, x.at)}: ${x.kind} ${x.ref}${x.spec ? ' (as ' + x.spec + ')' : ''}`);
      if (r.text !== text) { fs.writeFileSync(full, r.text); n++; }
    }
    console.log(`${n} files rewritten`);
  } else {
    console.log('usage: refs.cjs check | fix-anchors | remap <map.json> [files…]');
    process.exit(2);
  }
}
