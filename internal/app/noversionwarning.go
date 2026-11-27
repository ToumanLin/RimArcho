package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// No Version Warning (08 §2.3, 05 §4.2): emipa606's lists of Mods known to work on a game
// version their About.xml does not list. The repository keeps one ModIdsToFix.xml per game
// version (<repo>/<major.minor>/ModIdsToFix.xml); RimArcho stores the repository zip as
// downloaded. A plain ModIdsToFix.xml given as the source applies to every game version.

// NoVersionWarningName names the dataset in evidence and in the interface.
const NoVersionWarningName = "No Version Warning"

// nvwAll is the version key of a plain ModIdsToFix.xml: it applies to every game version.
const nvwAll = "*"

var nvwVersionDir = regexp.MustCompile(`^\d+\.\d+$`)

// nvwLists is what the dataset says: game major.minor (or nvwAll) → normalized packageIds.
type nvwLists struct {
	byVersion map[string]map[string]bool
	modified  time.Time // the newest list file in a zip
}

func (l *nvwLists) entries() int {
	n := 0
	for _, s := range l.byVersion {
		n += len(s)
	}
	return n
}

func (l *nvwLists) versions() []string {
	var vs []string
	for v := range l.byVersion {
		vs = append(vs, v)
	}
	slices.SortFunc(vs, func(a, b string) int {
		if a == b {
			return 0
		}
		if a == nvwAll || b != nvwAll && env.VersionLess(a, b) {
			return -1
		}
		return 1
	})
	return vs
}

// versionText is the sidecar's version: the newest list's date and the game versions.
func (l *nvwLists) versionText() string {
	vs := strings.Join(l.versions(), " ")
	vs = strings.ReplaceAll(vs, nvwAll, "all versions")
	if l.modified.IsZero() {
		return vs
	}
	return l.modified.UTC().Format("2006-01-02") + "; " + vs
}

// parseNoVersionWarning reads the repository zip or a plain ModIdsToFix.xml. A list file
// that cannot be read is skipped alone (08 §2.3 "one bad record"); the dataset fails only
// when no list file is readable.
func parseNoVersionWarning(b []byte) (*nvwLists, error) {
	out := &nvwLists{byVersion: map[string]map[string]bool{}}
	if !bytes.HasPrefix(b, []byte("PK")) {
		ids, err := modIdsToFix(b)
		if err != nil {
			return nil, err
		}
		out.byVersion[nvwAll] = ids
		return out, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	found := 0
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if !strings.EqualFold(path.Base(name), "ModIdsToFix.xml") {
			continue
		}
		v := path.Base(path.Dir(name))
		if !nvwVersionDir.MatchString(v) {
			continue
		}
		found++
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 16<<20))
		rc.Close()
		if err != nil {
			continue
		}
		ids, err := modIdsToFix(data)
		if err != nil {
			continue
		}
		set := out.byVersion[v]
		if set == nil {
			set = map[string]bool{}
			out.byVersion[v] = set
		}
		for id := range ids {
			set[id] = true
		}
		if f.Modified.After(out.modified) {
			out.modified = f.Modified
		}
	}
	if len(out.byVersion) == 0 {
		if found == 0 {
			return nil, errors.New("no <version>/ModIdsToFix.xml in the archive")
		}
		return nil, errors.New("no readable ModIdsToFix.xml in the archive")
	}
	return out, nil
}

// modIdsToFix reads <ModIdsToFix><li>packageId</li>…</ModIdsToFix>.
func modIdsToFix(b []byte) (map[string]bool, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var v struct {
		XMLName xml.Name `xml:"ModIdsToFix"`
		Li      []string `xml:"li"`
	}
	if err := xml.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("not a No Version Warning list (%v)", err)
	}
	ids := map[string]bool{}
	for _, s := range v.Li {
		if p := store.VersionMuteKey(s); p != "" {
			ids[p] = true
		}
	}
	return ids, nil
}

// VersionConfirmed reports whether No Version Warning lists packageID (case-insensitive,
// without the game's _steam suffix) for game major.minor mm. It never counts as a
// declaration: supported versions, plan defaults and allowed versions ignore it (07 §1.6).
func (a *App) VersionConfirmed(packageID, mm string) bool {
	if mm == "" {
		return false
	}
	ix := a.dataIndexes()
	if ix.noVersion == nil {
		return false
	}
	p := store.VersionMuteKey(packageID)
	return ix.noVersion.byVersion[mm][p] || ix.noVersion.byVersion[nvwAll][p]
}
