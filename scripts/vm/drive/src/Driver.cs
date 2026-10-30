using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text;
using HarmonyLib;
using RimWorld;
using RimWorld.Planet;
using Verse;

namespace ArchoDrive
{
    // Archo Drive plays one scripted round of RimWorld for the VM tests (scripts/vm/README.md,
    // "Game rounds"), so a round needs no screenshots and no clicks. It calls what the game's
    // own buttons call: each page's Next (CanDoNext, then DoNext), the save and load dialogs'
    // file action, "quit to main menu" and "quit to OS". The Archo Monitor phase hooks see the
    // same calls a player's clicks make.
    //
    // Test only, never shipped. round.ps1 copies ArchoDrive.dll into the installed Archo
    // Monitor folder (so the mod list stays the same) together with ArchoDrive.txt, the
    // script, and removes both after the round. Without ArchoDrive.txt it does nothing.
    public class DriveMod : Mod
    {
        public DriveMod(ModContentPack content) : base(content)
        {
            Driver.Boot(content?.RootDir);
        }
    }

    internal static class Driver
    {
        internal const string ScriptName = "ArchoDrive.txt";
        static Script script;
        static StreamWriter log;
        static readonly Stopwatch clock = Stopwatch.StartNew();
        static int index = -1;
        static Step step;
        static Runner runner;
        static double stepStart, nextPoll, lastBeat;
        static bool finished;

        internal static void Boot(string root)
        {
            try
            {
                if (string.IsNullOrEmpty(root)) return;
                string file = Path.Combine(root, ScriptName);
                if (!File.Exists(file)) return;
                script = Script.Parse(File.ReadAllText(file));
                string outDir = !string.IsNullOrEmpty(script.Out) ? script.Out : Path.Combine(GenFilePaths.SaveDataFolderPath, "ArchoDrive");
                Directory.CreateDirectory(outDir);
                // One round per script: the next start of the game is not driven.
                string taken = Path.Combine(outDir, "script.txt");
                if (File.Exists(taken)) File.Delete(taken);
                File.Move(file, taken);
                var fs = new FileStream(Path.Combine(outDir, "drive.log"), FileMode.Append, FileAccess.Write, FileShare.ReadWrite);
                log = new StreamWriter(fs, new UTF8Encoding(false)) { AutoFlush = true };
                Write(J("start").Str("game", Safe(() => VersionControl.CurrentVersionStringWithRev)).Str("data", Safe(() => GenFilePaths.SaveDataFolderPath))
                    .Str("playerLog", Safe(() => UnityEngine.Application.consoleLogPath)).Num("steps", script.Steps.Count));
                if (script.Errors.Count > 0)
                {
                    Finish("fail", "script: " + string.Join("; ", script.Errors.ToArray()));
                    return;
                }
                var h = new Harmony("archo.vmdrive");
                var post = new HarmonyMethod(AccessTools.Method(typeof(Driver), nameof(Frame)));
                int patched = 0;
                // Every frame in both scenes (RimDevTools' heartbeat uses the same two).
                foreach (var t in new[] { "Verse.Root_Entry", "Verse.Root_Play" })
                {
                    var type = AccessTools.TypeByName(t);
                    var m = type == null ? null : AccessTools.Method(type, "Update");
                    if (m == null) continue;
                    h.Patch(m, postfix: post);
                    patched++;
                }
                if (patched < 2)
                {
                    Finish("fail", "Root_Entry.Update or Root_Play.Update not found");
                    return;
                }
                // The VM's saves sync to the player's Steam Cloud: an autosave would overwrite
                // one of their autosave slots. Only this run; Prefs are not touched.
                var autosave = AccessTools.Method(typeof(Autosaver), nameof(Autosaver.DoAutosave));
                if (autosave == null)
                {
                    Finish("fail", "Autosaver.DoAutosave not found; not playing with autosave on");
                    return;
                }
                h.Patch(autosave, prefix: new HarmonyMethod(AccessTools.Method(typeof(Driver), nameof(NoAutosave))));
#if RW_UNITY
                // Keep frames coming when RimArcho or the desktop has the focus; not saved to Prefs.
                try { UnityEngine.Application.runInBackground = true; } catch { }
#endif
            }
            catch (Exception e)
            {
                try { Finish("fail", "boot: " + e); } catch { }
            }
        }

