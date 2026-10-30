using System;
using ArchoDrive;

static class Tests
{
    static int failed;

    static void Check(bool ok, string what)
    {
        if (ok) return;
        failed++;
        Console.WriteLine("FAIL " + what);
    }

    static bool Has(Script s, string part)
    {
        foreach (var e in s.Errors) if (e.Contains(part)) return true;
        return false;
    }

    static int Main()
    {
        // The standard round (round.ps1 writes it).
        var s = Script.Parse("out C:\\x\\r1\r\n# comment\nmenu\nnew difficulty=Peaceful coverage=0.3\nplay 120 speed=3\nsave Archo Round 1\nload Archo Round 1\nplay 60 tabs=off\nmenu\nquit\n");
        Check(s.Errors.Count == 0, "standard round parses: " + string.Join("; ", s.Errors));
        Check(s.Out == "C:\\x\\r1", "out: " + s.Out);
        Check(s.Steps.Count == 8, "8 steps: " + s.Steps.Count);
        Check(s.Steps[1].Opt("difficulty") == "Peaceful" && s.Steps[1].Opt("coverage") == "0.3", "new options");
        Check(s.Steps[2].Seconds == 120 && s.Steps[2].Timeout == 420, "play length and timeout: " + s.Steps[2].Timeout);
        Check(s.Steps[3].Arg == "Archo Round 1", "save name keeps its spaces: " + s.Steps[3].Arg);
        Check(s.Steps[5].Opt("tabs") == "off", "tabs off");
        Check(s.Steps[1].Timeout == 2400 && s.Steps[0].Timeout == 900, "default limits");
        Check(Script.Parse("new timeout=60").Steps[0].Timeout == 60, "timeout option");
        var del = Script.Parse("delete Archo Round 1\nquit");
        Check(del.Errors.Count == 0 && del.Steps[0].Kind == "delete" && del.Steps[0].Arg == "Archo Round 1", "delete step: " + string.Join("; ", del.Errors));
        Check(Has(Script.Parse("delete"), "delete needs a save name"), "delete without name");
        Check(Script.Parse("NEW").Steps[0].Kind == "new", "kinds are case-insensitive");

        // Mistakes are reported with their line, and nothing runs.
        Check(Has(Script.Parse("menu\nfly 3"), "line 2: unknown step 'fly'"), "unknown step");
        Check(Has(Script.Parse("play"), "play needs a number of seconds"), "play without seconds");
        Check(Has(Script.Parse("play -5"), "play needs a number of seconds"), "negative play");
        Check(Has(Script.Parse("play 5 speed=4"), "speed is 1, 2 or 3"), "bad speed");
        Check(Has(Script.Parse("play 5 tabs=yes"), "tabs is on or off"), "bad tabs");
        Check(Has(Script.Parse("save"), "save needs a save name"), "save without name");
        Check(Has(Script.Parse("load a/b"), "save names cannot contain"), "bad save name");
        Check(Has(Script.Parse("new colour=red"), "new has no option 'colour'"), "unknown option");
        Check(Has(Script.Parse("new coverage=2"), "coverage is a fraction"), "bad coverage");
        Check(Has(Script.Parse("new permadeath=yes"), "permadeath is on or off"), "bad permadeath");
        Check(Has(Script.Parse("menu now"), "menu takes no argument"), "stray argument");
        Check(Has(Script.Parse("menu timeout=0"), "timeout must be a positive"), "zero timeout");
        Check(Has(Script.Parse("quit\nmenu"), "line 1: quit must be the last step"), "quit not last");
        Check(Has(Script.Parse("# nothing\n"), "no steps"), "empty script");
        Check(Has(Script.Parse("out\nmenu"), "out needs a folder"), "out without folder");

        // Json: one line, escaped.
        string j = new Json().Str("msg", "a \"b\"\\\n\u0001").Num("sec", 1.23456).Bool("ok", true).ToString();
        Check(j == "{\"msg\":\"a \\\"b\\\"\\\\\\n\\u0001\",\"sec\":1.235,\"ok\":true}", "json: " + j);

        Console.WriteLine(failed == 0 ? "PASS ArchoDrive script tests" : failed + " failed");
        return failed == 0 ? 0 : 1;
    }
}
