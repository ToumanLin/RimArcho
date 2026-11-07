package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// What the game's log says about loading a save (12 §4.3 "the last load", 06 §7): Archo
// Monitor marks the phase "save" with the save's file name (19 §4), so the errors and
// warnings counted in that phase belong to loading that save.

// SaveLoad is a run's load of one save: the log, when, and its errors and warnings.
type SaveLoad struct {
	Log      string    `json:"log"`
	At       time.Time `json:"at,omitzero"`
	Errors   int       `json:"errors"`   // occurrences while loading
	Warnings int       `json:"warnings"` // occurrences while loading
	Kinds    int       `json:"kinds"`    // their clusters
	First    string    `json:"first"`    // the first cluster's message
	Cluster  string    `json:"cluster"`  // its ID (E1 …), for the log page
}

// text is the row's line for the CLI and MCP: the counts, the log and when.
func (l *SaveLoad) text() string {
	var counts []string
	if l.Errors > 0 {
		counts = append(counts, render.Plural(l.Errors, "error"))
	}
	if l.Warnings > 0 {
		counts = append(counts, render.Plural(l.Warnings, "warning"))
	}
	t := fmt.Sprintf("%s while loading it (%s; %s", strings.Join(counts, " and "), render.Plural(l.Kinds, "kind"), filepath.Base(l.Log))
	if !l.At.IsZero() {
		t += " " + l.At.Local().Format("2006-01-02 15:04")
	}
	return t + ")"
}

// SaveKey is how a phase mark and a save file are compared: the file name without .rws,
// case ignored. Monitor records the name the game loads, with or without the extension.
func SaveKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSuffix(n, ".rws")
}

// LoadedSaves are the saves the run loaded, by the name Monitor recorded, in order, each
// once; nil without phase marks or names.
func (r *LogRun) LoadedSaves() []string {
	if r == nil {
		return nil
	}
	return loadedSaves(r.Marks)
}

func loadedSaves(marks []*logs.Phase) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range marks {
		if p == nil || p.Name != "save" || p.Save == "" || seen[SaveKey(p.Save)] {
			continue
		}
		seen[SaveKey(p.Save)] = true
		out = append(out, p.Save)
	}
	return out
}

// PlayedSave is the save the run last played: the save loaded before its last play phase,
// "" when that play began with a new colony, or the run has no names.
func (r *LogRun) PlayedSave() string {
	if r == nil {
		return ""
	}
	cur, played := "", ""
	for _, p := range r.Marks {
		switch {
		case p == nil:
		case p.Name == "save":
			cur = p.Save
		case p.Name == "new":
			cur = ""
		case p.Name == "play":
			played = cur
		}
	}
	return played
}

// LastSave is the save the run loaded last ("" none).
func (r *LogRun) LastSave() string {
	if r == nil {
		return ""
	}
	for i := len(r.Marks) - 1; i >= 0; i-- {
		if p := r.Marks[i]; p != nil && p.Name == "save" && p.Save != "" {
			return p.Save
		}
	}
	return ""
}

// storedLoads is one log's loads, from its stored report, by SaveKey; key is the file's
// path, size and mtime that report was made from. A run that loaded
// several different saves counts its "save" phase across all of them, so it attributes
// nothing (ambiguous): its saves map to nil.
type storedLoads struct {
	key   string // path|size|mtime
	loads map[string]*SaveLoad
}

// lastLoadOf is the newest load of save s that the environment's logs record (the current
// log, then the previous one), from their stored reports: logs are never parsed here, so a
// log not analysed yet says nothing. nil: no log records a load of it, or the run that did
// loaded other saves too.
func (a *App) lastLoadOf(s *Save, st *LoadoutState) *SaveLoad {
	if st == nil || s.Pack != "" {
		return nil
	}
	key := SaveKey(s.File)
	cur, prev := a.LogPaths(st)
	for _, p := range []string{cur, prev} {
		if p == "" {
			continue
		}
		loads := a.logLoads(p)
		if l, ok := loads[key]; ok {
			return l
		}
	}
	return nil
}

