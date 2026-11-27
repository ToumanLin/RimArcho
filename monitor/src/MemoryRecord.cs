using System;
using System.Diagnostics;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Threading;
using Verse;

namespace ArchoMonitor
{
    // MemoryRecord writes the game's own memory (19 §2, Monitor >= 0.13.0): when the menu or
    // play phase begins and once more SettleMs later if that phase still holds, one line of
    // kind memory with the process's working set, private bytes, their peaks since start and,
    // on 1.7, the device-local VRAM the driver reports. RimArcho compares it with its estimate
    // for the list (16 §2.17). Numbers only; one system call each, never per frame.
    internal static class MemoryRecord
    {
        internal const int SettleMs = 60000;
        const int MaxLines = 24; // phase changes are few; a long session cannot fill the log
        static int lines;
        static int generation;
        static Timer settle;

        internal static void At(string phase)
        {
            try
            {
                int g = Interlocked.Increment(ref generation);
                Write(phase, 0);
                var t = new Timer(_ =>
                {
                    if (Volatile.Read(ref generation) == g) Write(phase, SettleMs / 1000);
                }, null, SettleMs, Timeout.Infinite);
                Interlocked.Exchange(ref settle, t)?.Dispose();
            }
            catch (Exception e) { Writer.Failure(e); }
        }

        static void Write(string phase, int after)
        {
            try
            {
                if (!Writer.Ready || Interlocked.Increment(ref lines) > MaxLines) return;
                var s = Read();
                if (s.WorkingSet <= 0) return;
                Writer.WriteMemory(phase, after, s, Mods());
            }
            catch (Exception e) { Writer.Failure(e); }
        }

        internal struct Sample
        {
            // PrivateWorkingSet: Task Manager's Memory column (0 before Windows 10 1809);
            // WorkingSet also counts shared pages such as 1.7's mapped texture cache.
            internal long PrivateWorkingSet, WorkingSet, Private, PeakWorkingSet, PeakPrivate, VRAM;
        }

        [StructLayout(LayoutKind.Sequential)]
        struct Counters
        {
            internal uint cb, PageFaultCount;
            internal UIntPtr PeakWorkingSetSize, WorkingSetSize;
            internal UIntPtr QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage, QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage;
            internal UIntPtr PagefileUsage, PeakPagefileUsage, PrivateUsage;
            internal UIntPtr PrivateWorkingSetSize; // PROCESS_MEMORY_COUNTERS_EX2 (Windows 10 1809+)
            internal ulong SharedCommitUsage;
        }

        [DllImport("kernel32.dll")]
        static extern IntPtr GetCurrentProcess();

        [DllImport("kernel32.dll", EntryPoint = "K32GetProcessMemoryInfo")]
        static extern bool GetProcessMemoryInfo(IntPtr process, out Counters c, uint cb);

        static bool noKernel32;

        internal static Sample Read()
        {
            var s = new Sample();
            if (!noKernel32)
            {
                try
                {
                    // EX2 first; older systems refuse its size, then the EX part is read alone
                    uint ex2 = (uint)Marshal.SizeOf(typeof(Counters)), ex = ex2 - (uint)(IntPtr.Size + 8);
                    bool ok = GetProcessMemoryInfo(GetCurrentProcess(), out var c, ex2);
                    if (ok) s.PrivateWorkingSet = (long)c.PrivateWorkingSetSize.ToUInt64();
                    else ok = GetProcessMemoryInfo(GetCurrentProcess(), out c, ex);
                    if (ok)
                    {
                        s.WorkingSet = (long)c.WorkingSetSize.ToUInt64();
                        s.PeakWorkingSet = (long)c.PeakWorkingSetSize.ToUInt64();
                        s.Private = (long)c.PrivateUsage.ToUInt64();
                        s.PeakPrivate = (long)c.PeakPagefileUsage.ToUInt64();
                    }
                }
                catch (Exception) { noKernel32 = true; } // not Windows
            }
            if (s.WorkingSet <= 0)
            {
                using (var p = Process.GetCurrentProcess())
                {
                    s.WorkingSet = p.WorkingSet64;
                    s.PeakWorkingSet = p.PeakWorkingSet64;
                    s.Private = p.PrivateMemorySize64;
                }
            }
            s.VRAM = VRAM();
            return s;
        }

        // 1.7: Waylith.Graphics.GpuMemoryReport.TryGetDeviceLocalDriverUsage, the VMA heap usage
        // the engine itself compares with its cap. 1.5 / 1.6 have no such counter: 0.
        static MethodInfo usage;
        static bool usageLooked;

        static long VRAM()
        {
            if (!usageLooked)
            {
                usageLooked = true;
                try { usage = Type.GetType("Waylith.Graphics.GpuMemoryReport, Waylith")?.GetMethod("TryGetDeviceLocalDriverUsage", BindingFlags.Public | BindingFlags.Static); }
                catch (Exception) { usage = null; }
            }
            if (usage == null) return 0;
            try
            {
                var args = new object[] { 0L };
                if ((bool)usage.Invoke(null, args)) return (long)args[0];
            }
            catch (Exception) { usage = null; }
            return 0;
        }

        static int Mods()
        {
            try { return LoadedModManager.RunningModsListForReading.Count; }
            catch (Exception) { return 0; }
        }
    }
}
