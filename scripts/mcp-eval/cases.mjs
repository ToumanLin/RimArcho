// Generates the eval cases from the pristine real Mods: only the injected faults differ.
// usage: node cases.mjs [case...] [--out dir]   (default: every case, into D:/RimArcho-test/mcp-eval/cases)
import fs from 'fs'; import path from 'path'; import crypto from 'crypto'; import {execFileSync} from 'child_process';

const WS = 'D:/SteamLibrary/steamapps/workshop/content/294100';
const LIB = 'D:/rimworld16/Mods';
const args = process.argv.slice(2); const oi = args.indexOf('--out');
const OUT = oi >= 0 ? args[oi + 1] : 'D:/RimArcho-test/mcp-eval/cases';
const only = args.filter((a, i) => !a.startsWith('--') && args[i - 1] !== '--out');
const want = id => !only.length || only.includes(id);

const OSP = {pkg: 'khamenman.OutfitStandsPLus', dir: `${WS}/3545172389`, name: 'Outfit Stands Plus'};
const ODY = {pkg: 'odysseypatch.1trickPwnyta', dir: `${LIB}/3578444636`, name: "1trickPwnyta's Odyssey Patch"};

const read = (dir, rel) => fs.readFileSync(path.join(dir, rel), 'utf8').split('\r\n').join('\n');
function put(id, pkg, rel, text) {
  const f = path.join(OUT, id, 'overlay', pkg, rel); fs.mkdirSync(path.dirname(f), {recursive: true}); fs.writeFileSync(f, text);
}
function must(text, from, to) { if (!text.includes(from)) throw new Error('anchor missing: ' + from); return text.replace(from, to); }
function fixfile(id, pkg, rel, text) {
  const f = path.join(OUT, id, 'answer', pkg, rel); fs.mkdirSync(path.dirname(f), {recursive: true}); fs.writeFileSync(f, text);
}
function emit(id, spec, answer) {
  fs.mkdirSync(path.join(OUT, id), {recursive: true});
  fs.writeFileSync(path.join(OUT, id, 'case.json'), JSON.stringify({id, base: '../base-mods.json', ...spec}, null, 2));
  fs.writeFileSync(path.join(OUT, id, 'answer.json'), JSON.stringify(answer, null, 2));
}

// ---- Case 1: XML, a wrong class name in one Def (namespace typo) -----------------------------
if (want('c1')) {
  const rel = 'Defs/ThingDefs/Building_MendingOutfitStand.xml';
  let x = read(OSP.dir, rel);
  x = must(x, '<thingClass>OutfitStandsPlus.Things.MendingOutfitStand</thingClass>', '<thingClass>OutfitStandsPlus.Thing.MendingOutfitStand</thingClass>');
  put('c1', OSP.pkg, rel, x);
  fixfile('c1', OSP.pkg, rel, read(OSP.dir, rel));
  emit('c1', {overlay: [{pkg: OSP.pkg, files: 'overlay/' + OSP.pkg}]}, {
    culprit: [{pkg: OSP.pkg, name: OSP.name, role: 'broken file'}],
    required: [OSP.pkg],
    faultClass: 'xml.type-missing',
    files: [`${OSP.pkg}/${rel}`],
    fix: 'thingClass on the first MendingOutfitStand ThingDef must be OutfitStandsPlus.Things.MendingOutfitStand again (line 37).',
  });
}

// ---- Case 2: patch ordering conflict ----------------------------------------------------------
// Outfit Stands Plus matches the stand by its thingClass; the Odyssey Patch mod rewrites that thingClass.
// Listed before Outfit Stands Plus, the Odyssey Patch makes the xpath match nothing.
if (want('c2')) {
  const rel = 'Patches/OutfitStandsPlusPatches.xml';
  let x = read(OSP.dir, rel);
  x = must(x, '<xpath>/Defs/ThingDef[defName="Building_OutfitStand"]/comps</xpath>\n        <nomatch Class="PatchOperationAdd">\n            <xpath>/Defs/ThingDef[defName="Building_OutfitStand"]</xpath>',
    '<xpath>/Defs/ThingDef[thingClass="Building_OutfitStand"]/comps</xpath>\n        <nomatch Class="PatchOperationAdd">\n            <xpath>/Defs/ThingDef[thingClass="Building_OutfitStand"]</xpath>');
  put('c2', OSP.pkg, rel, x);
  fixfile('c2', OSP.pkg, rel, read(OSP.dir, rel));
  emit('c2', {overlay: [{pkg: OSP.pkg, files: 'overlay/' + OSP.pkg}], add: [{pkg: ODY.pkg, name: ODY.name, path: ODY.dir, before: OSP.pkg}]}, {
    culprit: [{pkg: OSP.pkg, name: OSP.name, role: 'its patch no longer matches'}, {pkg: ODY.pkg, name: ODY.name, role: 'rewrites thingClass first (cause of the miss)'}],
    required: [OSP.pkg],
    faultClass: 'patch.order-conflict',
    files: [`${OSP.pkg}/${rel}`],
    fix: 'Either match the stand by defName again in the first Operation of OutfitStandsPlusPatches.xml, or make Outfit Stands Plus load before the Odyssey Patch (loadBefore in About.xml / list order).',
  });
}


