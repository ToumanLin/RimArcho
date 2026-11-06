// Package logs parses RimWorld's Player.log (06 §1): it splits the file into entries
// (a message line plus its stack, Harmony patch lines and continuation lines), resolves
// "[Ref X] Duplicate stacktrace" back to the original stack, normalises messages into
// cluster keys and classifies each cluster. Attribution to mods happens in internal/app.
//
// Two formats: the Unity Player.log of RimWorld up to 1.6, and the Waylith log of 1.7
// ("=== Waylith log ===", every message "<date> <time> <LEVEL> [<category>] <message>" followed by
// the exception stack, if any, and the caller stack).
package logs

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"
)

// Classes (06 §1).
const (
	ClassXMLLoad     = "xml-load"
	ClassPatchFailed = "patch-failed"
	ClassXrefMissing = "xref-missing"
	ClassException   = "exception"
	ClassHarmony     = "harmony-stack"
	ClassMaterial    = "material"
	ClassTexture     = "texture-missing"
	ClassSave        = "save-load"
	// ClassGameError: a 1.6 Log.Error line without a stack in the game's own wording ("Tried to
	// …") that names a type outside the game (GameError).
	ClassGameError = "game-error"
	ClassModReport = "mod-report"
	ClassOther     = "other"
	// ClassNoise: a known engine line that reads like a problem but that no def, file or mod
	// setting causes (KnownNoise).
	ClassNoise = "noise"
)

// Classes lists the classes from most to least severe, for display.
var Classes = []string{ClassException, ClassHarmony, ClassXMLLoad, ClassPatchFailed, ClassXrefMissing,
	ClassMaterial, ClassTexture, ClassSave, ClassGameError, ClassModReport, ClassOther, ClassNoise}

// Informational reports classes that are context rather than problems: a mod reporting on itself
// ("[Mod] … not found, skipping"), unclassified lines and known noise.
func Informational(class string) bool {
	return class == ClassModReport || class == ClassOther || class == ClassNoise
}

// Noise is a known engine line that reads like a problem but has no cause an agent can find in
// a def, a file or the list (MCP transcript review 2026-11-04 §2.2: agents chased both in most
// runs). Such lines are clustered as ClassNoise, informational.
type Noise struct {
	ID     string // short id
	Prefix string // the message starts with it (cheap test before re)
	Label  string // a few words for a list line
	Note   string // what it is, honestly; shown in a cluster's detail and next to nearby lines
	re     *regexp.Regexp
}

// knownNoise: the message, not the stack, decides (a companion log's copy carries a stack).
var knownNoise = []*Noise{
	{ID: "solid-color-material", Prefix: "Material 'SolidColorMat-", Label: "material made in code",
		Note: "known noise: a material made in code (SolidColorMaterials, named SolidColorMat-<shader>-<colour>) had its main texture read or set, and its shader has no _MainTex; no def or file causes it, so searching defs or files (uiIconColor, the shader name) finds nothing. Harmless unless something visibly renders wrong",
		re:   regexp.MustCompile(`^Material 'SolidColorMat-[^']*' with Shader '[^']*' doesn't have a texture property`)},
	{ID: "translation-count", Prefix: "Translation data for language ", Label: "translation error count",
		Note: "informational: the game's count of problems in the active language's translation files (the game's and every mod's Languages folder: duplicate keys, injections for defs or fields that are not loaded); they affect only text, never loading, and do not cause the other errors in this log. The count names no mod; the game lists the entries when a translation report is generated, as the message says",
		re:   regexp.MustCompile(`^Translation data for language .* has \d+ errors`)},
}

// KnownNoise returns the known-noise kind of a message, nil if none.
func KnownNoise(message string) *Noise {
	for _, n := range knownNoise {
		if strings.HasPrefix(message, n.Prefix) && n.re.MatchString(message) {
			return n
		}
	}
	return nil
}

// Frame is one stack frame.
type Frame struct {
	Text    string
	Type    string // declaring type, compiler-generated parts removed; '+' kept for nesting
	Method  string
	Patched bool // a Harmony replacement (wrapper dynamic-method …_PatchN)
	Caller  bool // a frame of the stack that logged the message, not of an exception (Waylith, Unity's full stack traces)
	Harmony []Patch
}

