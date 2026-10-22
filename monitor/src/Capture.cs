using System;
using System.Diagnostics;
using System.Text.RegularExpressions;
using UnityEngine;
using Verse;

namespace ArchoMonitor
{
    // Capture receives every log call (03 §6.7.1). The hot path is Writer.Seen: one lookup of
    // the exact text. Only a message seen for the first time is analysed.
    public static class Capture
    {
        [ThreadStatic] static int busy;
        [ThreadStatic] static string lastVerse;

        public static void LogErrorPrefix(string text) => FromVerse(Level.Error, text);
        public static void LogWarningPrefix(string text) => FromVerse(Level.Warning, text);
        public static void LogMessagePrefix(string text) => FromVerse(Level.Info, text);

        static void FromVerse(int level, string text)
        {
            if (busy > 0 || !Writer.Ready || text == null)
            {
                return;
            }
            lastVerse = text;
            busy++;
            try
            {
                if (!Writer.Seen(level, text))
                {
                    Observe(level, text, null, true);
                }
            }
            catch (Exception e)
            {
                Writer.Failure(e);
            }
            finally
            {
                busy--;
            }
        }

        // FromEngine: the engine's log callback, for errors that bypass Verse.Log.
        internal static void FromEngine(string condition, string stackTrace, LogType type)
        {
            if (type != LogType.Exception && type != LogType.Error && type != LogType.Assert)
            {
                return;
            }
            if (busy > 0 || !Writer.Ready || condition == null)
            {
                return;
            }
            // Verse.Log forwards its text to the engine: already counted by the prefix.
            if (lastVerse != null && (ReferenceEquals(condition, lastVerse) || condition == lastVerse))
            {
                lastVerse = null;
                return;
            }
            if (stackTrace != null && stackTrace.IndexOf("Verse.Log", StringComparison.Ordinal) >= 0)
            {
                return;
            }
            busy++;
            try
            {
                string key = string.IsNullOrEmpty(stackTrace) ? condition : condition + "\n" + stackTrace;
                if (!Writer.Seen(Level.Error, key))
                {
                    Observe(Level.Error, key, null, false);
                }
            }
            catch (Exception e)
            {
                Writer.Failure(e);
            }
            finally
            {
                busy--;
            }
        }

        static void Observe(int level, string text, string provided, bool live)
        {
            var ctx = new Context();
            Normalize.Split(text, out ctx.Message, out string stack);
            if (!string.IsNullOrEmpty(provided))
            {
                stack = string.IsNullOrEmpty(stack) ? provided : stack + "\n" + provided;
            }
            StackTrace here = null;
            if (live && level >= Level.Warning)
            {
                // The caller of Verse.Log; Caller() drops the monitor's and the logger's frames.
                try { here = new StackTrace(1, false); } catch { }
            }
            if (!string.IsNullOrEmpty(stack))
            {
                Mods.FromText(stack, ctx);
            }
            if (ctx.Mods.Count == 0 && here != null)
            {
                Mods.FromFrames(here, ctx);
            }
            if (string.IsNullOrEmpty(stack) && here != null)
            {
                stack = Caller(here);
            }
            ctx.Details = Normalize.Cut(stack, Writer.MaxDetails);
            Enrich.Apply(ctx);
            ctx.Inferred = Infer(ctx);
            Writer.Add(level, text, ctx);
        }

        // Caller formats a live stack from the code that called Verse.Log: the monitor's own
        // frames and the logger's are dropped, so they never look like the culprit.
        internal static string Caller(StackTrace st)
        {
            var sb = new System.Text.StringBuilder();
            bool started = false;
            var self = typeof(Capture).Assembly;
            foreach (var f in st.GetFrames() ?? new StackFrame[0])
            {
                System.Reflection.MethodBase m = null;
                try { m = HarmonyLib.Harmony.GetOriginalMethodFromStackframe(f) ?? f.GetMethod(); } catch { m = f.GetMethod(); }
                var t = m?.DeclaringType;
                if (!started)
                {
                    if (t == null || t.Assembly == self || t == typeof(Log))
                    {
                        continue;
                    }
                    started = true;
                }
                sb.Append("  at ").Append(t == null ? "?" : t.FullName).Append('.').Append(m?.Name ?? "?").Append(" ()\n");
                if (sb.Length > Writer.MaxDetails)
                {
                    break;
                }
            }
            return sb.ToString().TrimEnd('\n');
        }

        // Infer is the entry's inferredType (03 §6.7.1); the first rule that matches wins.
        internal static string Infer(Context c)
        {
            string m = c.Message ?? "";
            if (m.StartsWith("Config error in ", StringComparison.Ordinal) || m.StartsWith("Exception in ConfigErrors() of ", StringComparison.Ordinal))
            {
                return "DefConfigError";
            }
            if (m.Contains("Could not resolve cross-reference") || m.Contains("Could not resolve reference to object with loadID"))
            {
                return "CrossRefError";
            }
            if (c.DuringPatches || m.Contains("Patch operation"))
            {
                return "PatchError";
            }
            if (c.DuringDefLoad)
            {
                return "XMLSyntaxError";
            }
            if (m.Contains("MissingMethodException") || m.Contains("TypeLoadException") || m.Contains("ReflectionTypeLoadException") || m.Contains("MissingFieldException"))
            {
                return "AssemblyConflict";
            }
            if (m.Contains("Exception ticking"))
            {
                return "TickException";
            }
            if (m.Contains("Exception drawing"))
            {
                return "DrawException";
            }
            if (m.Contains("NullReferenceException"))
            {
                return "NullReference";
            }
            if (m.Contains("OutOfMemoryException") || m.Contains("Could not allocate memory"))
            {
                return "OutOfMemory";
            }
            if (m.Contains("Failed to find any texture") || m.Contains("Could not load Texture2D") || m.Contains("Could not load UnityEngine.Texture2D"))
            {
                return "MissingTexture";
            }
            return "";
        }
    }

