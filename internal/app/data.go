package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/accounts"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Community data (08 §2.3): RimSort's sorting rules and Steam Workshop database and
// the "Use This Instead" replacements. Downloaded manually or by automatic maintenance; stored in
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
	Key      string    `json:"key"`
	Source   string    `json:"source"`
	ETag     string    `json:"etag,omitempty"`
	Modified string    `json:"modified,omitempty"`
	Checked  time.Time `json:"checked"`
	Version  string    `json:"version,omitempty"` // the dataset's own timestamp or version
	Fetched  time.Time `json:"fetched"`
	Entries  int       `json:"entries"`
	Bytes    int64     `json:"bytes"`
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
// one, and replaces the stored files only when all succeeded (08 §2.3 D2).
func (a *App) UpdateData(keys []string, dryRun bool) ([]*DataUpdate, error) {
	return a.UpdateDataProgress(keys, dryRun, nil)
}

// UpdateDataProgress reports each source without changing the all-or-nothing update.
func (a *App) UpdateDataProgress(keys []string, dryRun bool, progress func(key string, done int)) ([]*DataUpdate, error) {
	unlock := accounts.Lock(filepath.Join(a.Home, "cache", "community-data"))
	defer unlock()
	if err := a.subscriptionContext().Err(); err != nil {
		return nil, err
	}
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
	var ready []stagedDataFile
	cleanup := func() {
		for _, s := range ready {
			os.Remove(s.tmp)
		}
	}
	defer cleanup()
	failed := false
	for _, d := range Datasets {
		if !want(d.Key) {
			continue
		}
		if progress != nil {
			progress(d.Key, len(res))
		}
		u := &DataUpdate{Dataset: d, Info: &DataInfo{Key: d.Key, Source: a.DataSource(d), Fetched: a.Now().UTC(), Checked: a.Now().UTC()}}
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
		modified := ""
		if prev != nil && prev.Source == u.Info.Source && etag != "" {
			modified = prev.Modified
		}
		if prev != nil && prev.Source == u.Info.Source && etag == "" {
			if _, err := os.Stat(a.dataPath(d)); err == nil {
				modified = prev.Modified
			}
		}
		tmp, err := a.fetchData(d, u, etag, modified)
		if err != nil {
			u.Err, failed = err, true
			continue
		}
		if u.Same {
			if prev == nil || (etag == "" && modified == "") {
				u.Err, failed = errors.New("unexpected not-modified response"), true
				continue
			}
			copy := *prev
			copy.Checked = a.Now().UTC()
			u.Info = &copy
			continue
		}
		ready = append(ready, stagedDataFile{tmp, a.dataPath(d)})
	}
	if failed {
		return res, errors.New("community data not updated: a download failed or was invalid; the stored files are unchanged")
	}
	if err := a.subscriptionContext().Err(); err != nil {
		return res, err
	}
	if dryRun {
		return res, nil
	}
	stage := func(dst string, b []byte) error {
		file, err := stageDataFile(dst, b)
		if err == nil {
			ready = append(ready, file)
		}
		return err
	}
	for _, u := range res {
		b, err := json.MarshalIndent(u.Info, "", "  ")
		if err != nil {
			return res, err
		}
		if err := stage(a.dataPath(u.Dataset)+".source.json", b); err != nil {
			return res, err
		}
		if u.Dataset.Key == "steamdb" && !u.Same {
			for _, file := range ready {
				if file.dst != a.dataPath(u.Dataset) {
					continue
				}
				index, err := steamIndexData(file.tmp)
				if err != nil {
					return res, fmt.Errorf("steamDB index: %v", err)
				}
				if err := stage(a.steamIndexPath(), index); err != nil {
					return res, err
				}
				break
			}
		}
	}
	if err := a.subscriptionContext().Err(); err != nil {
		return res, err
	}
	err := replaceDataFiles(ready, os.Rename, func() error {
		_, err := a.Store.DB.Exec(`INSERT INTO meta(key,value) VALUES('community:generation','1') ON CONFLICT(key) DO UPDATE SET value=CAST(value AS INTEGER)+1`)
		return err
	})
	if err != nil {
		return res, err
	}
	a.dataIdx = nil
	return res, nil
}

// DropCommunityData forgets data cached before another connection updated it.
func (a *App) DropCommunityData() {
	a.dataIdx, a.prCache, a.planPS = nil, nil, nil
	a.prKey = ""
}

