// Package logs parses RimWorld's Player.log (03 §6.1): it splits the file into entries
// (a message line plus its stack, Harmony patch lines and continuation lines), resolves
// "[Ref X] Duplicate stacktrace" back to the original stack, normalises messages into
// cluster keys and classifies each cluster. Attribution to mods happens in internal/app.
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
	Line    int // 1-based line of the message
	Message string
	Extra   []string // continuation lines that are not frames (file:, Subnode:, inner messages)
	Frames  []Frame
	Ref     string // "[Ref X]" this entry defines or points to
	Dup     bool   // "Duplicate stacktrace, see ref for original"
}

// File is a parsed log.
type File struct {
	Entries []*Entry
	Lines   int
	Version string // "1.6.4871 rev…" from the header, if found
}

var (
	frameRe   = regexp.MustCompile(`^\s*at (.+?)(?: \(.*)?$`)
	wrapperRe = regexp.MustCompile(`^\s*(?:at )?\(wrapper ([\w-]+)\) (.+?)(?:\s*\(.*)?$`)
	patchRe   = regexp.MustCompile(`^\s*- (PREFIX|POSTFIX|TRANSPILER|FINALIZER|ILMANIPULATOR) ([^:]+): (?:\S+ )?([^\s:(]+):([\w<>.` + "`" + `]+)\(`)
	// A frame without "at " (inner exception stacks): Ns.Type.Method(args) or Ns.Type..ctor(args)
	bareFrameRe = regexp.MustCompile(`^([A-Za-z_][\w+<>` + "`" + `]*(?:\.[\w+<>` + "`" + `]+)*\.\.?[\w<>` + "`" + `]+) ?\([^()]*\)\s*$`)
	refRe       = regexp.MustCompile(`^\[Ref ([0-9A-Fa-f]+)\]( Duplicate stacktrace)?`)
	patchNumRe  = regexp.MustCompile(`_Patch\d+$`)
	versionRe   = regexp.MustCompile(`RimWorld (\d+\.\d+\.\d+ rev\d+)`)
)

func isContinuation(l string) bool {
	if l == "" {
		return false
	}
	switch {
	case l[0] == ' ' || l[0] == '\t':
		return true
	case strings.HasPrefix(l, "[Ref "), strings.HasPrefix(l, "(wrapper "), strings.HasPrefix(l, "Rethrow as "),
		strings.HasPrefix(l, "--- End of"), strings.HasPrefix(l, "file: "), strings.HasPrefix(l, "Subnode:"):
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
	resolveRefs(f)
	return f, sc.Err()
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
		fr.Type, fr.Method = splitMember(m[1])
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
	if i := strings.IndexByte(s, '['); i > 0 { // generic arguments: Foo.Bar[T]
		s = s[:i]
	}
	i := strings.LastIndexByte(s, '.')
	if i < 0 {
		return "", s
	}
	typ, method = s[:i], s[i+1:]
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
	if len(e.Frames) > 0 || strings.Contains(m, "Exception") {
		return ClassException
	}
	return ClassOther
}

var xmlRe = regexp.MustCompile(`XML error|XmlException|Exception parsing|Could not find parent node|parent node named|Duplicate def|Could not find a type named|Exception loading def from file|Exception reading .*\.xml|XML parse|failed to load|Could not find class|Exception loading .* from|is not a valid .*Def|DefOf field .* not found|Config error in`)

func hasFrame(e *Entry, method string) bool {
	for _, fr := range e.Frames {
		if fr.Method == method {
			return true
		}
	}
	return false
}

// Interesting reports whether an entry is an error worth clustering (Player.log mixes in
// plenty of informational lines).
func Interesting(e *Entry) bool {
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
			c = &Cluster{Key: key, Class: cls, First: e}
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