        static void Frame()
        {
            if (finished || script == null) return;
            try
            {
                double now = clock.Elapsed.TotalSeconds;
                if (now - lastBeat >= 5)
                {
                    lastBeat = now;
                    Write(J("beat").Str("state", G.State()));
                }
                if (now < nextPoll) return;
                nextPoll = now + 0.25;
                if (step == null)
                {
                    index++;
                    if (index >= script.Steps.Count)
                    {
                        Finish("done", "");
                        return;
                    }
                    step = script.Steps[index];
                    stepStart = now;
                    Write(J("step").Str("step", step.ToString()).Str("state", "begin").Str("at", G.State()));
                    if (step.Kind == "quit")
                    {
                        Write(J("step").Str("step", "quit").Str("state", "ok").Num("sec", 0));
                        Finish("done", "");
                        G.Act("quit to OS", Root.Shutdown);
                        return;
                    }
                    runner = Runner.For(step);
                }
                double t = now - stepStart;
                string result = runner.Poll(t);
                if (result == null)
                {
                    if (t > step.Timeout) Fail("timed out after " + (int)step.Timeout + " s: " + G.State());
                    return;
                }
                if (result != "")
                {
                    Fail(result);
                    return;
                }
                Write(J("step").Str("step", step.Kind).Str("state", "ok").Num("sec", t).Str("note", runner.Summary ?? ""));
                step = null;
            }
            catch (Exception e)
            {
                Fail("driver: " + e);
            }
        }

        static bool NoAutosave()
        {
            Note("autosave skipped");
            return false;
        }

        static void Fail(string why)
        {
            Write(J("step").Str("step", step?.Kind ?? "").Str("state", "fail").Str("msg", why));
            Finish("fail", why);
        }

        static void Finish(string ev, string msg)
        {
            finished = true;
            Write(J(ev).Str("msg", msg ?? ""));
            try { log?.Dispose(); } catch { }
            log = null;
        }

        internal static void Note(string msg)
        {
            Write(J("note").Str("msg", msg));
        }

        static Json J(string ev)
        {
            return new Json().Str("t", DateTime.Now.ToString("yyyy-MM-ddTHH:mm:ss.fff", CultureInfo.InvariantCulture)).Str("ev", ev).Num("n", index + 1);
        }

        static void Write(Json j)
        {
            try { log?.WriteLine(j.ToString()); } catch { }
        }

        internal static string Safe(Func<string> f)
        {
            try { return f() ?? ""; } catch { return ""; }
        }
    }

    // G: what the game is doing, and acting on it.
    internal static class G
    {
        internal static bool Busy => LongEventHandler.AnyEventNowOrWaiting;
        internal static bool InMenu => Current.ProgramState == ProgramState.Entry && Current.Game == null && !Busy;
        internal static bool Playing => Current.ProgramState == ProgramState.Playing && Find.CurrentMap != null && !Busy;

        internal static IList<Window> Windows
        {
            get
            {
                try { return Find.WindowStack?.Windows ?? (IList<Window>)new Window[0]; }
                catch { return new Window[0]; }
            }
        }

        internal static string State()
        {
            try
            {
                var s = new StringBuilder();
                s.Append(Current.ProgramState).Append(Busy ? " busy" : "");
                if (Current.ProgramState == ProgramState.Playing && Find.TickManager != null)
                    s.Append(" tick ").Append(Find.TickManager.TicksGame).Append(Find.TickManager.Paused ? " paused" : "");
                s.Append(" [").Append(string.Join(", ", Windows.Select(w => w.GetType().Name).ToArray())).Append(']');
                return s.ToString();
            }
            catch (Exception e)
            {
                return "state unknown: " + e.GetType().Name;
            }
        }

