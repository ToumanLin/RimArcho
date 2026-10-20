// Package logs parses RimWorld's Player.log (03 §6.1): it splits the file into entries
// (a message line plus its stack, Harmony patch lines and continuation lines), resolves
// "[Ref X] Duplicate stacktrace" back to the original stack, normalises messages into
// cluster keys and classifies each cluster. Attribution to mods happens in internal/app.
//
// Two formats are read: Unity's Player.log (game ≤1.6) and the Waylith log of 1.7
// ("=== Waylith log ===" header, then "<date> <time> <LEVEL> [<category>] message" lines
// with continuation lines and a stack under each).
package logs

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// Classes (03 §6.1).
const (
	ClassXMLLoad     = "xml-load"
	ClassPatchFailed = "patch-failed"
	ClassXrefMissing = "xref-missing"
	ClassException   = "exception"
	ClassHarmony     = "harmony-stack"
	ClassMaterial    = "material"
	ClassOther       = "other"
)

// Frame is one stack frame.
type Frame struct {
	Text    string
	Type    string // declaring type, compiler-generated parts removed; '+' kept for nesting
	Method  string
	Patched bool // a Harmony replacement (wrapper dynamic-method …_PatchN)
	Harmony []Patch
}

// Patch is a Harmony line under a frame: "- PREFIX owner: Ret Type:Method(args)".
type Patch struct {
	Kind, Owner, PatchType, PatchMethod, Text string
}

// Entry is one log message with its stack.
type Entry struct {
	Line     int    // 1-based line of the message
	Level    string // Waylith only: TRACE, DEBUG, INFO, WARN, ERROR, FATAL; "" in Unity logs
	Category string // Waylith only: the first [category] of the line ("Verse", "Vulkan")
	Message  string
	Extra    []string // continuation lines that are not frames (file:, Subnode:, inner messages)
	Frames   []Frame
	Ref      string // "[Ref X]" this entry defines or points to
	Dup      bool   // "Duplicate stacktrace, see ref for original"
}

// File is a parsed log.
type File struct {
	Entries []*Entry
	Lines   int
	Version string // "1.6.4871 rev…" from the header, if found
	Waylith bool   // the 1.7 format
}

// Format names.
const (
	FormatUnity   = "unity"
	FormatWaylith = "waylith"
)

// Format returns the log format name.
func (f *File) Format() string {
	if f.Waylith {
		return FormatWaylith
	}
	return FormatUnity
}

