package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Companion logs (03 §6.7.4): which one belongs to a Player.log, and what its entries add to
// a cluster's attribution.

// logDataRoot is the save data root a Player.log belongs to: 1.7 writes it under Logs/.
func logDataRoot(logPath string) string {
	dir := filepath.Dir(logPath)
	if strings.EqualFold(filepath.Base(dir), "Logs") {
		return filepath.Dir(dir)
	}
	return dir
}

// logMarker finds Archo Monitor's marker in a Player.log's first 4 MiB: its session id and
// the ArchoMonitor.log path it names.
func logMarker(logPath string) (session, path string) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 4<<20))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		l := sc.Text()
		if !strings.Contains(l, "[Archo Monitor]") {
			continue
		}
		if m := logs.SessionRe.FindStringSubmatch(l); m != nil {
			return m[2], strings.TrimSpace(m[3])
		}
	}
	return "", ""
}

// CompanionFor is the companion log of a Player.log ("" for none): the ArchoMonitor.log of
// the same session, else RimCrow's log of the same generation, unless it was written after
// the Player.log (it then belongs to a later run).
func CompanionFor(logPath string) string {
	root := logDataRoot(logPath)
	if session, marked := logMarker(logPath); session != "" {
		var cands []string
		if marked != "" {
			cands = append(cands, marked, filepath.Join(filepath.Dir(marked), logs.MonitorPrevLog))
		}
		cands = append(cands, filepath.Join(root, logs.MonitorLog), filepath.Join(root, logs.MonitorPrevLog))
		for _, c := range cands {
			if logs.FirstSession(c) == session {
				return c
			}
		}
	}
	fi, err := os.Stat(logPath)
	if err != nil {
		return ""
	}
	gen := 0
	if strings.Contains(strings.ToLower(filepath.Base(logPath)), "-prev") {
		gen = 1
	}
	return rimCrowLog(root, gen, fi.ModTime())
}

// rimCrowLog is RimCrow's companion log of a generation (0 current, 1 previous) in a save
// data root; notAfter (unless zero) rejects one written more than a minute later.
func rimCrowLog(root string, gen int, notAfter time.Time) string {
	if root == "" {
		return ""
	}
	for _, name := range logs.RimCrowLogs[gen] {
		p := filepath.Join(root, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !notAfter.IsZero() && fi.ModTime().After(notAfter.Add(time.Minute)) {
			continue
		}
		return p
	}
	return ""
}

// companionKey fingerprints the companion log for the log cache (03 §6.5).
func companionKey(path string) string {
	if path == "" {
		return "-"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return path + "|missing"
	}
	return fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
}

// companionOnly appends the companion's warnings and errors Player.log does not have (the
// game stopped writing it, 03 §6.7.4) to lf; it returns them.
func companionOnly(lf *logs.File, comp *logs.Companion) map[*logs.Entry]bool {
	if comp == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, e := range lf.Entries {
		seen[firstLineKey(e.Message)] = true
	}
	out := map[*logs.Entry]bool{}
	for _, ce := range comp.Entries {
		if !ce.Monitor || (ce.Level != "ERROR" && ce.Level != "WARNING") || seen[ce.Match] {
			continue
		}
		seen[ce.Match] = true
		en := logs.EntryOf(ce)
		lf.Entries = append(lf.Entries, en)
		out[en] = true
	}
	return out
}

// companion adds what the companion log knows about a cluster (03 §6.7.4): time, level and
// count; files in enabled mods as source evidence; related mods by how they were found.
// It never removes or weakens RimArcho's own conclusions.
func (cx *attrCtx) companion(lc *LogCluster) {
	e := cx.comp.Lookup(lc.First.Message)
	if e == nil {
		return
	}
	label := filepath.Base(cx.comp.Path)
	lc.At, lc.Level, lc.Companion = e.At, e.Level, label
	if e.Count > lc.Count {
		lc.Count = e.Count
	}
	while := ""
	if e.Def != "" {
		while = " (while loading " + e.Def + ")"
	}
	withFile := map[string]bool{}
	for _, f := range e.Files {
		var paths []string
		if filepath.IsAbs(f) {
			paths = []string{f}
		} else {
			// RimCrow writes paths inside a mod: try each related mod's folder.
			for _, pkg := range e.Mods {
				for _, m := range cx.byPkg[pkg] {
					paths = append(paths, filepath.Join(cx.st.Ix.ModDir(m), f))
				}
			}
		}
		for _, p := range paths {
			id, rel := cx.dirOwner(p)
			if id == "" || cx.st.Position(id) == 0 || cx.official(id) {
				continue
			}
			lc.add(id, RoleSource, fmt.Sprintf("log file (%s): %s%s", label, rel, while))
			withFile[id] = true
			break
		}
	}
	for _, pkg := range e.Mods {
		role := e.Roles[pkg]
		for _, m := range cx.byPkg[pkg] {
			if cx.official(m.ID) || withFile[m.ID] {
				continue
			}
			switch role {
			case "patch":
				ev := label + ": patches code or defs in this error"
				for _, r := range e.Refs {
					if strings.HasPrefix(r, "patch:") && strings.HasSuffix(r, "@"+pkg) {
						ev = label + ": " + strings.TrimSuffix(strings.TrimPrefix(r, "patch:"), "@"+pkg)
						break
					}
				}
				lc.add(m.ID, RolePatcher, ev)
			case "xml", "def":
				lc.add(m.ID, RoleMentioned, label+": owns a def or file involved"+while)
			default:
				lc.add(m.ID, RoleMentioned, label+": relatedModIds "+pkg)
			}
		}
	}
	if e.Inferred != "" && e.Inferred != lc.Class {
		lc.Details = append(lc.Details, label+": inferredType "+e.Inferred)
	}
}
