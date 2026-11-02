package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

type linkRecovery struct {
	row                 store.ManagedLink
	path, target, phase string
	absent              bool
	snapshot            *source.LinkInfo
}

// Startup/needs/scan observations only: no directory, link or ledger writes.
func (a *App) linkRecoveries(rows []store.ManagedLink) ([]*GameLink, error) {
	var out []*GameLink
	for _, row := range rows {
		if store.LinkFinished(row.Phase) {
			continue
		}
		r := &linkRecovery{row: row, path: filepath.Join(row.Mods, row.Name), target: row.Target, phase: "rolled-back"}
		forward := row.Phase == "records-updated" || row.Target == ""
		if row.From != "" {
			forward = row.Phase == "records-updated"
			if !forward && missingTarget(row.From) {
				forward = row.To == ""
				if row.To != "" {
					state, _ := source.ProbeTarget(row.To)
					forward = state == source.TargetPresent
				}
			}
		} else if !forward && missingTarget(row.Target) {
			if row.NextTarget == "" {
				forward = true
			} else if row.Instance != "" {
				m, err := a.Store.Mod(row.Instance)
				if err != nil {
					return nil, err
				}
				forward = m != nil && env.PathKey(physicalLinkPath(m.RealPath)) == env.PathKey(physicalLinkPath(row.NextTarget))
			}
		}
		if forward {
			r.path, r.target, r.phase = row.NextPath, row.NextTarget, "complete"
		}
		display := r.path
		if display == "" {
			display = filepath.Join(row.Mods, row.Name)
		}
		l := &GameLink{Game: filepath.Dir(row.Mods), Mods: row.Mods, Path: display, Target: r.target, Kind: row.Kind, State: source.TargetUnknown, Code: CodeLinkRecovery, Reason: "unfinished link operation (" + row.Phase + "); review recovery", Evidence: "ledger operation " + row.Operation, Owned: true, Action: "keep", Instance: row.Instance, Pack: row.Pack, recovery: r}
		if r.path != "" {
			l.Mods = filepath.Dir(r.path)
			l.Game = filepath.Dir(l.Mods)
		}
		if filepath.Base(row.Name) != row.Name || row.Kind != "junction" && row.Kind != "symlink" || !managedLink(row.Name) && !packLink(row.Name) || r.path != "" && !managedLink(filepath.Base(r.path)) && !packLink(filepath.Base(r.path)) {
			l.Reason += "; unsupported ledger entry; manual resolution required"
			out = append(out, l)
			continue
		}
		if r.target == "" {
			// A completed target removal only needs acknowledgement; never delete an
			// unexpected link which appeared after the interrupted operation.
			old := filepath.Join(row.Mods, row.Name)
			if _, err := os.Lstat(old); os.IsNotExist(err) {
				r.absent = true
				l.Action = "complete recovery"
				l.State = source.TargetMissing
			}
		} else if state, reason := source.ProbeTarget(r.target); state == source.TargetPresent {
			l.State = state
			if _, err := os.Lstat(r.path); os.IsNotExist(err) {
				r.absent = true
				l.Action = "recover link"
			} else if err == nil {
				info, err := source.ReadLink(r.path)
				if err == nil && info.Kind == row.Kind && (info.Kind == "junction" || info.Kind == "symlink") && env.PathKey(physicalLinkPath(info.Target)) == env.PathKey(physicalLinkPath(r.target)) {
					r.snapshot = info
					l.Action = "complete recovery"
				}
			}
		} else {
			l.Reason += "; " + reason
		}
		out = append(out, l)
	}
	return out, nil
}