// ---- Case 3: C# — a Harmony patch whose target no longer exists in 1.6 (rebuilt DLL) ------------------
// Smart Firefight patches GenSpawn.Spawn by explicit parameter types; the injected build lacks one parameter.
function build(srcDir, outDll, edit) {
  const tmp = path.join(OUT, '_build', path.basename(outDll, '.dll')); fs.rmSync(tmp, {recursive: true, force: true}); fs.mkdirSync(tmp, {recursive: true});
  for (const f of fs.readdirSync(srcDir)) if (f.endsWith('.cs')) fs.writeFileSync(path.join(tmp, f), edit(f, fs.readFileSync(path.join(srcDir, f), 'utf8')));
  fs.cpSync(path.join(srcDir, 'Properties'), path.join(tmp, 'Properties'), {recursive: true});
  const G = 'D:/rimworld16/RimWorldWin64_Data/Managed/';
  const ref = n => `<Reference Include="${n}"><HintPath>${G}${n}.dll</HintPath><Private>false</Private></Reference>`;
  fs.writeFileSync(path.join(tmp, 'SmartFirefight.csproj'), `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>netstandard2.1</TargetFramework><LangVersion>latest</LangVersion><Nullable>enable</Nullable><AssemblyName>SmartFirefight</AssemblyName><GenerateAssemblyInfo>false</GenerateAssemblyInfo><OutDir>bin/</OutDir></PropertyGroup><ItemGroup>${['Assembly-CSharp', 'UnityEngine.CoreModule', 'UnityEngine.IMGUIModule', 'UnityEngine.TextRenderingModule'].map(ref).join('')}<PackageReference Include="Lib.Harmony.Ref" Version="2.3.6" /></ItemGroup></Project>`);
  execFileSync('dotnet', ['build', '-c', 'Release', '-nologo', '-v', 'q'], {cwd: tmp, stdio: 'pipe'});
  fs.mkdirSync(path.dirname(outDll), {recursive: true}); fs.copyFileSync(path.join(tmp, 'bin', 'SmartFirefight.dll'), outDll);
}
if (want('c3')) {
  const SF = {pkg: 'DarkDaskin.SmartFirefight', dir: `${LIB}/3530229241`, name: 'Smart Firefight'};
  const src = path.join(SF.dir, 'Sources/SmartFirefight');
  const bad = (f, t) => f === 'Patch_GenSpawn_Spawn.cs' ? must(t, 'typeof(WipeMode), typeof(bool), typeof(bool)]', 'typeof(WipeMode), typeof(bool)]') : f === 'Startup.cs' ? t.replace('Harmony.DEBUG = true;', 'Harmony.DEBUG = false;') : t;
  const good = (f, t) => f === 'Startup.cs' ? t.replace('Harmony.DEBUG = true;', 'Harmony.DEBUG = false;') : t;
  build(src, path.join(OUT, 'c3', 'overlay', SF.pkg, '1.6/Assemblies/SmartFirefight.dll'), bad);
  put('c3', SF.pkg, 'Sources/SmartFirefight/Patch_GenSpawn_Spawn.cs', must(read(SF.dir, 'Sources/SmartFirefight/Patch_GenSpawn_Spawn.cs'), 'typeof(WipeMode), typeof(bool), typeof(bool)]', 'typeof(WipeMode), typeof(bool)]'));
  put('c3', SF.pkg, 'Sources/SmartFirefight/Startup.cs', read(SF.dir, 'Sources/SmartFirefight/Startup.cs').replace('Harmony.DEBUG = true;', 'Harmony.DEBUG = false;'));
  build(src, path.join(OUT, 'c3', 'answer', SF.pkg, '1.6/Assemblies/SmartFirefight.dll'), good);
  emit('c3', {add: [{pkg: SF.pkg, name: SF.name, path: SF.dir}], overlay: [{pkg: SF.pkg, files: 'overlay/' + SF.pkg}]}, {
    culprit: [{pkg: SF.pkg, name: SF.name, role: 'broken assembly'}],
    required: [SF.pkg],
    faultClass: 'harmony.undefined-target',
    files: [`${SF.pkg}/1.6/Assemblies/SmartFirefight.dll`],
    fix: 'Patch_GenSpawn_Spawn must name the real GenSpawn.Spawn overload again: (Thing, IntVec3, Map, Rot4, WipeMode, bool, bool). Rebuild and replace 1.6/Assemblies/SmartFirefight.dll. A reference build is in answer/.',
  });
}

// ---- Case 4: a required framework is installed but not enabled -------------------------------------
if (want('c4')) {
  const VEF = {pkg: 'OskarPotocki.VanillaFactionsExpanded.Core', name: 'Vanilla Expanded Framework'};
  fs.mkdirSync(path.join(OUT, 'c4', 'answer'), {recursive: true}); fs.writeFileSync(path.join(OUT, 'c4', 'answer', 'enable.json'), JSON.stringify([VEF.pkg]));
  emit('c4', {drop: [VEF.pkg]}, {
    culprit: [{pkg: VEF.pkg, name: VEF.name, role: 'installed but not in the active list; dependency of Vanilla Furniture Expanded (vanillaexpanded.vfecore) and others'}],
    required: [VEF.pkg],
    faultClass: 'dependency.disabled',
    files: ['userdata/Config/ModsConfig.xml'],
    fix: 'Enable oskarpotocki.vanillafactionsexpanded.core in ModsConfig.xml, after brrainz.harmony and before vanillaexpanded.vfecore (any position before its dependents).',
  });
}


