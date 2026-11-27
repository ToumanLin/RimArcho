using System;
using System.Collections.Generic;
using System.IO;
using HarmonyLib;
using RimWorld.IO;
using Verse;

namespace ArchoMonitor
{
    // LanguageInfoFix keeps Monitor's own translations from hiding the language's metadata
    // (19 §2). The game reads LanguageInfo.xml (translator credits, language worker, native
    // name) from the first running Mod that has a folder for the language, and Monitor loads
    // before Core, so without this its Keyed-only folder wins and the language gets an empty
    // LanguageInfo: no translator list on the main menu, LanguageWorker_Default instead of the
    // language's grammar worker, the folder name instead of the native name.
    internal static class LanguageInfoFix
    {
        static ModContentPack own;
        static readonly HashSet<LanguageInfo> loaded = new HashSet<LanguageInfo>();

        // Install runs in the Mod constructor, before the game reads language metadata, and
        // whether or not the log opens: the folder is shipped either way.
        internal static void Install(ModContentPack content)
        {
            own = content;
            const string name = "LoadedLanguage.LoadMetadata";
            try
            {
                var m = AccessTools.Method(typeof(LoadedLanguage), "LoadMetadata", Type.EmptyTypes);
                if (m == null)
                {
                    Monitor.Skipped.Add(name);
                    return;
                }
                new Harmony(Monitor.PackageId + ".language").Patch(m, postfix: new HarmonyMethod(AccessTools.Method(typeof(LanguageInfoFix), nameof(Postfix))));
            }
            catch (Exception e)
            {
                Monitor.Skipped.Add(name + " (" + e.GetType().Name + ")");
            }
        }

        public static void Postfix(LoadedLanguage __instance)
        {
            try
            {
                if (own == null || __instance == null || (__instance.info != null && loaded.Contains(__instance.info)))
                {
                    return;
                }
                var mods = new List<ModContentPack>(LoadedModManager.RunningMods);
                var self = new bool[mods.Count];
                var dirs = new VirtualDirectory[mods.Count];
                for (int i = 0; i < mods.Count; i++)
                {
                    self[i] = ReferenceEquals(mods[i], own) || mods[i].PackageId == own.PackageId;
                    dirs[i] = Folder(mods[i], __instance);
                }
                int from = Source(self, Array.ConvertAll(dirs, d => d != null));
                if (from < 0)
                {
                    return;
                }
                // The game's own steps, from the Mod after Monitor.
                var dir = dirs[from];
                var info = DirectXmlLoader.ItemFromXmlFile<LanguageInfo>(dir, "LanguageInfo.xml", resolveCrossRefs: false);
                if (info.friendlyNameNative.NullOrEmpty() && dir.FileExists("FriendlyName.txt"))
                {
                    info.friendlyNameNative = dir.ReadAllText("FriendlyName.txt");
                }
                if (info.friendlyNameNative.NullOrEmpty())
                {
                    info.friendlyNameNative = __instance.folderName;
                }
                if (info.friendlyNameEnglish.NullOrEmpty())
                {
                    info.friendlyNameEnglish = __instance.folderName;
                }
                __instance.info = info;
                loaded.Add(info);
            }
            catch { }
        }

        // Source is the Mod to read the language's metadata from, or -1 to keep the game's
        // choice: only when the game's choice (the first Mod with a folder) is Monitor itself.
        internal static int Source(IList<bool> self, IList<bool> has)
        {
            int first = -1;
            for (int i = 0; i < has.Count; i++)
            {
                if (has[i])
                {
                    first = i;
                    break;
                }
            }
            if (first < 0 || !self[first])
            {
                return -1;
            }
            for (int i = first + 1; i < has.Count; i++)
            {
                if (has[i] && !self[i])
                {
                    return i;
                }
            }
            return -1;
        }

        // Folder is the Mod's folder for the language, found as the game finds it.
        static VirtualDirectory Folder(ModContentPack mod, LoadedLanguage lang)
        {
            foreach (string root in mod.foldersToLoadDescendingOrder)
            {
                string path = Path.Combine(root, "Languages");
                if (!new DirectoryInfo(path).Exists)
                {
                    continue;
                }
                foreach (VirtualDirectory d in AbstractFilesystem.GetDirectories(path, "*", SearchOption.TopDirectoryOnly))
                {
                    if (d.Name == lang.folderName || d.Name == lang.LegacyFolderName)
                    {
                        return d;
                    }
                }
            }
            return null;
        }
    }
}
