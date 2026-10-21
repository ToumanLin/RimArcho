package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Packet is the evidence for one (log, cluster). Text is what the labeller reads.
type Packet struct {
	Item      string   `json:"item"`
	Gist      string   `json:"gist"`
	Key       string   `json:"key"`
	Class     string   `json:"class"`
	Tpl       string   `json:"tpl"`
	ClusterID string   `json:"cluster_id"`
	Count     int      `json:"count"`
	Rekeyed   bool     `json:"rekeyed,omitempty"` // the sampled key was not found; matched by template
	Game      string   `json:"game"`
	LogLines  int      `json:"log_lines"`
	Mods      int      `json:"mods"`
	Resolved  int      `json:"resolved"`
	LogMods   []string `json:"log_mods"` // every packageId of the log's list, in order (validation)
	Involved  []PMod   `json:"involved"`
	RimArcho  RArch    `json:"rimarcho"` // not in Text: the labeller is blind to RimArcho's verdict
	Text      string   `json:"text"`
	Bytes     int      `json:"bytes"`
}

// PMod is a mod of the log's list that the cluster's evidence points at.
type PMod struct {
	Pkg     string   `json:"pkg"`
	Name    string   `json:"name"`
	Pos     int      `json:"pos"` // 1-based position in the log's load order
	Version string   `json:"version,omitempty"`
	InLib   bool     `json:"in_lib"`
	Signals []string `json:"signals"`
}

type RArch struct {
	Confidence string   `json:"conf"`
	Why        string   `json:"why,omitempty"`
	Attr       []RAttr  `json:"attr,omitempty"`
	Details    []string `json:"details,omitempty"`
}

type RAttr struct {
	Pkg      string   `json:"pkg"`
	Role     string   `json:"role"`
	Evidence []string `json:"ev"`
}

type parsedMod struct {
	Name string   `json:"name"`
	Pkg  string   `json:"pkg"`
	MV   string   `json:"mv"`
	Asm  []string `json:"asm"`
}

type parsedRec struct {
	ID      string      `json:"id"`
	Version string      `json:"version"`
	Mods    []parsedMod `json:"mods"`
}

func buildPackets(home, corpus, samplePath, outDir, coocPath, steamPath string) error {
	co, err := readCooc(coocPath)
	if err != nil {
		return err
	}
	steam, err := readSteamDB(steamPath)
	if err != nil {
		return err
	}
	items, err := readSample(samplePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	want := map[string][]Item{}
	for _, it := range items {
		want[it.Gist] = append(want[it.Gist], it)
	}
	recs, err := readParsed(filepath.Join(corpus, "parsed.jsonl"), want)
	if err != nil {
		return err
	}
	a, userData, err := openEnv(home)
	if err != nil {
		return err
	}
	defer a.Close()
	for _, it := range items {
		r := recs[it.Gist]
		if r == nil {
			fmt.Printf("%s %s: not in parsed.jsonl\n", it.Item, it.Gist)
			continue
		}
		t := time.Now()
		p, err := packetFor(a, userData, corpus, r, it, co, steam)
		if err != nil {
			fmt.Printf("%s %s: %v\n", it.Item, it.Gist[:8], err)
			continue
		}
		j, _ := json.MarshalIndent(p, "", "  ")
		if err := os.WriteFile(filepath.Join(outDir, it.Item+".json"), j, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, it.Item+".txt"), []byte(p.Text), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s %s %-15s %s %5d bytes, %d mods involved, rimarcho %d attr (%s)%s\n", it.Item, it.Gist[:8], it.Class, p.ClusterID, p.Bytes,
			len(p.Involved), len(p.RimArcho.Attr), time.Since(t).Round(time.Millisecond), map[bool]string{true: " REKEYED"}[p.Rekeyed])
	}
	return nil
}

func readParsed(path string, want map[string][]Item) (map[string]*parsedRec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]*parsedRec{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	for sc.Scan() {
		b := sc.Bytes()
		var head struct{ ID string }
		if json.Unmarshal(b[:min(len(b), 200)], &head); head.ID == "" {
			// the prefix is not valid JSON on its own; pull the id out by hand
			if s := string(b[:min(len(b), 60)]); strings.HasPrefix(s, `{"id":"`) {
				head.ID, _, _ = strings.Cut(s[7:], `"`)
			}
		}
		if want[head.ID] == nil {
			continue
		}
		var r parsedRec
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		out[r.ID] = &r
	}
	return out, sc.Err()
}

