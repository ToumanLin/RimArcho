package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RAttr is one of RimArcho's candidates for the task's cluster.
type RAttr struct {
	Pkg      string   `json:"pkg"`
	Role     string   `json:"role"`
	Evidence []string `json:"ev"`
}

// RimArcho is RimArcho's own verdict on a task's cluster (no agent).
type RimArcho struct {
	Item    string  `json:"item"`
	Cluster string  `json:"cluster"`
	Found   bool    `json:"found"`
	Conf    string  `json:"conf"`
	Attr    []RAttr `json:"attr"`
	MS      int64   `json:"ms"`
	Err     string  `json:"err,omitempty"`
}

// warm loads every task of one home in turn (so agents later find the analysis ready) and
// records RimArcho's attribution of the task's cluster.
func warm(cfg *Config, run, slot string, only []string) error {
	dir, err := runDir(cfg, run)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "rimarcho"), 0o755); err != nil {
		return err
	}
	ts, err := allTasks(cfg)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, x := range only {
		want[x] = true
	}
	h, ok := cfg.Homes[slot]
	if !ok {
		return fmt.Errorf("no home for slot %q", slot)
	}
	for _, t := range ts {
		if t.Slot != slot || (len(want) > 0 && !want[t.ID]) {
			continue
		}
		start := time.Now()
		r := &RimArcho{Item: t.ID}
		err := func() error {
			unlock, err := lockHome(h.Home)
			if err != nil {
				return err
			}
			defer unlock()
			a, err := openHome(h)
			if err != nil {
				return err
			}
			defer a.Close()
			logPath, _, err := loadTask(a, cfg, h, t)
			if err != nil {
				return err
			}
			rep, err := a.AnalyzeLog(logPath)
			if err != nil {
				return err
			}
			st, err := a.Loadout()
			if err != nil {
				return err
			}
			lc := taskCluster(rep, t)
			if lc == nil {
				return nil
			}
			r.Found, r.Cluster, r.Conf = true, lc.ID, lc.Confidence
			for _, at := range lc.Attr {
				pkg := at.Instance
				if m := st.Ix.ByID[at.Instance]; m != nil {
					pkg = strings.ToLower(m.PackageID)
				}
				r.Attr = append(r.Attr, RAttr{Pkg: pkg, Role: at.Role, Evidence: at.Evidence})
			}
			return nil
		}()
		if err != nil {
			r.Err = err.Error()
		}
		r.MS = time.Since(start).Milliseconds()
		b, _ := json.MarshalIndent(r, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "rimarcho", t.ID+".json"), b, 0o644); err != nil {
			return err
		}
		fmt.Printf("%s %s found=%v %s\n", t.ID, time.Duration(r.MS)*time.Millisecond, r.Found, r.Err)
	}
	return nil
}
