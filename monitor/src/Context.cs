using System;
using System.Collections.Generic;
using System.Text;

namespace ArchoMonitor
{
    // Context is what one entry knows besides its text (03 §6.7.3): the mods and how each was
    // found, files, references and the inferred type.
    internal sealed class Context
    {
        internal string Message, Details, Inferred, Def;
        internal readonly List<string> Mods = new List<string>();
        internal readonly List<string> Roles = new List<string>();
        internal readonly List<string> Files = new List<string>();
        internal readonly List<string> Refs = new List<string>();
        internal bool DuringDefLoad, DuringPatches;

        // Strongest first: a mod found several ways keeps the strongest role.
        static readonly string[] rank = { "xml", "def", "patch", "stack", "text" };

        internal void AddMod(string id, string role)
        {
            if (string.IsNullOrEmpty(id))
            {
                return;
            }
            int i = Mods.IndexOf(id);
            if (i < 0)
            {
                if (Mods.Count < 16)
                {
                    Mods.Add(id);
                    Roles.Add(role);
                }
                return;
            }
            if (Array.IndexOf(rank, role) < Array.IndexOf(rank, Roles[i]))
            {
                Roles[i] = role;
            }
        }

        internal void AddFile(string path, string mod, string role)
        {
            if (!string.IsNullOrEmpty(path) && Files.Count < 16 && !Files.Contains(path))
            {
                Files.Add(path);
            }
            AddMod(mod, role);
        }

        internal void AddRef(string r)
        {
            if (!string.IsNullOrEmpty(r) && Refs.Count < 24 && !Refs.Contains(r))
            {
                Refs.Add(r);
            }
        }
    }

    internal static class Normalize
    {
        // Fingerprint: hex addresses → <HEX>, digit runs → <NUM>, whitespace runs → one space,
        // lower case (03 §6.7.3). Hand-written: it runs once per distinct message.
        internal static string Fingerprint(string m)
        {
            if (string.IsNullOrEmpty(m))
            {
                return "";
            }
            var sb = new StringBuilder(Math.Min(m.Length, 512));
            bool space = false;
            for (int i = 0; i < m.Length && sb.Length < 1024; i++)
            {
                char c = m[i];
                if (c == '0' && i + 2 < m.Length && (m[i + 1] == 'x' || m[i + 1] == 'X') && IsHex(m[i + 2]))
                {
                    i += 2;
                    while (i + 1 < m.Length && IsHex(m[i + 1]))
                    {
                        i++;
                    }
                    sb.Append("<hex>");
                    space = false;
                    continue;
                }
                if (c >= '0' && c <= '9')
                {
                    while (i + 1 < m.Length && m[i + 1] >= '0' && m[i + 1] <= '9')
                    {
                        i++;
                    }
                    sb.Append("<num>");
                    space = false;
                    continue;
                }
                if (char.IsWhiteSpace(c))
                {
                    if (!space && sb.Length > 0)
                    {
                        sb.Append(' ');
                    }
                    space = true;
                    continue;
                }
                sb.Append(char.ToLowerInvariant(c));
                space = false;
            }
            return sb.ToString().TrimEnd();
        }

        static bool IsHex(char c) => (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');

        // Hash8 is FNV-1a over UTF-16 code units, as 8 hex digits: the entry id.
        internal static string Hash8(string s)
        {
            uint h = 2166136261;
            foreach (char c in s)
            {
                h ^= c;
                h *= 16777619;
            }
            return h.ToString("x8");
        }

        // Split separates the message from a stack the text carries (mods often log
        // "msg: " + exception). The message is what precedes the first stack line.
        internal static void Split(string text, out string message, out string stack)
        {
            message = text ?? "";
            stack = "";
            int pos = 0;
            while (true)
            {
                int nl = message.IndexOf('\n', pos);
                if (nl < 0)
                {
                    break;
                }
                int s = nl + 1;
                int k = s;
                while (k < message.Length && (message[k] == ' ' || message[k] == '\t'))
                {
                    k++;
                }
                if (StartsWithAny(message, k, "at ", "(wrapper ", "[Ref ", "--- End of", "Prefixes:", "Postfixes:", "Transpilers:", "Finalizers:", "Rethrow as ")
                    || (k == s && UnityFrame(message, s)))
                {
                    stack = message.Substring(s).Trim();
                    message = message.Substring(0, nl).TrimEnd('\r', ' ', '\t');
                    break;
                }
                pos = s;
            }
            message = message.Trim();
            if (message.Length > Writer.MaxMessage)
            {
                message = message.Substring(0, Writer.MaxMessage) + "…";
            }
        }

        // UnityFrame: a Unity stack line at column 0, "Ns.Type:Method (args)".
        static bool UnityFrame(string s, int at)
        {
            int end = s.IndexOf('\n', at);
            if (end < 0)
            {
                end = s.Length;
            }
            int colon = s.IndexOf(':', at, end - at);
            int paren = s.IndexOf(" (", at, end - at, StringComparison.Ordinal);
            if (colon <= at || paren < colon)
            {
                return false;
            }
            for (int i = at; i < colon; i++)
            {
                char c = s[i];
                if (!(char.IsLetterOrDigit(c) || c == '.' || c == '_' || c == '+' || c == '`' || c == '<' || c == '>'))
                {
                    return false;
                }
            }
            return true;
        }

        static bool StartsWithAny(string s, int at, params string[] ps)
        {
            foreach (var p in ps)
            {
                if (string.CompareOrdinal(s, at, p, 0, p.Length) == 0)
                {
                    return true;
                }
            }
            return false;
        }

        internal static string Cut(string s, int max)
        {
            if (s == null)
            {
                return "";
            }
            return s.Length <= max ? s : s.Substring(0, max) + "…";
        }
    }
}
