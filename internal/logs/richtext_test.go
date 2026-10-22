package logs

import (
	"reflect"
	"testing"
)

// 06 §2.10: Unity rich text in log messages becomes spans; nothing else is markup.
func TestRichSpans(t *testing.T) {
	cases := []struct {
		in   string
		want []Span
	}{
		{"plain error", []Span{{Text: "plain error"}}},
		{"<color=#ff8800>[MyMod]</color> loaded", []Span{{Text: "[MyMod]", Color: "#ff8800"}, {Text: " loaded"}}},
		{"<color=#66ffb5>[MeleeAnim]</color> Potential patch conflicts (11):", []Span{{Text: "[MeleeAnim]", Color: "#66ffb5"}, {Text: " Potential patch conflicts (11):"}}},
		{`<color="orange">x</color>`, []Span{{Text: "x", Color: "#ffa500"}}},
		{"<color=#F80A>short</color>", []Span{{Text: "short", Color: "#ff8800"}}},
		{"<color=#ff880080>alpha</color>", []Span{{Text: "alpha", Color: "#ff8800"}}},
		{"<b>bold <i>both</i></b> none", []Span{{Text: "bold ", Bold: true}, {Text: "both", Bold: true, Italic: true}, {Text: " none"}}},
		{"<size=20>big</size> text", []Span{{Text: "big text"}}},
		// not rich text: shown as written, never markup
		{"<script>alert(1)</script>", []Span{{Text: "<script>alert(1)</script>"}}},
		{`Full node: <li Class="ReGrowthCore.X" />`, []Span{{Text: `Full node: <li Class="ReGrowthCore.X" />`}}},
		{"<color=javascript:alert(1)>x</color>", []Span{{Text: "<color=javascript:alert(1)>x"}}},
		{`<color="#ff0000" onclick=x>y`, []Span{{Text: `<color="#ff0000" onclick=x>y`}}},
		{"a < b and c > d", []Span{{Text: "a < b and c > d"}}},
		{"unclosed <color=red>to the end", []Span{{Text: "unclosed "}, {Text: "to the end", Color: "#ff0000"}}},
		{"stray </color></b> close", []Span{{Text: "stray  close"}}},
		{"<b><color=red>x</b> after", []Span{{Text: "x", Color: "#ff0000", Bold: true}, {Text: " after"}}},
		{"cut <color=red", []Span{{Text: "cut <color=red"}}},
	}
	for _, c := range cases {
		if got := RichSpans(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q:\n got %+v\nwant %+v", c.in, got, c.want)
		}
	}
	if got := PlainText("<color=#ff8800>[MyMod]</color> <b>hi</b>"); got != "[MyMod] hi" {
		t.Fatalf("plain %q", got)
	}
	if got := TruncSpans(RichSpans("<color=red>abcdef</color>gh"), 4); !reflect.DeepEqual(got, []Span{{Text: "abc…", Color: "#ff0000"}}) {
		t.Fatalf("trunc %+v", got)
	}
	if got := TruncSpans([]Span{{Text: "日本語テキスト"}}, 3); got[0].Text != "日本…" {
		t.Fatalf("runes %q", got[0].Text)
	}
}
