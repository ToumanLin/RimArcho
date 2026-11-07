using System;
using System.IO;
using Verse;

namespace ArchoMonitor
{
    internal static class Phases
    {
        internal static void Menu()
        {
            Change("menu");
            SettingsRecord.Write();
            HugsLibRecord.Queue();
        }

        internal static void Save(string __0) { Change("save", __0); }
        internal static void New() { Change("new"); }
        internal static void Play() { Change("play"); }

        static void Change(string phase, string save = null)
        {
            if (!Writer.Ready) return;
            try
            {
                if (save != null) save = Path.GetFileName(save);
                if (!Writer.WritePhase(phase, save)) return;
                // Keep Player.log useful without the companion, including recordMessages=off.
                Log.Message("[Archo Monitor] phase " + phase);
            }
            catch (Exception e) { Writer.Failure(e); }
        }
    }
}
