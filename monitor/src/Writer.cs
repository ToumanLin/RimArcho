using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Text;
using System.Threading;

namespace ArchoMonitor
{
    internal static class Level
    {
        internal const int Info = 0, Warning = 1, Error = 2;
        internal static readonly string[] Names = { "INFO", "WARNING", "ERROR" };
    }

    // One distinct log entry (06 §7.3): the first line carries the details; later lines carry
    // the occurrences since the previous line (count) and the running total.
    internal sealed class Entry
    {
        internal string Id;
        internal int Level;
        internal string Message;
        internal Context Ctx;
        internal long Total;
        internal int Pending;
        internal bool Dirty;
    }

    // Writer owns ArchoMonitor.log: rotation, the dedupe tables, buffered writes and the flush
    // timer. Every member is safe to call from any thread.
    internal static class Writer
    {
        internal const int MaxMessage = 4 << 10, MaxDetails = 16 << 10;
        const long MaxBytes = 32L << 20;
        const int MaxExact = 8000, MaxEntries = 4000, MaxFailures = 20;

        internal static volatile bool Ready;
        internal static string Path;
        static readonly object sync = new object();
        static StreamWriter w;
        static Timer timer;
        static long bytes;
        static bool full, closed;
        static string phase = "loading";
        static int failures, distinct;
        static readonly Dictionary<string, Entry>[] exact = { new Dictionary<string, Entry>(), new Dictionary<string, Entry>(), new Dictionary<string, Entry>() };
        static readonly Dictionary<string, Entry> byPrint = new Dictionary<string, Entry>();
        static readonly List<Entry> dirty = new List<Entry>();

        internal static int EntryCount { get { lock (sync) { return distinct; } } }

        // Open rotates ArchoMonitor.log to ArchoMonitor-prev.log and starts a new file.
        internal static bool Open(string root)
        {
            try
            {
                Directory.CreateDirectory(root);
                Path = System.IO.Path.Combine(root, "ArchoMonitor.log");
                string prev = System.IO.Path.Combine(root, "ArchoMonitor-prev.log");
                Rotate(Path, prev);
                var fs = new FileStream(Path, FileMode.Create, FileAccess.Write, FileShare.ReadWrite | FileShare.Delete, 4096);
                w = new StreamWriter(fs, new UTF8Encoding(false), 64 << 10) { NewLine = "\n", AutoFlush = false };
                timer = new Timer(_ => Tick(), null, 1000, 1000);
                return true;
            }
            catch
            {
                w = null;
                return false;
            }
        }

        internal static void Rotate(string cur, string prev)
        {
            if (!File.Exists(cur))
            {
                return;
            }
            try
            {
                if (File.Exists(prev))
                {
                    File.Delete(prev);
                }
                File.Move(cur, prev);
            }
            catch
            {
                // A reader holds a handle without delete sharing: copy instead; the new file
                // is truncated on open.
                try { File.Copy(cur, prev, true); } catch { }
            }
        }

        // Seen is the hot path (06 §7.2): one lookup of the exact text; a hit only counts.
        internal static bool Seen(int level, string text)
        {
            lock (sync)
            {
                if (!exact[level].TryGetValue(text, out var e))
                {
                    return false;
                }
                Count(e);
                return true;
            }
        }

        static void Count(Entry e)
        {
            e.Total++;
            e.Pending++;
            if (!e.Dirty)
            {
                e.Dirty = true;
                dirty.Add(e);
            }
        }

        // Add records an entry the exact table did not know: either another text with the same
        // fingerprint (numbers, addresses differ) or a new entry, written at once.
        internal static void Add(int level, string text, Context ctx)
        {
            string fp = Level.Names[level] + "|" + (ctx.Inferred ?? "") + "|" + Normalize.Fingerprint(ctx.Message);
            lock (sync)
            {
                if (closed || w == null)
                {
                    return;
                }
                if (exact[level].TryGetValue(text, out var e) || byPrint.TryGetValue(fp, out e))
                {
                    Remember(level, text, e);
                    Count(e);
                    return;
                }
                if (byPrint.Count >= MaxEntries)
                {
                    FlushDirty(true);
                    byPrint.Clear();
                    foreach (var d in exact)
                    {
                        d.Clear();
                    }
                }
                e = new Entry { Id = Normalize.Hash8(fp), Level = level, Message = ctx.Message, Ctx = ctx, Total = 1 };
                byPrint[fp] = e;
                Remember(level, text, e);
                distinct++;
                WriteLine(EntryLine(e, 1, ctx.Details));
                if (level == ArchoMonitor.Level.Error)
                {
                    Flush(); // the game may crash right after an error
                }
            }
        }