// Patch is a Harmony line under a frame: "- PREFIX owner: Ret Type:Method(args)".
type Patch struct {
	Kind, Owner, PatchType, PatchMethod, Text string
}

// Entry is one log message with its stack.
type Entry struct {
	Phase   *Phase `json:",omitempty"`
	Line    int    // 1-based line of the message
	Message string
	Extra   []string // continuation lines that are not frames (file:, Subnode:, inner messages)
	Frames  []Frame
	Ref     string // "[Ref X]" this entry defines or points to
	Dup     bool   // "Duplicate stacktrace, see ref for original"
	Level   string // Waylith: INFO, WARN, ERROR …; "" for Unity logs, which carry no level
	// unityCaller: a Unity "Type:Method (args)" stack began with StackTraceUtility:ExtractStackTrace,
	// so its frames are the logging call's stack, not the exception's.
	unityCaller bool
}

// File is a parsed log.
type File struct {
	HasPhases bool     `json:",omitempty"`
	Phases    []*Phase `json:",omitempty"`
	Entries   []*Entry
	Lines     int
	Version   string // "1.6.4871 rev…" from the header, if found
	Waylith   bool   // the 1.7 log format
}

var (
	frameRe   = regexp.MustCompile(`^\s*at (.+?)(?: \(.*)?$`)
	wrapperRe = regexp.MustCompile(`^\s*(?:at )?\(wrapper ([\w-]+)\) (.+?)(?:\s*\(.*)?$`)
	patchRe   = regexp.MustCompile(`^\s*- (PREFIX|POSTFIX|TRANSPILER|FINALIZER|ILMANIPULATOR) ([^:]+): (?:\S+ )?([^\s:(]+):([\w<>.` + "`" + `]+)\(`)
	// A frame without "at " (inner exception stacks): Ns.Type.Method(args) or Ns.Type..ctor(args)
	bareFrameRe = regexp.MustCompile(`^([A-Za-z_][\w+<>` + "`" + `]*(?:\.[\w+<>` + "`" + `]+)*\.\.?[\w<>` + "`" + `]+) ?\([^()]*\)\s*$`)
	// Unity's own stack format: "Ns.Type/Nested:Method (args)", "List`1<Verse.Thing>:Add(Thing)",
	// "MonoMod.Utils.DynamicMethodDefinition:Verse.Root.Update_Patch1 (Verse.Root)"; some end ":0".
	unityFrameRe   = regexp.MustCompile(`^([A-Za-z_][\w.` + "`" + `/+<>-]*(?:, [\w.` + "`" + `/+<>-]+)*):([\w.<>` + "`" + `|$-]+(?:, [\w.` + "`" + `/+<>-]+)*) ?\([^()]*\)(?::\d+)?\s*$`)
	unityGenericRe = regexp.MustCompile(`([\w` + "`" + `])<[^<>]*>`)
	refRe          = regexp.MustCompile(`^\[Ref ([0-9A-Fa-f]+)\]( Duplicate stacktrace)?`)
	patchNumRe     = regexp.MustCompile(`_Patch\d+$`)
	genericArgsRe  = regexp.MustCompile(`\[[^\[\]]*\]`)
	versionRe      = regexp.MustCompile(`RimWorld (\d+\.\d+\.\d+ rev\d+)`)

	waylithLineRe  = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d+ +([A-Z]+) (?:\[[^\]]*\] )?(.*)$`)
	waylithVerRe   = regexp.MustCompile(`^Version:\s+(\d+\.\d+\.\d+)\.(\d+)`)
	callerFrameRe  = regexp.MustCompile(` \[0x[0-9A-F]+\]$`)
	waylithPatchRe = regexp.MustCompile(`^\s*- (PREFIX|POSTFIX|TRANSPILER|FINALIZER|ILMANIPULATOR) ([^:]+): \S+ <- (.*)$`)
)

// loggerFrame reports a caller frame of the logging system itself.
func loggerFrame(t string) bool {
	t = strings.TrimPrefix(t, ".")
	switch t {
	case "Verse.Log", "UnityEngine.StackTraceUtility", "UnityEngine.DebugLogHandler", "UnityEngine.Logger", "UnityEngine.Debug":
		return true
	}
	return strings.HasPrefix(t, "Brutal.Logging.")
}

// unityCallerFrame reports whether the next Unity-format frame of e belongs to the logging call:
// after StackTraceUtility:ExtractStackTrace, or under a message that is not an exception. Unity's
// own exception stacks ("ArgumentOutOfRangeException: …", "[Ref X] Duplicate stacktrace", frames)
// stay exception frames.
func unityCallerFrame(e *Entry) bool {
	return e.unityCaller || e.Ref == "" && !hasExceptionStack(e) && !strings.Contains(e.Message, "Exception")
}

// unityMember turns Unity's "Type:Method" into the Mono form splitMember reads:
// "Verse.Root/<>c:<Start>b__10_1" → "Verse.Root+<>c.<Start>b__10_1"; generic arguments go;
// a Harmony replacement "MonoMod.Utils.DynamicMethodDefinition:Verse.Root.Update_Patch1" keeps
// only the patched method's name.
func unityMember(s string) string {
	typ, method, _ := strings.Cut(s, ":")
	if i := strings.IndexByte(method, '<'); i > 0 {
		method = method[:i] // Do<System.Type>
	}
	if strings.HasSuffix(typ, "DynamicMethodDefinition") && strings.Contains(method, ".") {
		return method
	}
	typ = strings.ReplaceAll(typ, "/", "+")
	for {
		t := unityGenericRe.ReplaceAllString(typ, "$1")
		if t == typ {
			break
		}
		typ = t
	}
	return typ + "." + strings.TrimPrefix(method, ".") // .ctor
}

func isContinuation(l string) bool {
	if l == "" {
		return false
	}
	switch {
	case l[0] == ' ' || l[0] == '\t':
		return true
	case strings.HasPrefix(l, "[Ref "), strings.HasPrefix(l, "(wrapper "), strings.HasPrefix(l, "Rethrow as "),
		strings.HasPrefix(l, "--- End of"), strings.HasPrefix(l, "file: "), strings.HasPrefix(l, "Subnode:"),
		// "Exception loading UnityEngine.AudioClip from file." is followed by the file.
		strings.HasPrefix(l, "absFilePath: "),
		// .NET exception messages that span lines (ArgumentException and ObjectDisposedException).
		strings.HasPrefix(l, "Parameter name: "), strings.HasPrefix(l, "Actual value was "), strings.HasPrefix(l, "Object name: "),
		// A Mono native frame at column 0: "<0x2693319a2b0 + 0x000d2> <unknown method>".
		strings.HasPrefix(l, "<0x") && strings.Contains(l, "> <unknown method>"):
		return true
	}
	return bareFrameRe.MatchString(l) || unityFrameRe.MatchString(l)
}

// Parse reads a Player.log. Very long lines (XML dumps) are cut at 4 KiB.
func Parse(r io.Reader) (*File, error) {
	p := NewParser()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	for sc.Scan() {
		p.Line(sc.Text())
	}
	p.Close()
	f := p.F
	resolveRefs(f)
	return f, sc.Err()
}

// Parser reads a log line by line (Parse, and the live follower of a running game: 06 §6).
// Entries are appended to F.Entries; the last one may still grow until Open reports false.
type Parser struct {
	phase        *Phase
	F            *File
	cur          *Entry
	afterSubnode bool
	orig         map[string]*Entry // [Ref X] originals, for duplicates that follow them
	// An XML Extensions error block: "[Mod - Start of stack trace]", the failure lines,
	// "[End of stack trace]", then "Source file: <path>" (28% of the log corpus).
	xname  string
	xinner []string
	xended bool
	// HugsLib's log upload starts with its own header ("Log uploaded on …", the Mod list, the
	// Harmony patches) and puts the game's log after "Log file contents:"; every log of the
	// corpus has one, 2,061 lines at the median.
	hugsHeader bool
	start      int // the line before the game's log: 0, or the end of HugsLib's header
}

var xmlExtStartRe = regexp.MustCompile(`^(?:\S+\|0x[0-9A-Fa-f]+\|)?\[(.+) - Start of stack trace\]\s*$`)

// closeBlock turns an XML Extensions block into "[Mod] <first failure line>" with the other
// lines as continuation text.
func (p *Parser) closeBlock() {
	e := p.cur
	if e != nil {
		e.Message = "[" + p.xname + "]"
		if len(p.xinner) > 0 {
			e.Message += " " + p.xinner[0]
			e.Extra = append(e.Extra, p.xinner[1:]...)
		}
	}
	p.xname, p.xinner, p.xended = "", nil, false
}

// NewParser starts an empty log.
func NewParser() *Parser { return &Parser{F: &File{}, orig: map[string]*Entry{}} }

// Open reports whether the last entry can still receive continuation lines.
func (p *Parser) Open() bool { return p.cur != nil }

// Close ends the current entry (the log ended or the game exited).
func (p *Parser) Close() {
	if p.xname != "" {
		p.closeBlock()
	}
	p.finish()
	p.cur = nil
}

// finish resolves the current entry's duplicate stack against an earlier original.
func (p *Parser) finish() {
	e := p.cur
	if e == nil {
		return
	}
	if e.Ref != "" && !e.Dup && len(e.Frames) > 0 {
		if len(p.orig) > 4096 {
			clear(p.orig)
		}
		p.orig[e.Ref] = e
	} else if e.Dup && len(e.Frames) == 0 {
		if o := p.orig[e.Ref]; o != nil {
			e.Frames = o.Frames
		}
	}
}

// Line feeds one line (without its newline).
func (p *Parser) Line(l string) {
	f := p.F
	f.Lines++
	n := f.Lines
	l = strings.TrimRight(l, "\r")
	if len(l) > 4096 {
		l = l[:4096]
	}
	if n == 1 && strings.HasPrefix(l, "Log uploaded on ") {
		p.hugsHeader = true
		return
	}
	if p.hugsHeader {
		// An upload cut short may lack "Log file contents:"; the game's first lines end it too.
		if strings.HasPrefix(l, "Log file contents:") {
			p.hugsHeader, p.start = false, n
			return
		}
		if !strings.HasPrefix(l, "=== Waylith log ===") && !versionRe.MatchString(l) {
			return
		}
		p.hugsHeader, p.start = false, n-1
	}
	head := n-p.start < 50 // the game's version and format are in its first lines
	if !f.Waylith && head && strings.HasPrefix(l, "=== Waylith log ===") {
		p.finish()
		f.Waylith, p.cur = true, nil
		return
	}
	if f.Waylith {
		if p.xname != "" && p.waylithXMLBlock(l) {
			return
		}
		// INFO phase markers must be seen before the usual level filtering.
		if m := waylithLineRe.FindStringSubmatch(l); m != nil && p.marker(m[2], l, n) {
			return
		}
		next := waylithLine(f, p.cur, n, l)
		if next != p.cur {
			p.finish()
		}
		p.cur = next
		if next != nil && next.Line == n {
			next.Phase = p.phase
			if m := xmlExtStartRe.FindStringSubmatch(next.Message); m != nil {
				p.xname = m[1]
			}
		}
		return
	}
	if p.marker(l, "", n) {
		return
	}
	if f.Version == "" && head {
		if m := versionRe.FindStringSubmatch(l); m != nil {
			f.Version = m[1]
		}
	}
	if p.xname != "" {
		switch t := strings.TrimSpace(l); {
		case !p.xended && t == "[End of stack trace]":
			p.xended = true
			return
		case !p.xended:
			if t != "" {
				p.xinner = append(p.xinner, t)
			}
			return
		case strings.HasPrefix(l, "Source file: "):
			p.closeBlock()
			p.cur.Extra = append(p.cur.Extra, "file: "+strings.TrimPrefix(l, "Source file: "))
			p.finish()
			p.cur = nil
			return
		}
		p.closeBlock()
	}
	if m := xmlExtStartRe.FindStringSubmatch(l); m != nil {
		p.finish()
		p.cur = &Entry{Line: n, Message: "[" + m[1] + "]", Phase: p.phase}
		f.Entries = append(f.Entries, p.cur)
		p.xname = m[1]
		return
	}
	if strings.HasPrefix(l, "(Filename: ") || strings.TrimSpace(l) == "" {
		p.finish()
		p.cur, p.afterSubnode = nil, false
		return
	}
	if p.cur != nil && (isContinuation(l) || p.afterSubnode) {
		p.afterSubnode = false
		addLine(p.cur, l)
		if strings.HasPrefix(l, "Subnode:") {
			p.afterSubnode = true
		}
		return
	}
	p.finish()
	p.cur = &Entry{Line: n, Message: l, Phase: p.phase}
	f.Entries = append(f.Entries, p.cur)
}

// waylithXMLBlock takes a line of an XML Extensions block in a 1.7 log: the message is
// "[Mod - Start of stack trace]", the failure lines, "[End of stack trace]" and "Source file:"
// follow without a timestamp, then the caller stack. It closes the block and returns false at the
// first line that is not part of it.
func (p *Parser) waylithXMLBlock(l string) bool {
	t := strings.TrimSpace(l)
	switch {
	case waylithLineRe.MatchString(l), strings.HasPrefix(t, "at "): // a block without its end line
	case !p.xended && t == "[End of stack trace]":
		p.xended = true
		return true
	case !p.xended:
		if t != "" {
			p.xinner = append(p.xinner, t)
		}
		return true
	case strings.HasPrefix(l, "Source file: "):
		p.closeBlock()
		p.cur.Extra = append(p.cur.Extra, "file: "+strings.TrimPrefix(l, "Source file: "))
		return true
	}
	p.closeBlock()
	return false
}

func (p *Parser) marker(msg, line string, n int) bool {
	name, ok := strings.CutPrefix(msg, "[Archo Monitor] phase ")
	if !ok || !ValidPhase(name) {
		return false
	}
	p.finish()
	p.cur, p.afterSubnode = nil, false
	p.F.HasPhases = true
	p.phase = &Phase{Name: name, Index: len(p.F.Phases) + 1, Line: n}
	if len(line) >= 23 {
		p.phase.At, _ = time.ParseInLocation("2006-01-02 15:04:05.000", line[:23], time.Local)
	}
	p.F.Phases = append(p.F.Phases, p.phase)
	return true
}

// waylithLine handles one line of a 1.7 log: a new message, or a continuation of cur (any line
// that does not start with a timestamp, including unindented message lines).
func waylithLine(f *File, cur *Entry, n int, l string) *Entry {
	if m := waylithLineRe.FindStringSubmatch(l); m != nil {
		e := &Entry{Line: n, Message: m[2], Level: m[1]}
		f.Entries = append(f.Entries, e)
		return e
	}
	if cur == nil {
		if m := waylithVerRe.FindStringSubmatch(l); m != nil && f.Version == "" {
			f.Version = m[1] + " rev" + m[2]
		}
		return nil
	}
	if strings.TrimSpace(l) == "" {
		return cur
	}
	if m := waylithPatchRe.FindStringSubmatch(l); m != nil {
		// "- PREFIX owner: Target.Method <- Ret Type:Method(…)": the Unity form without the target.
		l = "- " + m[1] + " " + m[2] + ": " + m[3]
	}
	before := len(cur.Frames)
	addLine(cur, l)
	if len(cur.Frames) > before {
		fr := &cur.Frames[len(cur.Frames)-1]
		// .NET 10 prints a patched method as ".Ns.Type.Method_PatchN(…)".
		fr.Type = strings.TrimPrefix(fr.Type, ".")
		if p := patchNumRe.FindStringIndex(fr.Method); p != nil {
			fr.Method, fr.Patched = fr.Method[:p[0]], true
		}
		if callerFrameRe.MatchString(fr.Text) {
			if loggerFrame(fr.Type) {
				cur.Frames = cur.Frames[:before]
				return cur
			}
			fr.Caller = true
		}
	}
	return cur
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
	if m := unityFrameRe.FindStringSubmatch(l); m != nil {
		s := unityMember(m[1] + ":" + m[2])
		fr := Frame{Text: strings.TrimSpace(l), Patched: patchNumRe.MatchString(s), Caller: unityCallerFrame(e)}
		fr.Type, fr.Method = splitMember(dedupePatched(s))
		if loggerFrame(fr.Type) {
			// StackTraceUtility:ExtractStackTrace heads the stack Unity adds to every message when
			// the player turns full stack traces on.
			e.unityCaller = e.unityCaller || fr.Type == "UnityEngine.StackTraceUtility"
			return
		}
		e.Frames = append(e.Frames, fr)
		return
	}
	if m := wrapperRe.FindStringSubmatch(l); m != nil {
		fr := Frame{Text: strings.TrimSpace(l)}
		if strings.Contains(m[2], ":") { // Unity's format, inside a Unity stack
			m[2], fr.Caller = unityMember(m[2]), unityCallerFrame(e)
		}
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
	if i := strings.IndexByte(s, '('); i > 0 { // .NET 10: "Ns.Type.Method(Args a)", no space
		s = s[:i]
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	s = genericArgsRe.ReplaceAllString(s, "") // generic arguments: List`1[T].get_Item, Foo.Bar[T]
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