// ---- Case 5: wrong blame — the error names one Mod's Def, a different Mod's patch caused it --------------
// 10x Carrying Capacity gets a "compat" patch that rewrites the research prerequisite of Outfit Stands Plus
// with a misspelled Def name. The log blames Outfit Stands Plus' Defs; the patch lives in 10x Carrying Capacity.
if (want('c5')) {
  const X10 = {pkg: 'mlie.10xcarryingcapacity', dir: `${LIB}/2019000693`, name: '10x Carrying Capacity (Continued)'};
  const rel = 'Patches/Stats/Stats_Pawns_General.xml';
  const text = must(read(X10.dir, rel), '</Patch>', `  <!-- Compatibility: heavier carrying capacity means the mechanized stands need the stronger research gate. -->
  <Operation Class="PatchOperationReplace">
    <xpath>/Defs/ThingDef[@Name="OutfitStandsPlus_MechanizedOutfitStandBase"]/researchPrerequisites</xpath>
    <value>
      <researchPrerequisites>
        <li>OutfitStandsPlus_MechanizedOutfitStandResearch</li>
      </researchPrerequisites>
    </value>
  </Operation>
</Patch>`);
  put('c5', X10.pkg, rel, text);
  fixfile('c5', X10.pkg, rel, read(X10.dir, rel));
  emit('c5', {overlay: [{pkg: X10.pkg, files: 'overlay/' + X10.pkg}]}, {
    culprit: [{pkg: X10.pkg, name: X10.name, role: 'its patch writes the misspelled research name'}, ],
    decoy: [{pkg: OSP.pkg, name: OSP.name, role: 'its Defs are named in the log but are correct'}],
    required: [X10.pkg],
    faultClass: 'xml.cross-reference (wrong blame)',
    files: [`${X10.pkg}/${rel}`],
    fix: 'In the added Operation, OutfitStandsPlus_MechanizedOutfitStandResearch must read OutfitStandsPlus_MechanizedOutfitStandsResearch (missing s), or the Operation is removed.',
  });
}

// ---- Harder cases (2026-11-04). Faults modelled on real 1.6 logs of the private corpus (aggregate counts only). ----------
// Files keep their own line endings here, so a changed file does not stand out among its neighbours.
const raw = (dir, rel) => fs.readFileSync(path.join(dir, rel), 'utf8');
const VFE = {pkg: 'vanillaexpanded.vfecore', dir: `${WS}/1718190143`, name: 'Vanilla Furniture Expanded'};
const VEF = {pkg: 'oskarpotocki.vanillafactionsexpanded.core', name: 'Vanilla Expanded Framework'};

// Builds a Mod assembly from its sources with an SDK project (the Mods' own projects point at their authors' paths).
// edit(rel, text) returns the file's new text, or null to leave the file out.
function buildDll(srcDir, asmName, outDll, edit = (r, t) => t) {
  const tmp = path.join(OUT, '_build', asmName + '-' + path.basename(path.dirname(path.dirname(outDll)))); fs.rmSync(tmp, {recursive: true, force: true});
  for (const f of fs.readdirSync(srcDir, {recursive: true})) {
    if (!f.endsWith('.cs') || /^(obj|bin)[\\/]/.test(f)) continue;
    const t = edit(f.split('\\').join('/'), fs.readFileSync(path.join(srcDir, f), 'utf8'));
    if (t !== null) { fs.mkdirSync(path.dirname(path.join(tmp, f)), {recursive: true}); fs.writeFileSync(path.join(tmp, f), t); }
  }
  const G = 'D:/rimworld16/RimWorldWin64_Data/Managed/';
  const ref = n => `<Reference Include="${n}"><HintPath>${G}${n}.dll</HintPath><Private>false</Private></Reference>`;
  fs.writeFileSync(path.join(tmp, asmName + '.csproj'), `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>netstandard2.1</TargetFramework><LangVersion>latest</LangVersion><AssemblyName>${asmName}</AssemblyName><GenerateAssemblyInfo>false</GenerateAssemblyInfo><OutDir>bin/</OutDir></PropertyGroup><ItemGroup>${['Assembly-CSharp', 'UnityEngine', 'UnityEngine.CoreModule', 'UnityEngine.IMGUIModule', 'UnityEngine.TextRenderingModule', 'UnityEngine.UIModule'].map(ref).join('')}<PackageReference Include="Lib.Harmony.Ref" Version="2.3.6" /></ItemGroup></Project>`);
  execFileSync('dotnet', ['build', '-c', 'Release', '-nologo', '-v', 'q'], {cwd: tmp, stdio: 'pipe'});
  fs.mkdirSync(path.dirname(outDll), {recursive: true}); fs.copyFileSync(path.join(tmp, 'bin', asmName + '.dll'), outDll);
}

