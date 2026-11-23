using System;
using System.Collections.Generic;
using LudeonTK;
using UnityEngine;
using Verse;

namespace ArchoMonitor
{
    // ErrorWindow shows this session's errors grouped by Mod (19 §9), from the entries Monitor
    // already keeps for its log. It opens only from Monitor's settings page or the dev-mode
    // action, never by itself. The summary is built when it opens and at most once a second
    // while it stays open; closed, it costs nothing.
    internal static class ClipText
    {
        // Clip cuts s to one line that fits width, with …. Verse's Truncate measures the whole
        // text, so a very long message is cut to a few hundred characters first.
        internal static string Clip(this string s, float width, Dictionary<string, string> cache)
        {
            if (string.IsNullOrEmpty(s)) return "";
            if (s.Length > 300) s = s.Substring(0, 300);
            return s.Truncate(width, cache);
        }
    }

    internal sealed class ErrorWindow : Window
    {
        const float RowH = 28f, LineH = 24f, SubH = 22f, Indent = 20f, KindW = 110f, CopyW = 64f, MaxShown = 40;
        // One font size and few colours: grey text, three muted bar tones, amber only for what
        // came after the game stopped logging.
        static readonly Color Amber = new Color(0.90f, 0.72f, 0.42f);
        static readonly Color Dim = new Color(0.60f, 0.60f, 0.60f);
        static readonly Color Track = new Color(1f, 1f, 1f, 0.05f);
        static readonly Color LoadTone = new Color(0.50f, 0.58f, 0.68f);  // XML, Defs, patches, textures
        static readonly Color CodeTone = new Color(0.70f, 0.52f, 0.46f);  // exceptions in code
        static readonly Color OtherTone = new Color(0.48f, 0.48f, 0.48f); // warnings, other
        static readonly HashSet<string> loadKinds = new HashSet<string>
        {
            "DefConfigError", "CrossRefError", "PatchError", "XMLSyntaxError", "MissingTexture",
        };
        static readonly HashSet<string> codeKinds = new HashSet<string>
        {
            "AssemblyConflict", "TickException", "DrawException", "NullReference", "OutOfMemory",
        };

        readonly List<EntryView> views = new List<EntryView>();
        readonly Dictionary<string, string> names = new Dictionary<string, string>(StringComparer.Ordinal);
        readonly HashSet<string> open = new HashSet<string>(StringComparer.Ordinal);
        readonly Dictionary<string, string> truncCache = new Dictionary<string, string>();
        List<ModGroup> groups = new List<ModGroup>();
        bool warnings, stopped, fileFull, first = true;
        DateTime stoppedAt;
        int dropped;
        long total, after;
        float nextBuild;
        Vector2 scroll;
        string copiedId;
        float copiedUntil;

        public override Vector2 InitialSize => new Vector2(920f, 640f);

        internal static void Open()
        {
            try
            {
                var ws = Find.WindowStack;
                if (ws == null || ws.IsOpen<ErrorWindow>())
                {
                    return;
                }
                ws.Add(new ErrorWindow());
            }
            catch (Exception e)
            {
                Writer.Failure(e);
            }
        }

        // The dev-mode entry: Debug actions › Archo Monitor.
        [DebugAction("Archo Monitor", "Errors by Mod", allowedGameStates = AllowedGameStates.Entry | AllowedGameStates.Playing)]
        static void DevOpen() => Open();

        ErrorWindow()
        {
            doCloseX = true;
            draggable = true;
            resizeable = true;
            closeOnClickedOutside = false;
            absorbInputAroundWindow = false;
            forcePause = false;
            preventCameraMotion = false;
            onlyOneOfTypeAllowed = true;
            try
            {
                foreach (var m in LoadedModManager.RunningMods)
                {
                    string id = Mods.Id(m);
                    if (id != null && !names.ContainsKey(id))
                    {
                        names[id] = string.IsNullOrEmpty(m.Name) ? id : m.Name;
                    }
                }
            }
            catch { }
        }

