using System;
using System.IO;
using System.Reflection;
using UnityEngine;
using Verse;

namespace ArchoMonitor
{
    // Archo Monitor (RimArcho docs/specs 06 §7): writes what Player.log cannot say — the file
    // and def being loaded, the mods on the stack, and errors after the game stops logging — to
    // <save data root>/ArchoMonitor.log, one JSON object per line.

    public class MonitorSettings : ModSettings
    {
        // Info messages are the most frequent; Log.Message is hooked only when this is on, from
        // the next start (06 §7.1).
        public bool recordMessages;

        public override void ExposeData()
        {
            base.ExposeData();
            Scribe_Values.Look(ref recordMessages, "recordMessages", false);
        }
    }

    public class ArchoMonitorMod : Mod
    {
        internal static MonitorSettings Settings;

        public ArchoMonitorMod(ModContentPack content) : base(content)
        {
            try { Settings = GetSettings<MonitorSettings>(); }
            catch { Settings = new MonitorSettings(); }
            Monitor.Start(Settings);
        }

        public override string SettingsCategory() => "Archo Monitor";

        public override void DoSettingsWindowContents(Rect inRect)
        {
            var l = new Listing_Standard();
            l.Begin(inRect);
            l.CheckboxLabeled("ArchoMonitor_RecordMessages".Translate(), ref Settings.recordMessages,
                "ArchoMonitor_RecordMessagesTip".Translate(), 0f, 1f);
            l.Gap(12f);
            l.Label("ArchoMonitor_Log".Translate() + ": " + (Writer.Path ?? "-"));
            l.Label("ArchoMonitor_Entries".Translate() + ": " + Writer.EntryCount);
            if (Monitor.Skipped.Count > 0)
            {
                l.Label("ArchoMonitor_HooksNotInstalled".Translate() + ": " + string.Join(", ", Monitor.Skipped.ToArray()));
            }
            l.End();
        }
    }

    internal static class Monitor
    {
        internal const string PackageId = "toumanlin.archomonitor";
        internal static string Version = "0.0.0";
        internal static readonly System.Collections.Generic.List<string> Skipped = new System.Collections.Generic.List<string>();
        static bool started;

        internal static void Start(MonitorSettings settings)
        {
            if (started)
            {
                return;
            }
            started = true;
            try
            {
                Version = ReadVersion();
                string root = Paths.DataRoot();
                if (!Writer.Open(root))
                {
                    return;
                }
                Hooks.Install(settings != null && settings.recordMessages);
                EngineLog.Install();
                string session = Guid.NewGuid().ToString("N").Substring(0, 12);
                Writer.WriteSession(session, root, settings != null && settings.recordMessages);
                Writer.Ready = true;
                // The marker that pairs Player.log with this file (06 §7.3).
                Log.Message("[Archo Monitor] " + Version + " session " + session + ": " + Writer.Path);
            }
            catch (Exception e)
            {
                Writer.Failure(e);
            }
        }

        static string ReadVersion()
        {
            var a = typeof(Monitor).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>();
            string v = a?.InformationalVersion ?? "0.0.0";
            int plus = v.IndexOf('+');
            return plus > 0 ? v.Substring(0, plus) : v;
        }
    }

    internal static class Paths
    {
        // The save data root: GenFilePaths.SaveDataFolderPath follows -savedatafolder, so a
        // RimArcho pack writes into <pack>/userdata (10 §3).
        internal static string DataRoot()
        {
            try
            {
                string p = GenFilePaths.SaveDataFolderPath;
                if (!string.IsNullOrWhiteSpace(p))
                {
                    return Full(p);
                }
            }
            catch { }
            string arg = SaveDataArg(Environment.GetCommandLineArgs());
            if (!string.IsNullOrWhiteSpace(arg))
            {
                return Full(arg);
            }
            return Full(Application.persistentDataPath);
        }

        // SaveDataArg reads -savedatafolder=<dir>, -savedatafolder:<dir> or -savedatafolder <dir>.
        internal static string SaveDataArg(string[] args)
        {
            const string flag = "-savedatafolder";
            for (int i = 0; args != null && i < args.Length; i++)
            {
                string a = args[i];
                if (a == null || !a.StartsWith(flag, StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }
                if (a.Length > flag.Length && (a[flag.Length] == '=' || a[flag.Length] == ':'))
                {
                    return a.Substring(flag.Length + 1).Trim().Trim('"');
                }
                if (a.Length == flag.Length && i + 1 < args.Length && !args[i + 1].StartsWith("-", StringComparison.Ordinal))
                {
                    return args[i + 1].Trim().Trim('"');
                }
            }
            return null;
        }

        static string Full(string p)
        {
            try { return System.IO.Path.GetFullPath(p.Trim().Trim('"')); }
            catch { return p; }
        }

        internal static string PlayerLog()
        {
            try { return Application.consoleLogPath ?? ""; }
            catch { return ""; }
        }

        internal static string Game()
        {
            try { return RimWorld.VersionControl.CurrentVersionStringWithRev; }
            catch { return ""; }
        }
    }
}