func (a *App) applyLinkRecoveries(p *LinkRepairPlan) error {
	var selected []*GameLink
	operations := map[string]bool{}
	for _, l := range p.Links {
		if l.recovery != nil {
			if l.Action != "recover link" && l.Action != "complete recovery" {
				return fmt.Errorf("recovery needs manual resolution: %s: %s", l.Path, l.Reason)
			}
			selected = append(selected, l)
			operations[l.recovery.row.Operation] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}
	rows, err := a.Store.ManagedLinks()
	if err != nil {
		return err
	}
	current := map[int64]store.ManagedLink{}
	counts := map[string]int{}
	for _, row := range rows {
		current[row.ID] = row
		if !store.LinkFinished(row.Phase) {
			counts[row.Operation]++
		}
	}
	seen := map[string]int{}
	var unlocks []func()
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()
	locked := map[string]bool{}
	for _, l := range selected {
		if !locked[l.Mods] {
			unlock, err := lockGameLinks(l.Mods)
			if err != nil {
				return err
			}
			unlocks = append(unlocks, unlock)
			locked[l.Mods] = true
		}
		r := l.recovery
		if current[r.row.ID] != r.row {
			return fmt.Errorf("recovery changed since preview: %s", l.Path)
		}
		if a.probe()(l.Game) != nil {
			return fmt.Errorf("affected game is running: %s", l.Game)
		}
		if r.target != "" {
			state, _ := source.ProbeTarget(r.target)
			if state != source.TargetPresent {
				return fmt.Errorf("recovery target is unavailable: %s", r.target)
			}
		}
		if r.absent {
			path := r.path
			if path == "" {
				path = filepath.Join(r.row.Mods, r.row.Name)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return fmt.Errorf("recovery path changed: %s", path)
			}
		} else if err := source.VerifyLink(r.snapshot); err != nil {
			return err
		}
		seen[r.row.Operation]++
	}
	for op, n := range counts {
		if _, ok := operations[op]; ok && seen[op] != n {
			return fmt.Errorf("operation affects other games; use links repair --all: %s", op)
		}
	}
	updated := map[string]bool{}
	for _, l := range selected {
		r := l.recovery
		if r.phase == "complete" && r.row.From != "" && r.row.To != "" && r.row.Phase != "records-updated" {
			if r.row.Directory != "" && updated[r.row.Operation] {
				continue
			}
			if err := a.recoverLinkRecords(r.row); err != nil {
				return err
			}
			updated[r.row.Operation] = true
		}
	}
	for _, l := range selected {
		r := l.recovery
		if r.phase == "complete" {
			if err := a.Store.LinkEntryPhase(r.row.Operation, r.row.Mods, r.row.Name, "records-updated"); err != nil {
				return err
			}
		}
	}
	for _, l := range selected {
		r := l.recovery
		if l.Action == "recover link" {
			if a.probe()(l.Game) != nil {
				return fmt.Errorf("affected game is running: %s", l.Game)
			}
			if err := CreateLink(r.path, r.target); err != nil {
				return err
			}
		}
	}
	for _, l := range selected {
		r := l.recovery
		if err := a.Store.LinkEntryPhase(r.row.Operation, r.row.Mods, r.row.Name, r.phase); err != nil {
			return err
		}
	}
	a.modsCopies = nil
	return nil
}

// The player has reviewed the recorded old/new locations. Complete database work
// only when its current facts still name either endpoint of that exact operation.
func (a *App) recoverLinkRecords(row store.ManagedLink) error {
	if row.Directory != "" {
		d, err := a.managedDirectory(row.Directory)
		if err != nil {
			return err
		}
		key := env.PathKey(physicalLinkPath(d.Path))
		if key != env.PathKey(physicalLinkPath(row.From)) && key != env.PathKey(physicalLinkPath(row.To)) {
			return fmt.Errorf("directory records changed since interrupted operation: %s", row.Directory)
		}
		if err := a.validateDirectory(d, row.To); err != nil {
			return err
		}
		return a.setDirectoryRecords(d, row.To)
	}
	if row.Instance == "" || row.NextTarget == "" {
		return nil
	}
	m, err := a.Store.Mod(row.Instance)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("recovery instance is no longer registered: %s", row.Instance)
	}
	next := physicalLinkPath(row.NextTarget)
	if env.PathKey(physicalLinkPath(m.RealPath)) == env.PathKey(next) {
		return nil
	}
	old := row.Target
	if old == "" {
		old = row.From
	}
	if env.PathKey(physicalLinkPath(m.RealPath)) != env.PathKey(physicalLinkPath(old)) {
		return fmt.Errorf("instance records changed since interrupted operation: %s", row.Instance)
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return err
	}
	var destination *source.Source
	for _, s := range ss {
		if s.Status != source.StatusRetired && s.Kind != source.Official && source.Within(next, physicalLinkPath(s.Root)) && (destination == nil || len(s.Root) > len(destination.Root)) {
			destination = s
		}
	}
	if destination == nil {
		return fmt.Errorf("recovery destination has no registered source: %s", next)
	}
	rel, err := filepath.Rel(physicalLinkPath(destination.Root), next)
	if err != nil {
		return err
	}
	if err := a.Store.RelocateInstance(row.Instance, destination.ID, rel, next); err != nil {
		return err
	}
	a.ixCache, a.stCache, a.prCache, a.linkInst = nil, nil, nil, nil
	forgetLinks(a.Store)
	return nil
}
