package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// maxProvenanceFiles is how many differing files the provenance line names.
const maxProvenanceFiles = 5

// Provenance says whether a Mod's files are the Workshop copy's (or the copy it was made
// from) as of the last scan, in one line for Mod details ("" when the Mod has no such copy to
// compare with, or is the Workshop copy itself). It reads what the scan stored (the snapshot
// hash, and the file lists only to name the differences): nothing is hashed again. A file's
// content is compared for About, LoadFolders.xml, Defs and Patches XML and assemblies; any
// other file by size (04 §1.4).
func (a *App) Provenance(ix *Index, m *store.Mod) string {
	if m == nil || m.Kind.WorkshopLike() || m.Kind == source.Official {
		return ""
	}
	up := a.upstreamOf(ix, m)
	ws := ""
	if up != nil {
		ws = up.WorkshopID()
	}
	linked, linkWS := workshopLink(ix, m)
	if ws == "" {
		ws = linkWS
	}
	switch {
	case linked && (up == nil || sameBytes(up.SnapHash, m.SnapHash)):
		return fmt.Sprintf("files are the Workshop copy ws:%s (this folder links to it)%s", ws, asScanned)
	case up == nil:
		return ""
	case len(m.SnapHash) > 0 && sameBytes(up.SnapHash, m.SnapHash):
		return fmt.Sprintf("files identical to Workshop %s (%s)%s", ws, ix.Ref(up.ID), asScanned)
	}
	diffs, listed := a.fileDifferences(m, up)
	if !listed {
		return fmt.Sprintf("files differ from Workshop %s (%s)%s; the stored file lists do not say which (archo scan --full)", ws, ix.Ref(up.ID), asScanned)
	}
	shown := diffs
	more := ""
	if len(shown) > maxProvenanceFiles {
		shown, more = shown[:maxProvenanceFiles], fmt.Sprintf(", +%d more", len(diffs)-maxProvenanceFiles)
	}
	return fmt.Sprintf("%d %s from Workshop %s (%s)%s: %s%s", len(diffs), fileDiffers(len(diffs)), ws, ix.Ref(up.ID), asScanned, strings.Join(shown, ", "), more)
}

func fileDiffers(n int) string {
	if n == 1 {
		return "file differs"
	}
	return "files differ"
}

// asScanned says the comparison is the stored one: the folders may have changed since.
const asScanned = ", as of the last scan"

func sameBytes(a, b []byte) bool { return len(a) > 0 && bytes.Equal(a, b) }

// upstreamOf is the Workshop copy m comes from: the instance its origin marker names (a locked
// copy), else the Workshop-like instance of its Lineage, preferring the one with the Workshop
// ID m declares.
func (a *App) upstreamOf(ix *Index, m *store.Mod) *store.Mod {
	if m.Origin != "" {
		var og scan.Origin
		if json.Unmarshal([]byte(m.Origin), &og) == nil {
			if o := ix.ByID[og.InstanceID]; o != nil && o.ID != m.ID && o.TombstoneAt.IsZero() && o.Kind.WorkshopLike() {
				return o
			}
		}
	}
	var best *store.Mod
	for _, o := range ix.ByLineage[m.LineageID] {
		if o.ID == m.ID || !o.TombstoneAt.IsZero() || !o.Kind.WorkshopLike() {
			continue
		}
		if m.DeclaredWorkshop != "" && o.WorkshopID() == m.DeclaredWorkshop {
			return o
		}
		if best == nil {
			best = o
		}
	}
	return best
}

// workshopLink reports whether the folder m was found in links into a Workshop source's
// folder, and that folder's name (the Workshop ID). The files of such a folder are the Workshop
// copy's, whatever the scan stored.
func workshopLink(ix *Index, m *store.Mod) (bool, string) {
	if m.RealPath == "" {
		return false, ""
	}
	real := filepath.Clean(m.RealPath)
	for _, s := range ix.Sources {
		if !s.Kind.WorkshopLike() || s.Root == "" {
			continue
		}
		root, err := source.RealPath(s.Root)
		if err != nil {
			root = s.Root
		}
		root = filepath.Clean(root) + string(filepath.Separator)
		if strings.HasPrefix(strings.ToLower(real)+string(filepath.Separator), strings.ToLower(root)) {
			id, _, _ := strings.Cut(real[len(root):], string(filepath.Separator))
			return true, id
		}
	}
	return false, ""
}

// fileDifferences names the files of m that differ from up's, "<path> changed|added|removed"
// in path order (added: only m has it). listed is false when a stored file list is missing.
func (a *App) fileDifferences(m, up *store.Mod) ([]string, bool) {
	mf, err1 := a.Store.SnapshotFiles(m.SnapID)
	uf, err2 := a.Store.SnapshotFiles(up.SnapID)
	if err1 != nil || err2 != nil || mf == nil || uf == nil {
		return nil, false
	}
	var out []string
	for k, f := range mf {
		u, ok := uf[k]
		switch {
		case !ok:
			out = append(out, f.Rel+" added")
		case f.Size != u.Size || !bytes.Equal(f.Hash, u.Hash):
			out = append(out, f.Rel+" changed")
		}
	}
	for k, u := range uf {
		if _, ok := mf[k]; !ok {
			out = append(out, u.Rel+" removed")
		}
	}
	sort.Strings(out)
	return out, true
}
