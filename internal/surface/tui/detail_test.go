package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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
		"requires", "Harmony (not enabled)", "description · Workshop page", "Alpha adds things.", "• one", "[images ×1]", "… enter shows all", "technical information · i"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail lacks %q:\n%s", want, got)
		}
	}
	// the full-screen detail lists each image (no WezTerm: as text)
	if full := plainLines(m.detailLayout(detailOpts{width: 50, full: true})); !strings.Contains(full, "[image · i.imgur.com]") || strings.Contains(full, "images ×") {
		t.Fatalf("full-screen images:\n%s", full)
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

// T13 (06 §12.6): # → enter gives the picked mods the second candidate; the same again
// removes it ([✓]); a tag only some have shows as [~] and enter gives it to all; # ←
// then typing makes a new tag. The list rows show tags faintly.
func TestT13TagBar(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("tag", "add", "pkg:ludeon.rimworld", "official-stuff", "core")
	e.MustRun("tag", "add", "pkg:tester.a", "half")
	m := direct(t, e, 160, 40)
	press(m, "g")
	press(m, "down")
	press(m, "v")         // Alpha
	press(m, "ctrl+down") // the cursor alone
	press(m, "v")         // Harmony
	press(m, "#")
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
	if !m.picking() {
		t.Fatal("tagging ended the pick")
	}
	press(m, "#")
	press(m, "right")
	if s := ansi.Strip(m.View()); !strings.Contains(s, "[✓美化]") {
		t.Fatalf("no [✓美化]:\n%s", s)
	}
	press(m, "enter")
	if got := tagged("美化"); got != "" {
		t.Fatalf("not removed: %q", got)
	}
	// [~half]: Alpha only; enter gives it to Harmony too.
	press(m, "#")
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
	press(m, "#")
	press(m, "left")
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lib")})
	press(m, "enter")
	if got := tagged("lib"); got != "Alpha Harmony" {
		t.Fatalf("lib: %q", got)
	}
	press(m, "esc") // cancel the pick: tags are not part of the list, they stay
	if got := tagged("lib"); got != "Alpha Harmony" {
		t.Fatalf("esc removed the tag: %q", got)
	}
	c := &m.cols[cActive]
	if row := ansi.Strip(m.drawRow(c, 0, true, 60)); !strings.Contains(row, "official-stuff") {
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

func previewPNG(t *testing.T, path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 90
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// T6 (06 §11, §12.4): a mod with Preview.png shows its thumbnail at once; moving on erases
// the whole 18×9 block in the same write; scrolling never draws another mod's thumbnail;
// with ui.preview_dwell_ms set, the thumbnail waits until the cursor rests that long.
func TestT6Thumbnail(t *testing.T) {
	e, _ := fixture(t)
	previewPNG(t, filepath.Join(e.Workshop, "1001", "About", "Preview.png"), 1600, 900)
	m := direct(t, e, 160, 40)
	var buf bytes.Buffer
	m.images = &imgWriter{Writer: &buf}
	frame := func() string {
		m.View()
		buf.Reset()
		m.images.Write([]byte("F"))
		return buf.String()
	}
	press(m, "g")
	press(m, "down") // Alpha, with a 16:9 Preview.png
	id := m.current().id
	if m.thumbShown != id {
		t.Fatalf("no dwell, yet not shown: %q", m.thumbShown)
	}
	runCmd(m, m.imageCmds(m.visibleDetail()))
	out := frame()
	if !strings.Contains(out, "1337;File=inline=1") || !strings.Contains(out, "width=18;height=5;preserveAspectRatio=0") {
		t.Fatalf("thumbnail: %.200q", out)
	}
	m.View()
	ps := m.images.cur
	if len(ps) != 1 || ps[0].cols != 18 || ps[0].rows != 5 || ps[0].clr != 9 {
		t.Fatalf("placement %+v", ps)
	}
	p := ps[0]
	// the text beside it starts after the block
	if l := ansi.Strip(m.lastLines[0].text); !strings.HasPrefix(l, strings.Repeat(" ", 20)+"Alpha") {
		t.Fatalf("header %q", l)
	}
	// moving on: no image, and the whole 18×9 block erased in the same write
	press(m, "down")
	out = frame()
	if strings.Contains(out, "1337;File") {
		t.Fatal("thumbnail kept after moving on")
	}
	for r := 0; r < 9; r++ {
		if want := fmt.Sprintf("\x1b[%d;%dH\x1b[18X", p.row+r, p.col); !strings.Contains(out, want) {
			t.Fatalf("row %d of the block not erased: %q", r, out)
		}
	}
	// scrolling: a thumbnail only ever on its own mod, at once from the cache
	for i := 0; i < 20; i++ {
		press(m, map[bool]string{true: "up", false: "down"}[i%2 == 0])
		out := frame()
		if on := m.current().id == id; on != strings.Contains(out, "1337;File") {
			t.Fatalf("step %d: on Alpha %v, drawn %v", i, on, !on)
		}
	}
	// with a dwell: gone when the selection changes, back after a rest of that length
	m.dwell = 150 * time.Millisecond
	press(m, "g")
	press(m, "down") // Alpha
	if out := frame(); strings.Contains(out, "1337;File") {
		t.Fatal("thumbnail drawn before the dwell")
	}
	t0 := time.Now()
	msg := m.thumbDwell()() // the tick press scheduled, made again
	if d := time.Since(t0); d < 150*time.Millisecond || d > 600*time.Millisecond {
		t.Fatalf("dwell %v", d)
	}
	old := m.thumbSeq
	press(m, "down")
	m.update(thumbTick{old, id}) // stale: changes nothing
	if out := frame(); strings.Contains(out, "1337;File") {
		t.Fatal("stale thumbnail")
	}
	press(m, "up")
	m.update(msg) // stale too
	if m.thumbShown == id {
		t.Fatal("a stale tick showed the thumbnail")
	}
	m.update(thumbTick{m.thumbSeq, id})
	if out := frame(); !strings.Contains(out, "1337;File") {
		t.Fatal("no thumbnail after the dwell")
	}
	m.dwell = 0
	// a mod without Preview.png: after the dwell the block says so
	press(m, "g")
	press(m, "down")
	press(m, "down") // Harmony
	m.update(thumbTick{m.thumbSeq, m.current().id})
	runCmd(m, m.imageCmds(m.visibleDetail()))
	m.View()
	if got := plainLines(m.lastLines); !strings.Contains(got, "no preview") {
		t.Fatalf("no preview note:\n%s", got)
	}
	// the cache: one PNG under cache/previews
	if fs, _ := filepath.Glob(filepath.Join(e.App.Home, "cache", "previews", "*.png")); len(fs) != 1 {
		t.Fatalf("cache files %v", fs)
	}
}

// 06 §6.3: the full-screen detail has Def, patches, Harmony, rules and the full history
// as sub-sections, 1–5 jump to them; the column shows a one-line history.
func TestFullSections(t *testing.T) {
	e, _ := fixture(t)
	e.MustRun("rule", "add", "after", "pkg:tester.a", "pkg:brrainz.harmony")
	m := direct(t, e, 120, 40)
	press(m, "g")
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
	for _, want := range []string{"1  Defs · ", "2  patches · ", "3  Harmony", "4  rules", "5  full history · 1", "Alpha loads after Harmony (your rule)", "first seen"} {
		if !strings.Contains(all, want) {
			t.Fatalf("full detail lacks %q:\n%s", want, all)
		}
	}
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
// within the column at every width, in the column and the full screen, with or without
// the thumbnail block; nothing is cut off with "…" except the column's "enter shows all".
func TestT7Wrapping(t *testing.T) {
	e, _ := fixture(t)
	desc := "[h1]说明 Overview[/h1]\nRimCrow 的核心伴生模组。支持从游戏获取更加详细的日志。必须放在 Harmony 之后加载，最好放在列表最前端以便获取更加完整的加载日志（Player.log）。\n" +
		"[list][*]Supports [b]CombatExtended[/b] and Vanilla Expanded Framework 的全部补丁[*]见 https://github.com/example/a-very-long-repository-name/blob/main/docs/compatibility.md[/list]" +
		strings.Repeat("Long English text that keeps going without much punctuation ", 6)
	e.MustRun("note", "pkg:tester.a", "放在 Harmony 之后；D:\\SteamLibrary\\steamapps\\workshop\\content\\294100\\1001\\About\\About.xml 里写错了版本")
	e.MustRun("tag", "add", "pkg:tester.a", "a-really-long-tag-name-for-testing", "中文标签")
	m := direct(t, e, 160, 40)
	press(m, "g")
	press(m, "down")
	runCmd(m, m.loadData(m.current().id, false, false))
	m.dd.ws = bbcode.Parse(desc)
	m.dd.desc.Show = "both"
	m.dd.about = bbcode.Parse("中文 English 混排的 About 简介，含一个很长的路径 C:\\Users\\Someone\\AppData\\Roaming\\RimArcho\\cache\\workshop\\1001.json 以及标点。")
	m.detailTech = true
	m.detail = []string{"#abcd Alpha", "  path: D:\\SteamLibrary\\steamapps\\workshop\\content\\294100\\1001\\Defs\\ThingDefs\\VeryLongFileNameForTesting.xml", "  files: 12 (34 KB)"}
	var buf bytes.Buffer
	for _, cfg := range []struct{ images, tech bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		images := cfg.images
		m.detailTech = cfg.tech
		if images {
			m.images = &imgWriter{Writer: &buf}
		}
		for w := 24; w <= 80; w++ {
			for _, full := range []bool{false, true} {
				o := detailOpts{width: w, descMax: 12, images: images, full: full, techText: m.detail}
				if full {
					o.descMax = 0
				}
				for i, l := range m.detailLayout(o) {
					if cw := lipgloss.Width(l.text); cw > w {
						t.Fatalf("images=%v full=%v w=%d line %d is %d cells: %q", images, full, w, i, cw, ansi.Strip(l.text))
					}
					if p := ansi.Strip(l.text); strings.Contains(p, "…") && !strings.Contains(p, T("… enter shows all")) {
						t.Fatalf("images=%v full=%v w=%d line %d cut: %q", images, full, w, i, p)
					}
				}
			}
		}
	}
}
