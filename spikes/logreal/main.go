// Spike: parse a real Player.log with internal/logs and print the cluster summary.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rimarcho/rimarcho/internal/logs"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	t0 := time.Now()
	lf, err := logs.Parse(f)
	if err != nil {
		panic(err)
	}
	cs := logs.Clusters(lf)
	fmt.Printf("%d lines, %d entries, %d clusters, version %q, %v\n", lf.Lines, len(lf.Entries), len(cs), lf.Version, time.Since(t0))
	by := map[string]int{}
	for _, c := range cs {
		by[c.Class]++
	}
	fmt.Println(by)
	for i, c := range cs {
		if i > 40 {
			break
		}
		m := c.First.Message
		if len(m) > 110 {
			m = m[:110]
		}
		top := ""
		if len(c.First.Frames) > 0 {
			top = c.First.Frames[0].Type + "." + c.First.Frames[0].Method
		}
		fmt.Printf("%-4s %-13s x%-4d L%-5d %s | %s\n", c.ID, c.Class, c.Count, c.First.Line, m, top)
	}
}
