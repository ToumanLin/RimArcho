package csharp

import (
	"strings"
	"testing"
)

// An excerpt in ilspycmd's style: tabs, braces on their own lines, attributes above
// members, string literals holding braces, comments, a property with an initializer.
const sample = `using System;
using System.Collections.Generic;
using Verse;

namespace RimWorld
{
	[StaticConstructorOnStartup]
	public class PlayLogEntry_Interaction : PlayLogEntry, IThing<int>
	{
		private const string Tag = "{not a brace}";

		private InteractionDef intDef;

		private static readonly Dictionary<string, int> map = new Dictionary<string, int>
		{
			{ "a", 1 },
			{ "b", 2 }
		};

		public List<RulePackDef> Packs { get; set; } = new List<RulePackDef>();

		public string RecipientName => (recipient != null) ? recipient.LabelShort : "null";

		static PlayLogEntry_Interaction()
		{
			Log.Message("static {");
		}

		public PlayLogEntry_Interaction(InteractionDef intDef, Pawn initiator, Pawn recipient)
			: base(1)
		{
			this.intDef = intDef; // trailing { comment
			char c = '{';
			/* a block
			   comment with } */
		}

		[HarmonyPostfix]
		[Obsolete("x")]
		public static void Postfix(ref InteractionDef ___intDef, Pawn recipient)
		{
			if (recipient == null)
			{
				Log.ErrorOnce($"null {recipient?.Name ?? "pawn"} reference", 34422);
				return;
			}
		}

		public override IEnumerable<Thing> GetConcerns()
		{
			yield return null;
		}

		void IThing<int>.Do(int x)
		{
		}

		public T Generic<T>(T value) where T : class
		{
			return value;
		}

		public int this[int i]
		{
			get { return i; }
		}

		public event Action Changed;

		public static bool operator ==(PlayLogEntry_Interaction a, PlayLogEntry_Interaction b)
		{
			return true;
		}

		public static implicit operator int(PlayLogEntry_Interaction a)
		{
			return 0;
		}

		protected override string ToGameStringFromPOV_Worker(Pawn pov, bool forceLog)
		{
			string verbatim = @"C:\temp\{";
			string raw = """
				{ "json": "}" }
				""";
			return verbatim + raw;
		}

		public class Nested
		{
			public void Inner()
			{
			}
		}

		public enum Mode
		{
			A = 1,
			B
		}

		~PlayLogEntry_Interaction()
		{
		}
	}
}
`

func lineOf(t *testing.T, text string) int {
	t.Helper()
	for i, l := range strings.Split(sample, "\n") {
		if strings.Contains(l, text) {
			return i + 1
		}
	}
	t.Fatalf("no line with %q", text)
	return 0
}

