package gui

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// UI rules from 16 §3 (interaction) and §7 (visual design) that can be checked mechanically, on top of TestStyleRules.
// Each problem names the specification section, so the failure says what to read and why.
// Exceptions are listed below with their reason; an exception that no longer matches fails,
// so the lists cannot go stale. Run all UI lint quickly with:
//
//	go test -count=1 -run 'TestStyleRules|TestUIRules|TestEndlessAnimations' ./internal/surface/gui/

// Mod log rows keep the Mod's own rich text, as they keep its <color> (16 §7 log colours).
var italicAllowed = map[string]string{
	".lrow .text .ri": "a Mod log's own <i> markup",
}

// Underline marks a link on hover or keyboard focus. These are underlined at rest.
var underlineAllowed = map[string]string{
	".tag-drag.lead": "the first tag when it has a colour (16 §3.8)",
	".need-name":     "a Mod name link inside Can it run text, which has no link colour",
	".btn.identity-why, .page .btn.identity-why": "the identity question's text-styled answers",
}

// Animations whose timing is part of their own design rather than one of the motion tokens.
// Endless animations are exempt here and governed by endlessWhileWorking instead.
var motionAllowed = map[string]string{
	".need.flash":            "a jumped-to Can it run item holds its tint, then fades",
	".hm-go.starting::after": "Launch's fill while the game starts (16 §2.3 design)",
	".hm-go .dot":            "the running-game dot blinks three times (18)",
}

// Literal colours outside :root tokens: colour-picker drawing and the reference-number overlay.
var hexAllowed = map[string]string{
	".tm-swatch.own:not([style])": "the colour wheel swatch for a tag's own colour",
	`.accent-swatch.accent-custom:not(.has-colour), :root[data-scheme="dark"] .accent-swatch.accent-custom:not(.has-colour)`: "the colour wheel swatch for a custom accent",
	".cp-thumb":                       "colour picker thumb, white ring on any colour",
	".cp-light":                       "colour picker lightness track, black to white",
	".cp-light::-webkit-slider-thumb": "colour picker thumb, white ring on any colour",
	".refnum":                         "reference-number overlay, fixed red (16 §3.5 ctrl+\\)",
	".refnum > span":                  "reference-number overlay, fixed red (16 §3.5 ctrl+\\)",
}

// Browser fallbacks that never run in the app window.
var nativeDialogAllowed = map[string]string{
	"js/bg.js": "browser preview without Wails' native file dialog",
}

var (
	cssTime     = regexp.MustCompile(`^-?[\d.]+m?s$`)
	cssEasing   = regexp.MustCompile(`^(?:ease|ease-in|ease-out|ease-in-out|cubic-bezier\(.*\)|steps\(.*\))$`)
	cssHex      = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	cssMotion   = regexp.MustCompile(`^(?:transition|animation)(?:-duration|-timing-function)?$`)
	cssUnderlnP = regexp.MustCompile(`^text-decoration(?:-line)?$`)

	jsKbd    = regexp.MustCompile(`<kbd\b|\b(?:el|createElement)\(\s*["']kbd["']`)
	jsSelect = regexp.MustCompile(`<select\b|\b(?:el|createElement)\(\s*["']select["']`)
	jsDialog = regexp.MustCompile(`\bwindow\.(?:alert|confirm|prompt)\(|(?:^|[^.\w])alert\(`)

	jsMouseOnly = regexp.MustCompile(`\bon(?:dblclick|contextmenu|dragstart)\s*=|addEventListener\(\s*["'](?:dblclick|contextmenu|dragstart)["']`)
	jsKeysNote  = regexp.MustCompile(`//.*\bkeys:\s*\S`)
	jsHistory   = regexp.MustCompile(`\bhistory\.(?:pushState|replaceState|back|forward|go)\(|\blocation\.(?:hash|href)\s*=[^=]`)
	jsTabIndex  = regexp.MustCompile(`\btabIndex\s*=\s*[1-9]|\btabindex=["']?[1-9]|setAttribute\(\s*["']tabindex["']\s*,\s*["']?[1-9]`)
	jsSplit     = regexp.MustCompile(`\bel\(\s*["']div["']\s*,\s*["']split\b`)
	jsClipboard = regexp.MustCompile(`\bclipboard\.writeText\(|execCommand\(\s*["']copy["']`)
)

