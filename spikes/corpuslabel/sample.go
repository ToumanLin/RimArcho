package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Item is one sampled (log, cluster).
type Item struct {
	Item  string  `json:"item"` // P01 …
	Gist  string  `json:"gist"`
	Key   string  `json:"key"`
	Tpl   string  `json:"tpl"`
	Class string  `json:"class"`
	Count int     `json:"count"`
	Cov   float64 `json:"cov"`
	// TplLogs is how many covered logs have a cluster of this template (the stratum's weight).
	TplLogs int `json:"tpl_logs"`
}

type resultLine struct {
	ID       string `json:"id"`
	Mods     int    `json:"mods"`
	Resolved int    `json:"resolved"`
	Clusters []struct {
		Key   string `json:"key"`
		Tpl   string `json:"tpl"`
		Class string `json:"class"`
		Count int    `json:"count"`
	} `json:"clusters"`
}

func pickHash(seed, gist, key string) string {
	h := sha1.Sum([]byte(seed + "|" + gist + "|" + key))
	return hex.EncodeToString(h[:])
}

// sample: from logs with library coverage >= minCov, problem classes only (informational
// classes mod-report and other are noise), the tpl most frequent templates of each class by
// number of logs, and per clusters of each template, each from a different log.
func sample(results, out string, tpl, per int, minCov float64, seed string) error {
	f, err := os.Open(results)
	if err != nil {
		return err
	}
	defer f.Close()
	type cand struct {
		gist, key, hash string
		count           int
		cov             float64
	}
	byTpl := map[string][]cand{}
	tplClass := map[string]string{}
	tplLogs := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	nLogs := 0
	for sc.Scan() {
		var r resultLine
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Mods == 0 {
			continue
		}
		cov := float64(r.Resolved) / float64(r.Mods)
		if cov < minCov {
			continue
		}
		nLogs++
		seen := map[string]bool{}
		for _, c := range r.Clusters {
			if logs.Informational(c.Class) {
				continue
			}
			if !seen[c.Tpl] {
				seen[c.Tpl] = true
				tplLogs[c.Tpl]++
			}
			tplClass[c.Tpl] = c.Class
			byTpl[c.Tpl] = append(byTpl[c.Tpl], cand{r.ID, c.Key, pickHash(seed, r.ID, c.Key), c.Count, cov})
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	byClass := map[string][]string{}
	for t, c := range tplClass {
		byClass[c] = append(byClass[c], t)
	}
	var classes []string
	for c := range byClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	usedLog := map[string]bool{}
	var items []Item
	for _, c := range classes {
		ts := byClass[c]
		sort.Slice(ts, func(i, j int) bool {
			if tplLogs[ts[i]] != tplLogs[ts[j]] {
				return tplLogs[ts[i]] > tplLogs[ts[j]]
			}
			return ts[i] < ts[j]
		})
		if len(ts) > tpl {
			ts = ts[:tpl]
		}
		for _, t := range ts {
			cs := byTpl[t]
			sort.Slice(cs, func(i, j int) bool { return cs[i].hash < cs[j].hash })
			n := 0
			for _, x := range cs {
				if n == per {
					break
				}
				if usedLog[x.gist] {
					continue
				}
				usedLog[x.gist] = true
				n++
				items = append(items, Item{Gist: x.gist, Key: x.key, Tpl: t, Class: c, Count: x.count, Cov: x.cov, TplLogs: tplLogs[t]})
			}
		}
	}
	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()
	for i := range items {
		items[i].Item = fmt.Sprintf("P%02d", i+1)
		j, _ := json.Marshal(items[i])
		o.Write(append(j, '\n'))
		fmt.Printf("%s %-16s %5d logs  %.80s\n", items[i].Item, items[i].Class, items[i].TplLogs, items[i].Tpl)
	}
	fmt.Printf("%d items from %d covered logs (>= %.0f%%)\n", len(items), nLogs, minCov*100)
	return nil
}

func readSample(path string) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Item
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var it Item
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, sc.Err()
}