var (
	frameRe   = regexp.MustCompile(`^\s*at (.+?)(?: \(.*)?$`)
	wrapperRe = regexp.MustCompile(`^\s*(?:at )?\(wrapper ([\w-]+)\) (.+?)(?:\s*\(.*)?$`)
	patchRe   = regexp.MustCompile(`^\s*- (PREFIX|POSTFIX|TRANSPILER|FINALIZER|ILMANIPULATOR) ([^:]+): (?:\S+ )?([^\s:(]+):([\w<>.` + "`" + `]+)\(`)
	// A frame without "at " (inner exception stacks): Ns.Type.Method(args) or Ns.Type..ctor(args)
	bareFrameRe   = regexp.MustCompile(`^([A-Za-z_][\w+<>` + "`" + `]*(?:\.[\w+<>` + "`" + `]+)*\.\.?[\w<>` + "`" + `]+) ?\([^()]*\)\s*$`)
	refRe         = regexp.MustCompile(`^\[Ref ([0-9A-Fa-f]+)\]( Duplicate stacktrace)?`)
	patchNumRe    = regexp.MustCompile(`_Patch\d+$`)
	genericArgsRe = regexp.MustCompile(`\[[^\[\]]*\]`)
	versionRe     = regexp.MustCompile(`RimWorld (\d+\.\d+\.\d+ rev\d+)`)

	// Waylith (1.7).
	waylithHeader    = "=== Waylith log ==="
	waylithLineRe    = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(?:\.\d+)?\s+([A-Z]+)\s+(.*)$`)
	waylithCatRe     = regexp.MustCompile(`^\[([^\]]*)\]\s?`)
	waylithVersionRe = regexp.MustCompile(`^Version:\s+(\d+\.\d+\.\d+)\.(\d+)\s*$`)
)

func isContinuation(l string) bool {
	if l == "" {
		return false
	}
	switch {
	case l[0] == ' ' || l[0] == '\t':
		return true
	case strings.HasPrefix(l, "[Ref "), strings.HasPrefix(l, "(wrapper "), strings.HasPrefix(l, "Rethrow as "),
		strings.HasPrefix(l, "--- End of"), strings.HasPrefix(l, "file: "), strings.HasPrefix(l, "Subnode:"),
		// .NET exception messages that span lines (ArgumentException and ObjectDisposedException).
		strings.HasPrefix(l, "Parameter name: "), strings.HasPrefix(l, "Actual value was "), strings.HasPrefix(l, "Object name: "):
		return true
	}
	return bareFrameRe.MatchString(l)
}

// Parse reads a Player.log. Very long lines (XML dumps) are cut at 4 KiB.
func Parse(r io.Reader) (*File, error) {
	f := &File{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	var cur *Entry
	afterSubnode := false
	n := 0
	for sc.Scan() {
		n++
		l := strings.TrimRight(sc.Text(), "\r")
		if len(l) > 4096 {
			l = l[:4096]
		}
		if n == 1 && strings.TrimSpace(strings.TrimPrefix(l, "\ufeff")) == waylithHeader {
			f.Waylith = true
		}
		if f.Waylith {
			cur = parseWaylith(f, cur, n, l)
			continue
		}
		if f.Version == "" && n < 50 {
			if m := versionRe.FindStringSubmatch(l); m != nil {
				f.Version = m[1]
			}
		}
		if strings.HasPrefix(l, "(Filename: ") || strings.TrimSpace(l) == "" {
			cur, afterSubnode = nil, false
			continue
		}
		if cur != nil && (isContinuation(l) || afterSubnode) {
			afterSubnode = false
			addLine(cur, l)
			if strings.HasPrefix(l, "Subnode:") {
				afterSubnode = true
			}
			continue
		}
		cur = &Entry{Line: n, Message: l}
		f.Entries = append(f.Entries, cur)
	}
	f.Lines = n
	if f.Waylith {
		for _, e := range f.Entries {
			e.Frames = skipLoggerFrames(e.Frames)
		}
	}
	resolveRefs(f)
	return f, sc.Err()
}

// parseWaylith handles one line of a 1.7 log. Every message starts with a timestamped
// line; anything else (stack frames, but also column-0 continuation lines such as
// "Objects: 1" or "[Ref X]") belongs to the current message. Lines before the first
// message are the header.
func parseWaylith(f *File, cur *Entry, n int, l string) *Entry {
	m := waylithLineRe.FindStringSubmatch(l)
	if m == nil {
		if cur == nil {
			if v := waylithVersionRe.FindStringSubmatch(l); v != nil && f.Version == "" {
				f.Version = v[1] + " rev" + v[2]
			}
			return nil
		}
		if strings.TrimSpace(l) != "" {
			addLine(cur, l)
		}
		return cur
	}
	e := &Entry{Line: n, Level: m[1], Message: m[2]}
	if c := waylithCatRe.FindStringSubmatch(e.Message); c != nil {
		e.Category, e.Message = c[1], e.Message[len(c[0]):]
	}
	if f.Version == "" && e.Category == "Verse" {
		if v := versionRe.FindStringSubmatch(e.Message); v != nil {
			f.Version = v[1]
		}
	}
	f.Entries = append(f.Entries, e)
	return e
}

// loggerFrame reports frames of the logging system itself (Brutal.Logging, Verse.Log and
// Harmony replacements of Verse.Log methods): in 1.7 every message's stack starts with them.
func loggerFrame(fr Frame) bool {
	t := fr.Type
	return strings.HasPrefix(t, "Brutal.Logging.") || t == "Brutal.Logging" ||
		t == "Verse.Log" || strings.HasPrefix(t, "Verse.Log+") ||
		strings.HasPrefix(t, "Waylith.Logging.") || t == "Waylith.Logging"
}

func skipLoggerFrames(frs []Frame) []Frame {
	i := 0
	for i < len(frs) && loggerFrame(frs[i]) && len(frs[i].Harmony) == 0 {
		i++
	}
	return frs[i:]
}

// errorLevel reports a Waylith level that is a warning or an error.
func errorLevel(level string) bool {
	switch level {
	case "WARN", "WARNING", "ERROR", "ERR", "FATAL", "CRITICAL":
		return true
	}
	return false
}

func addLine(e *Entry, l string) {
	if m := refRe.FindStringSubmatch(l); m != nil {
		e.Ref, e.Dup = m[1], m[2] != ""
		return
	}
	if m := patchRe.FindStringSubmatch(l); m != nil {
		p := Patch{Kind: strings.ToLower(m[1]), Owner: strings.TrimSpace(m[2]), PatchType: m[3], PatchMethod: m[4], Text: strings.TrimSpace(l)}
		if len(e.Frames) > 0 {
			e.Frames[len(e.Frames)-1].Harmony = append(e.Frames[len(e.Frames)-1].Harmony, p)
		} else {
			e.Frames = append(e.Frames, Frame{Text: "?", Harmony: []Patch{p}})
		}
		return
	}
	if m := wrapperRe.FindStringSubmatch(l); m != nil {
		fr := Frame{Text: strings.TrimSpace(l)}
		if m[1] == "dynamic-method" {
			fr.Type, fr.Method = splitMember(dedupePatched(m[2]))
			fr.Patched = true
		} else {
			fr.Type, fr.Method = splitMember(m[2])
		}
		e.Frames = append(e.Frames, fr)
		return
	}
	if m := bareFrameRe.FindStringSubmatch(l); m != nil {
		fr := Frame{Text: strings.TrimSpace(l)}
		fr.Type, fr.Method = splitMember(m[1])
		e.Frames = append(e.Frames, fr)
		return
	}
	if m := frameRe.FindStringSubmatch(l); m != nil {
		fr := Frame{Text: strings.TrimSpace(l)}
		name := m[1]
		if i := strings.IndexByte(name, '('); i > 0 { // .NET Core: "at Ns.Type.Method(args) [0x0]"
			name = name[:i]
		}
		if strings.HasPrefix(name, ".") || patchNumRe.MatchString(name) { // Harmony replacement: ".Verse.Log.Error_Patch3"
			name = dedupePatched(strings.TrimPrefix(name, "."))
			fr.Patched = true
		}
		fr.Type, fr.Method = splitMember(name)
		e.Frames = append(e.Frames, fr)
		return
	}
	e.Extra = append(e.Extra, strings.TrimSpace(l))
}

// dedupePatched: "Verse.Pawn.Verse.Pawn.Tick_Patch1" → "Verse.Pawn.Tick".
func dedupePatched(s string) string {
	s = patchNumRe.ReplaceAllString(s, "")
	parts := strings.Split(s, ".")
	for k := 1; k*2 < len(parts); k++ {
		if strings.Join(parts[:k], ".") == strings.Join(parts[k:2*k], ".") {
			return strings.Join(parts[k:], ".")
		}
	}
	return s
}

// splitMember: "Verse.Pawn+<>c__DisplayClass1_0.<Tick>b__0" → ("Verse.Pawn", "Tick").
func splitMember(s string) (typ, method string) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	s = genericArgsRe.ReplaceAllString(s, "") // generic arguments: List`1[T].get_Item, Foo.Bar[T]
	// Lambdas in constructors: "Type+<>c.<.cctor>b__1_0" (the dot would split the name).
	s = strings.ReplaceAll(strings.ReplaceAll(s, "<.cctor>", "<cctor>"), "<.ctor>", "<ctor>")
	i := strings.LastIndexByte(s, '.')
	if i < 0 {
		return "", s
	}
	typ, method = s[:i], s[i+1:]
	if strings.HasSuffix(typ, ".") { // constructor: "Ns.Type..ctor" → ("Ns.Type", ".ctor")
		typ, method = typ[:len(typ)-1], "."+method
	}
	if j := strings.Index(typ, "+<"); j >= 0 { // compiler-generated closure or iterator
		if a, b := strings.IndexByte(method, '<'), strings.IndexByte(method, '>'); a == 0 && b > 1 {
			method = method[1:b]
		} else if a, b := strings.Index(typ[j:], "<"), strings.Index(typ[j:], ">"); a >= 0 && b > a+1 {
			method = typ[j+a+1 : j+b]
		}
		typ = typ[:j]
	}
	if strings.HasPrefix(method, "<") {
		if b := strings.IndexByte(method, '>'); b > 1 {
			method = method[1:b]
		}
	}
	return typ, method
}