// ---- h1: cascade — one Mod's patch drops a vanilla Def, and the errors that follow name other Mods' Defs -----------
// Vanilla Furniture Expanded adds its StatPart_Bin to the vanilla Beauty StatDef. Misspelled (StatParts_Bin), the type
// does not resolve and 1.6 drops the whole StatDef, as in the real ReGrowth logs ("Exception loading def from file
// Biomes_*.xml: Could not find type named ..."); every Def that names Beauty then fails to resolve. The first error
// names a vanilla file and no Mod. A case-only typo would not do: 1.6 looks types up ignoring case.
if (want('h1')) {
  const rel = '1.6/Patches/Core.xml';
  const orig = raw(VFE.dir, rel);
  put('h1', VFE.pkg, rel, must(orig, '<li Class="VanillaFurnitureEC.StatPart_Bin"/>', '<li Class="VanillaFurnitureEC.StatParts_Bin"/>'));
  fixfile('h1', VFE.pkg, rel, orig);
  emit('h1', {overlay: [{pkg: VFE.pkg, files: 'overlay/' + VFE.pkg}]}, {
    culprit: [{pkg: VFE.pkg, name: VFE.name, role: 'its patch adds a misspelled class to the vanilla Beauty StatDef'}],
    decoy: [{pkg: VEF.pkg, name: VEF.name, role: 'the framework Vanilla Furniture Expanded builds on; not involved'}],
    required: [VFE.pkg],
    faultClass: 'xml.type-missing (cascade)',
    files: [`${VFE.pkg}/${rel}`],
    fix: 'In 1.6/Patches/Core.xml the StatPart added to Beauty must read VanillaFurnitureEC.StatPart_Bin, or the operation is removed.',
  });
}

// ---- h2: same-name assembly — a compatibility Mod ships a stale copy of another Mod's DLL -------------------------
// The Odyssey Patch ships OutfitStandsPlus.dll in its Assemblies (a reference copied on build, a common modder
// mistake), from before the Mending and Mechanized stands. It loads first; Mono's Assembly.LoadFrom then hands Outfit
// Stands Plus the already loaded assembly of that name, so the stands' classes "do not exist" although Outfit Stands
// Plus' own DLL has them.
if (want('h2')) {
  const stale = 'odysseypatch.1trickPwnyta/1.6/Assemblies/OutfitStandsPlus.dll';
  buildDll(`${OSP.dir}/Source/OutfitStandsPlus`, 'OutfitStandsPlus', path.join(OUT, 'h2', 'overlay', stale),
    (rel, t) => /^Things\/(Mending|Mechanized)OutfitStand\.cs$/.test(rel) ? null : t);
  fs.mkdirSync(path.join(OUT, 'h2', 'answer'), {recursive: true}); fs.writeFileSync(path.join(OUT, 'h2', 'answer', 'delete.json'), JSON.stringify([stale]));
  emit('h2', {add: [{pkg: ODY.pkg, name: ODY.name, path: ODY.dir, before: OSP.pkg}], overlay: [{pkg: ODY.pkg, files: 'overlay/' + ODY.pkg}]}, {
    culprit: [{pkg: ODY.pkg, name: ODY.name, role: 'ships an old OutfitStandsPlus.dll that shadows the real one'}],
    decoy: [{pkg: OSP.pkg, name: OSP.name, role: 'its Defs and DLL are correct; its assembly is shadowed'}],
    required: [ODY.pkg],
    faultClass: 'asm.duplicate-name (shadowed)',
    files: [stale],
    fix: 'Remove 1.6/Assemblies/OutfitStandsPlus.dll from the Odyssey Patch (or replace it with the current build), or load Outfit Stands Plus before the Odyssey Patch.',
  });
}

// ---- h5: two unrelated faults in one start: c1 (Outfit Stands Plus) and c5 (10x Carrying Capacity) ------------------
// c5's errors name Outfit Stands Plus' Defs, which also has c1's real fault: fixing one is not the end.
if (want('h5')) {
  const X10 = {pkg: 'mlie.10xcarryingcapacity', dir: `${LIB}/2019000693`, name: '10x Carrying Capacity (Continued)'};
  for (const c of ['c1', 'c5']) for (const kind of ['overlay', 'answer']) fs.cpSync(path.join(OUT, c, kind), path.join(OUT, 'h5', kind), {recursive: true});
  emit('h5', {overlay: [{pkg: OSP.pkg, files: 'overlay/' + OSP.pkg}, {pkg: X10.pkg, files: 'overlay/' + X10.pkg}]}, {
    culprit: [{pkg: OSP.pkg, name: OSP.name, role: 'broken file (c1)'}, {pkg: X10.pkg, name: X10.name, role: 'its patch writes the misspelled research name (c5)'}],
    required: [OSP.pkg, X10.pkg],
    faultClass: 'two faults: xml.type-missing + xml.cross-reference (wrong blame)',
    files: [`${OSP.pkg}/Defs/ThingDefs/Building_MendingOutfitStand.xml`, `${X10.pkg}/Patches/Stats/Stats_Pawns_General.xml`],
    fix: 'Both c1 and c5 fixes.',
  });
}