        // Act runs a game action. An exception it throws would have been logged by the game
        // after a real click (Root level exception in OnGUI), so it goes to Player.log too.
        internal static string Act(string what, Action a)
        {
            try
            {
                a();
                return null;
            }
            catch (Exception e)
            {
                var inner = e is TargetInvocationException && e.InnerException != null ? e.InnerException : e;
                try { Log.Error("[Archo Drive] " + what + ": " + inner); } catch { }
                return what + ": " + inner.GetType().Name + ": " + inner.Message;
            }
        }

        internal static object Call(object target, string method, params object[] args)
        {
            var types = args.Select(x => x.GetType()).ToArray();
            var m = AccessTools.Method(target.GetType(), method, types);
            if (m == null) throw new MissingMethodException(target.GetType().Name, method);
            return m.Invoke(target, args);
        }

        internal static void SetField(object target, string field, object value)
        {
            var f = AccessTools.Field(target.GetType(), field);
            if (f == null) throw new MissingFieldException(target.GetType().Name, field);
            f.SetValue(target, value);
        }

        // AcceptDialog presses button A of the topmost message box, as its button does
        // (the action, then Close). Confirmations of the driver's own actions and the
        // "mods mismatch" warning ("Load anyway") are such boxes.
        internal static bool AcceptDialog(out string error)
        {
            error = null;
            var box = Windows.OfType<Dialog_MessageBox>().LastOrDefault();
            if (box == null) return false;
            string text = Line(box.text.ToString());
            Driver.Note("dialog " + (string.IsNullOrEmpty(box.title) ? "" : Line(box.title) + ": ") + text + " -> " + Line(box.buttonAText ?? "OK"));
            error = Act("dialog button A", () =>
            {
                if (box.buttonAAction != null) box.buttonAAction();
                else box.acceptAction?.Invoke();
                box.Close(false);
            });
            return true;
        }

        internal static string Line(string s)
        {
            s = (s ?? "").Replace("\r", " ").Replace("\n", " ").Trim();
            return s.Length > 240 ? s.Substring(0, 240) + "…" : s;
        }
    }

    internal abstract class Runner
    {
        internal string Summary;

        // Poll returns null while running, "" when done, otherwise why it failed.
        internal abstract string Poll(double t);

        internal static Runner For(Step s)
        {
            switch (s.Kind)
            {
                case "menu": return new MenuRunner();
                case "new": return new NewRunner(s);
                case "play": return new PlayRunner(s);
                case "save": return new SaveRunner(s.Arg);
                case "load": return new LoadRunner(s.Arg);
                case "delete": return new DeleteRunner(s.Arg);
                case "wait": return new WaitRunner(s.Seconds);
            }
            throw new ArgumentException("no runner for " + s.Kind);
        }

        // Settled is true once cond has held for 3 s in a row.
        double since = -1;
        protected bool Settled(bool cond, double t)
        {
            if (!cond)
            {
                since = -1;
                return false;
            }
            if (since < 0) since = t;
            return t - since >= 3;
        }
    }

    // menu: be at the main menu; from a game, what "quit to main menu" confirms.
    internal sealed class MenuRunner : Runner
    {
        bool asked;

        internal override string Poll(double t)
        {
            if (Settled(G.InMenu, t))
            {
                Summary = asked ? "from the game" : "already there";
                return "";
            }
            if (!asked && G.Playing)
            {
                asked = true;
                return G.Act("quit to main menu", GenScene.GoToMainMenu);
            }
            return null;
        }
    }

    // new: the main menu's "New colony", then Next on each page until the map is up.
    internal sealed class NewRunner : Runner
    {
        static readonly MethodInfo CanDoNext = AccessTools.Method(typeof(Page), "CanDoNext");
        static readonly MethodInfo DoNext = AccessTools.Method(typeof(Page), "DoNext");
        readonly Step s;
        bool opened;
        Page last;
        int tries;
        double lastTry;
        readonly List<string> pages = new List<string>();

