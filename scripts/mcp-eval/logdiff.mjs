// Error lines of a Player.log, normalised so two runs compare. Used by the grader and to check the answer keys.
// usage: node logdiff.mjs <caseDir> <logToJudge>   -> prints missing-fix lines (still in the log, absent from the reference) and gone lines
import fs from 'fs'; import path from 'path';

const ERR = /Config error|Could not|rror|xception|failed|Failed|Undefined|Patching|cross-reference|Tried to add pawn/;
// SteamAPI.Init: present or not depending on whether the host's Steam client runs, not on the submission
const SKIP = /SteamAPI\.Init\(\) failed|Native Crash|fatal error in the mono|Fallback handler|Translation data for language|0x[0-9a-f]+\]? in|^\s+at /;

export function errorLines(file) {
  const out = new Set();
  for (let l of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    if (!ERR.test(l) || SKIP.test(l)) continue;
    l = l.replace(/[A-Z]:[\\/][^\s"')]+/gi, '<path>').replace(/0x[0-9a-f]+/gi, '<hex>').replace(/data-[0-9A-F]+\.dll/g, 'data.dll').slice(0, 240);
    out.add(l.trim());
  }
  return out;
}

export function judge(caseDir, log) {
  const ref = errorLines(path.join(caseDir, 'reference.Player.log'));
  const broken = errorLines(path.join(caseDir, 'Player.log'));
  const got = errorLines(log);
  return {
    stillBroken: [...got].filter(l => !ref.has(l)),            // errors the reference fix does not have
    originalErrors: [...broken].filter(l => !ref.has(l)),       // what the injected fault added
  };
}

if (process.argv[1].endsWith('logdiff.mjs')) {
  const r = judge(path.resolve(process.argv[2]), process.argv[3]);
  console.log(JSON.stringify(r, null, 1).slice(0, 4000));
}
