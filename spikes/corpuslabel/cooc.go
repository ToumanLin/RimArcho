package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Cooc is a mod over-represented in the corpus logs that have a cluster key: the share of
// logs with the key that load the mod, against the share of the other logs. Correlation only;
// it points at a suspect when the log itself names no mod (missing assemblies, Core textures,
// shader warnings).
type Cooc struct {
	Pkg     string  `json:"pkg"`
	With    float64 `json:"with"`
	Without float64 `json:"without"`
}

type CoocKey struct {
	Logs  int    `json:"logs"`            // logs with the key
	Lists int    `json:"lists,omitempty"` // distinct mod lists among them (repeat uploads once)
	Of    int    `json:"of"`              // logs in the run
	Mods  []Cooc `json:"mods"`
}

const (
	coocMinLogs  = 20 // fewer logs are often one player's repeat uploads (design §6.2)
	coocMinLists = 15 // distinct mod lists (benchmark v2: repeat uploads collapsed)
	coocMinWith  = 0.5
	coocMinLift  = 2.0
	coocTop      = 8
)

// cooc computes Cooc for the sample's cluster keys over the logs of a corpusrun results file,
// with each log's mod list read from its raw upload (see loadLog on parsed.jsonl).
func cooc(results, corpus, samplePath, out string) error {
	items, err := readSample(samplePath)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, it := range items {
		want[it.Key] = true
	}
	f, err := os.Open(results)
	if err != nil {
		return err
	}
	defer f.Close()
	withKey := map[string]map[string]bool{} // key -> log ids
	var ids []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	for sc.Scan() {
		var r resultLine
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		ids = append(ids, r.ID)
		for _, c := range r.Clusters {
			if want[c.Key] {
				if withKey[c.Key] == nil {
					withKey[c.Key] = map[string]bool{}
				}
				withKey[c.Key][r.ID] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	mods := map[string][]string{}
	base := map[string]int{}
	for _, id := range ids {
		head, _, err := readRaw(filepath.Join(corpus, "raw", id+".txt"))
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, m := range rawMods(head) {
			if !seen[m.Pkg] {
				seen[m.Pkg] = true
				mods[id] = append(mods[id], m.Pkg)
				base[m.Pkg]++
			}
		}
	}
	n := len(mods)
	res := map[string]*CoocKey{}
	for key, logs := range withKey {
		ck := &CoocKey{Of: n}
		has, hasAll := map[string]int{}, map[string]int{}
		// Repeat uploads of one player (near-identical lists) count once: a log joins the
		// first earlier representative whose list it shares >= 80% with (Jaccard).
		var ids []string
		for id := range logs {
			if _, ok := mods[id]; ok {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		var reps []map[string]bool
		for _, id := range ids {
			ck.Logs++
			s := map[string]bool{}
			for _, pk := range mods[id] {
				s[pk] = true
				hasAll[pk]++
			}
			dup := false
			for _, r := range reps {
				if jaccard(s, r) >= 0.8 {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			reps = append(reps, s)
			for pk := range s {
				has[pk]++
			}
		}
		ck.Lists = len(reps)
		res[key] = ck
		if ck.Lists < coocMinLists {
			continue
		}
		for pk, h := range has {
			with := float64(h) / float64(ck.Lists)
			without := float64(base[pk]-hasAll[pk]) / float64(max(1, n-ck.Logs))
			if with >= coocMinWith && with >= coocMinLift*without {
				ck.Mods = append(ck.Mods, Cooc{pk, with, without})
			}
		}
		sort.Slice(ck.Mods, func(i, j int) bool {
			li := ck.Mods[i].With / (ck.Mods[i].Without + 0.01)
			lj := ck.Mods[j].With / (ck.Mods[j].Without + 0.01)
			if li != lj {
				return li > lj
			}
			return ck.Mods[i].Pkg < ck.Mods[j].Pkg
		})
		if len(ck.Mods) > coocTop {
			ck.Mods = ck.Mods[:coocTop]
		}
	}
	j, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(out, j, 0o644); err != nil {
		return err
	}
	for _, it := range items {
		ck := res[it.Key]
		if ck == nil {
			fmt.Printf("%s key not in results\n", it.Item)
			continue
		}
		var s []string
		for _, m := range ck.Mods {
			s = append(s, fmt.Sprintf("%s %.0f%%/%.0f%%", m.Pkg, 100*m.With, 100*m.Without))
		}
		fmt.Printf("%s %4d logs  %s\n", it.Item, ck.Logs, strings.Join(s, ", "))
	}
	return nil
}

func readCooc(path string) (map[string]*CoocKey, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]*CoocKey
	return m, json.Unmarshal(b, &m)
}

func jaccard(a, b map[string]bool) float64 {
	in := 0
	for k := range a {
		if b[k] {
			in++
		}
	}
	u := len(a) + len(b) - in
	if u == 0 {
		return 1
	}
	return float64(in) / float64(u)
}