        internal NewRunner(Step s) { this.s = s; }

        internal override string Poll(double t)
        {
            if (!opened)
            {
                if (!Settled(G.InMenu, t)) return null;
                opened = true;
                return G.Act("New colony", () => Find.WindowStack.Add(new Page_SelectScenario()));
            }
            if (G.Busy)
            {
                Settled(false, t);
                return null;
            }
            if (G.AcceptDialog(out var err)) return err;
            var page = G.Windows.OfType<Page>().LastOrDefault();
            if (Settled(page == null && G.Playing, t))
            {
                var map = Find.CurrentMap;
                Summary = string.Join(" > ", pages.ToArray()) + "; map " + map.Size.x + "x" + map.Size.z + ", " + map.mapPawns.FreeColonistsCount + " colonists";
                return "";
            }
            if (page == null) return null;
            if (page != last)
            {
                last = page;
                tries = 0;
                pages.Add(page.GetType().Name);
                string why = Configure(page);
                if (why != null) return why;
            }
            // World generation runs as a long event and closes its page itself.
            if (page is Page_CreateWorldParams && tries > 0 && Current.Game?.World != null) return null;
            if (tries > 0 && t - lastTry < 3) return null;
            if (tries >= 5) return page.GetType().Name + " refused Next " + tries + " times (its reason is a message on screen; see Player.log)";
            tries++;
            lastTry = t;
            if (page is Page_SelectStartingSite && !Find.WorldInterface.SelectedTile.Valid)
            {
                var e = G.Act("random site", () => Find.WorldInterface.SelectedTile = TileFinder.RandomStartingTile());
                if (e != null) return e;
            }
            return G.Act("Next on " + page.GetType().Name, () =>
            {
                if ((bool)CanDoNext.Invoke(page, null)) DoNext.Invoke(page, null);
            });
        }

        // Configure makes the choices a player makes on the page; nothing for pages that
        // have a default (ideology: classic; starting pawns: as generated).
        string Configure(Page page)
        {
            return G.Act("set up " + page.GetType().Name, () =>
            {
                if (page is Page_SelectScenario && s.Opt("scenario") is string sc)
                {
                    var scen = DefDatabase<ScenarioDef>.GetNamedSilentFail(sc)?.scenario
                        ?? ScenarioLister.AllScenarios().FirstOrDefault(x => string.Equals(x.name, sc, StringComparison.OrdinalIgnoreCase));
                    if (scen == null) throw new ArgumentException("no scenario '" + sc + "'");
                    G.SetField(page, "curScen", scen);
                    Driver.Note("scenario " + scen.name);
                }
                else if (page is Page_SelectStoryteller)
                {
                    if (s.Opt("storyteller") is string st)
                    {
                        var def = DefDatabase<StorytellerDef>.GetNamedSilentFail(st) ?? throw new ArgumentException("no storyteller '" + st + "'");
                        G.SetField(page, "storyteller", def);
                    }
                    string dn = s.Opt("difficulty", "Peaceful");
                    var diff = DefDatabase<DifficultyDef>.GetNamedSilentFail(dn) ?? throw new ArgumentException("no difficulty '" + dn + "'");
                    G.SetField(page, "difficulty", diff);
                    G.SetField(page, "difficultyValues", new Difficulty(diff));
                    Find.GameInitData.permadeathChosen = true;
                    Find.GameInitData.permadeath = s.Opt("permadeath", "off") == "on";
                    var chosen = AccessTools.Field(typeof(Page_SelectStoryteller), "storyteller").GetValue(page) as StorytellerDef;
                    Driver.Note("storyteller " + chosen?.defName + ", difficulty " + diff.defName + (Find.GameInitData.permadeath ? ", permadeath" : ""));
                }
                else if (page is Page_CreateWorldParams && s.Opt("coverage") is string cov)
                {
                    G.SetField(page, "planetCoverage", float.Parse(cov, CultureInfo.InvariantCulture));
                    Driver.Note("planet coverage " + cov);
                }
            });
        }
    }

