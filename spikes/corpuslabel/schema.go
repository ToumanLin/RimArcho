package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Label is the fixed output of a labeller (design R2: structured, with model and prompt).
// Schema corpuslabel/1 is the pilot's; corpuslabel/2 (SCHEMA.md) tightens cause type and
// fix kind, and adds difficulty, code quotes and version checks.
type Label struct {
	Schema string `json:"schema"` // "corpuslabel/1" or "corpuslabel/2"
	Item   string `json:"item"`
	Gist   string `json:"gist"`
	Key    string `json:"key"`
	// Verdict: "cause" (a cause is named), "unknown" (the evidence does not decide);
	// v1 also had "benign" (v2 says that with fix kind "ignore").
	Verdict string  `json:"verdict"`
	Causes  []Cause `json:"causes"`
	// Stage: when it fails.
	Stage string `json:"stage"`
	// CauseType: why it fails.
	CauseType  string   `json:"cause_type"`
	Fix        Fix      `json:"fix"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"` // verbatim quotes from the packet (or prefixed log:/env:/corpus:)
	// v2
	Code       []CodeQuote    `json:"code,omitempty"`       // decisive decompiled code (tier 2)
	Versions   []VersionCheck `json:"versions,omitempty"`   // log header version vs research library
	Difficulty string         `json:"difficulty,omitempty"` // L1 fact lookup, L2 cross-mod reasoning, L3 needs code
	Model      string         `json:"model"`
	Prompt     string         `json:"prompt"`
	Note       string         `json:"note,omitempty"`
}

// Cause is one mod of the cause. Role:
//
//	source   its own content or code is wrong or out of date (the mod to fix, update or drop)
//	partner  the other side of an incompatibility or ordering problem with a source
//	missing  a mod that should be active and is not (a dependency); pkg need not be in the list
type Cause struct {
	Pkg  string `json:"pkg"`
	Role string `json:"role"`
}

type Fix struct {
	Kind string   `json:"kind"`
	Mods []string `json:"mods,omitempty"`
	Note string   `json:"note,omitempty"`
}

// CodeQuote is a verbatim piece of decompiled C#: File is the .cs file `archo source`
// wrote (a research home's cache/source/…), Lines its 1-based line range.
type CodeQuote struct {
	File   string `json:"file"`
	Type   string `json:"type"`
	Method string `json:"method,omitempty"`
	Lines  string `json:"lines"`
	Text   string `json:"text"`
}

// VersionCheck compares a mod's version in the log header with the research library's copy.
type VersionCheck struct {
	Pkg       string `json:"pkg"`
	Log       string `json:"log"`       // [mv:]/[v:]/[ov:] of the log header, "" when none
	Installed string `json:"installed"` // About.xml modVersion of the library copy, "" when none
	Same      string `json:"same"`      // yes | no | unknown
}

