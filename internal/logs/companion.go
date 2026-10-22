package logs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Companion logs (03 §6.7): one JSON object per line, written by an in-game companion mod —
// Archo Monitor's ArchoMonitor.log, or RimCrow Companion's RimCrow_Realtime.log (and its old
// name RMM_Realtime.log). Archo Monitor's format is a superset of RimCrow's, so one reader
// handles both.

const (
	// MonitorLog is Archo Monitor's file in the save data root; the previous run's is
	// MonitorPrevLog.
	MonitorLog     = "ArchoMonitor.log"
	MonitorPrevLog = "ArchoMonitor-prev.log"
	// companionMax bounds the entries kept per file; more only update existing ones.
	companionMax = 20000
)

// RimCrowLogs are RimCrow's companion names, newest first, for the current run and the
// previous one (read only, never renamed).
var RimCrowLogs = [2][]string{
	{"RimCrow_Realtime.log", "RMM_Realtime.log"},
	{"RimCrow_Realtime-prev.log", "RMM_Realtime-prev.log"},
}

// SessionRe finds Archo Monitor's marker in Player.log: "[Archo Monitor] 0.9.0 session
// 3f2a9c10ab12: D:\…\ArchoMonitor.log".
var SessionRe = regexp.MustCompile(`\[Archo Monitor\] (\S+) session ([0-9a-f]{12})(?::\s*(.*?))?\s*$`)

// CompanionEntry is one distinct entry of a companion log, its lines merged.
type CompanionEntry struct {
	Key      string // Archo Monitor's id, else level|normalised message
	Match    string // Normalize(first message line): how clusters find it
	Level    string // ERROR, WARNING, INFO
	Message  string // as written, RimCrow's "[Mod, …] " prefix removed
	Details  string
	At       time.Time // the first line's time
	Count    int       // occurrences: the sum of count, or the highest total
	Mods     []string
	Roles    map[string]string
	Files    []string
	Refs     []string
	Inferred string
	Def      string
	Monitor  bool // written by Archo Monitor
}

// Companion is a companion log read so far.
type Companion struct {
	Path     string
	Monitor  bool   // the file is Archo Monitor's (its first line is a session line)
	Session  string // Archo Monitor's session id
	Version  string // Archo Monitor's version
	Started  time.Time
	BadLines int
	Entries  []*CompanionEntry

	by      map[string]*CompanionEntry
	byMatch map[string]*CompanionEntry
	off     int64
	pending []byte
	lines   int
}

// NewCompanion starts reading path from its beginning.
func NewCompanion(path string) *Companion {
	return &Companion{Path: path, by: map[string]*CompanionEntry{}, byMatch: map[string]*CompanionEntry{}}
}

// ReadCompanion reads a whole companion log.
func ReadCompanion(path string) (*Companion, error) {
	c := NewCompanion(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 256<<20))
	if err != nil {
		return nil, err
	}
	c.Feed(b, true)
	return c, nil
}

type companionLine struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Details   string `json:"details"`
	Count     *int   `json:"count"`
	Context   struct {
		RelatedModIDs []string          `json:"relatedModIds"`
		RelatedFiles  []string          `json:"relatedFiles"`
		InferredType  string            `json:"inferredType"`
		Roles         map[string]string `json:"roles"`
		Refs          []string          `json:"refs"`
		Def           string            `json:"def"`
		Session       string            `json:"session"`
		Version       string            `json:"version"`
	} `json:"context"`
	Source string `json:"source"`
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Total  int    `json:"total"`
	Kind   string `json:"kind"`
}

// Feed adds bytes read from the file; a last line without a newline waits for the next call
// unless final is set. It returns the entries that appeared.
func (c *Companion) Feed(b []byte, final bool) []*CompanionEntry {
	c.pending = append(c.pending, b...)
	end := bytes.LastIndexByte(c.pending, '\n')
	var chunk []byte
	switch {
	case end >= 0:
		chunk = c.pending[:end+1]
	case final:
		chunk = c.pending
	default:
		return nil
	}
	var fresh []*CompanionEntry
	sc := bufio.NewScanner(bytes.NewReader(chunk))
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		if e := c.line(bytes.TrimSpace(sc.Bytes())); e != nil {
			fresh = append(fresh, e)
		}
	}
	if end >= 0 && !final {
		c.pending = append([]byte(nil), c.pending[end+1:]...)
	} else {
		c.pending = nil
	}
	return fresh
}