// user-select: none is for controls only (16 §3.1 rule 13); every selector that takes it is
// listed here. A body.<state> rule that stops selection during a drag or resize is allowed.
var unselectableAllowed = map[string]string{
	"button": "control", "select": "control", "kbd": "key cap", "svg": "icon",
	"#side": "sidebar", "#crumbs": "breadcrumbs", "#hints": "hint line",
	".area .head": "area heading", ".hbar": "area heading bar", ".split": "splitter", ".lib-th": "table header",
	".row": "multi-select row", ".lib-row": "multi-select row", ".tk-row": "multi-select row",
	".fbar-chip": "filter chip", ".drop-tip": "drag hint", ".archo-slot": "Archo's figure",
	".color-pop": "colour picker", ".sv-col": "saves tree row", ".sv-save": "saves tree row",
	".sv-chartscroll.drag": "chart while dragged", ".badge-pop .bp-hint": "key hint in a popover",
	".badge-pop .bp-acts": "action buttons in a popover", ".refnum > span": "reference-number label",
	".filesearch .fs-code-number": "line numbers, so a copy takes only the code",
	".choice-control":             "control", ".choice-menu": "menu",
}

// Visuals named somewhere the check cannot see from the role's line (16 §7 principle 9).
var visualNameAllowed = map[string]string{
	"js/saves.js:progressBox":           "every caller names the box with setProgress, which sets aria-label",
	"js/colorpicker.js:openColorPicker": "paint(), run before the picker shows, names the preview \"new colour #hex\"",
}

var (
	// a role that makes an element a visual: img, meter or progressbar, set by setAttribute
	// (possibly in a ternary) or as an svgEl attribute
	jsVisualRole = regexp.MustCompile(`(\w+)\.setAttribute\(\s*["']role["']\s*,[^)]*["'](?:img|meter|progressbar)["']|\brole:\s*["'](?:img|meter|progressbar)["']`)
	jsFuncName   = regexp.MustCompile(`\bfunction\s+(\w+)\s*\(`)
)

var cssBodyState = regexp.MustCompile(`^body\.[\w-]+(?: \*)?$`)