// Classify returns the class of an entry (06 §1).
func Classify(e *Entry) string {
	m := e.Message
	switch {
	case KnownNoise(m) != nil:
		return ClassNoise
	case strings.Contains(m, "] Patch operation ") || strings.HasPrefix(m, "Patch operation ") ||
		strings.HasPrefix(m, "Verse.PatchOperation") && strings.Contains(m, "): Failed"):
		return ClassPatchFailed
	case strings.HasPrefix(m, "Verse.PatchOperationSequence: Error in the operation"),
		// An XML Extensions block: "[Mod] Verse.PatchOperationAdd(xpath=…): Failed …". Every
		// operation in a block failed, whatever its class or ending ("): Error", an xpath that
		// goes on to the next line, AnomalyPatch.PatchOperationRemoveIf).
		strings.Contains(m, "] Verse.PatchOperation") && (strings.Contains(m, "): Failed") || strings.Contains(m, ": Error in")),
		strings.HasPrefix(m, "[") && strings.Contains(m, "PatchOperation") && xmlExtOpRe.MatchString(m):
		return ClassPatchFailed
	case strings.Contains(m, "Could not resolve cross-reference") || strings.HasPrefix(m, "Could not load reference to") ||
		defNamedRe.MatchString(m):
		return ClassXrefMissing
	case strings.Contains(m, "_MainTex") || hasFrame(e, "ResolveIcon"):
		return ClassMaterial
	case xmlRe.MatchString(m):
		return ClassXMLLoad
	case textureRe.MatchString(m):
		return ClassTexture
	case saveRe.MatchString(m):
		return ClassSave
	}
	for _, fr := range e.Frames {
		if len(fr.Harmony) > 0 {
			return ClassHarmony
		}
	}
	if hasExceptionStack(e) || strings.Contains(m, "Exception") {
		return ClassException
	}
	if GameError(e) {
		return ClassGameError
	}
	if modReportRe.MatchString(m) && !levelWordRe.MatchString(m) {
		return ClassModReport
	}
	return ClassOther
}