    // play: real seconds at a speed; opens each main tab, then a few colonists' inspect tabs.
    internal sealed class PlayRunner : Runner
    {
        readonly double seconds;
        readonly TimeSpeed speed;
        readonly bool tabs;
        int ticks0 = -1, opened;
        double next;
        Queue<Action> plan;
        readonly Dictionary<Window, double> pausing = new Dictionary<Window, double>();
        readonly HashSet<string> closedTypes = new HashSet<string>();

        internal PlayRunner(Step s)
        {
            seconds = s.Seconds;
            speed = s.Opt("speed", "3") == "1" ? TimeSpeed.Normal : s.Opt("speed", "3") == "2" ? TimeSpeed.Fast : TimeSpeed.Superfast;
            tabs = s.Opt("tabs", "on") == "on";
        }

        internal override string Poll(double t)
        {
            if (G.Busy) return null; // autosave and the like
            if (!G.Playing) return "not in a game: " + G.State();
            var tm = Find.TickManager;
            if (ticks0 < 0)
            {
                ticks0 = tm.TicksGame;
                plan = tabs ? Plan() : new Queue<Action>();
            }
            if (G.AcceptDialog(out var err) && err != null) return err;
            string e = ClosePausing(t);
            if (e != null) return e;
            if (tm.CurTimeSpeed != speed) tm.CurTimeSpeed = speed;
            if (t >= seconds)
            {
                e = G.Act("close tabs", () =>
                {
                    Find.MainTabsRoot.EscapeCurrentTab(false);
                    Find.Selector.ClearSelection();
                });
                if (e != null) return e;
                int ticks = tm.TicksGame - ticks0;
                Summary = ticks + " ticks, " + opened + " windows and tabs opened";
                return ticks > 0 ? "" : "game time did not move: " + G.State();
            }
            if (plan.Count > 0 && t >= next)
            {
                next = t + 3;
                opened++;
                return G.Act("open a tab", plan.Dequeue());
            }
            return null;
        }

        static Queue<Action> Plan()
        {
            var q = new Queue<Action>();
            foreach (var def in DefDatabase<MainButtonDef>.AllDefsListForReading)
            {
                if (def.tabWindowClass == null || def == MainButtonDefOf.Menu || def == MainButtonDefOf.Inspect) continue;
                if (!def.Worker.Visible || def.Worker.Disabled) continue;
                var d = def;
                q.Enqueue(() => Find.MainTabsRoot.SetCurrentTab(d, false));
            }
            q.Enqueue(() => Find.MainTabsRoot.EscapeCurrentTab(false));
            foreach (var pawn in Find.CurrentMap.mapPawns.FreeColonists.Take(3).ToList())
            {
                var p = pawn;
                q.Enqueue(() =>
                {
                    Find.Selector.ClearSelection();
                    Find.Selector.Select(p, false, true);
                    Find.MainTabsRoot.SetCurrentTab(MainButtonDefOf.Inspect, false);
                });
                foreach (var tab in p.GetInspectTabs().ToList())
                {
                    var type = tab.GetType();
                    q.Enqueue(() =>
                    {
                        if (Find.Selector.SingleSelectedThing == p && tab.IsVisible) InspectPaneUtility.OpenTab(type);
                    });
                }
            }
            return q;
        }

        // ClosePausing closes windows that pause the game (the scenario's intro, naming the
        // colony) after 2 s, as a player would, so game time moves.
        string ClosePausing(double t)
        {
            foreach (var w in G.Windows.ToList())
            {
                if (!w.forcePause || w is MainTabWindow) continue;
                if (!pausing.TryGetValue(w, out var seen))
                {
                    pausing[w] = t;
                    continue;
                }
                if (t - seen < 2) continue;
                string name = w.GetType().Name;
                if (closedTypes.Add(name)) Driver.Note("closed " + name + " (it pauses the game)");
                pausing.Remove(w);
                var e = G.Act("close " + name, () => w.Close(false));
                if (e != null) return e;
            }
            return null;
        }
    }

