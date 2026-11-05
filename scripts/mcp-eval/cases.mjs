// Generates the eval cases from the pristine real Mods: only the injected faults differ.
// usage: node cases.mjs [case...] [--out dir]   (default: every case, into D:/RimArcho-test/mcp-eval/cases)
import fs from 'fs'; import path from 'path'; import {execFileSync} from 'child_process';

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

console.log('written to', OUT);
