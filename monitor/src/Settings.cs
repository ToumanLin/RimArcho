using System;
using System.Collections.Generic;
using System.Reflection;
using HarmonyLib;
using Verse;

namespace ArchoMonitor
{
    // SettingsFile is one Mod class's settings file as the game names it (19 §2).
    internal struct SettingsFile
    {
        internal string PackageId; // lower case, player-facing
        internal string Folder;    // the folder the game sees the Mod under
        internal string Class;     // the Mod subclass's simple name
        internal string File;      // the file name in Config/
        internal string Title;     // the settings window's title ("" without one)
        internal bool Stored;      // the Mod read its settings through the game (GetSettings)
    }

    // SettingsRecord writes, once per session when the main menu first opens, which settings
    // file each Mod reads and writes and its settings window's title. RimArcho uses it to keep a
    // Mod's settings when its folder name changes (09 §3) and to list Mods with settings. It
    // calls only what the game calls itself: SettingsCategory (the options list) and the file
    // name the game builds for GetSettings.
    internal static class SettingsRecord
    {
        static bool written, looked;
        static FieldInfo stored;
        static MethodInfo name;

        internal static void Write()
        {
            if (written || !Writer.Ready) return;
            written = true;
            try
            {
                var files = new List<SettingsFile>();
                foreach (Mod m in LoadedModManager.ModHandles)
                {
                    try
                    {
                        var f = Describe(m);
                        if (f.HasValue) files.Add(f.Value);
                    }
                    catch (Exception e) { Writer.Failure(e); }
                }
                Writer.WriteSettings(files);
            }
            catch (Exception e) { Writer.Failure(e); }
        }

        // Describe is m's settings file, or null when m has neither a settings window nor
        // settings read through the game.
        internal static SettingsFile? Describe(Mod m)
        {
            if (!looked)
            {
                looked = true;
                stored = AccessTools.Field(typeof(Mod), "modSettings");
                name = AccessTools.Method(typeof(LoadedModManager), "GetSettingsFilename", new[] { typeof(string), typeof(string) });
            }
            ModContentPack content = m?.Content;
            if (content == null) return null;
            string title = "";
            try { title = m.SettingsCategory() ?? ""; }
            catch { }
            bool has = stored != null && stored.GetValue(m) != null;
            if (title.Length == 0 && !has) return null;
            string folder = content.FolderName ?? "";
            string cls = m.GetType().Name;
            string file = null;
            if (name != null)
            {
                try { file = System.IO.Path.GetFileName((string)name.Invoke(null, new object[] { folder, cls })); }
                catch { }
            }
            if (string.IsNullOrEmpty(file)) file = GenText.SanitizeFilename("Mod_" + folder + "_" + cls + ".xml");
            return new SettingsFile { PackageId = Mods.Id(content) ?? "", Folder = folder, Class = cls, File = file, Title = title, Stored = has };
        }
    }
}