    // Enrich adds what the XML tracker knows (03 §6.7.1): the first rule with a result wins for
    // files; the flags for inferredType are always set.
    internal static class Enrich
    {
        static readonly Regex pkgRe = new Regex(@"\[([a-zA-Z0-9\-_]+(?:\.[a-zA-Z0-9\-_]+)+)\]", RegexOptions.Compiled);
        static readonly Regex asmRe = new Regex(@"assembly name:\s*([a-zA-Z0-9\-_.]+)", RegexOptions.Compiled | RegexOptions.IgnoreCase);
        const string configPrefix = "Config error in ", configErrorsPrefix = "Exception in ConfigErrors() of ";

        internal static void Apply(Context c)
        {
            string m = c.Message ?? "";
            bool inDef = XmlTracker.InDef(out string file, out string mod, out string def);
            var op = XmlTracker.CurrentOp();
            c.DuringDefLoad = inDef;
            c.DuringPatches = XmlTracker.Patching || op != null;

            bool done = false;
            if (m.StartsWith(configPrefix, StringComparison.Ordinal) || m.StartsWith(configErrorsPrefix, StringComparison.Ordinal))
            {
                int start = m.StartsWith(configPrefix, StringComparison.Ordinal) ? configPrefix.Length : configErrorsPrefix.Length;
                int colon = m.IndexOf(':', start);
                if (colon > start)
                {
                    done = DefConfig(m.Substring(start, colon - start).Trim(), c);
                }
            }
            else if (m.Contains("Could not resolve cross-reference") || m.Contains("Could not resolve reference to object with loadID"))
            {
                done = CrossRef(m, c);
            }
            if (!done && op != null)
            {
                string pmod = XmlTracker.ModOf(op);
                c.AddFile(XmlTracker.SourceFile(op), pmod, "xml");
                done = pmod != null;
            }
            if (!done && inDef)
            {
                c.AddFile(file, mod, "xml");
                c.Def = def;
            }
            // Package ids and assembly names the text names.
            string all = m + "\n" + (c.Details ?? "");
            foreach (Match x in pkgRe.Matches(all))
            {
                string id = x.Groups[1].Value.ToLowerInvariant();
                if (Mods.Running(id))
                {
                    c.AddMod(id, "text");
                }
            }
            var a = asmRe.Match(all);
            if (a.Success)
            {
                c.AddMod(Mods.OfAssemblyName(a.Groups[1].Value), "text");
            }
        }

        static bool DefConfig(string name, Context c)
        {
            c.Def = name;
            bool any = false;
            string cur = name;
            for (int i = 0; i < 11 && !string.IsNullOrEmpty(cur); i++)
            {
                if (XmlTracker.Defs.TryGetValue(cur, out var src))
                {
                    c.AddFile(src.File, src.Mod, i == 0 ? "xml" : "def");
                    any = true;
                }
                if (XmlTracker.PatchesByDef.TryGetValue(cur, out var ps))
                {
                    lock (ps)
                    {
                        foreach (var p in ps)
                        {
                            c.AddRef("patch:" + cur + ":" + p.Type + "@" + p.Mod);
                            c.AddMod(p.Mod, "patch");
                            any = true;
                        }
                    }
                }
                string next = src?.Parent;
                if (next == cur)
                {
                    break;
                }
                cur = next;
            }
            if (!any)
            {
                c.AddRef("def:" + name);
            }
            return any;
        }

        static bool CrossRef(string m, Context c)
        {
            int at = m.IndexOf(" named ", StringComparison.Ordinal);
            bool loadId = false;
            if (at >= 0)
            {
                at += " named ".Length;
            }
            else if ((at = m.IndexOf("loadID ", StringComparison.OrdinalIgnoreCase)) >= 0)
            {
                at += "loadID ".Length;
                loadId = true;
            }
            else
            {
                return false;
            }
            int end = m.IndexOfAny(new[] { ' ', '\n', '\r', ',', ';', ')', ']' }, at);
            string token = (end < 0 ? m.Substring(at) : m.Substring(at, end - at)).Trim();
            if (token.Length == 0)
            {
                return false;
            }
            if (loadId)
            {
                c.AddRef("loadID:" + token);
                return true;
            }
            bool any = false;
            if (XmlTracker.Uses.TryGetValue(token, out var uses))
            {
                lock (uses)
                {
                    foreach (var u in uses)
                    {
                        c.AddFile(u.File, u.Mod, "xml");
                        c.AddRef("xref:" + token + ":" + u.Def + "/" + u.Field);
                        any = true;
                        if (u.Def != null && XmlTracker.PatchesByDef.TryGetValue(u.Def, out var ps))
                        {
                            lock (ps)
                            {
                                foreach (var p in ps)
                                {
                                    string v = XmlTracker.ValueText(p.Op);
                                    if (v != null && v.IndexOf(token, StringComparison.OrdinalIgnoreCase) >= 0)
                                    {
                                        c.AddRef("patch:" + u.Def + ":" + p.Type + "@" + p.Mod);
                                        c.AddMod(p.Mod, "patch");
                                    }
                                }
                            }
                        }
                    }
                }
            }
            return any;
        }
    }
}