func TestUIRules(t *testing.T) {
	t.Parallel()
	b, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, p := range uiStyleProblems(string(b), used) {
		t.Error(p)
	}
	err = fs.WalkDir(assets, "assets", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || (path.Ext(name) != ".js" && path.Ext(name) != ".html") {
			return err
		}
		src, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		for _, p := range uiScriptProblems(strings.TrimPrefix(name, "assets/"), string(src), used) {
			t.Error(p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range []map[string]string{italicAllowed, underlineAllowed, motionAllowed, hexAllowed, nativeDialogAllowed, unselectableAllowed} {
		for k := range list {
			if !used[k] {
				t.Errorf("%s: listed as a UI rule exception but nothing needs it any more; remove it from ui_rules_test.go", k)
			}
		}
	}
}

type cssDecl struct {
	sel, name, value string
	line             int
}

// cssDeclList lists every declaration with its rule's selector; @media wrappers are skipped
// and keyframe steps (0%, to) appear as selectors.
func cssDeclList(source string) []cssDecl {
	css := cssComments.ReplaceAllStringFunc(source, func(s string) string {
		b := []byte(s)
		for i := range b {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
		return string(b)
	})
	var out []cssDecl
	for _, m := range cssRule.FindAllStringSubmatchIndex(css, -1) {
		sel := strings.Join(strings.Fields(css[m[2]:m[3]]), " ")
		body := css[m[4]:m[5]]
		for _, d := range cssDeclarations.FindAllStringSubmatchIndex(body, -1) {
			out = append(out, cssDecl{
				sel:   sel,
				name:  body[d[2]:d[3]],
				value: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(body[d[4]:d[5]]), "!important")),
				line:  1 + strings.Count(source[:m[4]+d[2]], "\n"),
			})
		}
	}
	return out
}

func uiStyleProblems(source string, used map[string]bool) []string {
	var problems []string
	report := func(d cssDecl, rule string) {
		problems = append(problems, fmt.Sprintf("assets/style.css:%d: %s { %s: %s } breaks %s", d.line, d.sel, d.name, d.value, rule))
	}
	allowed := func(list map[string]string, sel string) bool {
		if _, ok := list[sel]; ok {
			used[sel] = true
			return true
		}
		return false
	}
	for _, d := range cssDeclList(source) {
		if strings.HasPrefix(d.name, "--") {
			continue // tokens are defined here; the rules apply where they are used
		}
		switch {
		case d.name == "text-transform" && strings.Contains(d.value, "uppercase"):
			report(d, "16 §7.3 rule 8: no uppercase labels; show hierarchy with weight, colour or spacing")
		case d.name == "font-style" && d.value != "normal" && !allowed(italicAllowed, d.sel):
			report(d, "16 §7 principle 5: no italics; bold marks identity only")
		case cssUnderlnP.MatchString(d.name) && strings.Contains(d.value, "underline") && !underlineOnInteraction(d.sel) && !allowed(underlineAllowed, d.sel):
			report(d, "16 §7 principle 5: no underlining, except a link on :hover or :focus-visible")
		case d.name == "backdrop-filter" || d.name == "-webkit-backdrop-filter":
			report(d, "16 §7.1: no backdrop-filter; WebView2 repaints it on every frame")
		case cssMotion.MatchString(d.name) && !strings.Contains(d.value, "infinite") && !allowed(motionAllowed, d.sel):
			if p := motionProblem(d.value); p != "" {
				report(d, p)
			}
		case d.name == "user-select" && d.value == "none":
			for _, part := range splitStyleShadows(d.sel) {
				part = strings.TrimSpace(part)
				if !cssBodyState.MatchString(part) && !allowed(unselectableAllowed, part) {
					report(d, "16 §3.1 rule 13: text stays selectable; user-select: none only on controls (add "+part+" to unselectableAllowed only if it is one)")
				}
			}
		case cssHex.MatchString(d.value) && !strings.HasPrefix(d.sel, ":root") && !strings.HasPrefix(d.name, "mask") && !allowed(hexAllowed, d.sel):
			report(d, "16 §7: colours come from the variables in :root; name a token there instead of a literal")
		}
	}
	return problems
}

// motionProblem checks each comma-separated transition or animation: the first time is its
// duration and must be a token; a later time is a delay, which choreography may set freely.
func motionProblem(value string) string {
	for _, item := range splitStyleShadows(value) {
		durationSeen := false
		for _, tok := range cssTokens.FindAllString(item, -1) {
			switch {
			case strings.HasPrefix(tok, "var(--t-"):
				durationSeen = true
			case cssTime.MatchString(tok):
				f, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(tok, "s"), "m"), 64)
				if !durationSeen && f != 0 {
					return "16 §7.1: durations use var(--t-press), var(--t-enter), var(--t-exit), var(--t-layout) or var(--t-flash)"
				}
				durationSeen = true
			case cssEasing.MatchString(tok):
				return "16 §7.1: easing uses var(--ease) for entry and movement, var(--ease-in) for exits"
			}
		}
	}
	return ""
}

// underlineOnInteraction reports whether every selector in the list underlines only on hover or focus.
func underlineOnInteraction(sel string) bool {
	for _, part := range splitStyleShadows(sel) {
		if !strings.Contains(part, ":hover") && !strings.Contains(part, ":focus-visible") {
			return false
		}
	}
	return true
}