        string Name(string id) => id != null && names.TryGetValue(id, out var n) ? n : id;

        void Build()
        {
            nextBuild = Time.realtimeSinceStartup + 1f;
            Writer.View(views, out stopped, out stoppedAt, out fileFull, out dropped);
            groups = ErrorSummary.Build(views, warnings, Name);
            total = after = 0;
            foreach (var g in groups)
            {
                total += g.Total;
                after += g.After;
            }
            if (first)
            {
                first = false;
                if (groups.Count > 0 && groups[0].Id.Length > 0)
                {
                    open.Add(groups[0].Id); // the largest starts open
                }
            }
        }

        public override void DoWindowContents(Rect inRect)
        {
            try
            {
                Draw(inRect);
            }
            catch (Exception e)
            {
                Writer.Failure(e);
                Close(false);
            }
            finally
            {
                Text.WordWrap = true;
                Text.Font = GameFont.Small;
                Text.Anchor = TextAnchor.UpperLeft;
                GUI.color = Color.white;
            }
        }

        void Draw(Rect r)
        {
            Text.WordWrap = false; // every line is one line, cut with …
            if (Time.realtimeSinceStartup >= nextBuild)
            {
                Build();
            }
            // Heading, then the switch for warnings.
            Text.Font = GameFont.Small;
            Text.Anchor = TextAnchor.MiddleLeft;
            Widgets.Label(new Rect(r.x, r.y, r.width - 200f, LineH), "ArchoMonitor_ErrorsTitle".Translate());
            Text.Anchor = TextAnchor.UpperLeft;
            bool w = warnings;
            Widgets.CheckboxLabeled(new Rect(r.xMax - 190f, r.y, 160f, LineH), "ArchoMonitor_Warnings".Translate(), ref w);
            if (w != warnings)
            {
                warnings = w;
                Build();
            }
            float y = r.y + LineH + 4f;
            y = Strip(new Rect(r.x, y, r.width, LineH));
            Widgets.DrawLineHorizontal(r.x, y + 5f, r.width);
            if (groups.Count == 0)
            {
                Empty(new Rect(r.x, y, r.width, r.yMax - y));
                return;
            }
            var outer = new Rect(r.x, y + 10f, r.width, r.yMax - y - 10f);
            float h = 0f;
            foreach (var g in groups)
            {
                h += RowH;
                if (open.Contains(g.Id))
                {
                    h += EntriesHeight(g) + 6f;
                }
            }
            var view = new Rect(0f, 0f, outer.width - 16f, h);
            Widgets.BeginScrollView(outer, ref scroll, view);
            long max = 1;
            foreach (var g in groups)
            {
                max = Math.Max(max, g.Total);
            }
            float yy = 0f;
            foreach (var g in groups)
            {
                bool visible = yy + RowH >= scroll.y && yy <= scroll.y + outer.height;
                if (visible)
                {
                    Row(new Rect(0f, yy, view.width, RowH), g, max);
                }
                yy += RowH;
                if (open.Contains(g.Id))
                {
                    yy = Entries(view.width, yy, g, outer.height) + 6f;
                }
            }
            Widgets.EndScrollView();
        }

