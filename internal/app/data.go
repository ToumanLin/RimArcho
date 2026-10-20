package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Community data (04 §5.2, 10 §5): RimSort's sorting rules and Steam Workshop database and
// the "Use This Instead" replacements. Downloaded only by archo data update; stored in
// <home>/cache with a sidecar recording where and when each came from.

// Dataset is one community data file.
type Dataset struct {
	Key  string // config key data.<Key>.url
	File string // in <home>/cache
	URL  string // default source
	Desc string
}

// Datasets are the community data files, in update order.
var Datasets = []Dataset{
	{Key: "rules", File: "communityRules.json", Desc: "RimSort community sorting rules",
		URL: "https://raw.githubusercontent.com/RimSort/Community-Rules-Database/main/communityRules.json"},
	{Key: "steamdb", File: "steamDB.json", Desc: "RimSort Steam Workshop database (packageIds, dependencies, versions)",
		URL: "https://raw.githubusercontent.com/RimSort/Steam-Workshop-Database/main/steamDB.json"},
	{Key: "replacements", File: "replacements.json.gz", Desc: "Use This Instead replacements",
		URL: "https://raw.githubusercontent.com/emipa606/UseThisInstead/main/replacements.json.gz"},
}

// DataSource is where a dataset is fetched from: config data.<key>.url, else the default.
func (a *App) DataSource(d Dataset) string {
	if v := a.Store.Meta("cfg:data." + d.Key + ".url"); v != "" {
		return v
	}
	return d.URL
}

// DataInfo is a dataset's sidecar (<file>.source.json).
type DataInfo struct {
	Key     string    `json:"key"`
	Source  string    `json:"source"`
	ETag    string    `json:"etag,omitempty"`
	Version string    `json:"version,omitempty"` // the dataset's own timestamp or version
	Fetched time.Time `json:"fetched"`
	Entries int       `json:"entries"`
	Bytes   int64     `json:"bytes"`
}

func (a *App) dataPath(d Dataset) string { return filepath.Join(a.Home, "cache", d.File) }

// DataStatus reads the sidecars (nil entries for datasets never fetched).
func (a *App) DataStatus() map[string]*DataInfo {
	out := map[string]*DataInfo{}
	for _, d := range Datasets {
		b, err := os.ReadFile(a.dataPath(d) + ".source.json")
		if err != nil {
			out[d.Key] = nil
			continue
		}
		var di DataInfo
		if json.Unmarshal(b, &di) == nil {
			out[d.Key] = &di
		}
	}
	return out
}

// DataUpdate is the result of one dataset in archo data update.
type DataUpdate struct {
	Dataset Dataset
	Info    *DataInfo
	Same    bool // not modified since the last download
	Err     error
}

