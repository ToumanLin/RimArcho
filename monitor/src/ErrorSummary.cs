using System;
using System.Collections.Generic;

namespace ArchoMonitor
{
    // ModGroup is one row of the error window: the entries whose main Mod is Id ("" when no
    // Mod was found), with their occurrences by kind.
    internal sealed class ModGroup
    {
        internal string Id = "";
        internal string Name = "";
        internal long Total, After;
        internal readonly List<EntryView> Entries = new List<EntryView>();
        // Kinds by occurrences, largest first after Build.
        internal readonly List<KeyValuePair<string, long>> Kinds = new List<KeyValuePair<string, long>>();
    }

    // ErrorSummary groups the entries Monitor already keeps by the Mod each one points at most
    // strongly (19 §9). It is built only while the error window is open; it touches no game type,
    // so it is tested without the game.
    internal static class ErrorSummary
    {
        // Strongest first, as Context ranks roles.
        static readonly string[] rank = { "xml", "def", "patch", "stack", "text" };

        // Kind is the entry's kind for the window: its inferredType, "Warning" for a warning
        // without one, "Other" for an error without one.
        internal static string Kind(EntryView e)
        {
            string k = e.Ctx?.Inferred;
            if (!string.IsNullOrEmpty(k))
            {
                return k;
            }
            return e.Level == Level.Warning ? "Warning" : "Other";
        }

        // Primary is the Mod the entry is filed under: the strongest role, an infrastructure Mod
        // (Harmony, HugsLib, …) only when there is no other; "" when the entry names none.
        internal static string Primary(Context c)
        {
            if (c == null || c.Mods.Count == 0)
            {
                return "";
            }
            string best = null, infra = null;
            int bestRank = int.MaxValue, infraRank = int.MaxValue;
            for (int i = 0; i < c.Mods.Count; i++)
            {
                int r = Array.IndexOf(rank, c.Roles[i]);
                if (r < 0)
                {
                    r = rank.Length;
                }
                if (Mods.Infrastructure(c.Mods[i]))
                {
                    if (r < infraRank)
                    {
                        infra = c.Mods[i];
                        infraRank = r;
                    }
                }
                else if (r < bestRank)
                {
                    best = c.Mods[i];
                    bestRank = r;
                }
            }
            return best ?? infra ?? "";
        }

        // Build groups the entries of the levels shown (errors; warnings too when asked), largest
        // first; name gives a Mod's display name.
        internal static List<ModGroup> Build(IList<EntryView> entries, bool warnings, Func<string, string> name)
        {
            var byId = new Dictionary<string, ModGroup>(StringComparer.Ordinal);
            var kinds = new Dictionary<ModGroup, Dictionary<string, long>>();
            foreach (var e in entries)
            {
                if (e.Level < Level.Warning || (e.Level == Level.Warning && !warnings))
                {
                    continue;
                }
                string id = Primary(e.Ctx);
                if (!byId.TryGetValue(id, out var g))
                {
                    g = new ModGroup { Id = id, Name = id.Length == 0 ? "" : (name?.Invoke(id) ?? id) };
                    byId[id] = g;
                    kinds[g] = new Dictionary<string, long>(StringComparer.Ordinal);
                }
                g.Entries.Add(e);
                g.Total += e.Total;
                g.After += e.After;
                var ks = kinds[g];
                string k = Kind(e);
                ks.TryGetValue(k, out long n);
                ks[k] = n + e.Total;
            }
            var list = new List<ModGroup>(byId.Values);
            foreach (var g in list)
            {
                g.Kinds.AddRange(kinds[g]);
                g.Kinds.Sort((a, b) => a.Value != b.Value ? b.Value.CompareTo(a.Value) : string.CompareOrdinal(a.Key, b.Key));
                g.Entries.Sort((a, b) => a.Total != b.Total ? b.Total.CompareTo(a.Total) : string.CompareOrdinal(a.Id, b.Id));
            }
            // Largest first; the group without a Mod last, since it points nowhere.
            list.Sort((a, b) =>
            {
                if ((a.Id.Length == 0) != (b.Id.Length == 0))
                {
                    return a.Id.Length == 0 ? 1 : -1;
                }
                return a.Total != b.Total ? b.Total.CompareTo(a.Total) : string.CompareOrdinal(a.Id, b.Id);
            });
            return list;
        }

        // Others are the entry's Mods besides the one it is filed under, with how each was found.
        internal static List<KeyValuePair<string, string>> Others(Context c, string primary)
        {
            var r = new List<KeyValuePair<string, string>>();
            if (c == null)
            {
                return r;
            }
            for (int i = 0; i < c.Mods.Count; i++)
            {
                if (c.Mods[i] != primary)
                {
                    r.Add(new KeyValuePair<string, string>(c.Mods[i], c.Roles[i]));
                }
            }
            return r;
        }

        // CopyText is what "copy" puts on the clipboard: the message, the Mods and how each was
        // found, the files and Def, then the stack.
        internal static string CopyText(EntryView e, Func<string, string> name)
        {
            var sb = new System.Text.StringBuilder();
            var c = e.Ctx;
            sb.Append('[').Append(Level.Names[e.Level]).Append("] ").Append(e.Message ?? "").Append('\n');
            sb.Append("count: ").Append(e.Total);
            if (e.After > 0)
            {
                sb.Append(" (").Append(e.After).Append(" after the game stopped logging)");
            }
            sb.Append('\n');
            if (c != null)
            {
                string k = Kind(e);
                sb.Append("kind: ").Append(k).Append('\n');
                for (int i = 0; i < c.Mods.Count; i++)
                {
                    string n = name?.Invoke(c.Mods[i]);
                    sb.Append("mod: ").Append(c.Mods[i]);
                    if (!string.IsNullOrEmpty(n) && n != c.Mods[i])
                    {
                        sb.Append(" (").Append(n).Append(')');
                    }
                    sb.Append(" — ").Append(c.Roles[i]).Append('\n');
                }
                foreach (var f in c.Files)
                {
                    sb.Append("file: ").Append(f).Append('\n');
                }
                if (!string.IsNullOrEmpty(c.Def))
                {
                    sb.Append("def: ").Append(c.Def).Append('\n');
                }
                foreach (var r in c.Refs)
                {
                    sb.Append("ref: ").Append(r).Append('\n');
                }
                if (!string.IsNullOrEmpty(c.Details))
                {
                    sb.Append(c.Details).Append('\n');
                }
            }
            sb.Append("(Archo Monitor ").Append(Monitor.Version).Append(", id ").Append(e.Id).Append(')');
            return sb.ToString();
        }
    }
}
