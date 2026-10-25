package gui

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Keep the source line numbers when ignoring comments, including multiline comments.
var cssComments = regexp.MustCompile(`(?s)/\*.*?\*/`)
var cssDeclarations = regexp.MustCompile(`([\w-]+)\s*:\s*([^;{}]+)`)
var cssTokens = regexp.MustCompile(`(?:var|rgba?|color-mix|cubic-bezier)\([^)]*\)|[^\s/]+|/`)
var cssBorderWidth = regexp.MustCompile(`^border-(?:(?:top|bottom|left|right|block|inline)(?:-start|-end)?(?:-width)?|width)$`)
var cssFontSize = regexp.MustCompile(`^(?:[\d.]+(?:px|em|rem|%|vw|vh)|(?:var|clamp|calc)\()`)

func TestStyleRules(t *testing.T) {
	t.Parallel()
	b, err := fs.ReadFile(assets, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range styleProblems(string(b)) {
		t.Error(problem)
	}
}

func styleProblems(source string) []string {
	// The Swiss-grid home design has its own sizes, rules and radii (06 §2.3, 06 §7.3).
	// Only these three checks are exempt inside the explicit home section; side strips
	// and token definitions still obey the shared rules. Keep offsets for diagnostics.
	const homeStart = "/* home: special case, 06 §7.3 */"
	const homeEnd = "/* end home */"
	start, end := strings.Index(source, homeStart), strings.Index(source, homeEnd)
	var problems []string
	if start >= 0 || end >= 0 {
		if strings.Count(source, homeStart) != 1 || strings.Count(source, homeEnd) != 1 || end < start {
			problems = append(problems, "assets/style.css: invalid home exception boundaries")
			start, end = -1, -1
		}
	}
	css := cssComments.ReplaceAllStringFunc(source, func(s string) string {
		b := []byte(s)
		for i := range b {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
		return string(b)
	})
	decls := cssDeclarations.FindAllStringSubmatchIndex(css, -1)
	vars := map[string][]string{}
	for _, d := range decls {
		name, value := css[d[2]:d[3]], strings.TrimSpace(css[d[4]:d[5]])
		if strings.HasPrefix(name, "--") {
			vars[name] = append(vars[name], value)
		}
	}
	fontOK := func(value string) bool {
		if strings.HasPrefix(value, "var(--") && strings.HasSuffix(value, ")") {
			values := vars[strings.TrimSuffix(strings.TrimPrefix(value, "var("), ")")]
			if len(values) == 0 {
				return false
			}
			for _, v := range values {
				if !styleFontSizeOK(v) {
					return false
				}
			}
			return true
		}
		return styleFontSizeOK(value)
	}
	for _, d := range decls {
		name, value := css[d[2]:d[3]], strings.TrimSpace(css[d[4]:d[5]])
		value = strings.TrimSpace(strings.TrimSuffix(value, "!important"))
		bad := false
		home := start >= 0 && d[2] > start && d[2] < end
		switch {
		case name == "--r-lg":
			bad = value != "12px"
		case name == "--r-sm":
			bad = value != "6px"
		case name == "--hairline":
			bad = value != "1px" && value != "0.5px"
		case !home && strings.HasPrefix(name, "border-") && strings.HasSuffix(name, "radius"):
			for _, v := range cssTokens.FindAllString(value, -1) {
				if v != "var(--r-lg)" && v != "var(--r-sm)" && v != "50%" && v != "999px" && v != "inherit" && v != "/" {
					bad = true
				}
			}
		case !home && name == "font-size":
			bad = !fontOK(value)
		case !home && name == "font" && value != "inherit":
			found := false
			for _, v := range cssTokens.FindAllString(value, -1) {
				if cssFontSize.MatchString(v) {
					bad, found = !fontOK(v), true
					break // The later number, if present, is line-height.
				}
			}
			bad = bad || !found
		case !home && (name == "border" || cssBorderWidth.MatchString(name)):
			// Shorthands must name their width first; width lists must use only the token or zero.
			tokens := cssTokens.FindAllString(value, -1)
			if len(tokens) == 0 {
				bad = true
				break
			}
			if strings.HasSuffix(name, "width") {
				for _, v := range tokens {
					if !styleLineWidthOK(v) {
						bad = true
					}
				}
			} else {
				bad = !styleLineWidthOK(tokens[0])
			}
		}
		// Side borders may only be neutral structural separators or width/style resets.
		if strings.HasPrefix(name, "border-left") || strings.HasPrefix(name, "border-right") || strings.HasPrefix(name, "border-inline") {
			neutral := strings.Contains(value, "var(--line)") || strings.Contains(value, "var(--line-2)") || value == "transparent"
			reset := styleLineWidthOK(value)
			if !neutral && !reset {
				bad = true
			}
		}
		// A full inset ring and a top edge have zero horizontal offset. Nonzero x is a side strip.
		if name == "box-shadow" || strings.HasPrefix(name, "--") {
			for _, shadow := range splitStyleShadows(value) {
				fields := strings.Fields(shadow)
				for i, f := range fields {
					if f == "inset" && (i+1 >= len(fields) || fields[i+1] != "0") {
						bad = true
					}
				}
			}
		}
		if bad {
			line := 1 + strings.Count(source[:d[2]], "\n")
			problems = append(problems, fmt.Sprintf("assets/style.css:%d: %s: %s", line, name, value))
		}
	}
	return problems
}

func styleFontSizeOK(value string) bool {
	return value == "12px" || value == "13px" || value == "15px" || value == "32px"
}

func styleLineWidthOK(value string) bool {
	return value == "var(--hairline)" || value == "0" || value == "none"
}

func splitStyleShadows(value string) []string {
	depth, start := 0, 0
	var out []string
	for i, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, value[start:i])
				start = i + 1
			}
		}
	}
	return append(out, value[start:])
}