func resolveRefs(f *File) {
	orig := map[string]*Entry{}
	for _, e := range f.Entries {
		if e.Ref != "" && !e.Dup && len(e.Frames) > 0 {
			orig[e.Ref] = e
		}
	}
	for _, e := range f.Entries {
		if e.Dup && len(e.Frames) == 0 {
			if o := orig[e.Ref]; o != nil {
				e.Frames = o.Frames
			}
		}
	}
}

// Classify returns the class of an entry (03 §6.1).
func Classify(e *Entry) string {
	m := e.Message
	switch {
	case strings.Contains(m, "] Patch operation ") || strings.HasPrefix(m, "Patch operation ") ||
		strings.HasPrefix(m, "Verse.PatchOperation") && strings.Contains(m, "): Failed"):
		return ClassPatchFailed
	case strings.Contains(m, "Could not resolve cross-reference") || strings.HasPrefix(m, "Could not load reference to"):
		return ClassXrefMissing
	case strings.Contains(m, "_MainTex") || hasFrame(e, "ResolveIcon"):
		return ClassMaterial
	case xmlRe.MatchString(m):
		return ClassXMLLoad
	}
	for _, fr := range e.Frames {
		if len(fr.Harmony) > 0 {
			return ClassHarmony
		}
	}
	// In 1.7 every message has a stack, warnings included: a warning's frames are just
	// its caller, not an exception.
	frames := len(e.Frames) > 0 && !isWarning(e.Level)
	if frames || strings.Contains(m, "Exception") {
		return ClassException
	}
	return ClassOther
}