var (
	// DefDatabase.GetNamed: "Failed to find Verse.TerrainDef named X. There are N defs of this type loaded."
	defNamedRe = regexp.MustCompile(`^Failed to find [\w.]+Def named `)
	// "[Mod] Ns.PatchOperationX(": a failed operation of an XML Extensions block; the mod's name
	// may hold brackets ("[[FSF] Vanilla Bionics Expansion] …").
	xmlExtOpRe = regexp.MustCompile(`^\[.+\] (?:[\w.]+\.)?PatchOperation\w*\(`)
	textureRe  = regexp.MustCompile(`^(?:Could not load Texture2D at |Failed to find any textures at |Could not load (?:Texture|Material|Shader|AudioClip|SoundDef)\b)`)
	saveRe     = regexp.MustCompile(`^Could not resolve reference to object with loadID|is referenced \(xml node name: .*\) but is not deep-saved|^Error in PostExposeData|^Could not find think node with key|^Exception loading .* from save|^Could not load .* from save|^Could not get load ID\. `)
	// A mod reporting on itself: "[Mod] …", "<color=…>[Mod]</color> …", "Mod :: …", "MOD: …".
	modReportRe = regexp.MustCompile(`^(?:(?:<color=[^>]*>\s*)?\[[^\]]{2,80}\]|[\w .+-]{2,40} :: |[A-Z][\w.+-]{1,30}: )`)
	// "ERROR: …" is a level written by the engine or a library, not a mod's name.
	levelWordRe = regexp.MustCompile(`^(?i:error|warning|warn|err|info|exception): `)
	// Engine and framework status lines that read like errors but are not. One ^ before all
	// the branches (here and above): with a ^ in each branch Go retries the pattern at every
	// offset of a long message.
	noiseRe = regexp.MustCompile(`^(?:Shader '[^']*': fallback shader '[^']*' not found|\[XML Extensions\] \d+ total patch operations run|Curl error \d+: Callback aborted|Mod .* is missing supported versions list in About\.xml)`)
)