        static void Remember(int level, string text, Entry e)
        {
            var d = exact[level];
            if (d.Count >= MaxExact)
            {
                d.Clear();
            }
            d[text] = e;
        }

        static void Tick()
        {
            lock (sync)
            {
                if (closed || w == null)
                {
                    return;
                }
                FlushDirty(false);
                Flush();
            }
        }

        // FlushDirty writes the counts gained since the last line of each entry; a full file
        // keeps them for the exit (06 §7.3).
        static void FlushDirty(bool closing)
        {
            if (full && !closing)
            {
                return;
            }
            foreach (var e in dirty)
            {
                if (e.Pending > 0)
                {
                    WriteLine(EntryLine(e, e.Pending, null), force: full);
                    e.Pending = 0;
                }
                e.Dirty = false;
            }
            dirty.Clear();
        }

        static void Flush()
        {
            try { w.Flush(); } catch { }
        }

        static void WriteLine(string line, bool force = false)
        {
            if (w == null || (full && !force))
            {
                return;
            }
            try
            {
                w.WriteLine(line);
                bytes += Encoding.UTF8.GetByteCount(line) + 1;
                if (!full && bytes >= MaxBytes)
                {
                    full = true;
                    w.WriteLine(new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] log is full; only counts are written from now on").Kind("full").Tail());
                }
            }
            catch { }
        }

        static string EntryLine(Entry e, int count, string details)
        {
            var j = new Json().Head(DateTime.Now, Level.Names[e.Level], e.Message);
            j.Str("details", details ?? "");
            j.Num("count", count);
            j.Ctx(e.Ctx);
            j.Str("source", "archo-monitor").Num("schema", 1).Str("id", e.Id).Num("total", e.Total);
            return j.End();
        }

        internal static void WriteSession(string session, string root, bool messages)
        {
            var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] session " + session);
            j.Str("details", "").Num("count", 1);
            j.Raw("context").Open();
            j.Str("session", session).Str("version", Monitor.Version).Str("game", Paths.Game()).Str("dataRoot", root).Str("playerLog", Paths.PlayerLog());
            j.Bool("messages", messages).List("skipped", Monitor.Skipped);
            j.Close();
            j.Str("source", "archo-monitor").Num("schema", 1).Str("kind", "session");
            lock (sync)
            {
                WriteLine(j.End());
                Flush();
            }
        }

