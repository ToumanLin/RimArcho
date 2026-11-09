package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// applyLinks journals deployment independently of the scan's disposable aliases.
func (a *App) applyLinks(mods string, changes []LinkChange) error {
	mods = realOrAbs(mods)
	if len(changes) == 0 {
		return nil
	}
	unlock, err := lockGameLinks(mods)
	if err != nil {
		return err
	}
	defer unlock()
	for _, c := range changes {
		if a.probe()(filepath.Dir(mods)) != nil {
			return fmt.Errorf("affected game is running: %s", filepath.Dir(mods))
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		row := store.ManagedLink{Operation: hex.EncodeToString(id[:]), Mods: mods, Name: c.Name, Kind: "junction", Instance: c.Mod}
		if runtime.GOOS != "windows" {
			row.Kind = "symlink"
		}
		path := filepath.Join(mods, c.Name)
		var snapshot *source.LinkInfo
		if c.Op == "remove" {
			info, err := source.ReadLink(path)
			if err != nil {
				return err
			}
			if info.Kind != "junction" && info.Kind != "symlink" {
				return fmt.Errorf("unverified deployment link: %s", path)
			}
			row.Kind, row.Target = info.Kind, info.Target
			snapshot = info
		} else {
			row.NextPath, row.NextTarget = path, c.Target
			if packLink(c.Name) {
				row.Pack = filepath.Base(filepath.Dir(filepath.Dir(c.Target)))
			}
		}
		_, statErr := os.Lstat(path)
		existed := statErr == nil
		if err := a.Store.RecordLinkIntent([]store.ManagedLink{row}); err != nil {
			return err
		}
		if snapshot != nil {
			if err := source.RemoveLinkChecked(snapshot); err != nil {
				a.settleFailedLink(row.Operation, path, snapshot, existed)
				return err
			}
		} else if err := applyLinks(mods, []LinkChange{c}); err != nil {
			a.settleFailedLink(row.Operation, path, nil, existed)
			return err
		}
		if err := a.Store.LinkPhase(row.Operation, "complete"); err != nil {
			return err
		}
	}
	return nil
}

// settleFailedLink closes the record of a single link step that failed without changing
// the disk: a removal whose link is still the one read before (before), or a creation that
// left nothing new at path. Left at intent, it would report a recovery that blocks launch
// although there is nothing to recover (09 §3). A step whose effect is unclear stays at
// intent for the confirmed repair.
func (a *App) settleFailedLink(op, path string, before *source.LinkInfo, existed bool) {
	unchanged := false
	if before != nil {
		unchanged = source.VerifyLink(before) == nil
	} else {
		_, err := os.Lstat(path)
		unchanged = existed || os.IsNotExist(err) // CreateLink removes what it made when it fails
	}
	if unchanged {
		a.Store.LinkPhase(op, "rolled-back")
	}
}

func (p *LinkLifecyclePlan) Lines() []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, l := range p.Links {
		action := "detach link"
		if l.NextTarget != "" {
			action = "detach and reattach -> " + l.NextTarget
			if l.Target == "" {
				action = "create link -> " + l.NextTarget
			}
		}
		out = append(out, fmt.Sprintf("%s: %s -> %s; %s", l.Game, filepath.Join(l.Mods, l.Name), l.Target, action))
	}
	return out
}