// hasExceptionStack reports frames other than a Waylith caller stack.
func hasExceptionStack(e *Entry) bool {
	for _, fr := range e.Frames {
		if !fr.Caller {
			return true
		}
	}
	return false
}

// Top returns the frame that identifies where an entry happened: the first exception frame, or
// for a Waylith message without an exception the first caller frame after the logging system.
func (e *Entry) Top() *Frame {
	for i := range e.Frames {
		if !e.Frames[i].Caller {
			return &e.Frames[i]
		}
	}
	if len(e.Frames) > 0 {
		return &e.Frames[0]
	}
	return nil
}

var xmlRe = regexp.MustCompile(`XML error|XmlException|Exception parsing|Could not find parent node|parent node named|Duplicate def|Could not find (?:a )?type named|Exception loading def from file|Exception reading .*\.xml|XML parse|failed to load|Could not find class|Exception loading .* from|is not a valid .*Def|DefOf field .* not found|Config error in`)

// GameError reports a message without a stack in the game's own error wording ("Tried to add pawn
// X to lord with lord jobNs.LordJob_Y but …") that names a type outside the game's namespaces.
// 1.6 writes Log.Error without a stack, and these lines hold none of the problem words of
// interesting; the corpus has 124,268 such lines naming mod types (2026-11-05).
func GameError(e *Entry) bool {
	if hasExceptionStack(e) || !strings.HasPrefix(e.Message, "Tried to ") {
		return false
	}
	for _, t := range typeTokRe.FindAllString(e.Message, -1) {
		if strings.Contains(t, ".") && !GameNamespace(t) && !GameNamespace(Unglue(t)) {
			return true
		}
	}
	return false
}