        // Strip: the counts in one line; amber text when the game stopped logging.
        float Strip(Rect r)
        {
            int mods = 0;
            foreach (var g in groups)
            {
                if (g.Id.Length > 0) mods++;
            }
            Text.Anchor = TextAnchor.MiddleLeft;
            float x = r.x;
            x = Stat(x, r, total.ToString("N0"), warnings ? "ArchoMonitor_StatProblems".Translate() : "ArchoMonitor_StatErrors".Translate());
            x = Stat(x + 16f, r, mods.ToString(), "ArchoMonitor_StatMods".Translate());
            if (stopped)
            {
                string s = "ArchoMonitor_Stopped".Translate(after.ToString("N0"), stoppedAt.ToString("HH:mm"));
                var note = new Rect(x + 16f, r.y, Text.CalcSize(s).x + 4f, r.height);
                GUI.color = Amber;
                Widgets.Label(note, s);
                TooltipHandler.TipRegion(note, "ArchoMonitor_StoppedTip".Translate());
                x = note.xMax;
            }
            if (fileFull || dropped > 0)
            {
                GUI.color = Dim;
                string s = fileFull ? "ArchoMonitor_FileFull".Translate() : "ArchoMonitor_Dropped".Translate();
                var note = new Rect(x + 16f, r.y, r.xMax - x - 16f, r.height);
                Widgets.Label(note, s.Clip(note.width, truncCache));
                TooltipHandler.TipRegion(note, s);
            }
            GUI.color = Color.white;
            Text.Anchor = TextAnchor.UpperLeft;
            return r.yMax;
        }

        static float Stat(float x, Rect r, string n, string what)
        {
            float nw = Text.CalcSize(n).x;
            Widgets.Label(new Rect(x, r.y, nw + 2f, r.height), n);
            GUI.color = Dim;
            float ww = Text.CalcSize(what).x;
            Widgets.Label(new Rect(x + nw + 4f, r.y, ww + 2f, r.height), what);
            GUI.color = Color.white;
            return x + nw + 4f + ww;
        }

        static void Empty(Rect r)
        {
            Text.Anchor = TextAnchor.MiddleCenter;
            Widgets.Label(new Rect(r.x, r.y + r.height * 0.3f, r.width, LineH), "ArchoMonitor_NoErrors".Translate());
            GUI.color = Dim;
            Widgets.Label(new Rect(r.x, r.y + r.height * 0.3f + LineH, r.width, LineH), "ArchoMonitor_NoErrorsSub".Translate());
            GUI.color = Color.white;
            Text.Anchor = TextAnchor.UpperLeft;
        }

        // Row: a Mod's name, one bar of its kinds scaled to the largest Mod, the count and what
        // came after the game stopped logging. Clicking opens its entries.
        void Row(Rect r, ModGroup g, long max)
        {
            Widgets.DrawHighlightIfMouseover(r);
            bool isOpen = open.Contains(g.Id);
            float nameW = Mathf.Min(280f, r.width * 0.32f);
            string name = g.Id.Length == 0 ? (string)"ArchoMonitor_NoMod".Translate() : g.Name;
            Text.Anchor = TextAnchor.MiddleLeft;
            GUI.color = Dim;
            Widgets.Label(new Rect(r.x + 4f, r.y, 16f, r.height), isOpen ? "–" : "+");
            GUI.color = g.Id.Length == 0 ? Dim : Color.white;
            Widgets.Label(new Rect(r.x + 20f, r.y, nameW - 20f, r.height), name.Clip(nameW - 24f, truncCache));
            GUI.color = Color.white;
            float countW = 130f;
            var bar = new Rect(r.x + nameW + 8f, r.y + 10f, r.width - nameW - countW - 16f, r.height - 20f);
            Widgets.DrawBoxSolid(bar, Track);
            float full = Mathf.Max(3f, bar.width * g.Total / max);
            float x = bar.x;
            foreach (var k in g.Kinds)
            {
                float wk = full * k.Value / Math.Max(1, g.Total);
                if (wk <= 0f) continue;
                Widgets.DrawBoxSolid(new Rect(x, bar.y, Mathf.Max(1f, wk), bar.height), KindColor(k.Key));
                x += wk;
            }
            var cr = new Rect(bar.xMax + 8f, r.y, countW, r.height);
            string count = "×" + g.Total.ToString("N0");
            float cw = Text.CalcSize(count).x;
            Widgets.Label(cr, count);
            if (g.After > 0)
            {
                GUI.color = Amber;
                Widgets.Label(new Rect(cr.x + cw + 6f, cr.y, countW - cw - 6f, cr.height), "+" + g.After.ToString("N0"));
                GUI.color = Color.white;
            }
            Text.Anchor = TextAnchor.UpperLeft;
            TooltipHandler.TipRegion(r, RowTip(g));
            if (Widgets.ButtonInvisible(r))
            {
                if (isOpen) open.Remove(g.Id);
                else open.Add(g.Id);
            }
        }