// fetchData downloads (or copies a local path) to a temp file and validates it.
func (a *App) fetchData(d Dataset, u *DataUpdate, etag string, modified ...string) (string, error) {
	src := u.Info.Source
	var body io.ReadCloser
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err := http.NewRequestWithContext(a.subscriptionContext(), "GET", src, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", UserAgent)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		if len(modified) > 0 && modified[0] != "" {
			req.Header.Set("If-Modified-Since", modified[0])
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
		u.Info.Modified = resp.Header.Get("Last-Modified")
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
	n, copyErr := io.Copy(tmp, io.LimitReader(body, 512<<20))
	err = errors.Join(copyErr, tmp.Close())
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
		defer zr.Close()
		b, err = io.ReadAll(io.LimitReader(zr, 512<<20))
		if err != nil {
			return nil, err
		}
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
			Timestamp looseString                `json:"timestamp"`
			Rules     map[string]json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Rules == nil {
			return fmt.Errorf("not RimSort community rules (%v)", err)
		}
		info.Version, info.Entries = string(v.Timestamp), len(v.Rules)
	case "steamdb":
		var v struct {
			Version  looseString                `json:"version"`
			Database map[string]json.RawMessage `json:"database"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Database == nil {
			return fmt.Errorf("not a RimSort Steam Workshop database (%v)", err)
		}
		info.Version, info.Entries = string(v.Version), len(v.Database)
	case "replacements":
		var v struct {
			Version looseString       `json:"version"`
			Rules   []json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Rules == nil {
			return fmt.Errorf("not a Use This Instead list (%v)", err)
		}
		info.Version, info.Entries = string(v.Version), len(v.Rules)
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
	key := a.Store.Meta("community:generation")
	if a.dataIdx != nil && a.dataKey == key {
		return a.dataIdx
	}
	a.dataKey = key
	ix := &dataIndex{byOldWS: map[string]*Replacement{}, byOldPkg: map[string]*Replacement{}, byPkg: map[string]WorkshopEntry{}}
	if b, err := readDataFile(a.dataPath(Datasets[2])); err == nil {
		for _, x := range replacementRules(b) {
			if x.OldWorkshopID != "" {
				ix.byOldWS[x.OldWorkshopID] = x
			}
			if x.OldPackageID != "" {
				ix.byOldPkg[x.OldPackageID] = x
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

func steamIndexData(path string) ([]byte, error) {
	b, err := readDataFile(path)
	if err != nil {
		return nil, err
	}
	var v struct {
		Database map[string]json.RawMessage `json:"database"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	// One entry at a time: a malformed entry is skipped, not the whole database.
	m := map[string][2]string{}
	readable := 0
	for id, raw := range v.Database {
		var e struct {
			PackageID looseString     `json:"packageId"`
			Name      looseString     `json:"name"`
			AppID     json.RawMessage `json:"appid"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		readable++
		p := strings.ToLower(string(e.PackageID))
		if p == "" || string(e.AppID) == "true" {
			continue
		}
		if prev, ok := m[p]; !ok || id < prev[0] {
			m[p] = [2]string{id, string(e.Name)}
		}
	}
	if readable == 0 && len(v.Database) > 0 {
		return nil, errors.New("no readable entry in the database")
	}
	return json.Marshal(m)
}

// looseString is a community-data text field that tolerates the maintainers' slips: a
// number is taken as its text, null as empty. Anything else (an object, a list, true or
// false) fails only the record holding it, never the whole file (08 §2.3).
type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case string(b) == "null":
		*s = ""
	case len(b) > 0 && b[0] == '"':
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = looseString(v)
	case len(b) > 0 && (b[0] == '-' || b[0] >= '0' && b[0] <= '9'):
		var n json.Number
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		*s = looseString(n.String())
	default:
		return fmt.Errorf("not text: %.20s", b)
	}
	return nil
}

// replacementRules reads the Use This Instead list one rule at a time, so a malformed
// rule is skipped instead of dropping the whole list.
func replacementRules(b []byte) []*Replacement {
	var v struct {
		Rules []json.RawMessage `json:"rules"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	out := make([]*Replacement, 0, len(v.Rules))
	for _, raw := range v.Rules {
		var r struct {
			OldWorkshopID looseString   `json:"oldWorkshopId"`
			OldPackageID  looseString   `json:"oldPackageId"`
			OldName       looseString   `json:"oldName"`
			NewWorkshopID looseString   `json:"newWorkshopId"`
			NewPackageID  looseString   `json:"newPackageId"`
			NewName       looseString   `json:"newName"`
			NewAuthor     looseString   `json:"newAuthor"`
			NewVersions   []looseString `json:"newVersions"`
		}
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		versions := make([]string, len(r.NewVersions))
		for i, v := range r.NewVersions {
			versions[i] = string(v)
		}
		out = append(out, &Replacement{OldWorkshopID: string(r.OldWorkshopID), OldPackageID: strings.ToLower(string(r.OldPackageID)), OldName: string(r.OldName),
			NewWorkshopID: string(r.NewWorkshopID), NewPackageID: strings.ToLower(string(r.NewPackageID)), NewName: string(r.NewName),
			NewAuthor: string(r.NewAuthor), NewVersions: versions})
	}
	return out
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