func uiScriptProblems(name, source string, used map[string]bool) []string {
	var problems []string
	lines := strings.Split(source, "\n")
	wired := strings.Contains(source, "wireSeparator(")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		report := func(rule string) {
			problems = append(problems, fmt.Sprintf("assets/%s:%d: %s breaks %s", name, i+1, strings.TrimSpace(line), rule))
		}
		if jsMouseOnly.MatchString(line) && !jsKeysNote.MatchString(line) && (i == 0 || !jsKeysNote.MatchString(lines[i-1])) {
			report("16 §3.1 rule 6: a double-click, right-click or drag needs a keyboard path; implement it and name it in a `// keys: …` comment on this line or the line above")
		}
		if jsHistory.MatchString(line) {
			report("16 §3.1 rule 11: back and forward go through go() and the app's own history, never the browser's history or address")
		}
		if jsTabIndex.MatchString(line) {
			report("16 §3.5: tab moves through controls in page order; no positive tabIndex")
		}
		if jsSplit.MatchString(line) && !wired {
			report("16 §3.1 rule 10: a card gap is a splitter; wire it with wireSeparator (separator.js) for drag, keys and reset")
		}
		if jsClipboard.MatchString(line) && name != "js/fields.js" {
			report("16 §3.1 rule 12: copy through copy() in fields.js (the window's clipboard first, with feedback), not the page clipboard")
		}
		if jsKbd.MatchString(line) && name != "js/state.js" {
			report("16 §7.3 rule 13: show keys with keyCaps (state.js), not a hand-made <kbd>")
		}
		if jsSelect.MatchString(line) {
			report("16 §3.10: single-value choices use choiceControl (choices.js), not a native <select>")
		}
		if m := jsVisualRole.FindStringSubmatch(line); m != nil && !visualNamed(lines, i, m[1]) {
			fn := ""
			for k := i; k >= 0 && fn == ""; k-- {
				if f := jsFuncName.FindStringSubmatch(lines[k]); f != nil {
					fn = f[1]
				}
			}
			if _, ok := visualNameAllowed[name+":"+fn]; ok {
				used[name+":"+fn] = true
			} else {
				report("16 §7 principle 9: a role img, meter or progressbar needs a non-empty accessible name (aria-label or aria-labelledby, or visTip / progress) set beside it")
			}
		}
		if jsDialog.MatchString(line) {
			if _, ok := nativeDialogAllowed[name]; ok {
				used[name] = true
			} else {
				report("16 §3.1 rule 7: no native browser dialogs; use the popups.js confirmation or an inline form")
			}
		}
	}
	return problems
}

// visualNamed: the element given a visual role at line i is named within the next lines: an
// aria-label or aria-labelledby set on the same variable (or in the same svgEl attributes),
// or a progress() call on it, which names it by its visible text.
func visualNamed(lines []string, i int, v string) bool {
	end := min(len(lines), i+12)
	for _, l := range lines[i:end] {
		if v == "" {
			if strings.Contains(l, `"aria-label":`) || strings.Contains(l, `"aria-labelledby":`) {
				return true
			}
			continue
		}
		q := regexp.QuoteMeta(v)
		// a name that is a value (not "") or a progress(v, label) call
		if regexp.MustCompile(`\b` + q + `\.setAttribute\(\s*["']aria-(?:label|labelledby)["']\s*,\s*(?:[^"'\s)]|["'][^"']+["'])|\bprogress\(\s*` + q + `\b`).MatchString(l) {
			return true
		}
	}
	return false
}