        string RowTip(ModGroup g)
        {
            var sb = new System.Text.StringBuilder();
            sb.Append(g.Id.Length == 0 ? (string)"ArchoMonitor_NoMod".Translate() : g.Name);
            if (g.Id.Length > 0) sb.Append("  (").Append(g.Id).Append(')');
            sb.Append('\n').Append("ArchoMonitor_Distinct".Translate(g.Entries.Count.ToString())).Append('\n');
            foreach (var k in g.Kinds)
            {
                sb.Append('\n').Append(KindName(k.Key)).Append(": ").Append(k.Value.ToString("N0"));
            }
            if (g.After > 0)
            {
                sb.Append("\n\n").Append("ArchoMonitor_After".Translate(g.After.ToString("N0")));
            }
            return sb.ToString();
        }

        static bool HasLoad(EntryView e) => e.Ctx != null && (e.Ctx.Files.Count > 0 || !string.IsNullOrEmpty(e.Ctx.Def));

        static bool HasOthers(EntryView e, string primary)
        {
            var c = e.Ctx;
            if (c == null) return false;
            foreach (var m in c.Mods) if (m != primary) return true;
            return false;
        }

        static float EntryHeight(EntryView e, string primary) => LineH + (HasLoad(e) ? SubH : 0f) + (HasOthers(e, primary) ? SubH : 0f) + 4f;

        static float EntriesHeight(ModGroup g)
        {
            float h = 0f;
            int n = 0;
            foreach (var e in g.Entries)
            {
                if (n++ >= MaxShown) { h += SubH; break; }
                h += EntryHeight(e, g.Id);
            }
            return h;
        }

        float Entries(float width, float y, ModGroup g, float viewH)
        {
            int n = 0;
            foreach (var e in g.Entries)
            {
                if (n++ >= MaxShown)
                {
                    GUI.color = Dim;
                    Text.Anchor = TextAnchor.MiddleLeft;
                    Widgets.Label(new Rect(Indent + KindW, y, width - Indent - KindW, SubH), "ArchoMonitor_More".Translate((g.Entries.Count - (int)MaxShown).ToString()));
                    Text.Anchor = TextAnchor.UpperLeft;
                    GUI.color = Color.white;
                    return y + SubH;
                }
                float h = EntryHeight(e, g.Id);
                if (y + h >= scroll.y && y <= scroll.y + viewH)
                {
                    Entry(new Rect(Indent, y, width - Indent, h), e, g.Id);
                }
                y += h;
            }
            return y;
        }

