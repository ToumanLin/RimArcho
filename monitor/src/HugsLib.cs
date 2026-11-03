using System;
using System.Collections;
using System.Collections.Generic;
using HarmonyLib;
using Verse;

namespace ArchoMonitor
{
    // HugsLibPack is one HugsLib settings pack: its identifier, the Mod it belongs to and its
    // settings (03 §6.7.1).
    internal struct HugsLibPack
    {
        internal string Id;        // the pack's identifier: its element in HugsLib/ModSettings.xml
        internal string PackageId; // the owning Mod ("" when not found)
        internal string Folder;
        internal string Title;     // the entry's name in HugsLib's settings dialog
        internal List<HugsLibHandle> Handles;
    }

    internal struct HugsLibHandle
    {
        internal string Name;  // the element in the pack's element
        internal string Title, Description;
        internal string Value, Default; // as HugsLib writes them (ToString)
        internal string Type;  // the value type's simple name
        internal bool Hidden;  // never shown in the dialog
        internal string[] Choices; // an enum's names; null otherwise
    }

    // HugsLibRecord writes, once per session after the start-up work queued with the main menu
    // has run (HugsLib creates its settings then), every HugsLib settings pack with each
    // setting's name, title, description, value, default and type. HugsLib keeps its settings
    // in its own file and dialog, so neither the settings record nor the window record sees
    // them. HugsLib is optional: everything is found by reflection, and only property getters
    // run.
    internal static class HugsLibRecord
    {
        internal const int MaxHandles = 300;
        const int MaxText = 300, MaxChoices = 50;
        static bool queued;

        internal static void Queue()
        {
            if (queued || !Writer.Ready) return;
            queued = true;
            if (AccessTools.TypeByName("HugsLib.HugsLibController") == null) return;
            try { LongEventHandler.ExecuteWhenFinished(Write); }
            catch (Exception e) { Writer.Failure(e); }
        }

        static void Write()
        {
            try
            {
                var controller = AccessTools.TypeByName("HugsLib.HugsLibController");
                object manager = Static(controller, "SettingsManager");
                if (!(Prop(manager, "ModSettingsPacks") is IEnumerable packs)) return;
                // the Mod each settings identifier belongs to
                var owners = new Dictionary<string, ModContentPack>();
                object instance = Static(controller, "Instance");
                if (instance != null && AccessTools.Field(controller, "childMods")?.GetValue(instance) is IEnumerable mods)
                {
                    foreach (object m in mods)
                    {
                        if (Prop(m, "SettingsIdentifier") is string id && Prop(m, "ModContentPack") is ModContentPack p) owners[id] = p;
                    }
                }
                // HugsLib's own settings are not a child Mod's
                if (!owners.ContainsKey("HugsLib") && Static(controller, "OwnContentPack") is ModContentPack own) owners["HugsLib"] = own;
                var list = new List<HugsLibPack>();
                foreach (object pack in packs)
                {
                    if (!(Prop(pack, "ModId") is string id) || id.Length == 0) continue;
                    owners.TryGetValue(id, out var content);
                    var hp = new HugsLibPack
                    {
                        Id = id, Title = Cut(Prop(pack, "EntryName") as string), Handles = new List<HugsLibHandle>(),
                        PackageId = content == null ? "" : Mods.Id(content) ?? "", Folder = content?.FolderName ?? "",
                    };
                    if (Prop(pack, "Handles") is IEnumerable handles)
                    {
                        foreach (object h in handles)
                        {
                            if (hp.Handles.Count >= MaxHandles) break;
                            var type = Prop(h, "ValueType") as Type;
                            hp.Handles.Add(new HugsLibHandle
                            {
                                Name = Prop(h, "Name") as string ?? "",
                                Title = Cut(Prop(h, "Title") as string),
                                Description = Cut(Prop(h, "Description") as string),
                                Value = Cut(Text(Prop(h, "Value"))),
                                Default = Cut(Text(Prop(h, "DefaultValue"))),
                                Type = type?.Name ?? "",
                                Choices = Choices(type),
                                Hidden = Prop(h, "NeverVisible") is bool b && b,
                            });
                        }
                    }
                    list.Add(hp);
                }
                Writer.WriteHugsLib(list);
            }
            catch (Exception e) { Writer.Failure(e); }
        }

        static object Static(Type t, string name)
        {
            try { return t == null ? null : AccessTools.Property(t, name)?.GetValue(null, null); }
            catch { return null; }
        }

        static object Prop(object o, string name)
        {
            try { return o == null ? null : AccessTools.Property(o.GetType(), name)?.GetValue(o, null); }
            catch { return null; }
        }

        // Choices is an enum's names (at most MaxChoices, else none); null for other types.
        static string[] Choices(Type t)
        {
            try
            {
                if (t == null || !t.IsEnum) return null;
                string[] names = Enum.GetNames(t);
                return names.Length <= MaxChoices ? names : null;
            }
            catch { return null; }
        }

        static string Text(object v)
        {
            try { return v?.ToString() ?? ""; }
            catch { return ""; }
        }

        static string Cut(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            s = s.Trim();
            return s.Length > MaxText ? s.Substring(0, MaxText) : s;
        }
    }
}