func TestParseMembers(t *testing.T) {
	types := Parse(sample)
	typ := Find(types, "RimWorld.PlayLogEntry_Interaction")
	if typ == nil || typ.Name != "PlayLogEntry_Interaction" || typ.Kind != "class" {
		t.Fatalf("type: %+v", typ)
	}
	if typ.Start != lineOf(t, "[StaticConstructorOnStartup]") || typ.End != lineOf(t, "~PlayLogEntry")+3 {
		t.Errorf("type lines %d-%d", typ.Start, typ.End)
	}
	want := []struct {
		name, kind string
		start      int
		end        int
	}{
		{"Tag", "field", lineOf(t, "const string Tag"), lineOf(t, "const string Tag")},
		{"intDef", "field", lineOf(t, "private InteractionDef intDef;"), lineOf(t, "private InteractionDef intDef;")},
		{"map", "field", lineOf(t, "readonly Dictionary"), lineOf(t, "\t\t};")},
		{"Packs", "property", lineOf(t, "public List<RulePackDef> Packs"), lineOf(t, "public List<RulePackDef> Packs")},
		{"RecipientName", "property", lineOf(t, "RecipientName =>"), lineOf(t, "RecipientName =>")},
		{"cctor", "cctor", lineOf(t, "static PlayLogEntry_Interaction()"), lineOf(t, "static {") + 1},
		{"ctor", "ctor", lineOf(t, "public PlayLogEntry_Interaction(InteractionDef"), lineOf(t, "comment with } */") + 1},
		{"Postfix", "method", lineOf(t, "[HarmonyPostfix]"), lineOf(t, "return;") + 2},
		{"GetConcerns", "method", lineOf(t, "GetConcerns()"), lineOf(t, "yield return null;") + 1},
		{"Do", "method", lineOf(t, "void IThing<int>.Do"), lineOf(t, "void IThing<int>.Do") + 2},
		{"Generic", "method", lineOf(t, "public T Generic<T>"), lineOf(t, "return value;") + 1},
		{"this[]", "indexer", lineOf(t, "public int this[int i]"), lineOf(t, "get { return i; }") + 1},
		{"Changed", "event", lineOf(t, "public event Action Changed;"), lineOf(t, "public event Action Changed;")},
		{"operator ==", "operator", lineOf(t, "operator ==("), lineOf(t, "operator ==(") + 3},
		{"operator int", "operator", lineOf(t, "implicit operator int"), lineOf(t, "implicit operator int") + 3},
		{"ToGameStringFromPOV_Worker", "method", lineOf(t, "ToGameStringFromPOV_Worker"), lineOf(t, "return verbatim + raw;") + 1},
		{"Nested", "type", lineOf(t, "public class Nested"), lineOf(t, "public class Nested") + 5},
		{"Mode", "type", lineOf(t, "public enum Mode"), lineOf(t, "public enum Mode") + 4},
		{"~PlayLogEntry_Interaction", "finalizer", lineOf(t, "~PlayLogEntry_Interaction()"), lineOf(t, "~PlayLogEntry_Interaction()") + 2},
	}
	if len(typ.Members) != len(want) {
		for _, m := range typ.Members {
			t.Logf("got %+v", m)
		}
		t.Fatalf("%d members, want %d", len(typ.Members), len(want))
	}
	for i, w := range want {
		m := typ.Members[i]
		if m.Name != w.name || m.Kind != w.kind || m.Start != w.start || m.End != w.end {
			t.Errorf("member %d: got %+v, want %+v", i, m, w)
		}
	}
	if nested := Find(types, "Nested"); nested == nil || len(nested.Members) != 1 || nested.Members[0].Name != "Inner" {
		t.Errorf("nested type: %+v", nested)
	}
	if mode := Find(types, "Mode"); mode == nil || mode.Kind != "enum" || len(mode.Members) != 0 {
		t.Errorf("enum: %+v", mode)
	}
}

func TestMatchAndIndex(t *testing.T) {
	typ := Find(Parse(sample), "PlayLogEntry_Interaction")
	for _, c := range []struct {
		want string
		n    int
	}{
		{"Postfix", 1}, {"postfix", 1}, {"ctor", 1}, {".ctor", 1}, {"PlayLogEntry_Interaction", 1}, {"cctor", 1},
		{"Do", 1},             // after the interface prefix
		{"RecipientN", 1},     // a unique substring
		{"Tick", 0}, {"e", 0}, // nothing, and too many
		{"GetConcerns", 1}, {"operator ==", 1},
	} {
		if got := typ.Match(c.want); len(got) != c.n {
			t.Errorf("Match(%q) = %d members %+v, want %d", c.want, len(got), got, c.n)
		}
	}
	idx := typ.Index(0)
	for _, part := range []string{"ctor " + itoaLine(t, "public PlayLogEntry_Interaction(InteractionDef"), "Postfix ", "GetConcerns ", "Nested ", "3 fields"} {
		if !strings.Contains(idx, part) {
			t.Errorf("index lacks %q: %s", part, idx)
		}
	}
	if strings.Contains(idx, "intDef") {
		t.Errorf("a field in the index: %s", idx)
	}
	if short := typ.Index(3); !strings.Contains(short, "… ") || strings.Count(short, ",") != 3 {
		t.Errorf("limited index: %s", short)
	}
}

func itoaLine(t *testing.T, text string) string {
	t.Helper()
	n := lineOf(t, text)
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestOverloadsShareAnEntry(t *testing.T) {
	src := "class A\n{\n\tvoid F()\n\t{\n\t}\n\tvoid F(int x)\n\t{\n\t}\n\tint G;\n}\n"
	typ := Find(Parse(src), "A")
	if got := typ.Index(0); got != "F 3/6; 1 fields" {
		t.Errorf("index %q", got)
	}
	if got := typ.Match("F"); len(got) != 2 || got[1].Start != 6 || got[1].End != 8 {
		t.Errorf("overloads %+v", got)
	}
}

func TestTruncatedAndOddInput(t *testing.T) {
	for _, src := range []string{"", "class", "class A {", "class A { void F( {", "namespace N { class B { int x = new int[] { 1, 2", "class A { string s = \"unterminated\n", "class A { char c = '", "}}}}"} {
		Parse(src) // must return, whatever it finds
	}
	if typ := Find(Parse("class A { void F() { } "), "A"); typ == nil || len(typ.Members) != 1 {
		t.Errorf("unclosed type: %+v", typ)
	}
	if Find(nil, "X") != nil {
		t.Error("Find on nothing")
	}
}