func (c *Companion) line(b []byte) *CompanionEntry {
	if len(b) == 0 {
		return nil
	}
	c.lines++
	var j companionLine
	if json.Unmarshal(b, &j) != nil || j.Message == "" && j.Kind == "" {
		c.BadLines++
		return nil
	}
	at, _ := time.ParseInLocation("2006-01-02 15:04:05.000", j.Timestamp, time.Local)
	switch j.Kind {
	case "session":
		if c.lines == 1 {
			c.Monitor, c.Session, c.Version, c.Started = true, j.Context.Session, j.Context.Version, at
		}
		return nil
	case "exit", "full":
		return nil
	}
	if j.Source == "archo-monitor" {
		c.Monitor = true
	}
	level := strings.ToUpper(strings.TrimSpace(j.Level))
	msg := j.Message
	if j.Source != "archo-monitor" && len(j.Context.RelatedModIDs) > 0 && strings.HasPrefix(msg, "[") {
		// RimCrow prefixes the mods it found: "[Mod A, Mod B] message".
		if i := strings.Index(msg, "] "); i > 0 {
			msg = msg[i+2:]
		}
	}
	first := msg
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	match := Normalize(strings.TrimSpace(first))
	key := j.ID
	if key == "" || j.Source != "archo-monitor" {
		key = level + "|" + match
	}
	n := 1
	if j.Count != nil {
		n = *j.Count
	}
	e := c.by[key]
	if e == nil {
		if len(c.Entries) >= companionMax {
			return nil
		}
		e = &CompanionEntry{Key: key, Match: match, Level: level, Message: msg, Details: j.Details, At: at,
			Mods: lower(j.Context.RelatedModIDs), Roles: j.Context.Roles, Inferred: j.Context.InferredType, Def: j.Context.Def,
			Monitor: j.Source == "archo-monitor"}
		e.Files, e.Refs = splitFiles(j.Context.RelatedFiles)
		e.Refs = appendNew(e.Refs, j.Context.Refs...)
		c.by[key] = e
		if c.byMatch[match] == nil {
			c.byMatch[match] = e
		}
		c.Entries = append(c.Entries, e)
		e.Count = n
		if j.Total > e.Count {
			e.Count = j.Total
		}
		return e
	}
	e.Count += n
	if j.Total > e.Count {
		e.Count = j.Total
	}
	if e.Details == "" {
		e.Details = j.Details
	}
	f, r := splitFiles(j.Context.RelatedFiles)
	e.Files = appendNew(e.Files, f...)
	e.Refs = appendNew(e.Refs, append(r, j.Context.Refs...)...)
	for _, m := range lower(j.Context.RelatedModIDs) {
		e.Mods = appendNew(e.Mods, m)
	}
	return nil
}

// Lookup finds the entry of a log message (its first line, normalised).
func (c *Companion) Lookup(message string) *CompanionEntry {
	if c == nil {
		return nil
	}
	if i := strings.IndexByte(message, '\n'); i >= 0 {
		message = message[:i]
	}
	return c.byMatch[Normalize(strings.TrimSpace(message))]
}

// Poll reads what the file gained since the last call (live tracking, 03 §6.6). A file that
// got shorter started over (the game restarted): everything is forgotten.
func (c *Companion) Poll(maxRead int64) ([]*CompanionEntry, error) {
	fi, err := os.Stat(c.Path)
	if err != nil {
		return nil, nil
	}
	if fi.Size() < c.off {
		*c = *NewCompanion(c.Path)
	}
	if fi.Size() == c.off {
		return nil, nil
	}
	f, err := os.Open(c.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n := min(fi.Size()-c.off, maxRead)
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, c.off); err != nil && err != io.EOF {
		return nil, err
	}
	c.off += n
	return c.Feed(buf, false), nil
}

