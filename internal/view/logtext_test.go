package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.10, §7: log lines take the game's log colours; a mod's <color> is kept, adjusted
// only for contrast; tags that are not rich text stay text.
func TestLogText(t *testing.T) {
	// the game's colours (LudeonTK.EditWindow_Log / Verse.LogMessage.Color)
	if view.GameLogColors[view.LevelError] != "#ff6666" || view.GameLogColors[view.LevelWarning] != "#ffeb04" || view.GameLogColors[view.LevelInfo] != "#ffffff" {
		t.Fatalf("game colours %v", view.GameLogColors)
	}
	for _, c := range []struct {
		level  string
		status int
		want   string
	}{
		{"ERROR", 0, view.LevelError}, {"WARN", view.StatusErr, view.LevelWarning}, {"WARNING", 0, view.LevelWarning},
		{"INFO", view.StatusErr, view.LevelInfo}, {"", view.StatusErr, view.LevelError}, {"", view.StatusWarn, view.LevelWarning}, {"", view.StatusNone, view.LevelInfo},
	} {
		if got := view.LevelOf(c.level, c.status); got != c.want {
			t.Errorf("LevelOf(%q, %d) = %s, want %s", c.level, c.status, got, c.want)
		}
	}
	// the light theme keeps the hue and darkens for contrast; style.css carries the result
	css, err := os.ReadFile(filepath.Join("..", "surface", "gui", "assets", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, lv := range []string{view.LevelError, view.LevelWarning} {
		c := view.LightLogColors[lv]
		if c == view.GameLogColors[lv] || !strings.Contains(string(css), "--log-"+lv+": "+c) {
			t.Errorf("light %s colour %s not in style.css", lv, c)
		}
		if !strings.Contains(string(css), "--log-"+lv+": "+view.GameLogColors[lv]) {
			t.Errorf("dark %s colour not in style.css", lv)
		}
	}

	// a mod's colour: bright orange reads on the dark card as is, and is darkened on white
	sp := view.LogSpans("<color=#ff8800>[MyMod]</color> <b>loaded</b>\n  at Foo.Bar ()", 240)
	if len(sp) != 3 || sp[0].T != "[MyMod]" || sp[0].C != "#ff8800" || sp[0].L == "#ff8800" || sp[0].L == "" || sp[1].C != "" || !sp[2].B || view.SpanText(sp) != "[MyMod] loaded" {
		t.Fatalf("mod colour %+v", sp)
	}
	// navy on the dark card is lightened, kept on white
	if sp := view.LogSpans("<color=navy>deep</color>", 240); sp[0].C == "#000080" || sp[0].L != "#000080" {
		t.Fatalf("navy %+v", sp)
	}
	if got := view.ForBackground("#ffeb04", "#ffffff"); got == "#ffeb04" || !strings.HasPrefix(got, "#") {
		t.Fatalf("yellow on white %s", got)
	}
	// not markup: a script tag and unbalanced tags are text, nothing is coloured by them
	sp = view.LogSpans(`<script>alert("x")</script></color><b>ok`, 240)
	if view.SpanText(sp) != `<script>alert("x")</script>ok` || sp[0].C != "" || !sp[len(sp)-1].B {
		t.Fatalf("script %+v", sp)
	}
	t.Logf("light: error %s, warning %s", view.LightLogColors[view.LevelError], view.LightLogColors[view.LevelWarning])
}