        // The lock covers both pending counts and the boundary: a count line cannot span
        // two phases, even when the timer or a background logging thread is active.
        internal static bool WritePhase(string next, string save = null)
        {
            lock (sync)
            {
                if (closed || w == null || phase == next) return false;
                FlushDirty(true);
                var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] phase " + next);
                j.Str("details", "").Num("count", 1);
                j.Raw("context").Open().Str("phase", next);
                if (!string.IsNullOrEmpty(save)) j.Str("save", save);
                j.Close().Kind("phase");
                WriteLine(j.End(), force: true);
                phase = next;
                Flush();
                return true;
            }
        }

        // WriteSettings writes the settings record (06 §7.1): one line, kind settings, with
        // context.settings listing each Mod class's settings file.
        internal static void WriteSettings(IList<SettingsFile> files)
        {
            var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] settings " + files.Count);
            j.Str("details", "").Num("count", 1);
            j.Raw("context").Open().Raw("settings").Arr();
            foreach (var f in files)
            {
                j.Next().Open().Str("packageId", f.PackageId).Str("folder", f.Folder).Str("class", f.Class)
                    .Str("file", f.File).Str("title", f.Title).Bool("stored", f.Stored).Close();
            }
            j.EndArr().Close().Kind("settings");
            lock (sync)
            {
                if (closed || w == null) return;
                FlushDirty(true);
                WriteLine(j.End(), force: true);
                Flush();
            }
        }

        // WriteSettingsWindow writes what a settings window drew when it opened (06 §7.1): one
        // line, kind settings-window, with context.window naming the Mod's settings file and the
        // controls in drawing order.
        internal static void WriteSettingsWindow(SettingsFile f, IList<SettingsControl> controls)
        {
            var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] settings window " + f.Title);
            j.Str("details", "").Num("count", 1);
            j.Raw("context").Open().Raw("window").Open().Str("packageId", f.PackageId).Str("folder", f.Folder).Str("class", f.Class)
                .Str("file", f.File).Str("title", f.Title).Bool("full", controls.Count < SettingsWindow.MaxControls);
            j.Raw("controls").Arr();
            foreach (var c in controls)
            {
                j.Next().Open().Str("kind", c.Kind);
                if (!string.IsNullOrEmpty(c.Label)) j.Str("label", c.Label);
                if (!string.IsNullOrEmpty(c.Value)) j.Str("value", c.Value);
                if (!string.IsNullOrEmpty(c.Min)) j.Str("min", c.Min).Str("max", c.Max);
                j.Close();
            }
            j.EndArr().Close().Close().Kind("settings-window");
            lock (sync)
            {
                if (closed || w == null) return;
                FlushDirty(true);
                WriteLine(j.End(), force: true);
                Flush();
            }
        }

        // WriteHugsLib writes the HugsLib record (06 §7.1): one line, kind hugslib, with
        // context.hugslib listing each settings pack and its settings.
        internal static void WriteHugsLib(IList<HugsLibPack> packs)
        {
            var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] hugslib " + packs.Count);
            j.Str("details", "").Num("count", 1);
            j.Raw("context").Open().Raw("hugslib").Arr();
            foreach (var p in packs)
            {
                j.Next().Open().Str("id", p.Id).Str("packageId", p.PackageId).Str("folder", p.Folder).Str("title", p.Title)
                    .Bool("full", p.Handles.Count < HugsLibRecord.MaxHandles);
                j.Raw("settings").Arr();
                foreach (var h in p.Handles)
                {
                    j.Next().Open().Str("name", h.Name);
                    if (!string.IsNullOrEmpty(h.Title)) j.Str("title", h.Title);
                    if (!string.IsNullOrEmpty(h.Description)) j.Str("description", h.Description);
                    j.Str("value", h.Value).Str("default", h.Default).Str("type", h.Type);
                    if (h.Hidden) j.Bool("hidden", true);
                    if (h.Choices != null) j.List("choices", h.Choices);
                    j.Close();
                }
                j.EndArr().Close();
            }
            j.EndArr().Close().Kind("hugslib");
            lock (sync)
            {
                if (closed || w == null) return;
                FlushDirty(true);
                WriteLine(j.End(), force: true);
                Flush();
            }
        }

        // Close writes the remaining counts and the exit line (Application.quitting, ProcessExit).
        internal static void Close()
        {
            lock (sync)
            {
                if (closed || w == null)
                {
                    return;
                }
                Ready = false;
                FlushDirty(true);
                var j = new Json().Head(DateTime.Now, "INFO", "[Archo Monitor] exit").Str("details", "").Num("count", 1);
                j.Raw("context").Open().Num("entries", distinct).Close();
                j.Str("source", "archo-monitor").Num("schema", 1).Str("kind", "exit");
                WriteLine(j.End(), force: true);
                closed = true;
                try { timer?.Dispose(); } catch { }
                try { w.Flush(); w.Dispose(); } catch { }
            }
        }

        // Failure records the monitor's own error, a few times at most; it never throws.
        internal static void Failure(Exception ex)
        {
            try
            {
                lock (sync)
                {
                    if (w == null || closed || ++failures > MaxFailures)
                    {
                        return;
                    }
                    var j = new Json().Head(DateTime.Now, "WARNING", "[Archo Monitor] internal error: " + ex.GetType().Name + ": " + ex.Message);
                    j.Str("details", ex.StackTrace ?? "").Num("count", 1);
                    j.Raw("context").Open().List("relatedModIds", null).List("relatedFiles", null).Str("inferredType", "ObserverFailure").Close();
                    j.Str("source", "archo-monitor").Num("schema", 1);
                    WriteLine(j.End());
                    Flush();
                }
            }
            catch { }
        }
    }

    // Json builds one line; strings are escaped per RFC 8259, non-ASCII kept as UTF-8.
    internal sealed class Json
    {
        readonly StringBuilder sb = new StringBuilder(256);
        bool first = true;

        internal Json Head(DateTime t, string level, string message)
        {
            sb.Append('{');
            Str("timestamp", t.ToString("yyyy-MM-dd HH:mm:ss.fff", CultureInfo.InvariantCulture));
            Str("level", level);
            Str("message", message ?? "");
            return this;
        }

        internal Json Raw(string key)
        {
            Comma();
            Quote(key);
            sb.Append(':');
            first = true;
            return this;
        }

        internal Json Open()
        {
            sb.Append('{');
            first = true;
            return this;
        }

        internal Json Close()
        {
            sb.Append('}');
            first = false;
            return this;
        }

        // Arr opens an array after Raw(key); Next separates its elements.
        internal Json Arr()
        {
            sb.Append('[');
            first = true;
            return this;
        }

        internal Json EndArr()
        {
            sb.Append(']');
            first = false;
            return this;
        }

        internal Json Next()
        {
            Comma();
            return this;
        }

        internal Json Str(string key, string v)
        {
            Comma();
            Quote(key);
            sb.Append(':');
            Quote(v ?? "");
            return this;
        }

        internal Json Num(string key, long v)
        {
            Comma();
            Quote(key);
            sb.Append(':').Append(v.ToString(CultureInfo.InvariantCulture));
            return this;
        }

        internal Json Bool(string key, bool v)
        {
            Comma();
            Quote(key);
            sb.Append(':').Append(v ? "true" : "false");
            return this;
        }

        internal Json List(string key, IEnumerable<string> vs)
        {
            Comma();
            Quote(key);
            sb.Append(":[");
            bool f = true;
            if (vs != null)
            {
                foreach (var v in vs)
                {
                    if (!f)
                    {
                        sb.Append(',');
                    }
                    f = false;
                    Quote(v);
                }
            }
            sb.Append(']');
            return this;
        }

        internal Json Ctx(Context c)
        {
            Raw("context").Open();
            List("relatedModIds", c.Mods);
            List("relatedFiles", c.Files);
            Str("inferredType", c.Inferred ?? "");
            Raw("roles").Open();
            for (int i = 0; i < c.Mods.Count; i++)
            {
                Str(c.Mods[i], c.Roles[i]);
            }
            Close();
            List("refs", c.Refs);
            if (!string.IsNullOrEmpty(c.Def))
            {
                Str("def", c.Def);
            }
            return Close();
        }

        internal Json Kind(string kind)
        {
            Str("source", "archo-monitor").Num("schema", 1).Str("kind", kind);
            return this;
        }

        internal string Tail() => End();

        internal string End()
        {
            sb.Append('}');
            return sb.ToString();
        }

        void Comma()
        {
            if (!first)
            {
                sb.Append(',');
            }
            first = false;
        }

        void Quote(string s)
        {
            sb.Append('"');
            foreach (char ch in s ?? "")
            {
                switch (ch)
                {
                    case '"': sb.Append("\\\""); break;
                    case '\\': sb.Append("\\\\"); break;
                    case '\n': sb.Append("\\n"); break;
                    case '\r': sb.Append("\\r"); break;
                    case '\t': sb.Append("\\t"); break;
                    default:
                        if (ch < 0x20 || ch == (char)0x2028 || ch == (char)0x2029)
                        {
                            sb.Append("\\u").Append(((int)ch).ToString("x4", CultureInfo.InvariantCulture));
                        }
                        else
                        {
                            sb.Append(ch);
                        }
                        break;
                }
            }
            sb.Append('"');
        }
    }
}