// ---- h7: runtime error with no culprit in the stack — a comp that removes itself from its pawn's comp list mid-tick ----
// [SSR]Beautiful Body as of June 2026 (the user's local copy; the Workshop version has since dropped the comp). Its
// TC_ForceBodyShape on Human swaps itself to the end of AllComps and removes itself during CompTick. Vanilla's
// ThingWithComps.Tick indexes with a cached count, so SSR alone logs nothing. Pawn Comp Profiler (our own small Mod,
// scripts/mcp-eval/mods) ticks pawn comps with foreach, as the vanilla loop would, timed; the list change then throws
// "Collection was modified" for every colonist and world pawn, with the profiler in the stack and no SSR frame. With
// the user's fixed DLL (the comp never touches AllComps) the profiler logs nothing. Needs a map: case.json `runtime`.
if (want('h7')) {
  const SSR = {pkg: 'ssr.body', dir: 'D:/SteamLibrary/steamapps/common/RimWorld/Mods/_2985408740_', name: '[SSR]Beautiful Body'};
  const PCP = {pkg: 'tallowbyte.pawncompprofiler', name: 'Pawn Comp Profiler'};
  const asm = `${SSR.dir}/1.6/Assemblies`, dll = '1.6/Assemblies/FS_SSR_Body.dll';
  const sha = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex').slice(0, 16);
  const buggy = `${asm}/FS_SSR_Body.pre-codex-backup.dll`, fixed = `${asm}/FS_SSR_Body.dll`;   // 2026-06-16 original, 2026-06-17 fix
  if (sha(buggy) !== '15af1d654787f297' || sha(fixed) !== 'bf0b5821b3db1b98') throw new Error('SSR DLLs changed in ' + asm);
  // the profiler as if downloaded: its source and About from the repo, its DLL built here
  const pcp = path.join(OUT, '_mods', PCP.pkg); fs.rmSync(pcp, {recursive: true, force: true});
  fs.cpSync(path.join(import.meta.dirname, 'mods', 'pawncompprofiler'), pcp, {recursive: true});
  buildDll(path.join(pcp, 'Source'), 'PawnCompProfiler', path.join(pcp, '1.6/Assemblies/PawnCompProfiler.dll'));
  for (const [kind, from] of [['overlay', buggy], ['answer', fixed]]) {
    fs.rmSync(path.join(OUT, 'h7', kind), {recursive: true, force: true});   // the recorded logs stay
    const f = path.join(OUT, 'h7', kind, SSR.pkg, dll); fs.mkdirSync(path.dirname(f), {recursive: true}); fs.copyFileSync(from, f);
  }
  // The VM has only the Workshop copy (the 2026-09 rewrite: no comp). Host and VM both start from it; the overlay turns
  // it into the June version: these files are all that differ apart from textures, which the June code does not name.
  for (const rel of ['About/About.xml', 'Patches/Patches.xml', '1.4/Assemblies/FS_SSR_Body.dll', '1.5/Assemblies/FS_SSR_Body.dll']) {
    const f = path.join(OUT, 'h7', 'overlay', SSR.pkg, rel); fs.mkdirSync(path.dirname(f), {recursive: true}); fs.copyFileSync(path.join(SSR.dir, rel), f);
  }
  emit('h7', {
    runtime: {ticks: 3000},
    add: [{pkg: SSR.pkg, name: SSR.name, path: `${WS}/2985408740`, replace: true}, {pkg: PCP.pkg, name: PCP.name, path: pcp.split(path.sep).join('/')}],
    overlay: [{pkg: SSR.pkg, files: 'overlay/' + SSR.pkg}],
  }, {
    culprit: [{pkg: SSR.pkg, name: SSR.name, role: 'its comp TC_ForceBodyShape swaps itself to the end of the pawn\'s AllComps and removes itself while the comps are being ticked'}],
    decoy: [{pkg: PCP.pkg, name: PCP.name, role: 'named in every stack; enumerates the comps with foreach, which is correct as long as no comp changes the list'}],
    required: [SSR.pkg],
    faultClass: 'runtime.collection-modified (no culprit frame)',
    files: [`${SSR.pkg}/${dll}`],
    fix: 'TC_ForceBodyShape must not change parent.AllComps inside CompTick: set the body type once and then do nothing (the reference fix), or defer the removal outside the tick loop. Rebuild 1.6/Assemblies/FS_SSR_Body.dll (decompile, edit, build); a reference build is in answer/. Rewriting the profiler\'s loop only hides it.',
  });
}

