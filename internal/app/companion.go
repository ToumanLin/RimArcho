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

// CompanionFor returns the companion of this run, preferring Archo Monitor. A marker
// proves the session; without one, only a non-empty file within the run is eligible.
func CompanionFor(logPath string) string {
	return CompanionForRun(logPath, logDataRoot(logPath))
}

// CompanionForRun applies the pairing rules in a run's save data root, including packs.
func CompanionForRun(logPath, root string) string {
	session, marked := logMarker(logPath)
	c := findCompanion(logPath, root, session, marked)
	return uniqueCompanion(logPath, root, c, session)
}

func findCompanion(logPath, root, session, marked string) string {
	if session != "" {
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
	if err != nil || root == "" {
		return ""
	}
	start := logStart(logPath, fi)
	gen := 0
	monitor := logs.MonitorLog
	if strings.Contains(strings.ToLower(filepath.Base(logPath)), "-prev") {
		gen, monitor = 1, logs.MonitorPrevLog
	} else if prev, err := os.Stat(filepath.Join(filepath.Dir(logPath), "Player-prev.log")); err == nil && !prev.ModTime().After(fi.ModTime()) {
		// Unity can truncate an existing Player.log, keeping its old creation time.
		if boundary := prev.ModTime().Add(time.Nanosecond); boundary.After(start) {
			start = boundary
		}
	}
	names := logs.RimCrowLogs[gen]
	if session == "" {
		names = append([]string{monitor}, names...)
	}
	for _, name := range names {
		p := filepath.Join(root, name)
		ci, err := os.Stat(p)
		if err != nil || !ci.Mode().IsRegular() || ci.Size() == 0 || ci.ModTime().Before(start) || ci.ModTime().After(fi.ModTime().Add(time.Minute)) {
			continue
		}
		if name == monitor && logs.FirstSession(p) == "" {
			continue
		}
		return p
	}
	return ""
}

// logStart uses Waylith's explicit start (also valid for copied logs), then the native
// creation time. With neither, mtime is a conservative bound: ambiguous files stay unpaired.
func logStart(path string, fi os.FileInfo) time.Time {
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(io.LimitReader(f, 4096))
		for sc.Scan() {
			if value, ok := strings.CutPrefix(sc.Text(), "Started:"); ok {
				if at, err := time.Parse("2006-01-02 15:04:05.000 -07:00", strings.TrimSpace(value)); err == nil && !at.After(fi.ModTime()) {
					return at
				}
			}
		}
	}
	if at := logCreationTime(fi); !at.IsZero() && !at.After(fi.ModTime()) {
		return at
	}
	return fi.ModTime()
}

// uniqueCompanion resolves conflicting claims by the current and previous Player.log,
// including aliases/hard links: a session marker wins, then the current generation.
func uniqueCompanion(path, root, candidate, candidateSession string) string {
	if candidate == "" {
		return ""
	}
	var sibling string
	previous := false
	switch {
	case strings.EqualFold(filepath.Base(path), "Player.log"):
		sibling = "Player-prev.log"
	case strings.EqualFold(filepath.Base(path), "Player-prev.log"):
		sibling, previous = "Player.log", true
	default:
		return candidate
	}
	sibling = filepath.Join(filepath.Dir(path), sibling)
	session, markedPath := logMarker(sibling)
	other := findCompanion(sibling, root, session, markedPath)
	ci, err := os.Stat(candidate)
	if err != nil {
		return ""
	}
	if oi, err := os.Stat(other); err == nil && os.SameFile(ci, oi) {
		marked := candidateSession != "" && logs.FirstSession(candidate) == candidateSession
		otherMarked := session != "" && logs.FirstSession(other) == session
		if !marked && otherMarked || marked == otherMarked && previous {
			return ""
		}
	}
	return candidate
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