// logLoads reads (and keeps per App while the log is unchanged) one log's loads.
func (a *App) logLoads(path string) map[string]*SaveLoad {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	// Only a report made from the file as it is now counts; the key has the stored report's
	// stamp too, so a log analysed after it was first asked about is read then.
	size, mtime, ok := a.Store.LogCacheStamp(path)
	if !ok || size != fi.Size() || mtime != fi.ModTime().UnixNano() {
		return nil
	}
	k := path + "|" + strconv.FormatInt(size, 10) + "|" + strconv.FormatInt(mtime, 10)
	if a.saveLoads == nil {
		a.saveLoads = map[string]*storedLoads{}
	}
	if m := a.saveLoads[path]; m != nil && m.key == k {
		return m.loads
	}
	m := &storedLoads{key: k}
	a.saveLoads[path] = m
	row, err := a.Store.LogCache(path)
	if err != nil || row == nil || row.Size != fi.Size() || row.MTime != fi.ModTime().UnixNano() {
		return nil
	}
	var rep struct {
		HasPhases bool
		Phases    []*logs.Phase
		Clusters  []struct {
			ID     string
			Class  string
			Count  int
			First  *struct{ Message string }
			Phase  *logs.Phase // the first occurrence's
			Phases []logs.PhaseCount
		}
	}
	if json.Unmarshal(row.Report, &rep) != nil || !rep.HasPhases {
		return nil
	}
	names := loadedSaves(rep.Phases)
	if len(names) == 0 {
		return nil
	}
	m.loads = map[string]*SaveLoad{}
	if len(names) > 1 {
		for _, n := range names {
			m.loads[SaveKey(n)] = nil
		}
		return m.loads
	}
	l := &SaveLoad{Log: path}
	firstBegan := false
	firstSeverity := ""
	for _, p := range rep.Phases {
		if p != nil && p.Name == "save" {
			l.At = p.At
			break
		}
	}
	for _, c := range rep.Clusters {
		severity := logSeverity[c.Class]
		if severity != finding.Error && severity != finding.Warning {
			continue
		}
		n := 0
		for _, pc := range c.Phases {
			if logs.PhaseName(pc.Phase) == "save" {
				n += pc.Count
			}
		}
		if n == 0 {
			continue
		}
		// Prefer errors to warnings, then clusters that began while loading; clusters
		// within each group are in order of first occurrence.
		began := logs.PhaseName(c.Phase) == "save"
		if l.Kinds == 0 || severity == finding.Error && firstSeverity != finding.Error || severity == firstSeverity && began && !firstBegan {
			l.Cluster, l.First, firstBegan = c.ID, "", began
			firstSeverity = severity
			if c.First != nil {
				l.First = c.First.Message
			}
		}
		l.Kinds++
		if severity == finding.Error {
			l.Errors += n
		} else {
			l.Warnings += n
		}
	}
	m.loads[SaveKey(names[0])] = l
	return m.loads
}

// LastLoadedSave is the save the newest of runs loaded last (Archo Monitor's mark, 19 §4),
// checked against st's list as it is now (Can it run's Before you start, 16 §2.4): nil when
// no run names a save, the run was a pack's, or the file is gone. Only that one file is
// read (its head, and its full read from the cache), not the Saves folder.
func (a *App) LastLoadedSave(st *LoadoutState, runs []*LogRun) (*SaveCheck, *LogRun, error) {
	if st == nil || st.Plan == nil {
		return nil, nil, nil
	}
	var run *LogRun
	name := ""
	for _, r := range runs {
		if r == nil || r.OtherGame || r.OtherEnv != "" {
			continue
		}
		if name = r.LastSave(); name != "" {
			run = r
			break
		}
	}
	if run == nil || a.RunOfLog(st.Ix.Env.ID, run.ModTime) != "" {
		return nil, nil, nil
	}
	if !strings.EqualFold(filepath.Ext(name), ".rws") {
		name += ".rws"
	}
	folders, err := a.SaveFolders()
	if err != nil {
		return nil, nil, err
	}
	var sv *Save
	for _, f := range folders {
		if f.Pack != "" || !folderHasEnv(f, st.Ix.Env.ID) {
			continue
		}
		p := filepath.Join(f.Dir, filepath.Base(name))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			sv = &Save{Path: p, File: filepath.Base(p), Folder: f}
			break
		}
	}
	if sv == nil {
		return nil, run, nil
	}
	sv.Ref = SaveRef(sv.Path)
	sv.Prefix, sv.Name = SplitSaveName(sv.File)
	sv.Auto = autosaveRe.MatchString(sv.Name)
	saves := []*Save{sv}
	a.readHeads(saves)
	if err := a.attachSummaries(saves); err != nil {
		return nil, run, err
	}
	m, err := a.newSaveMatcher()
	if err != nil {
		return nil, run, err
	}
	m.match(sv)
	groupColonies(saves)
	// Against the list Can it run is about, whatever list the save matched.
	sv.Env, sv.Plan, sv.Pack = st.Ix.Env, st.Plan, ""
	c, err := a.CheckSave(sv)
	return c, run, err
}

func folderHasEnv(f *SaveFolder, id string) bool {
	for _, e := range f.Envs {
		if e.ID == id {
			return true
		}
	}
	return false
}