// ---- h4: runtime NRE in one Mod's postfix, caused by another Mod's call (stack top names the wrong Mod) -----------
// Modelled on real 1.6 logs: EcoDialogues builds a one-pawn PlayLogEntry_Interaction (recipient null), and Cyanobot's
// Genes' postfix on that constructor reads recipient.genes and throws. EcoDialogues is not on disk, so the caller is
// Colonist Mutterings, our own small Mod of the same shape (scripts/mcp-eval/mods/colonistmutterings). Vanilla's
// PlayLogEntry_Interaction needs both pawns ("has a null pawn reference" when shown); a one-pawn line belongs in its
// own LogEntry. Reference fix: PlayLogEntry_Mutter derives from LogEntry. Needs a map: case.json `runtime`.
if (want('h4')) {
  const CG = {pkg: 'cyanobot.CyanobotsGenes', dir: `${LIB}/2889844371`, name: "Cyanobot's Genes"};
  const CM = {pkg: 'mossbrook.colonistmutterings', name: 'Colonist Mutterings'};
  const src = 'Source/ColonistMutterings.cs', dll = '1.6/Assemblies/ColonistMutterings.dll';
  const cm = path.join(OUT, '_mods', CM.pkg); fs.rmSync(cm, {recursive: true, force: true});
  fs.cpSync(path.join(import.meta.dirname, 'mods', 'colonistmutterings'), cm, {recursive: true});
  buildDll(path.join(cm, 'Source'), 'ColonistMutterings', path.join(cm, dll));
  const fixedSrc = must(must(must(raw(cm, src),
    'public class PlayLogEntry_Mutter : PlayLogEntry_Interaction\n    {\n        string text;',
    'public class PlayLogEntry_Mutter : LogEntry\n    {\n        Pawn initiator;\n        string text;'),
    ': base(InteractionDefOf.Chitchat, pawn, null, null)\n        {\n            this.text = text;',
    '\n        {\n            initiator = pawn;\n            this.text = text;'),
    '            base.ExposeData();\n            Scribe_Values.Look(ref text, "text");\n        }',
    '            base.ExposeData();\n            Scribe_References.Look(ref initiator, "initiator", saveDestroyedThings: true);\n            Scribe_Values.Look(ref text, "text");\n        }\n\n' +
    '        public override bool Concerns(Thing t) => t == initiator;\n\n        public override IEnumerable<Thing> GetConcerns() { if (initiator != null) yield return initiator; }\n\n' +
    '        public override UnityEngine.Texture2D IconFromPOV(Thing pov) => InteractionDefOf.Chitchat.GetSymbol();');
  fs.rmSync(path.join(OUT, 'h4', 'answer'), {recursive: true, force: true});
  fixfile('h4', CM.pkg, src, fixedSrc);
  const fixDir = path.join(OUT, '_build', 'h4-fix-src'); fs.rmSync(fixDir, {recursive: true, force: true}); fs.mkdirSync(fixDir, {recursive: true});
  fs.writeFileSync(path.join(fixDir, 'ColonistMutterings.cs'), fixedSrc);
  buildDll(fixDir, 'ColonistMutterings', path.join(OUT, 'h4', 'answer', CM.pkg, dll));
  emit('h4', {
    runtime: {ticks: 3000},
    add: [{pkg: CG.pkg, name: CG.name, path: CG.dir}, {pkg: CM.pkg, name: CM.name, path: cm.split(path.sep).join('/')}],
  }, {
    culprit: [{pkg: CM.pkg, name: CM.name, role: 'builds a PlayLogEntry_Interaction with a null recipient, which vanilla does not allow'}],
    decoy: [{pkg: CG.pkg, name: CG.name, role: 'top of every stack; its postfix assumes both pawns of an interaction exist, as vanilla does'}],
    required: [CM.pkg],
    faultClass: 'runtime.nre-in-other-mods-patch (wrong blame)',
    files: [`${CM.pkg}/${dll}`],
    fix: 'PlayLogEntry_Mutter must not be a PlayLogEntry_Interaction with a null recipient: derive it from LogEntry (reference in answer/, source and DLL), or give it a real recipient. Rebuild 1.6/Assemblies/ColonistMutterings.dll from the shipped source. A null check in Cyanobot\'s Genes only hides it.',
  });
}

