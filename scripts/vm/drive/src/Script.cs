using System;
using System.Collections.Generic;
using System.Globalization;

namespace ArchoDrive
{
    // One step of a round script. The script is plain text, one step per line (README "Game
    // rounds"):
    //
    //   out C:\Users\Public\Documents\VM-Test\rounds\r1     where drive.log goes
    //   menu                                                 be at the main menu
    //   new difficulty=Peaceful coverage=0.3                 a new colony, page by page
    //   play 120 speed=3 tabs=on                             play 120 s of real time
    //   save Round1                                          save from the game's save dialog
    //   load Round1                                          load from the load dialog
    //   delete Round1                                        the load dialog's delete button
    //   wait 5                                               do nothing for 5 s
    //   quit                                                 quit to the desktop
    //
    // Any step takes timeout=<seconds>. '#' starts a comment. Autosave is off while a script
    // runs: the VM's saves sync to the player's Steam Cloud.
    public sealed class Step
    {
        public int Line;
        public string Kind;
        public string Arg = "";
        public readonly Dictionary<string, string> Options = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        public double Timeout;

        public string Opt(string key, string fallback = null)
        {
            return Options.TryGetValue(key, out var v) ? v : fallback;
        }

        public double Seconds => double.Parse(Arg, CultureInfo.InvariantCulture);

        public override string ToString()
        {
            var s = Kind + (Arg != "" ? " " + Arg : "");
            foreach (var kv in Options) s += " " + kv.Key + "=" + kv.Value;
            return s;
        }
    }

    public sealed class Script
    {
        public string Out;
        public readonly List<Step> Steps = new List<Step>();
        public readonly List<string> Errors = new List<string>();

        // Default time limits per step, in seconds. A new colony on software Vulkan (1.7 in the
        // VM) takes many minutes; play adds its own length.
        static readonly Dictionary<string, double> Limits = new Dictionary<string, double>
        {
            { "menu", 900 }, { "new", 2400 }, { "play", 300 }, { "save", 600 }, { "load", 2400 }, { "delete", 60 }, { "wait", 60 }, { "quit", 60 },
        };

        static readonly Dictionary<string, string[]> Allowed = new Dictionary<string, string[]>
        {
            { "menu", new string[0] },
            { "new", new[] { "scenario", "storyteller", "difficulty", "coverage", "permadeath" } },
            { "play", new[] { "speed", "tabs" } },
            { "save", new string[0] },
            { "load", new string[0] },
            { "delete", new string[0] },
            { "wait", new string[0] },
            { "quit", new string[0] },
        };