// These examples guard the lint itself.
func TestUIRulesDetection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		css  string
		want int
	}{
		{":root { --t-x: 300ms; --c: #123456; } .a { transition: opacity var(--t-press), color var(--t-press) var(--ease); }", 0},
		{".a:hover, .b:focus-visible { text-decoration: underline; } .c { text-transform: none; font-style: normal; }", 0},
		{".a { animation: spin .9s linear infinite; } .b { transition: visibility 0s linear var(--t-layout); }", 0},
		{"/* text-transform: uppercase */ .a { color: var(--fg); } :root[data-scheme=\"dark\"] .a { color: #fff; }", 0},
		{".a { text-transform: uppercase; font-style: italic; text-decoration: underline; }", 3},
		{".a:hover, .b { text-decoration: underline; }", 1},
		{".a { transition: opacity .12s; } .b { animation: fade var(--t-enter) ease-out; } .c { animation: fade var(--t-enter) var(--ease) 60ms; animation-delay: 60ms; }", 2},
		{".a { backdrop-filter: blur(8px); color: #fde68a; }", 2},
		{"button, .row { user-select: none; } body.dragging, body.dragging * { user-select: none; } .b { user-select: text; }", 0},
		{".desc, .row { user-select: none; }", 1},
		{"@media (prefers-reduced-motion: reduce) {\n.a { transition: color 1s; }\n}", 1},
	} {
		if got := uiStyleProblems(tc.css, map[string]bool{}); len(got) != tc.want {
			t.Errorf("%s: got %v; want %d failures", tc.css, got, tc.want)
		}
	}
	got := uiStyleProblems("\n\n.a {\n  color: red;\n  font-style: italic;\n}", map[string]bool{})
	if len(got) != 1 || !strings.HasPrefix(got[0], "assets/style.css:5: .a { font-style: italic }") {
		t.Errorf("wrong source location: %v", got)
	}
	for _, tc := range []struct {
		name, js string
		want     int
	}{
		{"js/x.js", `b.append(keyCaps("enter")); const c = choiceControl(T("x"), opts, v); confirm(T("delete?"), [], T("delete"), go);`, 0},
		{"js/x.js", "// el(\"kbd\") in a comment\nexport function confirm(title) {}", 0},
		{"js/state.js", `const cap = (k) => el("kbd", "", k);`, 0},
		{"js/x.js", "q.append(el(\"kbd\", \"\", \"↵\"));\nreturn `<button>${t}<kbd>u</kbd></button>`;", 2},
		{"js/x.js", `const s = el("select", "field"); const h = "<select>";`, 1},
		{"js/x.js", "window.prompt(\"path\");\nalert(\"x\");\nwindow.confirm(\"y\");", 3},
		{"js/x.js", "r.ondblclick = () => open(); // keys: enter\n// keys: Menu, shift+F10\nr.oncontextmenu = (e) => menu(e);", 0},
		{"js/x.js", "r.ondblclick = () => open();\nbox.addEventListener(\"contextmenu\", m);\nc.ondragstart = (e) => {};", 3},
		{"js/x.js", "history.pushState({}, \"\", \"#x\");\nlocation.hash = \"#y\";\nif (location.hash == \"#y\") go();", 2},
		{"js/x.js", "b.tabIndex = -1; c.tabIndex = 0; d.tabIndex = 2;", 1},
		{"js/x.js", "const gap = el(\"div\", \"split x-split\");", 1},
		{"js/x.js", "const gap = el(\"div\", \"split x-split\");\nwireSeparator(gap, {});", 0},
		{"js/x.js", "navigator.clipboard.writeText(t); const p = await navigator.clipboard.readText();", 1},
		{"js/fields.js", "await navigator.clipboard.writeText(text);", 0},
		{"js/x.js", "bar.setAttribute(\"role\", \"img\");\nbar.setAttribute(\"aria-label\", text);", 0},
		{"js/x.js", "s.setAttribute(\"role\", busy ? \"progressbar\" : \"status\");\nif (busy) progress(s, s);", 0},
		{"js/x.js", "const svg = svgEl(\"svg\", { role: \"img\",\n  \"aria-label\": Tf(\"%d days\", n) });", 0},
		{"js/x.js", "p.setAttribute(\"role\", \"img\");\np.append(a, b);", 1},
		{"js/x.js", "m.setAttribute(\"role\", \"meter\");\nm.setAttribute(\"aria-label\", \"\");", 1},
	} {
		if got := uiScriptProblems(tc.name, tc.js, map[string]bool{}); len(got) != tc.want {
			t.Errorf("%s: %s: got %v; want %d failures", tc.name, tc.js, got, tc.want)
		}
	}
}