var (
	verdicts   = set("cause", "unknown", "benign")
	roles      = set("source", "partner", "missing")
	stages     = set("assembly-load", "xml-load", "patch-apply", "def-resolve", "static-init", "game-start", "save-load", "map-gen", "tick", "ui", "other")
	causeTypes = set("missing-dependency", "load-order", "incompatible-mods", "outdated-mod", "game-version", "mod-bug", "missing-content", "other")
	fixKinds   = set("disable", "enable-dependency", "reorder", "update", "other", "none")
	confs      = set("high", "medium", "low")
	pkgRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_.\-]*$`)

	// v2 (SCHEMA.md)
	verdicts2    = set("cause", "unknown")
	causeTypes2  = set("missing-dependency", "optional-absent", "load-order", "incompatible-mods", "outdated-mod", "version-mismatch", "mod-bug", "unknown")
	fixKinds2    = set("enable-dependency", "reorder", "update", "disable", "report", "ignore", "unknown")
	difficulties = set("L1", "L2", "L3")
	sames        = set("yes", "no", "unknown")
	// fixFor: the fix kinds a cause type allows (SCHEMA.md "cause type → fix").
	fixFor = map[string]map[string]bool{
		"missing-dependency": set("enable-dependency"),
		"optional-absent":    set("ignore", "enable-dependency"),
		"load-order":         set("reorder"),
		"incompatible-mods":  set("disable", "report", "ignore"),
		"outdated-mod":       set("disable", "report", "ignore"),
		"version-mismatch":   set("update"),
		"mod-bug":            set("report", "disable", "ignore"),
		"unknown":            set("unknown", "ignore", "report", "disable"),
	}
)

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func keys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, "|")
}

// vopts are the optional sources validate checks prefixed quotes against.
type vopts struct {
	Corpus   string // hugslogs folder: "log: " quotes are checked against raw/<gist>.txt
	ExecLogs string // directory of <item>.log transcripts of `corpuslabel exec`: "env: " quotes
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

var rawCache = map[string]string{}

func rawText(path string) string {
	if s, ok := rawCache[path]; ok {
		return s
	}
	b, _ := os.ReadFile(path)
	s := norm(string(b))
	rawCache[path] = s
	return s
}

// validate checks a label against its sample item and packet. Referenced gist ids and
// cluster keys must exist; packageIds must be in the log's list (or, for role missing, at
// least appear in the packet text); quotes must be verbatim. For corpuslabel/2 also the
// tighter enums, the cause type → fix table, code quotes against the decompiled files and,
// when the sources are given, log:/env: quotes against the raw log and the exec transcript.
func validate(l *Label, items map[string]Item, packets map[string]*Packet, o *vopts) []string {
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	v2 := l.Schema == "corpuslabel/2"
	if l.Schema != "corpuslabel/1" && !v2 {
		bad("schema %q", l.Schema)
	}
	it, ok := items[l.Item]
	if !ok {
		bad("item %q not in the sample", l.Item)
		return errs
	}
	p := packets[l.Item]
	if l.Gist != it.Gist {
		bad("gist %q is not the item's", l.Gist)
	}
	if p == nil {
		bad("no packet")
		return errs
	}
	// the packet's "key:" line is clipped at 400 bytes (clip adds "…")
	clipped := strings.TrimSuffix(l.Key, "…")
	if l.Key != p.Key && l.Key != it.Key && !(len(clipped) >= 300 && strings.HasPrefix(p.Key, clipped)) {
		bad("cluster key does not match the item")
	}
	vs, cts, fks := verdicts, causeTypes, fixKinds
	if v2 {
		vs, cts, fks = verdicts2, causeTypes2, fixKinds2
	}
	if !vs[l.Verdict] {
		bad("verdict %q not in %s", l.Verdict, keys(vs))
	}
	inList := map[string]bool{}
	for _, pk := range p.LogMods {
		inList[pk] = true
	}
	lowText := strings.ToLower(p.Text)
	hasRole := map[string]bool{}
	causePk := map[string]bool{}
	for _, c := range l.Causes {
		if !roles[c.Role] {
			bad("role %q not in %s", c.Role, keys(roles))
		}
		hasRole[c.Role] = true
		pk := strings.ToLower(c.Pkg)
		causePk[pk] = true
		switch {
		case !pkgRe.MatchString(pk):
			bad("packageId %q malformed", c.Pkg)
		case c.Role == "missing" && !strings.Contains(lowText, pk) && !(v2 && o != nil && strings.Contains(strings.ToLower(execText(o, l.Item)), pk)):
			bad("missing mod %q appears nowhere in the packet", c.Pkg)
		case c.Role != "missing" && !inList[pk]:
			bad("packageId %q not in the log's mod list", c.Pkg)
		case c.Role == "source" && strings.HasPrefix(pk, "ludeon.rimworld"):
			bad("%q is the game, not a source", c.Pkg)
		}
	}
	if l.Verdict == "cause" && len(l.Causes) == 0 {
		bad("verdict cause without causes")
	}
	if l.Verdict != "cause" && len(l.Causes) > 0 {
		bad("causes given with verdict %q", l.Verdict)
	}
	if !stages[l.Stage] {
		bad("stage %q not in %s", l.Stage, keys(stages))
	}
	if !cts[l.CauseType] {
		bad("cause_type %q not in %s", l.CauseType, keys(cts))
	}
	if !fks[l.Fix.Kind] {
		bad("fix.kind %q not in %s", l.Fix.Kind, keys(fks))
	}
	for _, m := range l.Fix.Mods {
		if pk := strings.ToLower(m); !inList[pk] && !strings.Contains(lowText, pk) && !causePk[pk] {
			bad("fix mod %q not in the list or packet", m)
		}
	}
	if !confs[l.Confidence] {
		bad("confidence %q not in %s", l.Confidence, keys(confs))
	}
	if len(l.Evidence) == 0 && len(l.Code) == 0 {
		bad("no evidence quotes")
	}
	nt := norm(p.Text)
	for _, q := range l.Evidence {
		// A reviewer may cite what the packet lacks: "log: …" (raw log), "env: …" (research
		// environment lookup), "corpus: …" (co-occurrence). v1 does not check those.
		qq := norm(strings.TrimSuffix(strings.TrimSpace(q), "…"))
		switch {
		case strings.HasPrefix(q, "corpus: "):
		case strings.HasPrefix(q, "log: "):
			if v2 && o != nil && o.Corpus != "" {
				if !strings.Contains(rawText(filepath.Join(o.Corpus, "raw", l.Gist+".txt")), norm(strings.TrimPrefix(qq, "log: "))) {
					bad("log quote not in the raw log: %.60q", q)
				}
			}
		case strings.HasPrefix(q, "env: "):
			if v2 && o != nil && o.ExecLogs != "" {
				if !strings.Contains(norm(execText(o, l.Item)), norm(strings.TrimPrefix(qq, "env: "))) {
					bad("env quote not in the exec transcript: %.60q", q)
				}
			}
		default:
			if !strings.Contains(nt, qq) {
				bad("quote not verbatim: %.60q", q)
			}
		}
	}
	if l.Model == "" || l.Prompt == "" {
		bad("model and prompt are required")
	}
	if !v2 {
		return errs
	}
	// --- corpuslabel/2
	// A named source with an undecided reason is allowed (round 1 showed that requiring a cause
	// type made labellers drop clear sources); no source means no cause type.
	if l.Verdict == "unknown" && l.CauseType != "unknown" {
		bad("verdict unknown with cause_type %q", l.CauseType)
	}
	if ok := fixFor[l.CauseType]; ok != nil && !ok[l.Fix.Kind] {
		bad("fix.kind %q does not fit cause_type %q (allowed %s)", l.Fix.Kind, l.CauseType, keys(ok))
	}
	if l.Fix.Kind != "ignore" && l.Fix.Kind != "unknown" && len(l.Fix.Mods) == 0 {
		bad("fix.kind %q needs fix.mods", l.Fix.Kind)
	}
	if l.CauseType == "missing-dependency" && !hasRole["missing"] {
		bad("cause_type missing-dependency without a cause of role missing")
	}
	if (l.CauseType == "incompatible-mods" || l.CauseType == "load-order") && !hasRole["partner"] {
		bad("cause_type %s without a cause of role partner", l.CauseType)
	}
	if !difficulties[l.Difficulty] {
		bad("difficulty %q not in %s", l.Difficulty, keys(difficulties))
	}
	verMismatch := false
	for _, vc := range l.Versions {
		if !sames[vc.Same] {
			bad("versions same %q not in %s", vc.Same, keys(sames))
		}
		if lv, ok := p.LogVers[strings.ToLower(vc.Pkg)]; ok && norm(lv) != norm(vc.Log) {
			bad("versions: log version of %s is %q, not %q", vc.Pkg, lv, vc.Log)
		}
		if vc.Same == "no" && causePk[strings.ToLower(vc.Pkg)] {
			verMismatch = true
		}
	}
	if l.CauseType == "version-mismatch" && !verMismatch {
		bad("cause_type version-mismatch without a versions entry same=no for a cause mod")
	}
	if it.Tier == 2 && l.Verdict == "cause" && l.Difficulty == "L3" && len(l.Code) == 0 {
		bad("tier-2 L3 cause without a code quote")
	}
	for i, c := range l.Code {
		for _, e := range checkCode(c) {
			bad("code[%d]: %s", i, e)
		}
	}
	return errs
}

var linesRe = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)

// checkCode: the file exists, the text is in it (whitespace-insensitive) within the given
// lines (±3), and the type's and method's simple names occur in the file.
func checkCode(c CodeQuote) []string {
	var errs []string
	b, err := os.ReadFile(c.File)
	if err != nil {
		return []string{"file not readable: " + c.File}
	}
	if !strings.HasSuffix(strings.ToLower(c.File), ".cs") {
		errs = append(errs, "not a .cs file")
	}
	ls := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	m := linesRe.FindStringSubmatch(strings.TrimSpace(c.Lines))
	if m == nil {
		return append(errs, fmt.Sprintf("lines %q not a-b", c.Lines))
	}
	a, _ := strconv.Atoi(m[1])
	z := a
	if m[2] != "" {
		z, _ = strconv.Atoi(m[2])
	}
	if a < 1 || z < a || a > len(ls) {
		return append(errs, fmt.Sprintf("lines %q outside the file (%d lines)", c.Lines, len(ls)))
	}
	lo, hi := max(0, a-1-3), min(len(ls), z+3)
	want := norm(strings.TrimSuffix(strings.TrimSpace(c.Text), "…"))
	if want == "" {
		errs = append(errs, "empty text")
	} else if !strings.Contains(norm(strings.Join(ls[lo:hi], "\n")), want) {
		if strings.Contains(norm(string(b)), want) {
			errs = append(errs, fmt.Sprintf("text is in the file but not at lines %s", c.Lines))
		} else {
			errs = append(errs, fmt.Sprintf("text not in the file: %.60q", c.Text))
		}
	}
	simple := func(s string) string {
		s = strings.TrimSpace(s)
		if i := strings.IndexAny(s, "(<"); i >= 0 {
			s = s[:i]
		}
		if i := strings.LastIndexAny(s, ".+/"); i >= 0 {
			s = s[i+1:]
		}
		return s
	}
	if t := simple(c.Type); t == "" || !strings.Contains(string(b), t) {
		errs = append(errs, fmt.Sprintf("type %q not in the file", c.Type))
	}
	if mt := simple(c.Method); c.Method != "" && !strings.Contains(string(b), mt) {
		errs = append(errs, fmt.Sprintf("method %q not in the file", c.Method))
	}
	return errs
}

func execText(o *vopts, item string) string {
	if o == nil || o.ExecLogs == "" {
		return ""
	}
	b, _ := os.ReadFile(filepath.Join(o.ExecLogs, item+".log"))
	return string(b)
}

func readLabels(dir string) (map[string]*Label, map[string][]string, error) {
	out := map[string]*Label{}
	parseErr := map[string][]string{}
	fs, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, nil, err
	}
	for _, f := range fs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		var l Label
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		if err := dec.Decode(&l); err != nil {
			parseErr[name] = append(parseErr[name], "json: "+err.Error())
			continue
		}
		if l.Item == "" {
			l.Item = name
		}
		out[l.Item] = &l
	}
	return out, parseErr, nil
}

func readPackets(dir string) (map[string]*Packet, error) {
	out := map[string]*Packet{}
	fs, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var p Packet
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if p.Item != "" {
			out[p.Item] = &p
		}
	}
	return out, nil
}

func loadAll(samplePath, packetDir string) (map[string]Item, map[string]*Packet, error) {
	items, err := readSample(samplePath)
	if err != nil {
		return nil, nil, err
	}
	byItem := map[string]Item{}
	for _, it := range items {
		byItem[it.Item] = it
	}
	ps, err := readPackets(packetDir)
	return byItem, ps, err
}

func validateCmd(samplePath, packetDir, labelDir string, o *vopts) error {
	items, ps, err := loadAll(samplePath, packetDir)
	if err != nil {
		return err
	}
	ls, perr, err := readLabels(labelDir)
	if err != nil {
		return err
	}
	nBad := 0
	for name, es := range perr {
		nBad++
		fmt.Printf("%s: %s\n", name, strings.Join(es, "; "))
	}
	var names []string
	for n := range ls {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if es := validate(ls[n], items, ps, o); len(es) > 0 {
			nBad++
			fmt.Printf("%s: %s\n", n, strings.Join(es, "; "))
		}
	}
	fmt.Printf("%d labels, %d invalid, %d packets without a label\n", len(ls)+len(perr), nBad, len(ps)-len(ls))
	return nil
}
