package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestZZDry(t *testing.T) {
	dir := os.Getenv("DRY")
	if dir == "" {
		t.Skip()
	}
	ix, _ := loadIndex(dir)
	seen := map[string]bool{}
	v := map[string]int{}
	var short []int
	probN := []int{}
	for _, id := range ix.order {
		b, err := os.ReadFile(filepath.Join(dir, "raw", id+".txt"))
		if err != nil {
			continue
		}
		r := parse(id, "", b)
		if r.Status == "ok" && r.MM == "1.6" {
			if r.LogLines < 200 {
				short = append(short, r.LogLines)
			}
			n := 0
			for _, c := range r.Clusters {
				if isProblem(c.Class) {
					n++
				}
			}
			probN = append(probN, n)
		}
		vd, mm, h := judge(id, "", b, seen)
		if vd == verdictKept {
			seen[h] = true
		}
		v[vd+" "+mm]++
	}
	fmt.Println(fmtCounts(v, 0))
	sort.Ints(short)
	fmt.Println("1.6 logs under 200 lines:", len(short), short)
	fmt.Println("problem clusters per 1.6 log:", dist(probN))
}
