package tui

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rimarcho/rimarcho/internal/bbcode"
)

// run a command returned by the model and feed its message back (no program in tests).
func runCmd(m *model, c tea.Cmd) {
	if c == nil {
		return
	}
	msg := c()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, x := range b {
			runCmd(m, x)
		}
		return
	}
	if msg != nil {
		m.update(msg)
	}
}

func plainLines(ls []dline) string {
	var out []string
	for _, l := range ls {
		out = append(out, strings.TrimRight(ansi.Strip(l.text), " "))
	}
	return strings.Join(out, "\n")
}

// 06 §6.1: the default detail shows package id, version, Workshop id, tags, alias, note,
// dependencies with their state, and the description; technical information is folded.
func TestDetailContent(t *testing.T) {
	e, _ := fixture(t)
	desc := "[h1]Alpha[/h1]\n[b]Alpha[/b] adds things.\n[list][*]one[*]two[/list]\n[img]https://i.imgur.com/x.png[/img]\n" + strings.Repeat("More text here. ", 60)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[{"publishedfileid":%q,"result":1,"description":%q,"time_updated":1790000001}]}}`, r.Form.Get("publishedfileids[0]"), desc)
	}))
	defer srv.Close()
	e.App.SteamAPI = srv.URL
	e.MustRun("tag", "add", "pkg:tester.a", "weapons")
	e.MustRun("alias", "pkg:tester.a", "Alpha Plus")
	e.MustRun("note", "pkg:tester.a", "keep after Harmony")
	e.MustRun("disable", "pkg:brrainz.harmony") // Alpha's dependency: installed, not enabled
	m := direct(t, e, 160, 40)
	press(m, "g")
	press(m, "down") // Alpha Plus
	if m.current().name != "Alpha Plus" {
		t.Fatalf("cursor on %q", m.current().name)
	}
	runCmd(m, m.loadData(m.current().id, false, false)) // the cache is empty: fetched after the dwell
	if m.dd == nil || m.dd.desc.WorkshopState != "not cached" {
		t.Fatalf("data %+v", m.dd)
	}
	runCmd(m, m.onFetchTick(m.current().id))
	if m.dd.desc.WorkshopState != "fetched" {
		t.Fatalf("fetch: %s", m.dd.desc.WorkshopState)
	}
	got := plainLines(m.detailLayout(detailOpts{width: 50, descMax: 12}))
	for _, want := range []string{"Alpha Plus", "package", "tester.a", "workshop", "1001", "tags", "weapons", "alias", "(Alpha)", "note", "keep after Harmony",
		"requires", "Harmony (not enabled)", "description · Workshop page", "Alpha adds things.", "• one", "[image · i.imgur.com]", "… enter shows all", "technical information · i"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail lacks %q:\n%s", want, got)
		}
	}
	// the key appears once: "1001 · o opens the page" (not "o 按 o 打开页面")
	SetLang("zh")
	zh := plainLines(m.detailLayout(detailOpts{width: 50, descMax: 12}))
	SetLang("en")
	if !strings.Contains(got, "1001 · o opens the page") || !strings.Contains(zh, "1001 · o 打开页面") {
		t.Fatalf("workshop line:\n%s\n%s", got, zh)
	}
	for _, l := range m.detailLayout(detailOpts{width: 50, descMax: 12}) {
		if w := lipgloss.Width(l.text); w > 50 {
			t.Fatalf("line %d cells: %q", w, l.text)
		}
	}
	// The full screen shows everything and scrolls.
	press(m, "enter")
	if !m.full {
		t.Fatal("enter")
	}
	lines, _ := m.fullLines(16)
	all := m.detailLayout(detailOpts{width: m.w - 4, full: true})
	if strings.Contains(plainLines(all), "enter shows all") || len(all) <= 16 {
		t.Fatalf("full detail: %d lines", len(all))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "0% · ↑↓") {
		t.Fatalf("scroll line %q", ansi.Strip(lines[len(lines)-1]))
	}
	press(m, "G")
	lines, _ = m.fullLines(16)
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "100%") {
		t.Fatalf("after G: %q", ansi.Strip(lines[len(lines)-1]))
	}
	// o opens the Workshop page.
	var opened string
	m.opts.OpenURL = func(u string) error { opened = u; return nil }
	press(m, "o")
	if opened != "https://steamcommunity.com/sharedfiles/filedetails/?id=1001" {
		t.Fatalf("opened %q", opened)
	}
	press(m, "esc")
	if m.full {
		t.Fatal("esc")
	}
	// Offline: nothing fetched, About.xml only, a note says why.
	e.MustRun("config", "set", "workshop.fetch", "off")
	os.RemoveAll(filepath.Join(e.App.Home, "cache", "workshop")) // as if never fetched
	runCmd(m, m.loadData(m.current().id, false, false))
	if got := plainLines(m.detailLayout(detailOpts{width: 60})); !strings.Contains(got, "offline (workshop.fetch)") {
		t.Fatalf("offline note missing:\n%s", got)
	}
}

func TestImageWriter(t *testing.T) {
	var buf bytes.Buffer
	w := &imgWriter{Writer: &buf}
	w.set([]placement{{row: 5, col: 10, cols: 4, rows: 2, seq: []byte("<IMG>")}})
	w.Write([]byte("frame1"))
	out := buf.String()
	if !strings.Contains(out, "frame1\x1b7\x1b[?25h\x1b[5;10H<IMG>\x1b[?25l\x1b8") || !strings.HasPrefix(out, "\x1b[?2026h") || !strings.HasSuffix(out, "\x1b[?2026l") {
		t.Fatalf("draw: %q", out)
	}
	buf.Reset()
	w.set(nil)
	w.Write([]byte("frame2"))
	if out := buf.String(); !strings.Contains(out, "\x1b[5;10H\x1b[4X\x1b[6;10H\x1b[4X") || strings.Contains(out, "<IMG>") {
		t.Fatalf("clear: %q", out)
	}
	buf.Reset()
	w.Write([]byte("frame3"))
	if out := buf.String(); strings.Contains(out, "X") {
		t.Fatalf("cleared twice: %q", out)
	}
}

func TestImgRows(t *testing.T) {
	for _, c := range []struct{ w, h, cols, want int }{
		{1600, 900, 60, 15}, // 16:9: 60 × 13/30 × 9/16 = 14.6
		{500, 500, 40, 18},  // square
		{300, 900, 40, 20},  // tall: capped
		{0, 0, 60, 15},      // unknown: as 16:9
		{2000, 50, 60, 1},   // a thin banner: at least one row
	} {
		if got := imgRows(c.w, c.h, c.cols, 20); got != c.want {
			t.Errorf("imgRows(%d,%d,%d) = %d, want %d", c.w, c.h, c.cols, got, c.want)
		}
	}
}

// 06 §5.6: # on two marked mods, completion of tags in use, add and remove at once.
func TestTagEditing(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("tag", "add", "pkg:ludeon.rimworld", "official-stuff", "core")
	m := direct(t, e, 160, 40)
	press(m, "g")
	press(m, "down")
	press(m, "v") // Alpha
	press(m, "v") // Harmony
	press(m, "#")
	if m.prompt == nil || m.tagging == nil {
		t.Fatal("# did not open the tag line")
	}
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("offi")})
	if s := ansi.Strip(m.statusLine()); !strings.Contains(s, "official-stuff 1") {
		t.Fatalf("completions: %q", s)
	}
	m.update(tea.KeyMsg{Type: tea.KeyTab})
	if m.prompt.text != "official-stuff " {
		t.Fatalf("tab: %q", m.prompt.text)
	}
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lib")})
	press(m, "enter")
	if !strings.Contains(m.status, "tags of 2 mods: added official-stuff, lib") {
		t.Fatalf("status %q", m.status)
	}
	out := e.MustRun("list", "--filter", "tag:lib")
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Harmony") {
		t.Fatalf("tagged:\n%s", out)
	}
	// remove from one mod, with the - form; the list row shows the remaining tags faintly
	press(m, "g")
	press(m, "down")
	press(m, "#")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-lib")})
	press(m, "enter")
	if out := e.MustRun("list", "--filter", "tag:lib"); strings.Contains(out, "Alpha") {
		t.Fatalf("not removed:\n%s", out)
	}
	c := &m.cols[cActive]
	row := ansi.Strip(m.drawRow(c, 1, true, 60))
	if !strings.Contains(row, "official-stuff") {
		t.Fatalf("row without its tag: %q", row)
	}
}

// Kinsoku: across every width, no wrapped line starts with closing punctuation or ends
// with opening punctuation, nothing is lost, and every line fits.
func TestWrapKinsoku(t *testing.T) {
	texts := []string{
		"RimCrow 的核心伴生模组。支持从游戏获取更加详细的日志。必须放在 Harmony 之后加载。最好放在列表最前端以便获取更加完整的加载日志。",
		"他说：「这是《边缘世界》的模组（测试），好吗？」然后走了！",
		"中文后面跟英文标点.还有(括号)和逗号,以及问号?结束",
	}
	for _, text := range texts {
		for w := 8; w <= 40; w++ { // below that a glued group (吗？」) may not fit at all
			lines := wrapSpans([]bbcode.Span{{Text: text}}, w, lipgloss.NewStyle())
			var all strings.Builder
			for i, l := range lines {
				l = strings.TrimRight(ansi.Strip(l), " ")
				if cw := lipgloss.Width(l); cw > w {
					t.Fatalf("w=%d line %d is %d cells: %q", w, i, cw, l)
				}
				rs := []rune(l)
				if len(rs) > 0 && i > 0 && strings.ContainsRune(closingPunct, rs[0]) {
					t.Fatalf("w=%d line %d starts with %q: %q", w, i, rs[0], l)
				}
				if len(rs) > 0 && i < len(lines)-1 && strings.ContainsRune(openingPunct, rs[len(rs)-1]) {
					t.Fatalf("w=%d line %d ends with %q: %q", w, i, rs[len(rs)-1], l)
				}
				all.WriteString(l)
			}
			if strip := func(s string) string { return strings.ReplaceAll(s, " ", "") }; strip(all.String()) != strip(text) {
				t.Fatalf("w=%d lost text: %q", w, all.String())
			}
		}
	}
	// the case from the review: "…日志\n。必须…" becomes "…日\n志。必须…"
	lines := wrapSpans([]bbcode.Span{{Text: "支持从游戏获取更加详细的日志。必须放在"}}, 28, lipgloss.NewStyle())
	if got := strings.TrimRight(ansi.Strip(lines[1]), " "); !strings.HasPrefix(got, "志。") {
		t.Fatalf("second line %q", got)
	}
}