    // save: the in-game menu's "Save", a name, then the dialog's save button.
    internal sealed class SaveRunner : Runner
    {
        readonly string name;
        Dialog_SaveFileList_Save dialog;
        double opened;
        bool pressed;
        string path;
        DateTime before;

        internal SaveRunner(string name) { this.name = name; }

        internal override string Poll(double t)
        {
            if (G.Busy) return null;
            if (dialog == null)
            {
                if (!G.Playing) return "save needs a game: " + G.State();
                path = GenFilePaths.FilePathForSavedGame(name);
                before = File.Exists(path) ? File.GetLastWriteTimeUtc(path) : DateTime.MinValue;
                opened = t;
                return G.Act("open the save dialog", () =>
                {
                    dialog = new Dialog_SaveFileList_Save();
                    Find.WindowStack.Add(dialog);
                });
            }
            if (!pressed)
            {
                if (t - opened < 1.5) return null;
                pressed = true;
                return G.Act("save as " + name, () => G.Call(dialog, "DoFileInteraction", name));
            }
            if (File.Exists(path) && File.GetLastWriteTimeUtc(path) > before)
            {
                Summary = Path.GetFileName(path) + ", " + (new FileInfo(path).Length / 1024) + " KB";
                return "";
            }
            return null;
        }
    }

    // load: "Load game" from the main menu or the in-game menu, the save, its load button;
    // a "mods mismatch" warning is answered "Load anyway".
    internal sealed class LoadRunner : Runner
    {
        readonly string name;
        Dialog_SaveFileList_Load dialog;
        Game old;
        double opened;
        bool pressed;

        internal LoadRunner(string name) { this.name = name; }

        internal override string Poll(double t)
        {
            if (dialog == null)
            {
                if (!G.Playing && !G.InMenu) return null;
                if (!File.Exists(GenFilePaths.FilePathForSavedGame(name))) return "no save named " + name;
                old = Current.Game;
                opened = t;
                return G.Act("open the load dialog", () =>
                {
                    dialog = new Dialog_SaveFileList_Load();
                    Find.WindowStack.Add(dialog);
                });
            }
            if (!pressed)
            {
                if (t - opened < 1.5) return null;
                pressed = true;
                return G.Act("load " + name, () => G.Call(dialog, "DoFileInteraction", name));
            }
            if (G.Busy)
            {
                Settled(false, t);
                return null;
            }
            if (G.AcceptDialog(out var err)) return err;
            if (Settled(G.Playing && Current.Game != null && Current.Game != old, t))
            {
                Summary = "tick " + Find.TickManager.TicksGame;
                return "";
            }
            return null;
        }
    }

    // delete: the load dialog's delete button, once confirmed (the file is deleted). The
    // standard round deletes its save before quitting, so Steam Cloud never uploads it.
    internal sealed class DeleteRunner : Runner
    {
        readonly string name;

        internal DeleteRunner(string name) { this.name = name; }

        internal override string Poll(double t)
        {
            if (G.Busy) return null;
            string path = GenFilePaths.FilePathForSavedGame(name);
            if (!File.Exists(path)) return "no save named " + name;
            string e = G.Act("delete " + name, () => new FileInfo(path).Delete());
            if (e != null) return e;
            Summary = Path.GetFileName(path);
            return File.Exists(path) ? "the save is still there: " + path : "";
        }
    }

    internal sealed class WaitRunner : Runner
    {
        readonly double seconds;

        internal WaitRunner(double seconds) { this.seconds = seconds; }

        internal override string Poll(double t) => t >= seconds ? "" : null;
    }
}
