using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Reflection;
using System.Text.RegularExpressions;
using System.Xml;
using Verse;

namespace ArchoMonitor
{
    // XmlTracker follows the XML load (03 §6.7.1): the file and def being loaded on each thread,
    // which file defines each def, which defs list which names, and which patch operations
    // target which defs. Load-time cost: a push, a pop and a dictionary write per def.
    public static class XmlTracker
    {
        internal sealed class DefSource
        {
            internal string File, Mod, Parent;
        }

        internal sealed class Use
        {
            internal string Def, File, Field, Mod;
        }

        internal sealed class PatchRef
        {
            internal PatchOperation Op;
            internal string Mod, Type;
        }

        struct Frame
        {
            internal string File, Mod, Def;
        }

        const int MaxDefs = 300000, MaxUses = 500000, UsesPerName = 8;

        internal static readonly ConcurrentDictionary<string, DefSource> Defs = new ConcurrentDictionary<string, DefSource>(StringComparer.Ordinal);
        internal static readonly ConcurrentDictionary<string, List<Use>> Uses = new ConcurrentDictionary<string, List<Use>>(StringComparer.Ordinal);
        internal static readonly ConcurrentDictionary<string, List<PatchRef>> PatchesByDef = new ConcurrentDictionary<string, List<PatchRef>>(StringComparer.Ordinal);
        static readonly Dictionary<PatchOperation, string> opMod = new Dictionary<PatchOperation, string>();
        static int uses;
        static volatile bool collected;
        internal static volatile bool Patching;

        [ThreadStatic] static Frame[] stack;
        [ThreadStatic] static int depth;
        [ThreadStatic] static LoadableXmlAsset lastAsset;
        [ThreadStatic] static string lastFile, lastMod;
        [ThreadStatic] static PatchOperation[] ops;
        [ThreadStatic] static int opDepth;

        // --- DefFromNode / DefFromNodeNew ---

        public static void DefPrefix(XmlNode node, LoadableXmlAsset loadingAsset)
        {
            try
            {
                if (stack == null)
                {
                    stack = new Frame[16];
                }
                string name = null, parent = null;
                if (node != null)
                {
                    name = node["defName"]?.InnerText?.Trim();
                    var attrs = node.Attributes;
                    if (string.IsNullOrEmpty(name))
                    {
                        name = attrs?["Name"]?.Value?.Trim();
                    }
                    parent = attrs?["ParentName"]?.Value?.Trim();
                }
                string file = null, mod = null;
                if (loadingAsset != null)
                {
                    if (!ReferenceEquals(loadingAsset, lastAsset))
                    {
                        lastAsset = loadingAsset;
                        lastFile = SafeFile(loadingAsset);
                        lastMod = Mods.Id(loadingAsset.mod);
                    }
                    file = lastFile;
                    mod = lastMod;
                }
                if (depth < stack.Length)
                {
                    stack[depth] = new Frame { File = file, Mod = mod, Def = name };
                }
                depth++;
                if (!string.IsNullOrEmpty(name) && file != null && Defs.Count < MaxDefs)
                {
                    Defs.TryAdd(name, new DefSource { File = file, Mod = mod, Parent = parent });
                }
            }
            catch { }
        }

        public static Exception DefFinalizer(Exception __exception)
        {
            if (depth > 0)
            {
                depth--;
            }
            return __exception;
        }

        static string SafeFile(LoadableXmlAsset a)
        {
            try { return a.FullFilePath; }
            catch { return a.name; }
        }

        internal static bool InDef(out string file, out string mod, out string def)
        {
            file = mod = def = null;
            if (depth <= 0 || stack == null)
            {
                return false;
            }
            var f = stack[Math.Min(depth, stack.Length) - 1];
            file = f.File;
            mod = f.Mod;
            def = f.Def;
            return true;
        }

        // --- ValidateListNode: names listed by the def being loaded ---

        public static void ListPostfix(bool __result, XmlNode listEntryNode, XmlNode listRootNode, Type listItemType)
        {
            try
            {
                if (!__result || depth <= 0 || listEntryNode == null || listItemType == null || uses >= MaxUses)
                {
                    return;
                }
                if (listItemType.IsGenericType && listItemType.GetGenericTypeDefinition() == typeof(KeyValuePair<,>))
                {
                    var args = listItemType.GetGenericArguments();
                    if (typeof(Def).IsAssignableFrom(args[0]))
                    {
                        AddUse(listEntryNode["key"]?.InnerText, listRootNode);
                    }
                    if (typeof(Def).IsAssignableFrom(args[1]))
                    {
                        AddUse(listEntryNode["value"]?.InnerText, listRootNode);
                    }
                }
                else if (typeof(Def).IsAssignableFrom(listItemType))
                {
                    AddUse(listEntryNode.InnerText, listRootNode);
                }
            }
            catch { }
        }