func writeModsConfig(userData, version string, active []string) error {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModsConfigData>\n  <version>" + version + "</version>\n  <activeMods>\n")
	for _, p := range active {
		b.WriteString("    <li>" + p + "</li>\n")
	}
	b.WriteString("  </activeMods>\n</ModsConfigData>\n")
	return os.WriteFile(env.ModsConfigPath(userData), []byte(b.String()), 0o644)
}

// readRaw splits a HugsLib upload into its header (mod list, Harmony patches) and Player.log.
func readRaw(path string) (head, body string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", "", err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if h, body, ok := strings.Cut(text, "\nLog file contents:\n"); ok {
		return h, body, nil
	}
	if i := strings.Index(text, "\n=== Waylith log ===\n"); i >= 0 {
		return text[:i], text[i+1:], nil
	}
	return "", "", fmt.Errorf("no log section")
}

// modLineRe is a "Loaded mods:" line: Name(packageId)[mv:…][v:…][ov:…]: assemblies.
var modLineRe = regexp.MustCompile(`\(([^()\s]+)\)((?:\[[a-z]+:[^\]]*\])*): (.*)$`)
var verRe = regexp.MustCompile(`\[(?:ov|mv|v):([^\]]*)\]`)

func rawMods(head string) []parsedMod {
	_, rest, ok := strings.Cut(head, "Loaded mods:\n")
	if !ok {
		return nil
	}
	var out []parsedMod
	for _, l := range strings.Split(rest, "\n") {
		if strings.HasPrefix(l, "Active Harmony patches:") || strings.HasPrefix(l, "Platform information:") {
			break
		}
		m := modLineRe.FindStringSubmatchIndex(l)
		if m == nil {
			continue
		}
		pm := parsedMod{Name: strings.TrimSpace(l[:m[0]]), Pkg: strings.ToLower(l[m[2]:m[3]])}
		if v := verRe.FindStringSubmatch(l[m[4]:m[5]]); v != nil {
			pm.MV = v[1]
		}
		if asm := l[m[6]:m[7]]; asm != "(no assemblies)" {
			for _, x := range strings.Split(asm, "), ") {
				x = strings.TrimSpace(x)
				if x != "" && !strings.HasSuffix(x, ")") {
					x += ")"
				}
				pm.Asm = append(pm.Asm, x)
			}
		}
		out = append(out, pm)
	}
	return out
}

// Bounds of the packet (design: a few KB).
const (
	maxMsg      = 700
	maxExtra    = 8
	maxFrames   = 24
	maxHarmony  = 5
	maxLine     = 180
	ctxBefore   = 6
	ctxAfter    = 4
	maxInvolved = 10
	maxFactLine = 14
	maxText     = 7500
)

var (
	defNamedRe = regexp.MustCompile(`\b([A-Z]\w*Def)\b[^\n]{0,40}?\bnamed\s+"?([\w\-\.]+)`)
	defNameRe  = regexp.MustCompile(`(?i)\b(?:defName[=:\s]+|<defName>|for def\s+|def\s+)"?([A-Za-z][\w\-]{2,})`)
	refRe      = regexp.MustCompile(`#[0-9a-z]{2,8}\b`)
)