        public static Script Parse(string text)
        {
            var s = new Script();
            var lines = (text ?? "").Replace("\r\n", "\n").Split('\n');
            for (int i = 0; i < lines.Length; i++)
            {
                string line = lines[i];
                int hash = line.IndexOf('#');
                if (hash >= 0) line = line.Substring(0, hash);
                line = line.Trim();
                if (line == "") continue;
                int sp = line.IndexOfAny(new[] { ' ', '\t' });
                string kind = (sp < 0 ? line : line.Substring(0, sp)).ToLowerInvariant();
                string rest = sp < 0 ? "" : line.Substring(sp + 1).Trim();
                int n = i + 1;
                if (kind == "out")
                {
                    if (rest == "") s.Errors.Add("line " + n + ": out needs a folder");
                    s.Out = rest;
                    continue;
                }
                if (!Allowed.TryGetValue(kind, out var allowed))
                {
                    s.Errors.Add("line " + n + ": unknown step '" + kind + "'");
                    continue;
                }
                var st = new Step { Line = n, Kind = kind };
                var args = new List<string>();
                foreach (var word in rest.Split(new[] { ' ', '\t' }, StringSplitOptions.RemoveEmptyEntries))
                {
                    int eq = word.IndexOf('=');
                    if (eq <= 0)
                    {
                        args.Add(word);
                        continue;
                    }
                    string key = word.Substring(0, eq), val = word.Substring(eq + 1);
                    if (key.Equals("timeout", StringComparison.OrdinalIgnoreCase))
                    {
                        if (!double.TryParse(val, NumberStyles.Float, CultureInfo.InvariantCulture, out st.Timeout) || st.Timeout <= 0)
                            s.Errors.Add("line " + n + ": timeout must be a positive number of seconds");
                        continue;
                    }
                    if (Array.IndexOf(allowed, key.ToLowerInvariant()) < 0)
                    {
                        s.Errors.Add("line " + n + ": " + kind + " has no option '" + key + "'");
                        continue;
                    }
                    st.Options[key] = val;
                }
                // Save names keep their spaces; everything else takes at most one argument.
                st.Arg = string.Join(" ", args.ToArray());
                switch (kind)
                {
                    case "play":
                    case "wait":
                        if (!double.TryParse(st.Arg, NumberStyles.Float, CultureInfo.InvariantCulture, out var secs) || secs <= 0)
                            s.Errors.Add("line " + n + ": " + kind + " needs a number of seconds");
                        else if (kind == "play" && st.Timeout == 0) st.Timeout = secs + Limits["play"];
                        else if (kind == "wait" && st.Timeout == 0) st.Timeout = secs + Limits["wait"];
                        break;
                    case "save":
                    case "load":
                    case "delete":
                        if (st.Arg == "") s.Errors.Add("line " + n + ": " + kind + " needs a save name");
                        else if (st.Arg.IndexOfAny(new[] { '\\', '/', ':', '*', '?', '"', '<', '>', '|' }) >= 0)
                            s.Errors.Add("line " + n + ": save names cannot contain \\ / : * ? \" < > |");
                        break;
                    default:
                        if (st.Arg != "") s.Errors.Add("line " + n + ": " + kind + " takes no argument");
                        break;
                }
                if (kind == "play")
                {
                    string speed = st.Opt("speed", "3");
                    if (speed != "1" && speed != "2" && speed != "3") s.Errors.Add("line " + n + ": speed is 1, 2 or 3");
                    string tabs = st.Opt("tabs", "on");
                    if (tabs != "on" && tabs != "off") s.Errors.Add("line " + n + ": tabs is on or off");
                }
                if (kind == "new" && st.Options.TryGetValue("coverage", out var cov))
                {
                    if (!double.TryParse(cov, NumberStyles.Float, CultureInfo.InvariantCulture, out var c) || c <= 0 || c > 1)
                        s.Errors.Add("line " + n + ": coverage is a fraction, e.g. 0.3");
                }
                if (kind == "new" && st.Options.TryGetValue("permadeath", out var pd) && pd != "on" && pd != "off")
                    s.Errors.Add("line " + n + ": permadeath is on or off");
                if (st.Timeout == 0) st.Timeout = Limits[kind];
                s.Steps.Add(st);
            }
            if (s.Steps.Count == 0 && s.Errors.Count == 0) s.Errors.Add("no steps");
            for (int i = 0; i < s.Steps.Count - 1; i++)
            {
                if (s.Steps[i].Kind == "quit") s.Errors.Add("line " + s.Steps[i].Line + ": quit must be the last step");
            }
            return s;
        }
    }

    // Json writes one log line; only strings, numbers and booleans.
    public sealed class Json
    {
        readonly System.Text.StringBuilder b = new System.Text.StringBuilder("{");
        bool first = true;

        public Json Str(string k, string v)
        {
            Key(k);
            Quote(v ?? "");
            return this;
        }

        public Json Num(string k, double v)
        {
            Key(k);
            b.Append(Math.Round(v, 3).ToString(CultureInfo.InvariantCulture));
            return this;
        }

        public Json Bool(string k, bool v)
        {
            Key(k);
            b.Append(v ? "true" : "false");
            return this;
        }

        public override string ToString() => b.ToString() + "}";

        void Key(string k)
        {
            if (!first) b.Append(',');
            first = false;
            Quote(k);
            b.Append(':');
        }

        void Quote(string s)
        {
            b.Append('"');
            foreach (char c in s)
            {
                switch (c)
                {
                    case '"': b.Append("\\\""); break;
                    case '\\': b.Append("\\\\"); break;
                    case '\n': b.Append("\\n"); break;
                    case '\r': b.Append("\\r"); break;
                    case '\t': b.Append("\\t"); break;
                    default:
                        if (c < ' ') b.Append("\\u").Append(((int)c).ToString("x4"));
                        else b.Append(c);
                        break;
                }
            }
            b.Append('"');
        }
    }
}