// FirstSession reads the session id of an Archo Monitor log's first line ("" if it has none).
func FirstSession(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, 64<<10))
	l, _ := r.ReadBytes('\n')
	var j companionLine
	if json.Unmarshal(bytes.TrimSpace(l), &j) != nil || j.Kind != "session" {
		return ""
	}
	return j.Context.Session
}

// pseudoFile reports RimCrow's non-file entries of relatedFiles.
func pseudoFile(s string) bool {
	for _, p := range []string{"patch:", "xref:", "def:", "loadID:", "<root:"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// splitFiles separates real paths from RimCrow's references.
func splitFiles(l []string) (files, refs []string) {
	for _, s := range l {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if pseudoFile(s) {
			refs = appendNew(refs, s)
		} else {
			files = appendNew(files, s)
		}
	}
	return files, refs
}

func lower(l []string) []string {
	var out []string
	for _, s := range l {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = appendNew(out, s)
		}
	}
	return out
}

func appendNew(l []string, xs ...string) []string {
	for _, x := range xs {
		dup := false
		for _, y := range l {
			if x == y {
				dup = true
				break
			}
		}
		if !dup && x != "" {
			l = append(l, x)
		}
	}
	return l
}

// HintClass maps a companion's inferredType to a cluster class (03 §6.7.4): only used when
// RimArcho's own class is other or exception and the entry is a warning or an error.
func HintClass(inferred, message string) string {
	switch inferred {
	case "DefConfigError", "XMLSyntaxError":
		return ClassXMLLoad
	case "CrossRefError":
		if strings.Contains(message, "loadID") {
			return ClassSave
		}
		return ClassXrefMissing
	case "PatchError":
		return ClassPatchFailed
	case "MissingTexture":
		return ClassTexture
	}
	return ""
}

// Hint gives an entry's class hint ("" for none): the companion's class for a message
// RimArcho classed other or exception (03 §6.7.4).
type Hint func(message string) string

// HintFrom is the hint of a companion log.
func HintFrom(c *Companion) Hint {
	if c == nil {
		return nil
	}
	return func(message string) string {
		e := c.Lookup(message)
		if e == nil || e.Level == "INFO" {
			return ""
		}
		return HintClass(e.Inferred, e.Message)
	}
}

// ClusterKeyHint is ClusterKey with a companion's hint applied.
func ClusterKeyHint(e *Entry, hint Hint) (key, template, class string) {
	key, template, class = ClusterKey(e)
	if hint == nil || (class != ClassOther && class != ClassException) {
		return
	}
	h := hint(e.Message)
	if h == "" || h == class {
		return
	}
	// Re-key with the hinted class: the rest of the key is the same text.
	return h + strings.TrimPrefix(key, class), h + strings.TrimPrefix(template, class), h
}

// ClustersHint is Clusters with a companion's hint applied.
func ClustersHint(f *File, hint Hint) []*Cluster {
	by := map[string]*Cluster{}
	var out []*Cluster
	for _, e := range f.Entries {
		if !Interesting(e) {
			continue
		}
		key, tmpl, cls := ClusterKeyHint(e, hint)
		c := by[key]
		if c == nil {
			c = &Cluster{Key: key, Template: tmpl, Class: cls, First: e}
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

// EntryOf parses a companion entry's message and details as a log entry (an entry that is
// only in the companion log, 03 §6.7.4).
func EntryOf(e *CompanionEntry) *Entry {
	text := e.Message
	if d := strings.TrimSpace(e.Details); d != "" {
		var sb strings.Builder
		sb.WriteString(text)
		for _, l := range strings.Split(d, "\n") {
			l = strings.TrimRight(l, "\r")
			if l != "" && l[0] != ' ' && l[0] != '\t' {
				l = "  " + l // frames are continuation lines
			}
			sb.WriteString("\n")
			sb.WriteString(l)
		}
		text = sb.String()
	}
	f, _ := Parse(strings.NewReader(text))
	if len(f.Entries) == 0 {
		return &Entry{Message: e.Message, Level: levelOf(e.Level)}
	}
	en := f.Entries[0]
	en.Line = 0
	return en
}

func levelOf(l string) string {
	if l == "WARNING" {
		return "WARN"
	}
	return l
}
