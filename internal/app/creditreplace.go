package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ReplaceCreditItem switches only the recorded SteamCMD instance. Current lists and
// staging follow the client copy; immutable history stays intact. Every side effect
// is idempotent or journalled so a crash between plans, deployments or recycling resumes.
func (a *App) ReplaceCreditItem(it store.CreditItem) (string, error) {
	if err := a.checkNoGame(); err != nil {
		return "", err
	}
	old, err := a.Store.Mod(it.InstanceID)
	if err != nil {
		return "", err
	}
	if old == nil || old.Kind != source.SteamCMD || old.WorkshopID() != it.WorkshopID {
		return "retained: recorded SteamCMD instance is unavailable or has changed", nil
	}
	if old.Status != identity.Live {
		if it.State == "switching" {
			return "SteamCMD copy already removed", nil
		}
		return "retained: SteamCMD copy is no longer live", nil
	}
	if _, err := a.Scan(false); err != nil {
		return "", err
	}
	ix, err := a.Index()
	if err != nil {
		return "", err
	}
	var target *store.Mod
	for _, m := range ix.Mods {
		if m.Kind == source.Workshop && m.Status == identity.Live && !m.Residue && m.Unloadable == "" && m.WorkshopID() == it.WorkshopID && (it.TargetID == "" || m.ID == it.TargetID) {
			if _, err := os.ReadFile(filepath.Join(ix.ModDir(m), "About", "About.xml")); err == nil {
				target = m
				break
			}
		}
	}
	if target == nil {
		return "", fmt.Errorf("waiting for scanned Workshop instance %s with readable About.xml", it.WorkshopID)
	}
	if target.LineageID != old.LineageID || target.PackageID == "" {
		return "retained: Workshop instance identity needs review", nil
	}
	envName, openPlan, planName := a.EnvName, a.OpenPlan, a.PlanName
	defer func() { a.EnvName, a.OpenPlan, a.PlanName = envName, openPlan, planName }()
	if it.State != "switching" {
		// Capture the actual writer before workspace edits. The deployment obligations
		// outlive the task and prevent an interrupted switch from forgetting a game.
		envs, err := a.Store.Envs()
		if err != nil {
			return "", err
		}
		seen := map[string]bool{}
		for _, e := range envs {
			if e.UserDataRoot == "" || seen[store.UserDataKey(e.UserDataRoot)] {
				continue
			}
			seen[store.UserDataKey(e.UserDataRoot)] = true
			rec, err := a.Store.ModsConfigStateAt(e.UserDataRoot)
			if err != nil {
				return "", err
			}
			writer := e
			if rec.WriterEnv != "" {
				writer, err = a.Store.EnvByID(rec.WriterEnv)
				if err != nil {
					return "", err
				}
				if writer == nil {
					return "", fmt.Errorf("unknown ModsConfig writer %s", rec.WriterEnv)
				}
			}
			a.EnvName, a.OpenPlan, a.PlanName = writer.Name, rec.WriterPlan, ""
			if err := a.checkNoGame(); err != nil {
				return "", err
			}
			st, err := a.Loadout()
			if err != nil {
				return "", err
			}
			if st.FileErr != nil {
				return "", st.FileErr
			}
			if st.File == nil {
				continue
			}
			list, err := a.creditGameProjection(st, rec, old.ID)
			if err != nil {
				return "", err
			}
			if list.Index(old.ID) >= 0 {
				if err := a.Store.AddCreditDeployment(it, writer.ID, st.Plan.ID); err != nil {
					return "", err
				}
			}
		}
		it.State, it.TargetID = "switching", target.ID
		if err := a.Store.SaveCreditItem(it); err != nil {
			return "", err
		}
	}
	plans, err := a.Store.Plans()
	if err != nil {
		return "", err
	}
	for _, p := range plans {
		if err := a.checkNoGame(); err != nil {
			return "", err
		}
		_, err := a.editOther(p, "credit-replace", func(l loadout.List) (loadout.List, int) { return creditSwap(l, old.ID, target.ID) })
		if err != nil {
			return "", err
		}
		var staged []StageItem
		raw := a.Store.Meta(store.StageKey(p.ID))
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &staged); err != nil {
				return "", err
			}
			changed := false
			for i := range staged {
				if staged[i].Instance == old.ID {
					staged[i].Instance = target.ID
					changed = true
				}
				if staged[i].After == old.ID {
					staged[i].After = target.ID
					changed = true
				}
			}
			if changed {
				if err := a.setStaged(p.ID, staged); err != nil {
					return "", err
				}
			}
		}
	}
	deployments, err := a.Store.CreditDeployments(it)
	if err != nil {
		return "", err
	}
	for _, d := range deployments {
		e, err := a.Store.EnvByID(d.EnvID)
		if err != nil {
			return "", err
		}
		if e == nil {
			return "", fmt.Errorf("deployment environment %s no longer exists", d.EnvID)
		}
		if p, err := a.Store.PlanByID(d.PlanID); err != nil {
			return "", err
		} else if p == nil {
			return "", fmt.Errorf("deployment plan %d no longer exists", d.PlanID)
		}
		a.EnvName, a.OpenPlan, a.PlanName = e.Name, d.PlanID, ""
		if err := a.checkNoGame(); err != nil {
			return "", err
		}
		st, err := a.Loadout()
		if err != nil {
			return "", err
		}
		if !st.Reg.Loadable(target.ID) {
			return "", fmt.Errorf("Workshop instance cannot be loaded in %s", e.Name)
		}
		// Save owns registry tokens, _steam, link journal, external-edit checks and backups.
		if st.File == nil {
			return "", fmt.Errorf("deployment ModsConfig is missing in %s", e.Name)
		}
		rec, err := a.Store.ModsConfigStateAt(e.UserDataRoot)
		if err != nil {
			return "", err
		}
		projection, err := a.creditGameProjection(st, rec, old.ID)
		if err != nil {
			return "", err
		}
		st.List, _ = creditSwap(projection, old.ID, target.ID)
		if _, err := a.writeGame(st, false, false, false, old); err != nil {
			return "", err
		}
		if err := a.Store.FinishCreditDeployment(it, d.EnvID); err != nil {
			return "", err
		}
	}
	if err := a.checkNoGame(); err != nil {
		return "", err
	}
	held, err := a.creditReferenced(old.ID)
	if err != nil {
		return "", err
	}
	if held {
		return "retained: lists, staging, undo history, versions or packs still reference the SteamCMD copy", nil
	}
	links, err := a.IncomingLinks(old.RealPath)
	if err != nil {
		return "", err
	}
	if len(links) > 0 {
		return "retained: links still reference the SteamCMD copy", nil
	}
	// Recheck inside the shared lifecycle immediately before moving the folder.
	err = a.withIncomingLinks(old.RealPath, "", func() error {
		if err := a.checkNoGame(); err != nil {
			return err
		}
		held, err := a.creditReferenced(old.ID)
		if err != nil {
			return err
		}
		if held {
			return fmt.Errorf("SteamCMD copy acquired a reference; keeping it")
		}
		if err := a.recycleTarget(old.RealPath); err != nil {
			return err
		}
		_, err = a.Scan(false)
		return err
	})
	if err != nil {
		return "", err
	}
	return "switched to Workshop; SteamCMD copy moved to recycle bin", nil
}

