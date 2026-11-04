package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/bbcode"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// 06 §4.1: the default detail shows package id, version, Workshop id, tags, alias, note,
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
	press(m, "home")
	press(m, "down") // Alpha Plus
	if m.current().name != "Alpha Plus" {
		t.Fatalf("cursor on %q", m.current().name)
	}
	runCmd(m, m.loadData(m.current().id, false, false)) // the cache is empty: fetched after the dwell
	if m.dd == nil || m.dd.Desc.WorkshopState != "not cached" {
		t.Fatalf("data %+v", m.dd)
	}
	runCmd(m, m.onFetchTick(m.current().id))
	if m.dd.Desc.WorkshopState != "fetched" {
		t.Fatalf("fetch: %s", m.dd.Desc.WorkshopState)
	}
	got := plainLines(m.detailLayout(detailOpts{width: 50, descMax: 12}))
	for _, want := range []string{"Alpha Plus", "package", "tester.a", "workshop", "1001", "tags", "weapons", "alias", "(Alpha)", "note", "keep after Harmony",
		"requires", "Harmony (not enabled)", "description / Workshop page", "Alpha adds things.", "• one", "[images ×1]", "… enter shows all", "technical information, i"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail lacks %q:\n%s", want, got)
		}
	}
	// The full-screen detail lists each image as text.
	if full := plainLines(m.detailLayout(detailOpts{width: 50, full: true})); !strings.Contains(full, "[image  i.imgur.com]") || strings.Contains(full, "images ×") {
		t.Fatalf("full-screen images:\n%s", full)
	}
	// Both page shortcuts are shown, with the shorter button labels.
	SetLang("zh")
	zh := plainLines(m.detailLayout(detailOpts{width: 50, descMax: 12}))
	SetLang("en")
	if !strings.Contains(got, "1001  ctrl+w web page") || !strings.Contains(strings.Join(strings.Fields(got), " "), "ctrl+s steam page") || !strings.Contains(zh, "1001  ctrl+w 网页") || !strings.Contains(zh, "ctrl+s Steam 页面") {
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
	lines := m.fullLines(16)
	all := m.detailLayout(detailOpts{width: m.w - 4, full: true})
	if strings.Contains(plainLines(all), "enter shows all") || len(all) <= 16 {
		t.Fatalf("full detail: %d lines", len(all))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "0%, ↑↓") {
		t.Fatalf("scroll line %q", ansi.Strip(lines[len(lines)-1]))
	}
	press(m, "end")
	lines = m.fullLines(16)
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "100%") {
		t.Fatalf("after G: %q", ansi.Strip(lines[len(lines)-1]))
	}
	// o opens the Workshop page.
	var opened string
	m.opts.OpenURL = func(u string) error { opened = u; return nil }
	press(m, "ctrl+w")
	if opened != "https://steamcommunity.com/sharedfiles/filedetails/?id=1001" {
		t.Fatalf("opened %q", opened)
	}
	m.update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if opened != "steam://url/CommunityFilePage/1001" {
		t.Fatalf("Steam page opened %q", opened)
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

// T13 (06 §11): # → enter gives the picked mods the second candidate; the same again
// removes it ([✓]); a tag only some have shows as [~] and enter gives it to all; # ←
// then typing makes a new tag. The list rows show tags faintly.
func TestT13TagBar(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.MustRun("tag", "add", "pkg:ludeon.rimworld", "official-stuff", "core")
	e.MustRun("tag", "add", "pkg:tester.a", "half")
	m := direct(t, e, 160, 40)
	press(m, "home")
	press(m, "down")
	press(m, "v")         // Alpha
	press(m, "ctrl+down") // the cursor alone
	press(m, "v")         // Harmony
	press(m, "g")
	if m.tags == nil || m.tags.sel != 0 {
		t.Fatalf("# did not open the tag bar on the first candidate: %+v", m.tags)
	}
	press(m, "right")
	if bar := ansi.Strip(m.View()); !strings.Contains(bar, "✎ 输入…") && !strings.Contains(bar, "✎ type…") {
		t.Fatalf("no input in the bar:\n%s", bar)
	}
	press(m, "enter")
	tagged := func(tag string) string {
		out := e.MustRun("list", "--filter", "tag:"+tag)
		var got []string
		for _, n := range []string{"Alpha", "Harmony", "Core"} {
			if strings.Contains(out, n) {
				got = append(got, n)
			}
		}
		return strings.Join(got, " ")
	}
	if got := tagged("美化"); got != "Alpha Harmony" {
		t.Fatalf("after # → enter: %q (status %q)", got, m.status)
	}
	if len(m.marked) != 2 {
		t.Fatal("tagging dropped the selection")
	}
	press(m, "g")
	press(m, "right")
	if s := ansi.Strip(m.View()); !strings.Contains(s, "[✓美化]") {
		t.Fatalf("no the checked Visuals tag:\n%s", s)
	}
	press(m, "enter")
	if got := tagged("美化"); got != "" {
		t.Fatalf("not removed: %q", got)
	}
	// [~half]: Alpha only; enter gives it to Harmony too.
	press(m, "g")
	i := -1
	for j, c := range m.tags.cands {
		if c.tag == "half" {
			i = j
		}
	}
	if i < 0 || m.tags.cands[i].have != 1 {
		t.Fatalf("half: %+v", m.tags.cands)
	}
	for m.tags.sel < i {
		press(m, "right")
	}
	if s := ansi.Strip(m.View()); !strings.Contains(s, "[~half]") {
		t.Fatalf("no [~half] (scrolled to it):\n%s", s)
	}
	press(m, "enter")
	if got := tagged("half"); got != "Alpha Harmony" {
		t.Fatalf("half: %q", got)
	}
	// # ← then type: a new tag.
	press(m, "g")
	press(m, "left")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lib")})
	press(m, "enter")
	if got := tagged("lib"); got != "Alpha Harmony" {
		t.Fatalf("lib: %q", got)
	}
	press(m, "esc") // clears the selection; the tags stay
	if got := tagged("lib"); got != "Alpha Harmony" {
		t.Fatalf("esc removed the tag: %q", got)
	}
	c := &m.cols[cActive]
	// 06 §3.8: rows carry no tags; they are in the detail and in search.
	if row := ansi.Strip(m.drawRow(c, 0, true, 60)); strings.Contains(row, "official-stuff") {
		t.Fatalf("a row shows its tag: %q", row)
	}
}

// Kinsoku: across every width, no wrapped line starts with closing punctuation or ends
// with opening punctuation, nothing is lost, and every line fits.
func TestWrapKinsoku(t *testing.T) {
	t.Parallel()
	texts := []string{
		"RimCrow 的核心伴生模组。支持从游戏获取更加详细的日志。必须放在 Harmony 之后加载。最好放在列表最前端以便获取更加完整的加载日志。",
		"他说：「这是《边缘世界》的模组（测试），好吗？」然后走了！",
		"中文后面跟英文标点.还有(括号)和逗号,以及问号?结束",
	}
	for _, text := range texts {
		for w := 8; w <= 40; w++ { // below that a word glued to CJK closing marks may not fit at all
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
	// Regression: wrapping must keep CJK closing marks with the preceding character.
	lines := wrapSpans([]bbcode.Span{{Text: "支持从游戏获取更加详细的日志。必须放在"}}, 28, lipgloss.NewStyle())
	if got := strings.TrimRight(ansi.Strip(lines[1]), " "); !strings.HasPrefix(got, "志。") {
		t.Fatalf("second line %q", got)
	}
}

// 06 §4.2: the full-screen detail has Def, patches, Harmony, rules and the full history
// as sub-sections, 1–5 jump to them; the column shows a one-line history.
func TestFullSections(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	e.MustRun("rule", "add", "after", "pkg:tester.a", "pkg:brrainz.harmony")
	m := direct(t, e, 120, 40)
	press(m, "home")
	press(m, "down") // Alpha
	id := m.current().id
	runCmd(m, m.loadData(id, false, false))
	col := plainLines(m.detailLayout(detailOpts{width: 50, descMax: 12}))
	if !strings.Contains(col, "history") || !strings.Contains(col, "first seen") {
		t.Fatalf("history line:\n%s", col)
	}
	press(m, "enter")
	m.fsecs = nil
	runCmd(m, m.loadFullSecs())
	m.fullLines(36)
	all := plainLines(m.lastLines)
	t.Log(all)
	for _, want := range []string{"1  Defs  ", "2  patches  ", "3  Harmony", "4  rules", "5  full history  1", "Alpha loads after Harmony (your rule)", "first seen"} {
		if !strings.Contains(all, want) {
			t.Fatalf("full detail lacks %q:\n%s", want, all)
		}
	}
	// text names mods; only the Archo ID field shows the #ref (06 §4.1)
	all = regexp.MustCompile(`(?m)^\s*Archo ID .*$`).ReplaceAllString(all, "")
	if strings.Contains(all, "#") && regexp.MustCompile(`#[a-z0-9]{4}\b`).MatchString(all) {
		t.Fatalf("a #ref is shown:\n%s", all)
	}
	for k := 1; k <= 5; k++ {
		press(m, fmt.Sprint(k))
		m.fullLines(36)
		if got := ansi.Strip(m.lastLines[m.fullOff].text); !strings.HasPrefix(got, fmt.Sprint(k)+"  ") && m.fullOff != max(len(m.lastLines)-36, 0) {
			t.Fatalf("key %d: line %d is %q", k, m.fullOff, got)
		}
	}
	for _, l := range m.lastLines {
		if w := lipgloss.Width(l.text); w > m.w-4 {
			t.Fatalf("line %d cells > %d: %q", w, m.w-4, l.text)
		}
	}
}

// T7 (06 §11): a long description, a long path and mixed Chinese and English all wrap
// within the column at every width and in the full screen; only the column's
// "enter shows all" may cut the text.
func TestT7Wrapping(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	desc := "[h1]说明 Overview[/h1]\nRimCrow 的核心伴生模组。支持从游戏获取更加详细的日志。必须放在 Harmony 之后加载，最好放在列表最前端以便获取更加完整的加载日志（Player.log）。\n" +
		"[list][*]Supports [b]CombatExtended[/b] and Vanilla Expanded Framework 的全部补丁[*]见 https://github.com/example/a-very-long-repository-name/blob/main/docs/compatibility.md[/list]" +
		strings.Repeat("Long English text that keeps going without much punctuation ", 6)
	e.MustRun("note", "pkg:tester.a", "放在 Harmony 之后；D:\\SteamLibrary\\steamapps\\workshop\\content\\294100\\1001\\About\\About.xml 里写错了版本")
	e.MustRun("tag", "add", "pkg:tester.a", "a-really-long-tag-name-for-testing", "中文标签")
	m := direct(t, e, 160, 40)
	press(m, "home")
	press(m, "down")
	runCmd(m, m.loadData(m.current().id, false, false))
	m.dd.Workshop = bbcode.Parse(desc)
	m.dd.Desc.Show = "both"
	m.dd.About = bbcode.Parse("中文 English 混排的 About 简介，含一个很长的路径 C:\\Users\\Someone\\AppData\\Roaming\\RimArcho\\cache\\workshop\\1001.json 以及标点。")
	m.detailTech = true
	m.detail = []string{"#abcd Alpha", "  path: D:\\SteamLibrary\\steamapps\\workshop\\content\\294100\\1001\\Defs\\ThingDefs\\VeryLongFileNameForTesting.xml", "  files: 12 (34 KB)"}
	for _, tech := range []bool{false, true} {
		m.detailTech = tech
		for w := 24; w <= 80; w++ {
			for _, full := range []bool{false, true} {
				o := detailOpts{width: w, descMax: 12, full: full, techText: m.detail}
				if full {
					o.descMax = 0
				}
				for i, l := range m.detailLayout(o) {
					if cw := lipgloss.Width(l.text); cw > w {
						t.Fatalf("tech=%v full=%v w=%d line %d is %d cells: %q", tech, full, w, i, cw, ansi.Strip(l.text))
					}
					if p := ansi.Strip(l.text); strings.Contains(p, "…") && !strings.Contains(p, T("… enter shows all")) {
						t.Fatalf("tech=%v full=%v w=%d line %d cut: %q", tech, full, w, i, p)
					}
				}
			}
		}
	}
}

func TestDetailTextOnly(t *testing.T) {
	t.Parallel()
	e, _ := fixture(t)
	m := direct(t, e, 160, 40)
	press(m, "home")
	press(m, "down")
	runCmd(m, m.loadData(m.current().id, false, false))
	m.dd.Desc.Show = "workshop"
	m.dd.Workshop = bbcode.Parse("[url=https://example.org/docs]Read [b]the guide[/b][/url] now.\n[img]https://images.example.org/a.png[/img]\n[img]https://images.example.org/b.png[/img]")
	for _, full := range []bool{false, true} {
		layout := m.detailLayout(detailOpts{width: 80, full: full})
		text := plainLines(layout)
		var raw strings.Builder
		for _, line := range layout {
			raw.WriteString(line.text)
		}
		if !strings.HasPrefix(text, "Alpha\n") {
			t.Fatalf("header is indented: %q", text)
		}
		if !strings.Contains(text, "Read the guide (example.org) now.") || strings.Count(text, "(example.org)") != 1 {
			t.Fatalf("link domain: %s", text)
		}
		if strings.Contains(raw.String(), "\x1b]1337;") || strings.Contains(raw.String(), "\x1b]8;") {
			t.Fatalf("terminal image or hyperlink escape: %q", text)
		}
		if full {
			if strings.Count(text, "[image  images.example.org]") != 2 || strings.Contains(text, "images ×") {
				t.Fatal(text)
			}
		} else if strings.Count(text, "[images ×2]") != 1 || strings.Contains(text, "images.example.org") {
			t.Fatal(text)
		}
	}
}