func clip(s string, n int) string {
	s = strings.TrimRight(s, " \t")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// openEnv opens the research environment (a separate RIMARCHO_HOME, see spikes/corpusrun setup).
func openEnv(home string) (*app.App, string, error) {
	if home == "" {
		return nil, "", fmt.Errorf("-home is required")
	}
	os.Setenv("RIMARCHO_HOME", home)
	a, err := app.Open(home)
	if err != nil {
		return nil, "", err
	}
	a.AnalyzeRunner = app.InProcessAnalyze(a)
	st, err := a.Loadout()
	if err != nil {
		a.Close()
		return nil, "", err
	}
	return a, st.Ix.Env.UserDataRoot, nil
}

// loadLog makes the log's mod list the Loadout, brings the analysis (patch simulation) up to
// date and attributes the log body, as spikes/corpusrun does.
//
// The mod list is re-read from the raw upload: parsed.jsonl drops entries written with
// [v:…] or [ov:…] (spikes/hugslogs modRe accepts only [mv:…]), about a third of all entries.
// r.Mods is replaced.
func loadLog(a *app.App, userData, corpus string, r *parsedRec) (*app.LoadoutState, string, *app.LogReport, error) {
	head, body, err := readRaw(filepath.Join(corpus, "raw", r.ID+".txt"))
	if err != nil {
		return nil, "", nil, err
	}
	if ms := rawMods(head); len(ms) >= len(r.Mods) {
		r.Mods = ms
	}
	var pkgs []string
	for _, m := range r.Mods {
		pkgs = append(pkgs, strings.ToLower(m.Pkg))
	}
	if err := writeModsConfig(userData, r.Version, pkgs); err != nil {
		return nil, "", nil, err
	}
	if _, err := a.Loadout(); err != nil {
		return nil, "", nil, err
	}
	if _, err := a.EnsureAnalysis(nil); err != nil {
		return nil, "", nil, fmt.Errorf("analysis: %w", err)
	}
	rep, err := a.AnalyzeLogText(body)
	if err != nil {
		return nil, "", nil, err
	}
	st, err := a.Loadout()
	return st, body, rep, err
}

func execCmd(home, corpus, gist string, argv []string) error {
	recs, err := readParsed(filepath.Join(corpus, "parsed.jsonl"), map[string][]Item{gist: {{}}})
	if err != nil {
		return err
	}
	r := recs[gist]
	if r == nil {
		return fmt.Errorf("log %s not in parsed.jsonl", gist)
	}
	a, userData, err := openEnv(home)
	if err != nil {
		return err
	}
	defer a.Close()
	if _, _, _, err := loadLog(a, userData, corpus, r); err != nil {
		return err
	}
	out, _ := cmds.Execute(a, argv)
	fmt.Print(out)
	return nil
}

func packetFor(a *app.App, userData, corpus string, r *parsedRec, it Item, co map[string]*CoocKey, steam map[string]string) (*Packet, error) {
	st, body, rep, err := loadLog(a, userData, corpus, r)
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for _, m := range r.Mods {
		pkgs = append(pkgs, strings.ToLower(m.Pkg))
	}
	var lc *app.LogCluster
	for _, c := range rep.Clusters {
		if c.Key == it.Key {
			lc = c
			break
		}
	}
	p := &Packet{Item: it.Item, Gist: r.ID, Key: it.Key, Class: it.Class, Tpl: it.Tpl, Game: r.Version, LogLines: rep.Lines, Mods: len(r.Mods), LogMods: pkgs}
	if lc == nil { // the rules changed since the run that sampled it: same template, most frequent
		for _, c := range rep.Clusters {
			if c.Template == it.Tpl && (lc == nil || c.Count > lc.Count) {
				lc = c
			}
		}
		p.Rekeyed = true
		if lc == nil {
			return nil, fmt.Errorf("cluster not found")
		}
		p.Key, p.Class = lc.Key, lc.Class
	}
	p.ClusterID, p.Count = lc.ID, lc.Count

	pkgOf := map[string]string{} // instance -> packageId
	refPkg := map[string]string{}
	posOf := map[string]int{} // packageId -> position
	for i, e := range st.List {
		if e.Instance != "" {
			p.Resolved++
			if m := st.Ix.ByID[e.Instance]; m != nil {
				pk := strings.ToLower(m.PackageID)
				pkgOf[m.ID] = pk
				refPkg[st.Ix.Ref(m.ID)] = pk
				posOf[pk] = i + 1
			}
		}
	}
	for i, pk := range pkgs {
		if posOf[pk] == 0 {
			posOf[pk] = i + 1
		}
	}
	for _, at := range lc.Attr {
		p.RimArcho.Attr = append(p.RimArcho.Attr, RAttr{Pkg: pkgOf[at.Instance], Role: at.Role, Evidence: at.Evidence})
	}
	p.RimArcho.Confidence, p.RimArcho.Why, p.RimArcho.Details = lc.Confidence, lc.Why, lc.Details
	inactive := map[string]string{} // refs of installed mods that the log's list does not load
	for _, m := range st.Ix.Mods {
		if ref := st.Ix.Ref(m.ID); refPkg[ref] == "" {
			inactive[ref] = strings.ToLower(m.PackageID) + " (not in this list)"
		}
	}
	deref := func(s string) string {
		return refRe.ReplaceAllStringFunc(s, func(x string) string {
			if pk := refPkg[x]; pk != "" {
				return pk
			}
			if pk := inactive[x]; pk != "" {
				return pk
			}
			return x
		})
	}

	// --- the cluster as the log shows it
	e := lc.First
	var b strings.Builder
	fmt.Fprintf(&b, "# Evidence packet %s\n", it.Item)
	fmt.Fprintf(&b, "log: %s  game %s  %d lines  %d mods in list (%d found in the research library)\n", r.ID, r.Version, rep.Lines, len(r.Mods), p.Resolved)
	fmt.Fprintf(&b, "cluster: %s  class %s  occurs %d× (lines %v)\n", lc.ID, lc.Class, lc.Count, lc.Lines)
	fmt.Fprintf(&b, "key: %s\n\n", clip(p.Key, 400))
	b.WriteString("## Message\n" + clip(e.Message, maxMsg) + "\n")
	var clusterText strings.Builder
	clusterText.WriteString(e.Message + "\n")
	if len(e.Extra) > 0 {
		b.WriteString("\n## Extra lines\n")
		for i, x := range e.Extra {
			clusterText.WriteString(x + "\n")
			if i < maxExtra {
				b.WriteString(clip(x, maxLine) + "\n")
			}
		}
		if len(e.Extra) > maxExtra {
			fmt.Fprintf(&b, "… %d more\n", len(e.Extra)-maxExtra)
		}
	}
	var hotMethod string // the innermost game method with Harmony patches on it
	if len(e.Frames) > 0 {
		b.WriteString("\n## Stack (innermost first; Harmony lines indented)\n")
		for i, f := range e.Frames {
			clusterText.WriteString(f.Text + "\n")
			for _, h := range f.Harmony {
				clusterText.WriteString(h.Text + "\n")
			}
			if hotMethod == "" && (len(f.Harmony) > 0 || f.Patched) && logs.GameNamespace(f.Type) && f.Type != "" {
				hotMethod = f.Type + "." + f.Method
			}
			if i >= maxFrames {
				continue
			}
			b.WriteString(clip(strings.TrimSpace(f.Text), maxLine) + "\n")
			for j, h := range f.Harmony {
				if j == maxHarmony {
					fmt.Fprintf(&b, "    … %d more Harmony lines\n", len(f.Harmony)-maxHarmony)
					break
				}
				b.WriteString("    " + clip(strings.TrimSpace(h.Text), maxLine) + "\n")
			}
		}
		if len(e.Frames) > maxFrames {
			fmt.Fprintf(&b, "… %d more frames\n", len(e.Frames)-maxFrames)
		}
	}
	lines := strings.Split(body, "\n")
	if e.Line > 1 {
		b.WriteString("\n## Log lines just before the first occurrence\n")
		for i := max(0, e.Line-1-ctxBefore); i < e.Line-1 && i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				b.WriteString(clip(lines[i], maxLine) + "\n")
			}
		}
	}
	// Lines right after the message that the parser did not keep as Extra or frames, e.g.
	// "absFilePath: …" under "Exception loading UnityEngine.Texture2D from file."
	var after []string
	if len(e.Extra) == 0 && len(e.Frames) == 0 {
		for i := e.Line; i < len(lines) && i < e.Line+ctxAfter; i++ {
			l := strings.TrimSpace(lines[i])
			if l == "" || strings.HasPrefix(l, "at ") || strings.HasPrefix(l, "(wrapper") || strings.HasPrefix(l, "[Ref ") {
				continue
			}
			after = append(after, clip(lines[i], maxLine))
		}
	}
	if len(after) > 0 {
		b.WriteString("\n## Log lines just after the message\n" + strings.Join(after, "\n") + "\n")
	}

	// --- mods the evidence points at
	sig := map[string][]string{}
	addSig := func(pk, s string) {
		if pk == "" {
			return
		}
		for _, x := range sig[pk] {
			if x == s {
				return
			}
		}
		sig[pk] = append(sig[pk], s)
	}
	low := strings.ToLower(clusterText.String())
	for _, m := range r.Mods {
		pk := strings.ToLower(m.Pkg)
		if strings.HasPrefix(pk, "ludeon.") || pk == "brrainz.harmony" {
			continue
		}
		if strings.Contains(low, pk) {
			addSig(pk, "packageId appears in the cluster text")
		}
		if n := strings.ToLower(m.Name); len(n) >= 6 && strings.Contains(low, n) {
			addSig(pk, "name appears in the cluster text")
		}
		for _, as := range m.Asm {
			an, _, _ := strings.Cut(as, "(")
			al := strings.ToLower(an)
			if len(al) < 4 || al == "0harmony" || al == "harmonymod" || al == "0multiplayerapi" {
				continue
			}
			if regexp.MustCompile(`(?:^|[^\w])` + regexp.QuoteMeta(al) + `[\.:\s]`).MatchString(low) {
				addSig(pk, "its assembly "+an+" appears in the cluster text")
			}
		}
	}
	seenType := map[string]bool{}
	nTypes := 0
	for _, f := range e.Frames {
		if f.Type != "" && !logs.GameNamespace(f.Type) && !seenType[f.Type] && nTypes < 10 {
			seenType[f.Type] = true
			nTypes++
			if owners, err := a.Store.TypeOwners(f.Type); err == nil {
				for _, o := range owners {
					if pk := pkgOf[o[0]]; pk != "" {
						addSig(pk, "its assembly defines stack type "+f.Type)
					}
				}
			}
		}
		for _, h := range f.Harmony {
			var owners []string
			if h.PatchType != "" {
				if ow, err := a.Store.TypeOwners(h.PatchType); err == nil {
					for _, o := range ow {
						owners = append(owners, o[0])
					}
				}
			}
			if len(owners) == 0 {
				owners, _ = a.Store.HarmonyIDOwners(h.Owner)
			}
			for _, o := range owners {
				if pk := pkgOf[o]; pk != "" {
					addSig(pk, fmt.Sprintf("Harmony %s on %s.%s (ID %s)", strings.ToLower(h.Kind), f.Type, f.Method, h.Owner))
				}
			}
		}
	}
	defNames := map[string]bool{}
	var defList []string
	src := e.Message + "\n" + strings.Join(e.Extra, "\n")
	for _, m := range defNamedRe.FindAllStringSubmatch(src, -1) {
		if !defNames[m[2]] {
			defNames[m[2]] = true
			defList = append(defList, m[1]+"/"+m[2])
		}
	}
	for _, m := range defNameRe.FindAllStringSubmatch(src, -1) {
		if !defNames[m[1]] {
			defNames[m[1]] = true
			defList = append(defList, m[1])
		}
	}
	if len(defList) > 3 {
		defList = defList[:3]
	}
	for _, d := range defList {
		_, name, _ := strings.Cut(d, "/")
		if name == "" {
			name = d
		}
		if facts, err := a.Store.DefsNamed(name, false); err == nil {
			for _, df := range facts {
				if pk := pkgOf[df.Instance]; pk != "" {
					addSig(pk, "defines "+df.Type+"/"+df.Name)
				}
			}
		}
	}
	inList := map[string]bool{}
	for _, pk := range pkgs {
		inList[pk] = true
	}
	var instOf = map[string]*store.Mod{}
	for id, pk := range pkgOf {
		instOf[pk] = st.Ix.ByID[id]
	}

	// Paths from another machine: …\294100\<workshop id>\… or …\Mods\<folder>\…
	folderPkg := map[string]string{}
	for pk, m := range instOf {
		if m == nil {
			continue
		}
		if w := m.WorkshopID(); w != "" {
			folderPkg[strings.ToLower(w)] = pk
		}
		if d := st.Ix.ModDir(m); d != "" {
			folderPkg[strings.ToLower(filepath.Base(d))] = pk
		}
	}
	byName := map[string]string{} // lower-case list name -> packageId
	for _, m := range r.Mods {
		byName[strings.ToLower(m.Name)] = strings.ToLower(m.Pkg)
	}
	for _, src := range []struct{ text, where string }{{clusterText.String(), "in the cluster"}, {strings.Join(after, "\n"), "just after the message"}} {
		for _, fm := range folderRe.FindAllStringSubmatch(src.text, -1) {
			folder := fm[1] + fm[2]
			if pk := folderPkg[strings.ToLower(folder)]; pk != "" {
				addSig(pk, "a file path "+src.where+" is inside its folder ("+folder+")")
			} else if w := steam[folder]; w != "" && byName[strings.ToLower(w)] != "" {
				// not in the research library: Steam's title of the Workshop id names a mod of the list
				addSig(byName[strings.ToLower(w)], fmt.Sprintf("a file path %s is in Workshop folder %s, whose Steam title %q is this mod's name", src.where, folder, w))
			}
		}
	}

	// Corpus co-occurrence: mods of this list over-represented in other logs with the same key.
	var coocLines []string
	if ck := co[it.Key]; ck != nil && p.Key == it.Key {
		for _, c := range ck.Mods {
			if !inList[c.Pkg] {
				continue
			}
			s := fmt.Sprintf("in %.0f%% of the %d corpus logs with this same error vs %.0f%% of the other logs", 100*c.With, ck.Logs, 100*c.Without)
			addSig(c.Pkg, "corpus: "+s)
			coocLines = append(coocLines, c.Pkg+": "+s)
		}
		if len(coocLines) == 0 && ck.Logs >= coocMinLogs {
			coocLines = append(coocLines, fmt.Sprintf("(%d corpus logs have this same error; no mod of this list is over-represented in them)", ck.Logs))
		}
	}

	// --- RimArcho facts from the research environment (the same commands as the MCP tools)
	var facts []string
	if len(coocLines) > 0 {
		facts = append(facts, "Corpus co-occurrence (correlation only, not proof):\n"+strings.Join(coocLines, "\n"))
	}
	run := func(argv ...string) string {
		out, _ := cmds.Execute(a, argv)
		out = deref(out)
		ls := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(ls) > maxFactLine {
			ls = append(ls[:maxFactLine], fmt.Sprintf("… %d more lines", len(ls)-maxFactLine))
		}
		for i := range ls {
			ls[i] = clip(ls[i], maxLine)
		}
		facts = append(facts, "$ archo "+strings.Join(argv, " ")+"\n"+strings.Join(ls, "\n"))
		return out
	}
	// search runs `files search` for the first term that matches and marks the matching mods.
	search := func(terms ...string) {
		for _, t := range terms {
			if t == "" {
				continue
			}
			out, _ := cmds.Execute(a, []string{"files", "search", t})
			if strings.Contains(out, ": 0 matches") {
				facts = append(facts, "$ archo files search "+t+"\n(0 matches in the enabled mods' files)")
				continue
			}
			out = run("files", "search", t)
			for _, m := range searchHitRe.FindAllStringSubmatch(out, -1) {
				if inList[m[1]] {
					addSig(m[1], "its files contain \""+clip(t, 60)+"\"")
				}
			}
			return
		}
	}
	for _, d := range defList {
		run("def", d)
	}
	switch lc.Class {
	case logs.ClassTexture:
		if m := texPathRe.FindStringSubmatch(e.Message); m != nil {
			base := m[1][strings.LastIndexAny(m[1], "/\\")+1:]
			trimmed := base
			if i := strings.LastIndex(base, "_"); i > 0 {
				trimmed = base[:i]
			}
			search(m[1], base, map[bool]string{true: trimmed}[trimmed != base])
		}
	case logs.ClassXrefMissing:
		if m := defNamedRe.FindStringSubmatch(e.Message); m != nil {
			search(">"+m[2]+"<", m[2]) // who references the missing def: the exact XML value first
		}
	case logs.ClassXMLLoad:
		var terms []string
		if m := xmlLitRe.FindString(e.Message); m != "" {
			terms = append(terms, m)
		}
		if m := defNameTagRe.FindStringSubmatch(src); m != nil {
			terms = append(terms, "<defName>"+m[1]+"</defName>")
		}
		search(terms...)
	}
	if m := asmRefRe.FindStringSubmatch(src); m != nil {
		if hits := dllRefs(a, st, instOf, m[1]); len(hits) > 0 {
			for _, h := range hits {
				addSig(h[0], "its assembly "+h[1]+" references "+m[1])
			}
			var ls []string
			for i, h := range hits {
				if i == 10 {
					ls = append(ls, fmt.Sprintf("… %d more", len(hits)-10))
					break
				}
				ls = append(ls, h[0]+"  "+h[1])
			}
			facts = append(facts, "Assemblies of the list that reference "+m[1]+" (byte search of the DLLs):\n"+strings.Join(ls, "\n"))
		} else {
			facts = append(facts, "No assembly of the list's mods in the library references "+m[1]+" (byte search of the DLLs)")
		}
	}

	var inv []string
	for pk := range sig {
		inv = append(inv, pk)
	}
	sort.Slice(inv, func(i, j int) bool {
		if len(sig[inv[i]]) != len(sig[inv[j]]) {
			return len(sig[inv[i]]) > len(sig[inv[j]])
		}
		return posOf[inv[i]] < posOf[inv[j]]
	})
	if len(inv) > maxInvolved {
		inv = inv[:maxInvolved]
	}
	byPkg := map[string]parsedMod{}
	for _, m := range r.Mods {
		byPkg[strings.ToLower(m.Pkg)] = m
	}
	b.WriteString("\n## Mods of the list that the evidence points at\n")
	if len(inv) == 0 {
		b.WriteString("(none found by packageId, name, assembly, stack types, Harmony owners, def names, file paths or file search)\n")
	}
	for _, pk := range inv {
		pm := byPkg[pk]
		m := instOf[pk]
		x := PMod{Pkg: pk, Name: pm.Name, Pos: posOf[pk], Version: pm.MV, InLib: m != nil, Signals: sig[pk]}
		p.Involved = append(p.Involved, x)
		fmt.Fprintf(&b, "- %s [%s] position %d/%d", pm.Name, pk, x.Pos, len(r.Mods))
		if pm.MV != "" {
			b.WriteString(" v" + pm.MV)
		}
		b.WriteString("\n    evidence: " + strings.Join(sig[pk], "; ") + "\n")
		if m == nil {
			b.WriteString("    (not in the research library: no About.xml facts)\n")
			continue
		}
		d := m.Decl()
		fmt.Fprintf(&b, "    About.xml: authors %s; supports %s\n", clip(strings.Join(d.Authors, ", "), 60), strings.Join(d.SupportedVersions, " "))
		var deps, missing []string
		for _, x := range d.Dependencies {
			dp := strings.ToLower(x.PackageID)
			deps = append(deps, dp)
			if !inList[dp] && !inList[strings.TrimSuffix(dp, "_steam")] {
				missing = append(missing, dp)
			}
		}
		for _, x := range d.DependenciesByVersion["1.6"] {
			dp := strings.ToLower(x.PackageID)
			deps = append(deps, dp)
			if !inList[dp] {
				missing = append(missing, dp)
			}
		}
		if len(deps) > 0 {
			fmt.Fprintf(&b, "    depends on: %s", strings.Join(deps, ", "))
			if len(missing) > 0 {
				fmt.Fprintf(&b, "  — NOT in the log's list: %s", strings.Join(missing, ", "))
			}
			b.WriteString("\n")
		}
		var order []string
		for _, x := range append(append([]string{}, d.LoadAfter...), d.LoadAfterByVersion["1.6"]...) {
			x = strings.ToLower(x)
			if posOf[x] > posOf[pk] {
				order = append(order, "loadAfter "+x+" but loads before it")
			}
		}
		for _, x := range append(append([]string{}, d.LoadBefore...), d.LoadBeforeByVersion["1.6"]...) {
			x = strings.ToLower(x)
			if posOf[x] > 0 && posOf[x] < posOf[pk] {
				order = append(order, "loadBefore "+x+" but loads after it")
			}
		}
		for _, x := range append(append([]string{}, d.IncompatibleWith...), d.IncompatibleByVersion["1.6"]...) {
			if inList[strings.ToLower(x)] {
				order = append(order, "declares incompatibleWith "+strings.ToLower(x)+", which is active")
			}
		}
		if len(order) > 0 {
			b.WriteString("    declared rules broken here: " + strings.Join(order, "; ") + "\n")
		}
	}

	if hotMethod != "" {
		run("harmony", "--target", hotMethod)
	}
	if lc.Class == logs.ClassPatchFailed || lc.Class == logs.ClassXMLLoad {
		n := 0
		for _, pk := range inv {
			if m := instOf[pk]; m != nil && n < 2 {
				n++
				run("patches", "--mod", st.Ix.Ref(m.ID), "--failed")
			}
		}
	}
	if len(lc.Details) > 0 {
		var ds []string
		for _, d := range lc.Details {
			ds = append(ds, clip(deref(d), maxLine))
		}
		facts = append(facts, "RimArcho class facts:\n"+strings.Join(ds, "\n"))
	}
	if len(facts) > 0 {
		b.WriteString("\n## Facts from RimArcho's research environment (current mod versions, may differ from the player's)\n")
		budget := maxText - b.Len()
		for _, f := range facts {
			if len(f)+2 > budget {
				b.WriteString("… (facts cut to keep the packet small)\n")
				break
			}
			b.WriteString(f + "\n\n")
			budget -= len(f) + 2
		}
	}
	p.Text = b.String()
	p.Bytes = len(p.Text)
	return p, nil
}

