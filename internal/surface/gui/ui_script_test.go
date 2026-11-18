package gui

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// A small reader of the page scripts for the UI contract checks (ui_contract_test.go): it
// blanks comments, matches brackets and splits statements and arguments. It understands
// what the page scripts write, not all of JavaScript; TestUIScriptReader guards it.

// pageScripts returns every page script, keyed by its path under assets ("js/main.js").
func pageScripts(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(assets, "assets/js", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".js" {
			return err
		}
		b, err := fs.ReadFile(assets, name)
		out[strings.TrimPrefix(name, "assets/")] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// jsBlank replaces comments with spaces, keeping offsets and line numbers.
func jsBlank(src string) string {
	b := []byte(src)
	for i := 0; i < len(b); {
		switch {
		case b[i] == '"' || b[i] == '\'' || b[i] == '`':
			i = jsSkipString(src, i)
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '/':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				end = len(src) - i - 4
			}
			for j := i; j < i+end+4; j++ {
				if b[j] != '\n' {
					b[j] = ' '
				}
			}
			i += end + 4
		case b[i] == '/' && jsRegexStart(src, i):
			i = jsSkipRegex(src, i)
		default:
			i++
		}
	}
	return string(b)
}

// jsSkipString returns the index after the string or template literal starting at i.
func jsSkipString(src string, i int) int {
	q := src[i]
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == '\\':
			j++
		case q == '`' && src[j] == '$' && j+1 < len(src) && src[j+1] == '{':
			j = jsMatch(src, j+1)
		case src[j] == q:
			return j + 1
		}
	}
	return len(src)
}

// jsRegexStart reports whether the slash at i starts a regular expression literal.
func jsRegexStart(src string, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
		j--
	}
	if j < 0 {
		return true
	}
	if strings.ContainsRune("(,=:[!&|?{};+-*%<>~^", rune(src[j])) {
		return true
	}
	return strings.HasSuffix(src[:j+1], "return")
}

func jsSkipRegex(src string, i int) int {
	class := false
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '[':
			class = true
		case ']':
			class = false
		case '\n':
			return j
		case '/':
			if !class {
				return j + 1
			}
		}
	}
	return len(src)
}

// jsMatch returns the index of the bracket closing the one at open, or len(src).
func jsMatch(src string, open int) int {
	depth := 0
	for i := open; i < len(src); {
		switch c := src[i]; {
		case c == '"' || c == '\'' || c == '`':
			i = jsSkipString(src, i)
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			if end := strings.Index(src[i+2:], "*/"); end >= 0 {
				i += end + 4
				continue
			}
			return len(src)
		case c == '/' && jsRegexStart(src, i):
			i = jsSkipRegex(src, i)
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return len(src)
}

// jsSplitTop splits s at sep outside brackets and strings.
func jsSplitTop(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == '"' || c == '\'' || c == '`':
			i = jsSkipString(s, i)
			continue
		case c == '(' || c == '[' || c == '{':
			i = jsMatch(s, i) + 1
			continue
		case c == sep:
			out = append(out, s[start:i])
			start = i + 1
		}
		i++
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" {
		out = append(out, s[start:])
	}
	return out
}

