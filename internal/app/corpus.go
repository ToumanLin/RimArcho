package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/corpus"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Corpus data (05 §4.6): per game version, the newest section among the embedded bundle (A),
// every version folder of RimArcho's own Monitor directory (B) and the Workshop-subscribed
// Monitor (C). The choice follows the environment Env returns.

// CorpusCandidates lists the bundles RimArcho can read for an environment, A first.
func (a *App) CorpusCandidates(e *env.Environment) []corpus.Candidate {
	out := []corpus.Candidate{corpus.EmbeddedCandidate()}
	dir := a.MonitorDir()
	for _, v := range installedMonitors(dir) {
		if c, ok := corpus.ReadFolder(corpus.Monitor, filepath.Join(dir, monitorFolder+v)); ok {
			out = append(out, c)
		}
	}
	if e != nil {
		for _, p := range a.workshopMonitorPaths(e) {
			if c, ok := corpus.ReadFolder(corpus.Workshop, p); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

// workshopMonitorPaths are the folders of live Workshop Monitor Instances the environment sees.
func (a *App) workshopMonitorPaths(e *env.Environment) []string {
	rows, err := a.Store.DB.Query(`SELECT i.real_path FROM instance i JOIN source s ON s.id = i.source_id
		LEFT JOIN snapshot n ON n.id = i.current_snapshot_id
		WHERE i.game_id IN ('', ?) AND s.kind = ? AND i.status = 'live' AND lower(coalesce(n.package_id, '')) IN (?, ?)`,
		e.GameID, string(source.Workshop), MonitorPackage, MonitorPackage+"_steam")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil && p != "" {
			out = append(out, p)
		}
	}
	return out
}

// CorpusFor chooses the corpus data of an environment's game version and makes it the data in
// use. Without an environment it uses 1.6.
func (a *App) CorpusFor(e *env.Environment) corpus.Choice {
	game := "1.6"
	if e != nil {
		if mm := env.MajorMinor(e.GameVersion); mm != "" {
			game = mm
		}
	}
	key := game
	if e != nil {
		key += "|" + e.ID
	}
	if a.corpusKey == key && a.corpusSet {
		return a.corpusChoice
	}
	ch := corpus.Choose(a.CorpusCandidates(e), game)
	corpus.Use(ch)
	a.corpusKey, a.corpusChoice, a.corpusSet = key, ch, true
	return ch
}

// Corpus is the corpus data of the current environment.
func (a *App) Corpus() corpus.Choice {
	e, _ := a.Env()
	return a.CorpusFor(e)
}

// forgetCorpus makes the next Env choose again (a Monitor folder changed).
func (a *App) forgetCorpus() { a.corpusSet = false }

// corpusStamp is part of cache keys of results that use corpus data.
func (a *App) corpusStamp() string {
	if !a.corpusSet {
		return ""
	}
	return a.corpusChoice.Game + ":" + a.corpusChoice.Data + ":" + a.corpusChoice.Origin + ":" + a.corpusChoice.Label
}

// readCorpusData reads a Monitor folder's bundle week; "" when it has none or cannot be read.
func readCorpusData(dir string) string {
	c, ok := corpus.ReadFolder(corpus.Monitor, dir)
	if !ok || c.Err != nil || c.Bundle == nil {
		return ""
	}
	return c.Bundle.Data
}

// refreshMonitorCorpus copies from's Corpus folder over to's when it is newer (19 §6): data
// updates keep modVersion, so a Monitor folder of the same version takes new data in place.
// The game never loads the folder. It returns the new week, "" when nothing changed.
func refreshMonitorCorpus(from, to string) (string, error) {
	nw, old := readCorpusData(from), readCorpusData(to)
	if nw == "" || nw <= old {
		return "", nil
	}
	src, dst := filepath.Join(from, corpus.Dir), filepath.Join(to, corpus.Dir)
	tmp := dst + ".archo-tmp"
	os.RemoveAll(tmp)
	if err := copyPlainTree(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	bak := dst + ".archo-old"
	os.RemoveAll(bak)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, bak); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(bak, dst)
		return "", err
	}
	os.RemoveAll(bak) // RimArcho's own product (19 §6)
	return nw, nil
}

// corpusOnly reports whether a relative path inside a Monitor folder is corpus data, which
// the content fingerprint leaves out (19 §6).
func corpusOnly(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	d := strings.ToLower(corpus.Dir)
	return rel == d || strings.HasPrefix(rel, d+"/")
}