// typeTokRe matches a dotted name in a message: Ns.Type, Ns.Type.Method.
var typeTokRe = regexp.MustCompile(`\b[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+\b`)

// Unglue drops a lower-case word that the game glued to a type name: "lord jobNs.LordJob_X"
// prints "jobNs.LordJob_X". It returns t when there is nothing to drop. Callers try t first:
// some namespaces start in lower case ("aRandomKiwi.RimThemes").
func Unglue(t string) string {
	dot := strings.IndexByte(t, '.')
	if dot < 0 {
		dot = len(t)
	}
	for i := 1; i < dot; i++ {
		if t[i] >= 'A' && t[i] <= 'Z' {
			if t[i-1] >= 'a' && t[i-1] <= 'z' && t[0] >= 'a' && t[0] <= 'z' {
				return t[i:]
			}
			return t
		}
	}
	return t
}

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
func Interesting(e *Entry) bool { ok, _ := interesting(e); return ok }

// interesting is Interesting plus the entry's class when it was computed ("" otherwise).
func interesting(e *Entry) (bool, string) {
	if !hasExceptionStack(e) && noiseRe.MatchString(e.Message) {
		return false, ""
	}
	switch e.Level { // Waylith: the level decides
	case "":
	case "WARN", "WARNING", "ERROR", "FATAL", "CRITICAL":
		return true, ""
	default: // INFO, DEBUG, TRACE
		return false, ""
	}
	c := Classify(e)
	if hasExceptionStack(e) || !Informational(c) || c == ClassNoise {
		return true, c
	}
	if strings.HasPrefix(e.Message, "Tried to ") {
		return true, c
	}
	l := strings.ToLower(e.Message)
	for _, w := range []string{"error", "exception", "failed", "could not", "couldn't", "invalid", "missing", "not found"} {
		if strings.Contains(l, w) {
			return true, c
		}
	}
	return false, c
}