        static void AddUse(string name, XmlNode root)
        {
            if (string.IsNullOrWhiteSpace(name))
            {
                return;
            }
            var f = stack[Math.Min(depth, stack.Length) - 1];
            var l = Uses.GetOrAdd(name.Trim(), _ => new List<Use>(2));
            lock (l)
            {
                if (l.Count >= UsesPerName)
                {
                    return;
                }
                l.Add(new Use { Def = f.Def, File = f.File, Mod = f.Mod, Field = root?.Name });
            }
            System.Threading.Interlocked.Increment(ref uses);
        }

        // --- Patches: ApplyPatches, PatchOperation.Apply / Complete ---

        public static void PatchesPrefix()
        {
            Patching = true;
            if (collected)
            {
                return;
            }
            collected = true;
            try
            {
                foreach (var mod in LoadedModManager.RunningMods)
                {
                    string id = Mods.Id(mod);
                    foreach (var op in mod.Patches)
                    {
                        Walk(op, id, 0);
                    }
                }
            }
            catch (Exception e)
            {
                Writer.Failure(e);
            }
        }

        public static Exception PatchesFinalizer(Exception __exception)
        {
            Patching = false;
            return __exception;
        }

        public static void OpPrefix(PatchOperation __instance)
        {
            if (ops == null)
            {
                ops = new PatchOperation[16];
            }
            if (opDepth < ops.Length)
            {
                ops[opDepth] = __instance;
            }
            opDepth++;
        }

        public static Exception OpFinalizer(Exception __exception)
        {
            if (opDepth > 0)
            {
                opDepth--;
            }
            return __exception;
        }

        internal static PatchOperation CurrentOp()
        {
            if (opDepth <= 0 || ops == null)
            {
                return null;
            }
            return ops[Math.Min(opDepth, ops.Length) - 1];
        }

        internal static string ModOf(PatchOperation op)
        {
            lock (opMod)
            {
                return op != null && opMod.TryGetValue(op, out var m) ? m : null;
            }
        }

        static readonly Regex nameInXPath = new Regex(@"(?:defName|@Name)\s*=\s*[""']([^""']+)[""']|(?<![\w@])Name\s*=\s*[""']([^""']+)[""']", RegexOptions.Compiled);
        static readonly ConcurrentDictionary<Type, FieldInfo[]> fields = new ConcurrentDictionary<Type, FieldInfo[]>();

        static void Walk(PatchOperation op, string mod, int level)
        {
            if (op == null || level > 16)
            {
                return;
            }
            lock (opMod)
            {
                opMod[op] = mod;
            }
            var f = Fields(op.GetType());
            if (f[0] != null && f[0].GetValue(op) is string xpath)
            {
                foreach (Match m in nameInXPath.Matches(xpath))
                {
                    string name = m.Groups[1].Success ? m.Groups[1].Value : m.Groups[2].Value;
                    var l = PatchesByDef.GetOrAdd(name.Trim(), _ => new List<PatchRef>(1));
                    lock (l)
                    {
                        if (l.Count < 32)
                        {
                            l.Add(new PatchRef { Op = op, Mod = mod, Type = op.GetType().Name });
                        }
                    }
                }
            }
            if (f[1] != null && f[1].GetValue(op) is List<PatchOperation> children)
            {
                foreach (var c in children)
                {
                    Walk(c, mod, level + 1);
                }
            }
            for (int i = 2; i < 4; i++)
            {
                if (f[i] != null && f[i].GetValue(op) is PatchOperation c)
                {
                    Walk(c, mod, level + 1);
                }
            }
        }

        // Fields: xpath, operations, match, nomatch (null where the type has none).
        static FieldInfo[] Fields(Type t) => fields.GetOrAdd(t, x => new[]
        {
            Field(x, "xpath"), Field(x, "operations"), Field(x, "match"), Field(x, "nomatch"),
        });

        static FieldInfo Field(Type t, string name)
        {
            for (var x = t; x != null && x != typeof(object); x = x.BaseType)
            {
                var f = x.GetField(name, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly);
                if (f != null)
                {
                    return f;
                }
            }
            return null;
        }

        // ValueText is a patch's value as XML, read at error time only.
        internal static string ValueText(PatchOperation op)
        {
            try
            {
                var f = Field(op.GetType(), "value");
                object v = f?.GetValue(op);
                switch (v)
                {
                    case null: return null;
                    case XmlContainer xc: return xc.node?.OuterXml;
                    case XmlNode n: return n.OuterXml;
                    default: return v.ToString();
                }
            }
            catch
            {
                return null;
            }
        }

        internal static string SourceFile(PatchOperation op)
        {
            try { return op == null ? null : Field(op.GetType(), "sourceFile")?.GetValue(op) as string; }
            catch { return null; }
        }
    }
}