// These examples guard the lint itself: compact/multiline CSS, shorthands, aliases and comments.
func TestStyleRulesDetection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		css  string
		want int
	}{
		{"/* font-size: 7px;\n border: 4px solid red; */\n.a { border-radius: var(--r-sm); font: 600 13px/1.4 sans-serif; border: var(--hairline) solid red; }", 0},
		{":root { --small: 12px; } .a { font-size: var(--small); border-width: 0 var(--hairline); border-top: none; }", 0},
		{".a { outline: 2px solid red; border-right: var(--hairline) solid var(--line-2); box-shadow: inset 0 var(--hairline) 0 black; }", 0},
		{":root { --r-lg: 8px; --r-sm: 4px; --hairline: 2px; }", 3},
		{".a { border-left: var(--hairline) solid #4f46e5; }", 1},
		{".a { border-radius: 8px; font-size: 11px; border: 2px solid red; }", 3},
		{".a { font: 700 clamp(32px, 4vw, 64px)/1 sans-serif; border-top-width: 1px; border-top-left-radius: 4px; }", 3},
		{":root { --small: 11px; } .a { font-size: var(--small); }", 1},
		{".a { border-left: var(--hairline) solid var(--accent); border-right-color: var(--sel); box-shadow: 0 1px 2px black, inset 2px 0 0 var(--accent); }", 3},
		{"/* ignored 中文\n comment */\n.a {\nfont-size: 7px;\n}", 1},
	} {
		got := styleProblems(tc.css)
		if len(got) != tc.want {
			t.Errorf("%s: got %v; want %d failures", tc.css, got, tc.want)
		}
		if strings.Contains(tc.css, "ignored") && len(got) > 0 && !strings.Contains(got[0], ":4: font-size: 7px") {
			t.Errorf("wrong source location: %v", got)
		}
	}
}

// The exception must stop at its closing marker and must not bypass other rules (06 §7.3).
func TestStyleRulesHomeException(t *testing.T) {
	t.Parallel()
	const start = "/* home: special case, 06 §7.3 */"
	const end = "/* end home */"
	const sizes = ".hm-example { font-size: 17px; font: 700 clamp(48px, 7.6vw, 124px)/1 sans-serif; border-top: 2px solid var(--fg); border-radius: 4px; }"
	for _, tc := range []struct {
		css  string
		want int
	}{
		{start + sizes + end, 0},
		{sizes + start + sizes + end + sizes, 8},
		{start + ".hm-example { border-left: 2px solid var(--accent); box-shadow: inset 2px 0 0 var(--accent); }" + end, 2},
		{start + ":root { --r-sm: 4px; --hairline: 2px; }" + end, 2},
		{start + sizes, 5},
		{end + start + sizes, 5},
		{start + start + sizes + end, 5},
		{start + "\n" + sizes + "\n" + end + "\n.a { font-size: 7px; }", 1},
	} {
		got := styleProblems(tc.css)
		if len(got) != tc.want {
			t.Errorf("%s: got %v; want %d failures", tc.css, got, tc.want)
		}
		if strings.Contains(tc.css, ".a {") && len(got) > 0 && !strings.Contains(got[0], ":4: font-size: 7px") {
			t.Errorf("wrong source location after home exception: %v", got)
		}
	}
}