var (
	normHex      = regexp.MustCompile(`0x[0-9A-Fa-f]+|\b[0-9a-f]{16,}\b`)
	normNum      = regexp.MustCompile(`\d+`)
	normLordPawn = regexp.MustCompile(`^(Tried to add pawn ).+?( to lord with lord job)`)
	normPath     = regexp.MustCompile(`[A-Za-z]:[\\/][^\s"'<>]*`)
)

// Normalize turns a message into its cluster key part: numbers, addresses, IDs and paths
// become placeholders.
func Normalize(m string) string {
	if len(m) > 300 {
		m = m[:300]
	}
	m = normPath.ReplaceAllString(m, "<path>")
	// Lord.AddPawn repeats every tick for each pawn: one cluster, not one per pawn name.
	m = normLordPawn.ReplaceAllString(m, "${1}<pawn>${2}")
	m = normHex.ReplaceAllString(m, "<hex>")
	m = normNum.ReplaceAllString(m, "#")
	return m
}

var templateRules = []struct {
	re  *regexp.Regexp
	rep string
}{
	{regexp.MustCompile(`</?color[^>]*>`), ""},
	{regexp.MustCompile(`defName="?[^\]"]*"?`), "defName=…"},
	{regexp.MustCompile(`'[^']*'`), "'…'"},
	{regexp.MustCompile(`"[^"]*"`), `"…"`},
	{regexp.MustCompile(`\b(named|loadID|Type|type|field|at|for def|key) [^\s'"(),]+`), "$1 …"},
	{regexp.MustCompile(`(found to give to [\w.]+) [^\s(]+`), "$1 …"}, // the def that wanted it
	{regexp.MustCompile(`\(\w+\)`), "(…)"},                            // TypeByName(WorkGiver_DoBill)
	{regexp.MustCompile(`\b[a-z0-9_]+(?:\.[a-z0-9_]+)+\b`), "<pkg>"},  // packageIds
}

