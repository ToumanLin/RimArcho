using System;
using System.IO;
using System.Text.Json;
using ArchoMonitor;

// Runs without the game: only code that touches no Verse or Harmony type is called.
static class Tests
{
    static int failed;

    static void Check(bool ok, string what)
    {
        if (!ok)
        {
            failed++;
            Console.WriteLine("FAIL " + what);
        }
    }

    static int Main()
    {
        // Split: the message is what precedes the stack.
        Normalize.Split("Exception in Foo: System.Exception: boom\n  at Foo.Bar.Baz () [0x0] in <0>:0\n  at Verse.Root.Update ()", out var msg, out var stack);
        Check(msg == "Exception in Foo: System.Exception: boom", "split .NET message: " + msg);
        Check(stack.StartsWith("at Foo.Bar.Baz"), "split .NET stack: " + stack);
        Normalize.Split("Config error in Foo:\n  missing label", out msg, out stack);
        Check(msg == "Config error in Foo:\n  missing label" && stack == "", "split keeps indented detail: " + msg);
        Normalize.Split("boom\nVerse.Log:Error (string)\nFoo.Bar:Baz ()", out msg, out stack);
        Check(msg == "boom" && stack.StartsWith("Verse.Log:Error"), "split Unity stack: " + stack);

        // Fingerprint: numbers and addresses become placeholders.
        Check(Normalize.Fingerprint("Pawn 12 at 0x7FFA1 failed  x") == Normalize.Fingerprint("Pawn 345 at 0x10 failed x"), "fingerprint numbers");
        Check(Normalize.Fingerprint("A  B\tC") == "a b c", "fingerprint spaces: " + Normalize.Fingerprint("A  B\tC"));
        Check(Normalize.Hash8("x").Length == 8, "hash8");

        // Frame: .NET, Unity, Harmony replacement, constructor.
        Check(Mods.Frame("  at Foo.Bar.Baz (System.Int32 a) [0x0] in <0>:0", out var t, out var m) && t == "Foo.Bar" && m == "Baz", "frame .NET " + t + " " + m);
        Check(Mods.Frame("Foo.Bar:Baz (int)", out t, out m) && t == "Foo.Bar" && m == "Baz", "frame Unity " + t + " " + m);
        Check(Mods.Frame("  at (wrapper dynamic-method) Verse.Pawn.Verse.Pawn.Tick_Patch1(Verse.Pawn)", out t, out m) && t == "Verse.Pawn" && m == "Tick", "frame Harmony 2 " + t + " " + m);
        Check(Mods.Frame("   at .Verse.Pawn.Tick_Patch3(Pawn this)", out t, out m) && t == "Verse.Pawn" && m == "Tick", "frame .NET 10 " + t + " " + m);
        Check(Mods.Frame("  at Foo.Bar..ctor ()", out t, out m) && t == "Foo.Bar" && m == ".ctor", "frame ctor " + t + " " + m);
        Check(Mods.Frame("  at Foo.Bar`1[T].Baz[U] ()", out t, out m) && t == "Foo.Bar`1", "frame generic " + t + " " + m);

        // Writer: rotation, first line with details, a count line, the exit line.
        string dir = Path.Combine(Path.GetTempPath(), "archo-monitor-test-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        File.WriteAllText(Path.Combine(dir, "ArchoMonitor.log"), "old\n");
        Check(Writer.Open(dir), "open");
        Check(File.ReadAllText(Path.Combine(dir, "ArchoMonitor-prev.log")) == "old\n", "rotated");
        var c = new Context { Message = "Config error in Foo: \"quoted\" \\ path", Details = "  at Foo.Bar.Baz ()", Inferred = "DefConfigError", Def = "Foo" };
        c.AddFile(@"D:\Mods\A\Defs\X.xml", "a.mod", "xml");
        c.AddMod("b.mod", "patch");
        c.AddMod("a.mod", "stack"); // weaker role: stays xml
        c.AddRef("patch:Foo:PatchOperationAdd@b.mod");
        Writer.Ready = true;
        string text = c.Message + "\n" + c.Details;
        Writer.Add(Level.Error, text, c);
        Check(Writer.Seen(Level.Error, text), "seen after add");
        Check(Writer.Seen(Level.Error, text), "seen twice");
        var c2 = new Context { Message = "Config error in Foo: \"quoted\" \\ path", Inferred = "DefConfigError" };
        Writer.Add(Level.Error, "other text, same fingerprint", c2);
        Writer.Close();
        var lines = File.ReadAllLines(Path.Combine(dir, "ArchoMonitor.log"));
        Check(lines.Length == 3, "3 lines, got " + lines.Length);
        var first = JsonDocument.Parse(lines[0]).RootElement;
        Check(first.GetProperty("message").GetString() == c.Message, "message round trip");
        Check(first.GetProperty("count").GetInt32() == 1 && first.GetProperty("total").GetInt32() == 1, "first count");
        Check(first.GetProperty("details").GetString().Contains("Foo.Bar.Baz"), "details");
        Check(first.GetProperty("source").GetString() == "archo-monitor" && first.GetProperty("schema").GetInt32() == 1, "source schema");
        var ctx = first.GetProperty("context");
        Check(ctx.GetProperty("relatedFiles")[0].GetString() == @"D:\Mods\A\Defs\X.xml", "file");
        Check(ctx.GetProperty("roles").GetProperty("a.mod").GetString() == "xml", "role kept strongest");
        Check(ctx.GetProperty("roles").GetProperty("b.mod").GetString() == "patch", "role patch");
        Check(ctx.GetProperty("def").GetString() == "Foo", "def");
        var second = JsonDocument.Parse(lines[1]).RootElement;
        Check(second.GetProperty("count").GetInt32() == 3 && second.GetProperty("total").GetInt32() == 4, "count line: " + lines[1]);
        Check(second.GetProperty("id").GetString() == first.GetProperty("id").GetString(), "same id");
        Check(second.GetProperty("details").GetString() == "", "no details on count lines");
        var exit = JsonDocument.Parse(lines[2]).RootElement;
        Check(exit.GetProperty("kind").GetString() == "exit", "exit line");
        Directory.Delete(dir, true);

        Console.WriteLine(failed == 0 ? "ok" : failed + " failed");
        return failed == 0 ? 0 : 1;
    }
}