        // Entry: aligned columns (kind, count, message, copy); under the message what was loading
        // and the other Mods on the stack.
        void Entry(Rect r, EntryView e, string primary)
        {
            string kind = ErrorSummary.Kind(e);
            Text.Anchor = TextAnchor.MiddleLeft;
            GUI.color = Dim;
            Widgets.Label(new Rect(r.x, r.y, KindW - 8f, LineH), KindName(kind).Clip(KindW - 8f, truncCache));
            GUI.color = Color.white;
            float x = r.x + KindW;
            const float countCol = 84f;
            string count = "×" + e.Total.ToString("N0");
            float cw = Text.CalcSize(count).x;
            Widgets.Label(new Rect(x, r.y, cw + 2f, LineH), count);
            if (e.After > 0)
            {
                string a = "+" + e.After.ToString("N0");
                var ar = new Rect(x + cw + 4f, r.y, Mathf.Max(0f, countCol - cw - 4f), LineH);
                GUI.color = Amber;
                Widgets.Label(ar, a.Clip(ar.width, truncCache));
                TooltipHandler.TipRegion(ar, "ArchoMonitor_After".Translate(e.After.ToString("N0")));
                GUI.color = Color.white;
            }
            x += countCol;
            var copy = new Rect(r.xMax - CopyW, r.y + 1f, CopyW - 4f, LineH - 2f);
            var msg = new Rect(x, r.y, copy.x - x - 8f, LineH);
            string first = FirstLine(e.Message);
            Widgets.Label(msg, first.Clip(msg.width, truncCache));
            TooltipHandler.TipRegion(msg, Tip(e));
            bool copied = copiedId == e.Id && Time.realtimeSinceStartup < copiedUntil;
            if (Widgets.ButtonText(copy, copied ? "ArchoMonitor_Copied".Translate() : "ArchoMonitor_Copy".Translate()))
            {
                GUIUtility.systemCopyBuffer = ErrorSummary.CopyText(e, Name);
                copiedId = e.Id;
                copiedUntil = Time.realtimeSinceStartup + 2f;
            }
            Text.Anchor = TextAnchor.MiddleLeft;
            float y = r.y + LineH;
            GUI.color = Dim;
            var c = e.Ctx;
            if (HasLoad(e))
            {
                string file = c.Files.Count > 0 ? ShortFile(c.Files[0]) : "";
                string s = "ArchoMonitor_Loading".Translate() + " ";
                if (file.Length > 0) s += file;
                if (c.Files.Count > 1) s += " (+" + (c.Files.Count - 1) + ")";
                if (!string.IsNullOrEmpty(c.Def)) s += (file.Length > 0 ? "  ·  " : "") + "Def " + c.Def;
                var lr = new Rect(x, y, r.xMax - x, SubH);
                Widgets.Label(lr, s.Clip(lr.width, truncCache));
                TooltipHandler.TipRegion(lr, string.Join("\n", c.Files.ToArray()) + (string.IsNullOrEmpty(c.Def) ? "" : "\nDef " + c.Def));
                y += SubH;
            }
            if (HasOthers(e, primary))
            {
                var sb = new System.Text.StringBuilder();
                sb.Append("ArchoMonitor_AlsoMods".Translate()).Append(' ');
                bool firstMod = true;
                foreach (var o in ErrorSummary.Others(c, primary))
                {
                    if (!firstMod) sb.Append(",  ");
                    firstMod = false;
                    sb.Append(Name(o.Key)).Append(" (").Append(RoleName(o.Value)).Append(')');
                }
                var lr = new Rect(x, y, r.xMax - x, SubH);
                string s = sb.ToString();
                Widgets.Label(lr, s.Clip(lr.width, truncCache));
                TooltipHandler.TipRegion(lr, s);
            }
            GUI.color = Color.white;
            Text.Anchor = TextAnchor.UpperLeft;
        }

        static string FirstLine(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            int nl = s.IndexOf('\n');
            return (nl < 0 ? s : s.Substring(0, nl)).Trim();
        }

        static string ShortFile(string p)
        {
            try { return System.IO.Path.GetFileName(p); }
            catch { return p; }
        }

        static string Tip(EntryView e)
        {
            string m = Normalize.Cut(e.Message, 1200);
            string d = e.Ctx?.Details;
            if (!string.IsNullOrEmpty(d))
            {
                m += "\n\n" + Normalize.Cut(d, 1200);
            }
            return m;
        }

        static Color KindColor(string k) => loadKinds.Contains(k) ? LoadTone : codeKinds.Contains(k) ? CodeTone : OtherTone;

        static string KindName(string k)
        {
            string key = "ArchoMonitor_Kind_" + k;
            return key.CanTranslate() ? (string)key.Translate() : k;
        }

        static string RoleName(string role)
        {
            string key = "ArchoMonitor_Role_" + role;
            return key.CanTranslate() ? (string)key.Translate() : role;
        }
    }
}