// Template turns a message into a template that also merges messages reported once per def, type,
// texture or mod ("Could not load Texture2D at '…'", "Type … probably needs …"). Cluster keys keep
// these names because attribution needs them; templates are for counting kinds of problems.
func Template(m string) string {
	m = Normalize(m)
	for _, r := range templateRules {
		m = r.re.ReplaceAllString(m, r.rep)
	}
	return m
}

// Cluster groups entries with the same class, normalised message and top frame.
type Cluster struct {
	Phase    *Phase       `json:",omitempty"` // first occurrence, when the run has markers
	Phases   []PhaseCount `json:",omitempty"`
	ID       string       // E1, E2 … in order of first occurrence
	Key      string
	Template string // class|Template(message): the kind of problem, across defs, types and mods
	Class    string
	Count    int
	First    *Entry
	Lines    []int // first few occurrences
}

// ClusterKey is how Clusters groups an entry: the key, the template and the class.
func ClusterKey(e *Entry) (key, template, class string) { return clusterKey(e, Classify(e)) }

func clusterKey(e *Entry, class string) (key, template, cls string) {
	key = class + "|" + Normalize(e.Message)
	if class == ClassPatchFailed {
		key += "|" + strings.Join(e.Extra, "|") // the file distinguishes failures
	}
	for _, x := range e.Extra {
		// "Exception loading UnityEngine.Texture2D from file." names the file on the next line.
		if strings.HasPrefix(x, "absFilePath: ") {
			key += "|" + x
		}
	}
	if top := e.Top(); top != nil {
		key += "|" + top.Type + "." + top.Method
	}
	return key, class + "|" + Template(e.Message), class
}

// Clusters returns the error clusters of a log.
func Clusters(f *File) []*Cluster { return ClustersHint(f, nil) }

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
	for _, p := range []string{"Verse.", "RimWorld.", "System.", "UnityEngine.", "HarmonyLib.", "Mono.", "Unity.", "MonoMod.", "Ionic.", "LudeonTK.", "Steamworks.", "Waylith.", "Brutal.", "RimWorldWaylith."} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return t == "Verse" || t == "RimWorld" || t == "" || t == "?"
}