func creditSwap(l loadout.List, old, target string) (loadout.List, int) {
	if l.Index(old) < 0 {
		return l, 0
	}
	out := loadout.List{}
	// Keep the old position, removing an already-enabled duplicate of the target.
	for _, en := range l {
		if en.Instance == target {
			continue
		}
		if en.Instance == old {
			en = loadout.Entry{Instance: target}
		}
		out = append(out, en)
	}
	return out, 1
}

// History is a real reference too: never silently invalidate undo or old versions to
// obtain disk savings. Packs own their copies and are never rewritten by this feature.
func (a *App) creditReferenced(id string) (bool, error) {
	for _, q := range []string{`SELECT entries FROM workspace`, `SELECT entries FROM workspace_undo`, `SELECT entries FROM loadout_version`} {
		rows, err := a.Store.DB.Query(q)
		if err != nil {
			return false, err
		}
		found := false
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return false, err
			}
			if !json.Valid([]byte(raw)) {
				rows.Close()
				return false, fmt.Errorf("cannot verify references in unreadable list")
			}
			found = found || loadout.Decode(raw).Index(id) >= 0
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	plans, err := a.Store.Plans()
	if err != nil {
		return false, err
	}
	for _, p := range plans {
		var staged []StageItem
		raw := a.Store.Meta(store.StageKey(p.ID))
		if raw == "" {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &staged); err != nil {
			return false, err
		}
		for _, s := range staged {
			if s.Instance == id || s.After == id {
				return true, nil
			}
		}
	}
	packs, err := a.Packs()
	if err != nil {
		return false, err
	}
	for _, p := range packs {
		for _, en := range p.Entries {
			if en.Instance == id {
				return true, nil
			}
		}
	}
	return false, nil
}

// Adding a same-package local copy can change how the old token resolves before
// handoff. An unchanged file's writer snapshot still identifies the exact Instance
// selected by the player; use that identity only at the recorded old position.
func (a *App) creditGameProjection(st *LoadoutState, rec *store.ModsConfigState, old string) (loadout.List, error) {
	projection, _ := st.Reg.FromTokens(st.File.Active)
	if rec.WriterPlan == 0 || rec.WriterVersion == 0 || !bytes.Equal(rec.Written, loadout.Hash(st.File.Raw)) {
		return projection, nil
	}
	v, err := a.Store.Version(rec.WriterPlan, rec.WriterVersion)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return projection, nil
	}
	recorded := loadout.Decode(v.Entries)
	if len(recorded) == len(projection) {
		for i, en := range recorded {
			if en.Instance == old {
				projection[i] = en
			}
		}
	}
	return projection, nil
}
