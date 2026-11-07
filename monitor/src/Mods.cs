using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Reflection;
using HarmonyLib;
using Verse;

namespace ArchoMonitor
{
    // Mods maps code to the mod that loaded it (19 §2): assemblies, Harmony patch owners,
    // and type names read from stack text. Every lookup is cached; nothing here runs on the hot
    // path (only for a message seen for the first time).
    internal static class Mods
    {
        // Assemblies of the runtime, the game, Harmony and the monitor: never a mod's fault.
        static readonly string[] ignoredAssemblies =
        {
            "mscorlib", "System", "netstandard", "Microsoft", "Mono", "MonoMod", "Unity", "UnityEngine",
            "Assembly-CSharp", "0Harmony", "HarmonyLib", "Waylith", "Brutal", "ICSharpCode", "Newtonsoft",
            "NAudio", "NVorbis", "ISharpZipLib", "com.rlabrecque.steamworks.net", "UnityStubs", "ArchoMonitor",
        };

        // Infrastructure mods: on many stacks, rarely the cause; kept only when nothing else is.
        static readonly HashSet<string> infrastructure = new HashSet<string>(StringComparer.Ordinal)
        {
            "brrainz.harmony", "unlimitedhugs.hugslib", "imranfish.xmlextensions", "zetrith.prepatcher",
            "taranchuk.performanceoptimizer", "krkr.rocketman", "taranchuk.moderrorchecker", "startupprofiler.mypatches",
            Monitor.PackageId,
        };

        static readonly object sync = new object();
        static Dictionary<Assembly, string> byAssembly;
        static Dictionary<string, string> byAssemblyName;
        static HashSet<string> running;
        static readonly Dictionary<string, Type> types = new Dictionary<string, Type>(StringComparer.Ordinal);
        static readonly Dictionary<MethodBase, string[]> owners = new Dictionary<MethodBase, string[]>();
        static readonly Dictionary<string, string> byOwner = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

        internal static string Id(ModContentPack m)
        {
            string id = m?.PackageIdPlayerFacing ?? m?.PackageId;
            return string.IsNullOrWhiteSpace(id) ? null : id.ToLowerInvariant();
        }

        internal static bool Infrastructure(string id) => id != null && infrastructure.Contains(id);

        static void Build()
        {
            if (byAssembly != null)
            {
                return;
            }
            var a = new Dictionary<Assembly, string>();
            var n = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
            var r = new HashSet<string>(StringComparer.Ordinal);
            try
            {
                foreach (var mod in LoadedModManager.RunningMods)
                {
                    string id = Id(mod);
                    if (id == null)
                    {
                        continue;
                    }
                    r.Add(id);
                    var asms = mod.assemblies?.loadedAssemblies;
                    if (asms == null)
                    {
                        continue;
                    }
                    foreach (var asm in asms)
                    {
                        if (asm != null && !a.ContainsKey(asm))
                        {
                            a[asm] = id;
                            n[asm.GetName().Name] = id;
                        }
                    }
                }
            }
            catch { }
            byAssembly = a;
            byAssemblyName = n;
            running = r;
        }

        internal static bool Running(string id)
        {
            lock (sync)
            {
                Build();
                return running.Contains(id);
            }
        }

        internal static string OfAssemblyName(string name)
        {
            lock (sync)
            {
                Build();
                return byAssemblyName.TryGetValue(name, out var id) ? id : null;
            }
        }

        static bool Ignored(Assembly asm)
        {
            if (asm == null)
            {
                return true;
            }
            string n = asm.GetName().Name ?? "";
            foreach (var p in ignoredAssemblies)
            {
                if (n.StartsWith(p, StringComparison.OrdinalIgnoreCase))
                {
                    return true;
                }
            }
            return false;
        }

        internal static string OfAssembly(Assembly asm)
        {
            if (asm == null)
            {
                return null;
            }
            lock (sync)
            {
                Build();
                return byAssembly.TryGetValue(asm, out var id) ? id : null;
            }
        }

        // OfType is the mod whose assembly declares t ("" for the game and infrastructure code).
        internal static string OfType(Type t)
        {
            if (t == null || Ignored(t.Assembly))
            {
                return null;
            }
            return OfAssembly(t.Assembly);
        }

        // PatchOwners are the mods with Harmony patches on m.
        internal static string[] PatchOwners(MethodBase m)
        {
            if (m == null)
            {
                return new string[0];
            }
            lock (sync)
            {
                if (owners.TryGetValue(m, out var cached))
                {
                    return cached;
                }
            }
            var found = new List<string>();
            try
            {
                var info = Harmony.GetPatchInfo(m);
                if (info != null)
                {
                    foreach (var list in new[] { info.Prefixes, info.Postfixes, info.Transpilers, info.Finalizers })
                    {
                        if (list == null)
                        {
                            continue;
                        }
                        foreach (var p in list)
                        {
                            string id = OfAssembly(p.PatchMethod?.DeclaringType?.Assembly) ?? OfOwner(p.owner);
                            if (id != null && id != Monitor.PackageId && !found.Contains(id))
                            {
                                found.Add(id);
                            }
                        }
                    }
                }
            }
            catch { }
            var arr = found.ToArray();
            lock (sync)
            {
                owners[m] = arr;
            }
            return arr;
        }

        static string OfOwner(string owner)
        {
            if (string.IsNullOrWhiteSpace(owner))
            {
                return null;
            }
            lock (sync)
            {
                if (byOwner.TryGetValue(owner, out var c))
                {
                    return c;
                }
            }
            string found = null;
            try
            {
                foreach (var mod in LoadedModManager.RunningMods)
                {
                    string id = Id(mod);
                    if (id != null && owner.IndexOf(id, StringComparison.OrdinalIgnoreCase) >= 0)
                    {
                        found = id;
                        break;
                    }
                }
            }
            catch { }
            lock (sync)
            {
                byOwner[owner] = found;
            }
            return found;
        }

