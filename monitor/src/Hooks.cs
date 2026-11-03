using System;
using System.Reflection;
using System.Xml;
using HarmonyLib;
using UnityEngine;
using Verse;

namespace ArchoMonitor
{
    // Hooks installs the Harmony patches (03 §6.7.1). Every target is looked up by reflection;
    // a missing one is skipped and named in the session line (03 §6.7.5).
    internal static class Hooks
    {
        internal static void Install(bool messages)
        {
            var h = new Harmony(Monitor.PackageId);
            Patch(h, "Log.Error", AccessTools.Method(typeof(Log), "Error", new[] { typeof(string) }), typeof(Capture), prefix: "LogErrorPrefix", first: true);
            Patch(h, "Log.Warning", AccessTools.Method(typeof(Log), "Warning", new[] { typeof(string) }), typeof(Capture), prefix: "LogWarningPrefix", first: true);
            if (messages)
            {
                Patch(h, "Log.Message", AccessTools.Method(typeof(Log), "Message", new[] { typeof(string) }), typeof(Capture), prefix: "LogMessagePrefix", first: true);
            }
            var defFromNode = AccessTools.Method(typeof(DirectXmlLoader), "DefFromNode");
            Patch(h, "DirectXmlLoader.DefFromNode", defFromNode, typeof(XmlTracker), prefix: "DefPrefix", finalizer: "DefFinalizer");
            var newLoader = AccessTools.TypeByName("Verse.DirectXmlToObjectNew");
            if (newLoader != null) // 1.6 and later
            {
                Patch(h, "DirectXmlToObjectNew.DefFromNodeNew", AccessTools.Method(newLoader, "DefFromNodeNew"), typeof(XmlTracker), prefix: "DefPrefix", finalizer: "DefFinalizer");
            }
            Patch(h, "LoadedModManager.ApplyPatches", AccessTools.Method(typeof(LoadedModManager), "ApplyPatches"), typeof(XmlTracker), prefix: "PatchesPrefix", finalizer: "PatchesFinalizer");
            Patch(h, "PatchOperation.Apply", AccessTools.Method(typeof(PatchOperation), "Apply", new[] { typeof(XmlDocument) }), typeof(XmlTracker), prefix: "OpPrefix", finalizer: "OpFinalizer");
            Patch(h, "PatchOperation.Complete", AccessTools.Method(typeof(PatchOperation), "Complete", new[] { typeof(string) }), typeof(XmlTracker), prefix: "OpPrefix", finalizer: "OpFinalizer");
            Patch(h, "DirectXmlToObject.ValidateListNode", AccessTools.Method(typeof(DirectXmlToObject), "ValidateListNode"), typeof(XmlTracker), postfix: "ListPostfix");
            // UIRoot_Entry.Init finishes MainMenuDrawer.Init without patching its static
            // texture-owning type during the earlier mod-constructor stage.
            PhasePatch(h, "Verse.UIRoot_Entry", "Init", postfix: "Menu");
            PhasePatch(h, "Verse.SavedGameLoaderNow", "LoadGameFromSaveFileNow", prefix: "Save", args: new[] { typeof(string) });
            PhasePatch(h, "RimWorld.Planet.WorldGenerator", "GenerateWorld", prefix: "New", args: null, anyArgs: true);
            PhasePatch(h, "Verse.Game", "InitNewGame", prefix: "New");
            PhasePatch(h, "Verse.Game", "FinalizeInit", postfix: "Play");
            SettingsWindow.Install(h);
        }

        static void PhasePatch(Harmony h, string type, string method, string prefix = null, string postfix = null, Type[] args = null, bool anyArgs = false)
        {
            var t = AccessTools.TypeByName(type);
            Patch(h, type + "." + method, t == null ? null : AccessTools.Method(t, method, anyArgs ? null : args ?? Type.EmptyTypes), typeof(Phases), prefix: prefix, postfix: postfix);
        }

        static void Patch(Harmony h, string name, MethodBase target, Type host, string prefix = null, string postfix = null, string finalizer = null, bool first = false)
        {
            if (target == null)
            {
                Monitor.Skipped.Add(name);
                return;
            }
            try
            {
                h.Patch(target, prefix: Method(host, prefix, first), postfix: Method(host, postfix, false), finalizer: Method(host, finalizer, false));
            }
            catch (Exception e)
            {
                Monitor.Skipped.Add(name + " (" + e.GetType().Name + ")");
            }
        }

        static HarmonyMethod Method(Type host, string name, bool first)
        {
            if (name == null)
            {
                return null;
            }
            var hm = new HarmonyMethod(AccessTools.Method(host, name));
            if (first)
            {
                // Before any prefix that might skip the original (log silencers).
                hm.priority = Priority.First;
            }
            return hm;
        }
    }

    // EngineLog: the engine's log callback, and on Unity the stack trace levels (03 §6.7.1).
    internal static class EngineLog
    {
        internal static void Install()
        {
            try
            {
                Application.logMessageReceivedThreaded += Capture.FromEngine;
            }
            catch
            {
                Monitor.Skipped.Add("Application.logMessageReceivedThreaded");
            }
            try { Application.quitting += Writer.Close; } catch { }
            try { AppDomain.CurrentDomain.ProcessExit += (s, e) => Writer.Close(); } catch { }
#if RW_UNITY
            if (!OtherDiagnostics())
            {
                foreach (var t in new[] { LogType.Error, LogType.Exception })
                {
                    try
                    {
                        if (Application.GetStackTraceLogType(t) == StackTraceLogType.None)
                        {
                            Application.SetStackTraceLogType(t, StackTraceLogType.ScriptOnly);
                        }
                    }
                    catch { }
                }
            }
#endif
        }

#if RW_UNITY
        static bool OtherDiagnostics()
        {
            try
            {
                foreach (var m in LoadedModManager.RunningMods)
                {
                    string s = ((m.Name ?? "") + "|" + (m.PackageId ?? "")).ToLowerInvariant();
                    if (s.Contains("visualexceptions") || s.Contains("visual exceptions") || s.Contains("moderrorchecker") || s.Contains("mod error checker"))
                    {
                        return true;
                    }
                }
            }
            catch { }
            return false;
        }
#endif
    }
}
