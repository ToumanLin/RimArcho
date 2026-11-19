package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Outside changes of the mod folders (04 §1.6). Steam, another manager or the player add,
// remove and update mod folders while RimArcho is not looking; a list then names mods the
// library does not have yet (GitHub #10: "(not installed)" right after subscribing). Each
// scan records a cheap print of every folder it read; the GUI compares it now and then and
// rescans when it differs.

func folderPrintKey(sourceID string) string { return "folderprint:" + sourceID }

// folderPrint is a digest of a source's direct subfolders (name and modification time) and,
// for Workshop-type sources, of the Steam manifest's size and time. A folder added, removed
// or renamed changes its parent's listing; Steam rewrites the manifest when it finishes or
// updates an item. Changes deep inside a mod folder are not seen (rescan by hand: R).
func folderPrint(s *source.Source) string {
	des, err := os.ReadDir(s.Root)
	if err != nil {
		return "missing"
	}
	lines := make([]string, 0, len(des)+1)
	for _, de := range des {
		var mt int64
		if fi, err := de.Info(); err == nil {
			mt = fi.ModTime().UnixNano()
		}
		lines = append(lines, fmt.Sprintf("%s\x00%d", strings.ToLower(de.Name()), mt))
	}
	sort.Strings(lines)
	if s.Kind.WorkshopLike() {
		if fi, err := os.Stat(scan.ACFPathFor(s.Root)); err == nil {
			lines = append(lines, fmt.Sprintf("\x01acf\x00%d\x00%d", fi.Size(), fi.ModTime().UnixNano()))
		}
	}
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:12])
}

// watchedSources are the sources a full scan of the current environment reads.
func (a *App) watchedSources(e *env.Environment) ([]*source.Source, error) {
	srcs, err := a.Store.Sources(e.GameID)
	if err != nil {
		return nil, err
	}
	var out []*source.Source
	for _, s := range srcs {
		if s.Status == source.StatusRetired || s.Status == source.StatusRebindPending {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// recordFolderPrints stores the prints a scan started from; a change while it ran is seen
// by the next check.
func (a *App) recordFolderPrints(prints map[string]string) {
	for id, p := range prints {
		a.Store.SetMeta(folderPrintKey(id), p)
	}
}

// ModFoldersChanged says whether a mod folder of the current environment changed since the
// last scan read it (04 §1.6): a folder added, removed or renamed, or the Steam manifest
// rewritten. False before the first scan (the first-run guide scans). It reads one folder
// listing per source and stats one file: a few milliseconds for a thousand mods.
func (a *App) ModFoldersChanged() (bool, error) {
	e, err := a.Env()
	if err != nil || e == nil {
		return false, err
	}
	if sc, _, err := a.LastScan(e.GameID); err != nil || sc == nil {
		return false, err
	}
	srcs, err := a.watchedSources(e)
	if err != nil {
		return false, err
	}
	for _, s := range srcs {
		if a.Store.Meta(folderPrintKey(s.ID)) != folderPrint(s) {
			return true, nil
		}
	}
	return false, nil
}

// WorkshopPending lists the Workshop items Steam has subscribed, or is downloading, that are
// not on disk yet: the ID is in the manifest or in Steam's downloads folder
// (steamapps/workshop/downloads/294100/<id>), and the content folder has no About/About.xml.
// Only the Steam client's Workshop folders count; SteamCMD downloads are RimArcho's own
// tasks (11 §1).
func (a *App) WorkshopPending() map[string]bool {
	e, err := a.Env()
	if err != nil || e == nil {
		return nil
	}
	srcs, err := a.watchedSources(e)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, s := range srcs {
		if s.Kind != source.Workshop {
			continue
		}
		var ids []string
		if acf := scan.ReadACF(s.Root); acf != nil {
			for id, it := range acf.Items {
				if it.Subscribed {
					ids = append(ids, id)
				}
			}
		}
		if des, err := os.ReadDir(workshopDownloads(s.Root)); err == nil {
			for _, de := range des {
				if de.IsDir() {
					ids = append(ids, de.Name())
				}
			}
		}
		for _, id := range ids {
			if numericWorkshopID(id) == "" || out[id] {
				continue
			}
			if source.AboutState(filepath.Join(s.Root, id)) == source.AboutPresent {
				continue
			}
			out[id] = true
		}
	}
	return out
}

// workshopDownloads is where the Steam client puts items it is still downloading
// (…/workshop/content/294100 → …/workshop/downloads/294100).
func workshopDownloads(contentRoot string) string {
	root := filepath.Clean(contentRoot)
	return filepath.Join(filepath.Dir(filepath.Dir(root)), "downloads", filepath.Base(root))
}