        internal static Type TypeByName(string name)
        {
            lock (sync)
            {
                if (types.TryGetValue(name, out var t))
                {
                    return t;
                }
            }
            Type found = null;
            try { found = AccessTools.TypeByName(name); } catch { }
            lock (sync)
            {
                if (types.Count < 20000)
                {
                    types[name] = found;
                }
            }
            return found;
        }

        // FromFrames walks a live stack: patch owners of each frame, then the first mod frame.
        internal static void FromFrames(StackTrace st, Context ctx)
        {
            if (st == null)
            {
                return;
            }
            string fallback = null;
            foreach (var f in st.GetFrames() ?? new StackFrame[0])
            {
                MethodBase m = null;
                try { m = Harmony.GetOriginalMethodFromStackframe(f) ?? f.GetMethod(); } catch { m = f.GetMethod(); }
                if (Visit(m, m?.DeclaringType, ctx, ref fallback))
                {
                    return;
                }
            }
            if (fallback != null && ctx.Mods.Count == 0)
            {
                ctx.AddMod(fallback, "stack");
            }
        }

        // FromText reads frames from stack text: .NET ("at Ns.Type.Method (…)"), Unity
        // ("Ns.Type:Method (…)") and Harmony replacements ("Ns.Type.Method_Patch1").
        internal static void FromText(string text, Context ctx)
        {
            if (string.IsNullOrEmpty(text))
            {
                return;
            }
            string fallback = null;
            int n = 0;
            foreach (var raw in text.Split('\n'))
            {
                if (++n > 80)
                {
                    break;
                }
                if (!Frame(raw, out string type, out string method))
                {
                    continue;
                }
                var t = TypeByName(type);
                if (t == null)
                {
                    continue;
                }
                MethodBase m = null;
                try { m = AccessTools.DeclaredMethod(t, method); } catch { }
                if (m == null)
                {
                    try
                    {
                        foreach (var x in t.GetMethods(BindingFlags.Instance | BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly))
                        {
                            if (x.Name == method)
                            {
                                m = x;
                                break;
                            }
                        }
                    }
                    catch { }
                }
                if (Visit(m, t, ctx, ref fallback))
                {
                    return;
                }
            }
            if (fallback != null && ctx.Mods.Count == 0)
            {
                ctx.AddMod(fallback, "stack");
            }
        }

        // Visit adds a frame's patch owners and its own mod; true once a non-infrastructure mod
        // frame is found (the walk stops there).
        static bool Visit(MethodBase m, Type t, Context ctx, ref string fallback)
        {
            // Patches on the logger itself (log filters, this monitor) say nothing about the error.
            if (m != null && m.DeclaringType != typeof(Log))
            {
                foreach (var id in PatchOwners(m))
                {
                    if (Infrastructure(id))
                    {
                        fallback = fallback ?? id;
                    }
                    else
                    {
                        ctx.AddMod(id, "patch");
                    }
                }
            }
            string own = OfType(t);
            if (own == null)
            {
                return false;
            }
            if (Infrastructure(own))
            {
                fallback = fallback ?? own;
                return false;
            }
            ctx.AddMod(own, "stack");
            return true;
        }

        static string WithoutBrackets(string s)
        {
            if (s.IndexOf('[') < 0)
            {
                return s;
            }
            var sb = new System.Text.StringBuilder(s.Length);
            int depth = 0;
            foreach (char c in s)
            {
                if (c == '[')
                {
                    depth++;
                }
                else if (c == ']')
                {
                    depth = Math.Max(0, depth - 1);
                }
                else if (depth == 0)
                {
                    sb.Append(c);
                }
            }
            return sb.ToString();
        }

        internal static bool Frame(string line, out string type, out string method)
        {
            type = method = null;
            string s = line.Trim();
            if (s.StartsWith("at ", StringComparison.Ordinal))
            {
                s = s.Substring(3);
            }
            if (s.StartsWith("(wrapper ", StringComparison.Ordinal))
            {
                int close = s.IndexOf(')');
                if (close < 0)
                {
                    return false;
                }
                s = s.Substring(close + 1).Trim();
            }
            int paren = s.IndexOf('(');
            if (paren > 0)
            {
                s = s.Substring(0, paren).Trim();
            }
            s = WithoutBrackets(s); // generic arguments: Foo`1[T].Bar[U]
            s = s.TrimStart('.');
            int colon = s.IndexOf(':');
            int cut;
            if (colon > 0)
            {
                type = s.Substring(0, colon);
                method = s.Substring(colon + 1).Trim();
            }
            else if ((cut = s.LastIndexOf('.')) > 0)
            {
                // ".ctor" and ".cctor" keep their dot
                if (cut > 0 && s[cut - 1] == '.')
                {
                    cut--;
                }
                type = s.Substring(0, cut);
                method = s.Substring(cut + 1);
            }
            else
            {
                return false;
            }
            int p = method.IndexOf("_Patch", StringComparison.Ordinal);
            if (p > 0)
            {
                method = method.Substring(0, p);
                // Harmony 2 names a replacement "Ns.Type.Ns.Type.Method_Patch1": the type repeats
                int half = type.Length / 2;
                if (type.Length % 2 == 1 && type.Substring(0, half) == type.Substring(half + 1))
                {
                    type = type.Substring(0, half);
                }
            }
            if (type.Length == 0 || method.Length == 0 || type.IndexOf(' ') >= 0)
            {
                return false;
            }
            return true;
        }
    }
}