// UpdateData downloads the datasets (only those named in keys, or all), validates every
// one, and replaces the stored files only when all succeeded (10 §5 D2).
func (a *App) UpdateData(keys []string, dryRun bool) ([]*DataUpdate, error) {
	want := func(k string) bool {
		if len(keys) == 0 {
			return true
		}
		for _, x := range keys {
			if x == k {
				return true
			}
		}
		return false
	}
	if err := os.MkdirAll(filepath.Join(a.Home, "cache"), 0o755); err != nil {
		return nil, err
	}
	status := a.DataStatus()
	var res []*DataUpdate
	type staged struct{ tmp, dst string }
	var ready []staged
	cleanup := func() {
		for _, s := range ready {
			os.Remove(s.tmp)
		}
	}
	failed := false
	for _, d := range Datasets {
		if !want(d.Key) {
			continue
		}
		u := &DataUpdate{Dataset: d, Info: &DataInfo{Key: d.Key, Source: a.DataSource(d), Fetched: a.Now().UTC()}}
		res = append(res, u)
		if dryRun {
			continue
		}
		prev := status[d.Key]
		etag := ""
		if prev != nil && prev.Source == u.Info.Source {
			if _, err := os.Stat(a.dataPath(d)); err == nil {
				etag = prev.ETag
			}
		}
		tmp, err := a.fetchData(d, u, etag)
		if err != nil {
			u.Err, failed = err, true
			continue
		}
		if u.Same {
			u.Info = prev
			continue
		}
		ready = append(ready, staged{tmp, a.dataPath(d)})
	}
	if failed {
		cleanup()
		return res, errors.New("community data not updated: a download failed or was invalid; the stored files are unchanged")
	}
	for i, s := range ready {
		if err := os.Rename(s.tmp, s.dst); err != nil {
			cleanup()
			return res, err
		}
		ready[i].tmp = ""
	}
	for _, u := range res {
		if u.Err == nil && !u.Same && !dryRun {
			b, _ := json.MarshalIndent(u.Info, "", "  ")
			os.WriteFile(a.dataPath(u.Dataset)+".source.json", b, 0o644)
			if u.Dataset.Key == "steamdb" {
				if err := a.buildSteamIndex(); err != nil {
					return res, fmt.Errorf("steamDB index: %v", err)
				}
			}
		}
	}
	if !dryRun {
		a.dataIdx = nil
	}
	return res, nil
}

// fetchData downloads (or copies a local path) to a temp file and validates it.
func (a *App) fetchData(d Dataset, u *DataUpdate, etag string) (string, error) {
	src := u.Info.Source
	var body io.ReadCloser
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err := http.NewRequest("GET", src, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", UserAgent)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		client := *a.httpClient()
		client.Timeout = 10 * time.Minute // steamDB is ~50 MB
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("%s: %v", src, shortErr(err))
		}
		if resp.StatusCode == http.StatusNotModified {
			resp.Body.Close()
			u.Same = true
			return "", nil
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return "", fmt.Errorf("%s answered %s", src, resp.Status)
		}
		u.Info.ETag = resp.Header.Get("ETag")
		body = resp.Body
	} else {
		f, err := os.Open(src)
		if err != nil {
			return "", err
		}
		body = f
	}
	defer body.Close()
	tmp, err := os.CreateTemp(filepath.Join(a.Home, "cache"), d.File+".*.tmp")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(tmp, io.LimitReader(body, 512<<20))
	tmp.Close()
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	u.Info.Bytes = n
	if err := validateData(d, tmp.Name(), u.Info); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("%s: %v", d.File, err)
	}
	return tmp.Name(), nil
}

func readDataFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(zr, 512<<20))
	}
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), nil
}