// ---- h3: bad IL from two transpilers on one method; the error names only the second Mod's patch methods ----------------
// Modelled on the public 1.6 logs (private corpus, aggregate only): `Error during patching RimWorld.Pawn_RelationsTracker ::
// Single SecondaryLovinChanceFactor(Verse.Pawn) with: ... | trans: ...SecondaryLovinChanceFactorTranspiler ... Invalid IL
// code ... call` in 544 of 6,734 logs. That line comes from Humanoid Alien Races' AlienHarmony.Patch wrapper, which
// catches the exception and logs HAR's own patch methods, never a Mod name. The real HAR + Charmweavers pair loads cleanly
// in 1.6.4871 (host probe 2026-11-08), so the second transpiler is injected: ReSplice: Charmweavers (real Mod) gets a
// transpiler, patched from its Mod constructor (before every static constructor), that routes vanilla's race check
// `pawn.def != otherPawn.def` through a helper taking two ThingDefs. Alone it is valid IL. HAR's transpiler, applied
// after it, turns each `ldfld Thing::def` into `RaceProps.Humanlike` (a bool), so the helper call gets two bools and
// Mono's JIT rejects the method: HAR's patch fails, and the log names HAR's methods only. The agent has to list every
// patch on the method. Charmweavers ships no source: its DLL is decompiled (with references) and rebuilt.
if (want('h3')) {
  const CW = {pkg: 'ReSplice.XOTR.Charmweavers', dir: `${WS}/3131996691`, name: 'ReSplice: Charmweavers'};
  const HAR = {pkg: 'erdelf.HumanoidAlienRaces', dir: `${WS}/839005762`, name: 'Humanoid Alien Races'};
  const dll = '1.6/Assemblies/ReSpliceCharmweavers.dll';
  const tmp = path.join(OUT, '_build', 'h3-charmweavers'); fs.rmSync(tmp, {recursive: true, force: true}); fs.mkdirSync(tmp, {recursive: true});
  const G = 'D:/rimworld16/RimWorldWin64_Data/Managed';
  const refs = [G, `${WS}/2023507013/1.6/Assemblies`, `${WS}/2009463077/1.6/Assemblies`, `${WS}/2995385834/1.6/Assemblies`];   // game, VEF, Harmony, VRE Highmate
  execFileSync('ilspycmd', ['-p', '-o', tmp, ...refs.flatMap(r => ['-r', r]), `${CW.dir}/${dll}`], {stdio: 'pipe'});
  const src = path.join(tmp, 'ReSpliceCharmweavers');
  fs.rmSync(path.join(tmp, 'System.Runtime.CompilerServices'), {recursive: true, force: true});   // the publicizer below defines it
  const edit = (f, from, to) => fs.writeFileSync(path.join(src, f), must(fs.readFileSync(path.join(src, f), 'utf8').split('\r\n').join('\n'), from, to));
  // built against a publicized Assembly-CSharp: one override comes out with mixed accessibility
  edit('JobGiver_AIFollowMaster.cs', 'protected override int FollowJobExpireInterval\n\t{\n\t\tpublic get', 'public override int FollowJobExpireInterval\n\t{\n\t\tget');
  const ref = (n, p) => `<Reference Include="${n}"><HintPath>${p}</HintPath><Private>false</Private></Reference>`;
  const csproj = `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net48</TargetFramework><LangVersion>latest</LangVersion><AllowUnsafeBlocks>true</AllowUnsafeBlocks><AssemblyName>ReSpliceCharmweavers</AssemblyName><GenerateAssemblyInfo>false</GenerateAssemblyInfo><OutDir>bin/</OutDir><NoWarn>CS0618;CS8632</NoWarn></PropertyGroup><ItemGroup>${
    ['Assembly-CSharp', 'UnityEngine', 'UnityEngine.CoreModule', 'UnityEngine.IMGUIModule', 'UnityEngine.TextRenderingModule'].map(n => ref(n, `${G}/${n}.dll`)).join('')
  }${ref('0Harmony', `${refs[2]}/0Harmony.dll`)}${ref('VEF', `${refs[1]}/VEF.dll`)}${ref('VanillaRacesExpandedHighmate', `${refs[3]}/VanillaRacesExpandedHighmate.dll`)}<PackageReference Include="Krafs.Publicizer" Version="2.2.1"><PrivateAssets>all</PrivateAssets></PackageReference><Publicize Include="Assembly-CSharp" /></ItemGroup></Project>`;
  fs.writeFileSync(path.join(tmp, 'ReSpliceCharmweavers.csproj'), csproj);
  edit('ReSpliceCharmweaversMod.cs', 'using UnityEngine;', 'using HarmonyLib;\nusing UnityEngine;');
  edit('ReSpliceCharmweaversMod.cs', 'settings = GetSettings<ReSpliceCharmweaversSettings>();',
    'settings = GetSettings<ReSpliceCharmweaversSettings>();\n\t\tnew Harmony("ReSpliceCharmweavers.Mod").PatchCategory("EarlyPatches");');
  fs.writeFileSync(path.join(src, 'Pawn_RelationsTracker_SecondaryLovinChanceFactor_RaceCheck_Patch.cs'), `using System.Collections.Generic;
using System.Reflection;
using System.Reflection.Emit;
using HarmonyLib;
using RimWorld;
using Verse;

namespace ReSpliceCharmweavers;

[HarmonyPatch(typeof(Pawn_RelationsTracker), "SecondaryLovinChanceFactor")]
[HarmonyPatchCategory("EarlyPatches")]
public static class Pawn_RelationsTracker_SecondaryLovinChanceFactor_RaceCheck_Patch
{
\tpublic static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
\t{
\t\tFieldInfo defField = AccessTools.Field(typeof(Thing), "def");
\t\tMethodInfo racesDiffer = AccessTools.Method(typeof(Pawn_RelationsTracker_SecondaryLovinChanceFactor_RaceCheck_Patch), "RacesDiffer");
\t\tList<CodeInstruction> codes = new List<CodeInstruction>(instructions);
\t\tfor (int i = 1; i < codes.Count; i++)
\t\t{
\t\t\tif ((codes[i].opcode == OpCodes.Bne_Un_S || codes[i].opcode == OpCodes.Bne_Un) && codes[i - 1].LoadsField(defField))
\t\t\t{
\t\t\t\tobject target = codes[i].operand;
\t\t\t\tcodes[i] = new CodeInstruction(OpCodes.Call, racesDiffer).MoveLabelsFrom(codes[i]);
\t\t\t\tcodes.Insert(i + 1, new CodeInstruction(OpCodes.Brtrue, target));
\t\t\t\tbreak;
\t\t\t}
\t\t}
\t\treturn codes;
\t}

\t// Lovethralls of a xenotype with its own body def still count as their master's race.
\tpublic static bool RacesDiffer(ThingDef pawnRace, ThingDef otherRace)
\t{
\t\tif (pawnRace == otherRace)
\t\t{
\t\t\treturn false;
\t\t}
\t\treturn pawnRace?.race == null || otherRace?.race == null || !pawnRace.race.Humanlike || !otherRace.race.Humanlike;
\t}
}
`);
  execFileSync('dotnet', ['build', '-c', 'Release', '-nologo', '-v', 'q'], {cwd: tmp, stdio: 'pipe'});
  for (const [kind, from] of [['overlay', path.join(tmp, 'bin', 'ReSpliceCharmweavers.dll')], ['answer', `${CW.dir}/${dll}`]]) {
    fs.rmSync(path.join(OUT, 'h3', kind), {recursive: true, force: true});   // the recorded logs stay
    const f = path.join(OUT, 'h3', kind, CW.pkg, dll); fs.mkdirSync(path.dirname(f), {recursive: true}); fs.copyFileSync(from, f);
  }
  emit('h3', {
    // HAR is a framework: early in the list (after the DLCs); Charmweavers loads after VEF, at the end
    add: [{pkg: HAR.pkg, name: HAR.name, path: HAR.dir, before: 'nals.facialanimation'}, {pkg: CW.pkg, name: CW.name, path: CW.dir}],
    overlay: [{pkg: CW.pkg, files: 'overlay/' + CW.pkg}],
  }, {
    culprit: [{pkg: CW.pkg, name: CW.name, role: 'its transpiler on Pawn_RelationsTracker.SecondaryLovinChanceFactor (patched from the Mod constructor, so first) replaces the race check with a call taking two ThingDefs; HAR\'s transpiler then turns those ThingDefs into bools'}],
    decoy: [{pkg: HAR.pkg, name: HAR.name, role: 'its AlienHarmony wrapper logs the failure with its own patch methods; its transpiler is fine on vanilla IL'}],
    required: [CW.pkg],
    faultClass: 'harmony.invalid-il (transpiler conflict, no Mod named)',
    files: [`${CW.pkg}/${dll}`],
    fix: 'Charmweavers\' RaceCheck transpiler must leave the def comparison alone (back to the original postfix: the reference fix is the Workshop DLL), or match without assuming a ThingDef on the stack. Rebuild 1.6/Assemblies/ReSpliceCharmweavers.dll. Changing HAR only hides it; disabling either Mod is not a fix.',
  });
}