var (
	folderRe     = regexp.MustCompile(`(?i)(?:294100[\\/](\d+)|[\\/]Mods[\\/]([^\\/\]]+))[\\/]`)
	searchHitRe  = regexp.MustCompile(`(?m)^\s{4}([a-z0-9][a-z0-9_.\-]*)\s{2}`)
	texPathRe    = regexp.MustCompile(`at '([^']+)'`)
	xmlLitRe     = regexp.MustCompile(`<(\w+)>[^<>]{1,40}</(\w+)>`)
	defNameTagRe = regexp.MustCompile(`<defName>([^<]+)</defName>`)
	asmRefRe     = regexp.MustCompile(`(?:assembly|file or assembly) '([A-Za-z0-9_.\-]+?)(?:,|')`)
)

// dllRefs finds the list's assemblies whose bytes contain the name: an AssemblyRef is a plain
// string in the metadata. It returns [packageId, file] pairs.
func dllRefs(a *app.App, st *app.LoadoutState, instOf map[string]*store.Mod, name string) [][2]string {
	var out [][2]string
	needle := []byte(name)
	var pks []string
	for pk := range instOf {
		pks = append(pks, pk)
	}
	sort.Strings(pks)
	for _, pk := range pks {
		m := instOf[pk]
		if m == nil {
			continue
		}
		rows, err := a.Store.Assemblies(m.ID)
		if err != nil {
			continue
		}
		for _, r := range rows {
			if strings.EqualFold(strings.TrimSuffix(filepath.Base(r.File), ".dll"), name) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(st.Ix.ModDir(m), r.File))
			if err == nil && bytes.Contains(b, needle) {
				out = append(out, [2]string{pk, filepath.ToSlash(r.File)})
			}
		}
	}
	return out
}

// readSteamDB reads RimSort's steamDB.json: Workshop id -> Steam title (optional).
func readSteamDB(path string) (map[string]string, error) {
	out := map[string]string{}
	if path == "" {
		return out, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var db struct {
		Database map[string]struct {
			SteamName string `json:"steamName"`
			Name      string `json:"name"`
		} `json:"database"`
	}
	if err := json.Unmarshal(b, &db); err != nil {
		return nil, err
	}
	for id, e := range db.Database {
		if e.SteamName != "" {
			out[id] = e.SteamName
		} else if e.Name != "" {
			out[id] = e.Name
		}
	}
	return out, nil
}