// validateData checks the shape and counts the entries.
func validateData(d Dataset, path string, info *DataInfo) error {
	b, err := readDataFile(path)
	if err != nil {
		return err
	}
	switch d.Key {
	case "rules":
		var v struct {
			Timestamp json.Number                `json:"timestamp"`
			Rules     map[string]json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Rules == nil {
			return fmt.Errorf("not RimSort community rules (%v)", err)
		}
		info.Version, info.Entries = v.Timestamp.String(), len(v.Rules)
	case "steamdb":
		var v struct {
			Version  json.Number                `json:"version"`
			Database map[string]json.RawMessage `json:"database"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Database == nil {
			return fmt.Errorf("not a RimSort Steam Workshop database (%v)", err)
		}
		info.Version, info.Entries = v.Version.String(), len(v.Database)
	case "replacements":
		var v struct {
			Version string            `json:"version"`
			Rules   []json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Rules == nil {
			return fmt.Errorf("not a Use This Instead list (%v)", err)
		}
		info.Version, info.Entries = v.Version, len(v.Rules)
	}
	return nil
}

// ---- using the data ----

// Replacement is a "use this instead" suggestion.
type Replacement struct {
	OldWorkshopID, OldPackageID, OldName string
	NewWorkshopID, NewPackageID, NewName string
	NewAuthor                            string
	NewVersions                          []string
}

// WorkshopEntry is what steamDB knows of a workshop item.
type WorkshopEntry struct {
	WorkshopID, PackageID, Name string
}

type dataIndex struct {
	byOldWS, byOldPkg map[string]*Replacement
	byPkg             map[string]WorkshopEntry // steamDB: packageId → item
}

// dataIndex loads the replacements and a packageId index of steamDB once per process.
func (a *App) dataIndexes() *dataIndex {
	if a.dataIdx != nil {
		return a.dataIdx
	}
	ix := &dataIndex{byOldWS: map[string]*Replacement{}, byOldPkg: map[string]*Replacement{}, byPkg: map[string]WorkshopEntry{}}
	if b, err := readDataFile(a.dataPath(Datasets[2])); err == nil {
		var v struct {
			Rules []struct {
				OldWorkshopID string   `json:"oldWorkshopId"`
				OldPackageID  string   `json:"oldPackageId"`
				OldName       string   `json:"oldName"`
				NewWorkshopID string   `json:"newWorkshopId"`
				NewPackageID  string   `json:"newPackageId"`
				NewName       string   `json:"newName"`
				NewAuthor     string   `json:"newAuthor"`
				NewVersions   []string `json:"newVersions"`
			} `json:"rules"`
		}
		if json.Unmarshal(b, &v) == nil {
			for _, r := range v.Rules {
				x := &Replacement{OldWorkshopID: r.OldWorkshopID, OldPackageID: strings.ToLower(r.OldPackageID), OldName: r.OldName,
					NewWorkshopID: r.NewWorkshopID, NewPackageID: strings.ToLower(r.NewPackageID), NewName: r.NewName, NewAuthor: r.NewAuthor, NewVersions: r.NewVersions}
				if x.OldWorkshopID != "" {
					ix.byOldWS[x.OldWorkshopID] = x
				}
				if x.OldPackageID != "" {
					ix.byOldPkg[x.OldPackageID] = x
				}
			}
		}
	}
	if b, err := os.ReadFile(a.steamIndexPath()); err == nil {
		var m map[string][2]string
		if json.Unmarshal(b, &m) == nil {
			for p, v := range m {
				ix.byPkg[p] = WorkshopEntry{WorkshopID: v[0], PackageID: p, Name: v[1]}
			}
		}
	}
	a.dataIdx = ix
	return ix
}

// steamIndexPath is the compact packageId → [workshopId, name] index of steamDB, built
// when it is downloaded so nothing parses the 50 MB file afterwards.
func (a *App) steamIndexPath() string { return filepath.Join(a.Home, "cache", "steamDB.index.json") }

func (a *App) buildSteamIndex() error {
	b, err := readDataFile(a.dataPath(Datasets[1]))
	if err != nil {
		return err
	}
	var v struct {
		Database map[string]struct {
			PackageID string `json:"packageId"`
			Name      string `json:"name"`
			AppID     bool   `json:"appid"`
		} `json:"database"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	m := map[string][2]string{}
	for id, e := range v.Database {
		p := strings.ToLower(e.PackageID)
		if p == "" || e.AppID {
			continue
		}
		if prev, ok := m[p]; !ok || id < prev[0] {
			m[p] = [2]string{id, e.Name}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := a.steamIndexPath() + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, a.steamIndexPath())
}

// ReplacementFor returns the "use this instead" suggestion for an installed mod.
func (a *App) ReplacementFor(workshopID, packageID string) *Replacement {
	ix := a.dataIndexes()
	if r := ix.byOldWS[workshopID]; r != nil && workshopID != "" {
		return r
	}
	return ix.byOldPkg[strings.ToLower(packageID)]
}

// WorkshopFor looks a packageId up in steamDB (for missing mods: a direct Workshop link).
func (a *App) WorkshopFor(packageID string) (WorkshopEntry, bool) {
	e, ok := a.dataIndexes().byPkg[strings.ToLower(strings.TrimSuffix(packageID, "_steam"))]
	return e, ok
}