// ---- h6: save-load error — a Mod's patch removes vanilla Defs that an existing save uses -------------------------------
// The commonest save-load errors in the public 1.6 logs (private corpus, aggregate only; 29,134 of 40,326 logs load a
// save) are "Could not resolve reference to object with loadID" (10,305 logs) and "Could not load reference to <Def>
// named <defName>" (6,156); both name only the vanilla object or Def. Here Vanilla Hairs and Beards Retextured (base list)
// gets a patch that removes three vanilla beards "until they are redrawn" (the Mod's own way to hide a style is
// `styleTags` IgnoreMe, see its Anomaly_Hairs_Patch.xml). New games are fine; the player's save has pawns with those
// beards, so loading it logs one "Could not load reference to RimWorld.BeardDef named ..." per pawn. The save is a
// quicktest colony played 3000 ticks with the pristine base list (helper case h6save: `sandbox.mjs cases/h6save --run`,
// then copy run/h6save/userdata/Saves/Autosave-1.rws to cases/h6/save/autostart.rws); case.json `load` makes the sandbox
// load it at startup. The VM gets no save: archo's test_run never loads one, and the guest's Steam syncs saves to the
// player's real Steam Cloud.
if (want('h6')) {
  const RTX = {pkg: 'hourai.vanillahairsretex.1.6', dir: `${WS}/3626568836`, name: 'Vanilla Hairs and Beards Retextured'};
  const rel = 'Patches/Core_Beards_Patch.xml';
  const beards = ['BushyStyled', 'SideWhiskers', 'Seer'];
  const xp = `Defs/BeardDef[${beards.map(b => `defName="${b}"`).join(' or ')}]`;
  fs.mkdirSync(path.join(OUT, 'h6save'), {recursive: true});
  fs.writeFileSync(path.join(OUT, 'h6save', 'case.json'), JSON.stringify({id: 'h6save', base: '../base-mods.json', runtime: {ticks: 3000}}, null, 2));
  if (!fs.existsSync(path.join(OUT, 'h6', 'save', 'autostart.rws'))) throw new Error('h6 needs its save first: see the comment above the h6 block');
  for (const kind of ['overlay', 'answer']) fs.rmSync(path.join(OUT, 'h6', kind), {recursive: true, force: true});   // the save and the recorded logs stay
  put('h6', RTX.pkg, rel, `<?xml version="1.0" encoding="UTF-8"?>
<Patch>
    <!-- These three clip through the retextured heads: take them out until they are redrawn. -->
    <Operation Class="PatchOperationRemove">
        <xpath>${xp}</xpath>
    </Operation>
</Patch>
`);
  fixfile('h6', RTX.pkg, rel, `<?xml version="1.0" encoding="UTF-8"?>
<Patch>
    <!-- These three clip through the retextured heads: hide them until they are redrawn. -->
    <Operation Class="PatchOperationReplace">
        <xpath>${xp}/styleTags</xpath>
        <value>
            <styleTags>
                <li>IgnoreMe</li>
            </styleTags>
        </value>
    </Operation>
</Patch>
`);
  emit('h6', {load: {save: 'save/autostart.rws'}, overlay: [{pkg: RTX.pkg, files: 'overlay/' + RTX.pkg}]}, {
    culprit: [{pkg: RTX.pkg, name: RTX.name, role: 'its Patches/Core_Beards_Patch.xml removes the vanilla beards BushyStyled, SideWhiskers and Seer, which pawns in the save wear'}],
    // the log names no Mod; the natural wrong guess is the other style Mod (a removed or changed hair Mod)
    decoy: [{pkg: 'jintuzilamian.hairstyle.afumenshairstyles', name: 'AFU Men\'s Hairstyles', role: 'the other hair and beard style Mod in the list; not involved'}],
    required: [RTX.pkg],
    faultClass: 'save.def-missing (removed by another Mod\'s patch)',
    files: [`${RTX.pkg}/${rel}`],
    fix: 'Do not remove Defs that saves use: hide the beards instead (styleTags IgnoreMe, as the Mod does for the Anomaly hairs; the reference fix), or drop the operation or the file. Disabling the Mod is not a fix.',
  });
}

console.log('written to', OUT);