// jsCallArgs returns the arguments of the call whose "(" is at open.
func jsCallArgs(src string, open int) []string {
	end := jsMatch(src, open)
	if end >= len(src) {
		return nil
	}
	var out []string
	for _, a := range jsSplitTop(src[open+1:end], ',') {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

var jsFuncDecl = regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(|^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s*)?\([^)]*\)\s*=>\s*`)

// jsFunction returns the body of a module's top-level function, as statements: an arrow
// function's expression becomes "return <expression>".
func jsFunction(src, name string) (string, bool) {
	for _, m := range jsFuncDecl.FindAllStringSubmatchIndex(src, -1) {
		if m[2] >= 0 && src[m[2]:m[3]] == name {
			params := m[1] - 1
			brace := strings.IndexByte(src[jsMatch(src, params):], '{') + jsMatch(src, params)
			return src[brace+1 : jsMatch(src, brace)], true
		}
		if m[4] >= 0 && src[m[4]:m[5]] == name {
			at := m[1]
			if src[at] == '{' {
				return src[at+1 : jsMatch(src, at)], true
			}
			end := strings.IndexByte(src[at:], '\n') + at
			if src[at] == '[' || src[at] == '(' {
				end = jsMatch(src, at) + 1
			}
			return "return " + src[at:end], true
		}
	}
	return "", false
}

// jsStatements splits a function body into its top-level statements. An if keeps its else
// branches; a block statement keeps its braces.
func jsStatements(body string) []string {
	var out []string
	i := 0
	skip := func() {
		for i < len(body) && strings.ContainsRune(" \t\r\n;", rune(body[i])) {
			i++
		}
	}
	var one func() string
	one = func() string {
		skip()
		start := i
		word := regexp.MustCompile(`^(if|for|while)\b\s*`).FindString(body[i:])
		if word != "" {
			i += len(word)
			i = jsMatch(body, i) + 1
			skip0 := func() {
				for i < len(body) && strings.ContainsRune(" \t\r\n", rune(body[i])) {
					i++
				}
			}
			skip0()
			if i < len(body) && body[i] == '{' {
				i = jsMatch(body, i) + 1
			} else {
				one()
			}
			if strings.HasPrefix(word, "if") {
				save := i
				skip0()
				if strings.HasPrefix(body[i:], "else") {
					i += 4
					skip0()
					if i < len(body) && body[i] == '{' {
						i = jsMatch(body, i) + 1
					} else {
						one()
					}
				} else {
					i = save
				}
			}
			return body[start:i]
		}
		for i < len(body) {
			switch c := body[i]; {
			case c == '"' || c == '\'' || c == '`':
				i = jsSkipString(body, i)
				continue
			case c == '(' || c == '[' || c == '{':
				i = jsMatch(body, i) + 1
				continue
			case c == ';' || c == '\n' && jsStatementEnds(body[start:i], body[i:]):
				s := body[start:i]
				i++
				return s
			}
			i++
		}
		return body[start:i]
	}
	for skip(); i < len(body); skip() {
		if s := strings.TrimSpace(one()); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// jsStatementEnds reports whether a line break ends the statement (no semicolon written).
func jsStatementEnds(before, after string) bool {
	b, a := strings.TrimSpace(before), strings.TrimSpace(after)
	if b == "" || a == "" {
		return b != ""
	}
	if strings.ContainsRune(",([{=+-*/&|?:.", rune(b[len(b)-1])) || strings.ContainsRune(".,)]}?:+-*/&|", rune(a[0])) {
		return false
	}
	return true
}

// TestUIScriptReader guards the reader the UI contract checks rely on.
func TestUIScriptReader(t *testing.T) {
	t.Parallel()
	src := "const re = /[)}\\]]/g; // a ) in a comment\nexport function f(a = \")\") {\n  if (a) { return [1, 2]; } else return `x${a + \"}\"}`;\n  b.push(1)\n  return [3]\n}\nexport const g = () => [{ key: \"?\" }];\n"
	blank := jsBlank(src)
	if len(blank) != len(src) || strings.Contains(blank, "comment") {
		t.Fatalf("jsBlank: %q", blank)
	}
	body, ok := jsFunction(blank, "f")
	if !ok {
		t.Fatal("f not found")
	}
	st := jsStatements(body)
	if len(st) != 3 || !strings.HasPrefix(st[0], "if (a)") || !strings.HasSuffix(st[0], "`;") || st[1] != "b.push(1)" || st[2] != "return [3]" {
		t.Fatalf("statements: %q", st)
	}
	if body, ok := jsFunction(blank, "g"); !ok || body != `return [{ key: "?" }]` {
		t.Fatalf("arrow: %q", body)
	}
	if args := jsCallArgs(`Tf("%d of %d", n(1, 2), [a, b])`, 2); len(args) != 3 || args[2] != "[a, b]" {
		t.Fatalf("args: %q", args)
	}
}