var xmlRe = regexp.MustCompile(`XML error|XmlException|Exception parsing|Could not find parent node|parent node named|Duplicate def|Could not find a type named|Exception loading def from file|Exception reading .*\.xml|XML parse|failed to load|Could not find class|Exception loading .* from|is not a valid .*Def|DefOf field .* not found|Config error in`)

func isWarning(level string) bool { return level == "WARN" || level == "WARNING" }

func hasFrame(e *Entry, method string) bool {
	for _, fr := range e.Frames {
		if fr.Method == method {
			return true
		}
	}
	return false
}

// Interesting reports whether an entry is an error worth clustering (Player.log mixes in
// plenty of informational lines). A leveled (1.7) entry is interesting when it is a warning
// or an error.
func Interesting(e *Entry) bool {
	if e.Level != "" {
		return errorLevel(e.Level)
	}
	if len(e.Frames) > 0 || Classify(e) != ClassOther {
		return true
	}
	l := strings.ToLower(e.Message)
	for _, w := range []string{"error", "exception", "failed", "could not", "couldn't", "invalid", "missing", "not found"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

var (
	normHex  = regexp.MustCompile(`0x[0-9A-Fa-f]+|\b[0-9a-f]{16,}\b`)
	normNum  = regexp.MustCompile(`\d+`)
	normPath = regexp.MustCompile(`[A-Za-z]:[\\/][^\s"'<>]*`)
)

// Normalize turns a message into its cluster key part: numbers, addresses, IDs and paths
// become placeholders.
func Normalize(m string) string {
	if len(m) > 300 {
		m = m[:300]
	}
	m = normPath.ReplaceAllString(m, "<path>")
	m = normHex.ReplaceAllString(m, "<hex>")
	m = normNum.ReplaceAllString(m, "#")
	return m
}

// Cluster groups entries with the same class, normalised message and top frame.
type Cluster struct {
	ID    string // E1, E2 … in order of first occurrence
	Key   string
	Class string
	Level string // the first entry's level (1.7 logs); "" in Unity logs
	Count int
	First *Entry
	Lines []int // first few occurrences
}

// Clusters returns the error clusters of a log.
func Clusters(f *File) []*Cluster {
	by := map[string]*Cluster{}
	var out []*Cluster
	for _, e := range f.Entries {
		if !Interesting(e) {
			continue
		}
		cls := Classify(e)
		key := cls + "|" + Normalize(e.Message)
		if cls == ClassPatchFailed {
			key += "|" + strings.Join(e.Extra, "|") // the file distinguishes failures
		}
		if len(e.Frames) > 0 {
			key += "|" + e.Frames[0].Type + "." + e.Frames[0].Method
		}
		c := by[key]
		if c == nil {
			c = &Cluster{Key: key, Class: cls, Level: e.Level, First: e}
			by[key] = c
			out = append(out, c)
			c.ID = "E" + itoa(len(out))
		}
		c.Count++
		if len(c.Lines) < 5 {
			c.Lines = append(c.Lines, e.Line)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// GameNamespace reports frames of the game, Unity, .NET or Harmony itself.
func GameNamespace(t string) bool {
	for _, p := range []string{"Verse.", "RimWorld.", "System.", "UnityEngine.", "HarmonyLib.", "Mono.", "Unity.", "MonoMod.", "Ionic.", "LudeonTK.", "Steamworks."} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return t == "Verse" || t == "RimWorld" || t == "" || t == "?"
}
