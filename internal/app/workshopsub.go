package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Subscribing and unsubscribing through the Steam client (06 §2.9, 08 §10). The call runs in
// `archo steam-worker` started in %LOCALAPPDATA%\RimArcho\steam, which holds a copy of the
// game's steam_api64.dll and steam_appid.txt; RimArcho never writes into the game folder.

// ErrSteamUnavailable: Steam could not be reached (not running, no DLL in the game, the
// API did not start). The window then opens the item's Workshop page instead.
var ErrSteamUnavailable = errors.New("Steam is not available")

// WorkshopOp is a subscribe, unsubscribe, resubscribe or state call.
type WorkshopOp struct {
	Action string
	DryRun bool
	Items  []WorkshopItem
	Local  bool   // Steam unavailable: local folders only; account state is unknown
	Note   string // why the live state is unavailable
}

// WorkshopItem is one item of the call.
type WorkshopItem struct {
	ID         string // the instance
	Name       string
	WorkshopID string
	Folder     string // folder to remove before resubscribing (empty for residue)
	Result     string // ok, failed:<EResult>, timeout
	State      uint32
	Installed  bool // local fallback: the folder exists now, not a Steam account state
	// unsubscribe: the plans that use it. The player's word goes: it still goes, and they
	// show it as missing (06 §2.9).
	UsedBy []string
	Links  *LinkLifecyclePlan
}

// SteamUnavailable is ErrSteamUnavailable with why, and the Workshop pages to open instead.
type SteamUnavailable struct {
	Why  string
	URLs []string
}

func (e *SteamUnavailable) Error() string {
	return fmt.Sprintf("Steam is not available (%s)", e.Why)
}
func (e *SteamUnavailable) Unwrap() error { return ErrSteamUnavailable }

// Workshop subscribes to, unsubscribes from, or resubscribes to the
// Workshop items of refs. Unsubscribing makes Steam delete the item's folder, so it is
// refused while the game runs; the plans that use it (UsedBy) show it missing afterwards.
func (a *App) Workshop(action string, refs []string, dryRun bool) (result *WorkshopOp, workshopErr error) {
	if action != steamworks.Subscribe && action != steamworks.Unsubscribe && action != steamworks.Resubscribe {
		return nil, fmt.Errorf("unknown action %s: want subscribe, unsubscribe or resubscribe", action)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	op := &WorkshopOp{Action: action, DryRun: dryRun}
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		ws := m.WorkshopID()
		if m.Kind != source.Workshop || !isDigits(ws) {
			return nil, fmt.Errorf("%s is not a Workshop item (src:%s)", st.Ix.Ref(m.ID), m.SourceLabel)
		}
		it := WorkshopItem{ID: m.ID, Name: m.DisplayName(), WorkshopID: ws}
		if action == steamworks.Resubscribe && !st.Ix.CanResubscribe(m) {
			return nil, fmt.Errorf("%s is a link or locked copy: resubscribe to the original Workshop copy", st.Ix.Ref(m.ID))
		}
		if action == steamworks.Resubscribe && !m.Residue {
			it.Folder = st.Ix.ModDir(m)
			if src := st.Ix.Source(m.SourceID); src != nil {
				// Keep the source path so a replacement junction cannot redirect cleanup.
				it.Folder = filepath.Join(src.Root, m.Rel)
			}
		}
		switch action {
		case steamworks.Subscribe:
			if !m.Residue {
				return nil, fmt.Errorf("%s is subscribed already", st.Ix.Ref(m.ID))
			}
		case steamworks.Unsubscribe:
			if m.Residue {
				return nil, fmt.Errorf("%s is not subscribed (an unsubscribed leftover: archo trash %s moves the folder to the recycle bin)", st.Ix.Ref(m.ID), st.Ix.Ref(m.ID))
			}
			if it.UsedBy, err = a.usedBy(st, m.ID); err != nil {
				return nil, err
			}
			if m.Status == identity.Live {
				it.Folder = st.Ix.ModDir(m)
				it.Links, err = a.PlanIncomingLinks(it.Folder, "")
				if err != nil {
					return nil, err
				}
			}
		}
		op.Items = append(op.Items, it)
	}
	if action == steamworks.Unsubscribe || action == steamworks.Resubscribe {
		if run, err := a.GameRunning(); err == nil && run != nil {
			return nil, fmt.Errorf("the game is running (pid %d): Steam would delete folders it has loaded; close it first", run.PID)
		}
	}
	if dryRun || len(op.Items) == 0 {
		return op, nil
	}
	if action == steamworks.Resubscribe {
		return a.resubscribe(op)
	}
	ids := make([]string, len(op.Items))
	var detached []*LinkLifecyclePlan
	for k, it := range op.Items {
		ids[k] = it.WorkshopID
		if it.Links != nil {
			detached = append(detached, it.Links)
		}
	}
	batch := joinLinkPlans(detached)
	if len(detached) > 0 {
		if err := a.beginLinkLifecycle(batch); err != nil {
			return nil, err
		}
	}
	committed := false
	defer func() {
		if !committed && len(detached) > 0 {
			workshopErr = a.abortLinkLifecycle(batch, workshopErr)
		}
	}()
	rep, err := a.runSteam(action, ids)
	if err != nil {
		return nil, a.unavailable(err, op.Items)
	}
	by := map[string]steamworks.Item{}
	for _, x := range rep.Items {
		by[strconv.FormatUint(x.ID, 10)] = x
	}
	var failed []string
	for k := range op.Items {
		it := &op.Items[k]
		x, ok := by[it.WorkshopID]
		if !ok {
			it.Result = "timeout"
		} else {
			it.Result, it.State = x.Result, x.State
		}
		if it.Result != "ok" {
			failed = append(failed, fmt.Sprintf("%s (%s)", it.Name, it.Result))
		}
	}
	if len(failed) > 0 {
		return op, fmt.Errorf("Steam did not confirm: %s", strings.Join(failed, ", "))
	}
	if len(detached) > 0 {
		if _, err := a.Scan(false); err != nil {
			return op, err
		}
	}
	committed = true
	if len(detached) > 0 {
		if err := a.finishLinkLifecycle(batch); err != nil {
			return op, err
		}
	}
	return op, nil
}

// resubscribe preserves the recorded copy and incoming links while replacing its folder.
// Only confirmed unsubscribes whose folders are gone proceed to subscribe; residues skip
// cleanup. Failed items keep their own result for the GUI's batched queue.
func (a *App) resubscribe(op *WorkshopOp) (*WorkshopOp, error) {
	var remove []string
	for _, it := range op.Items {
		if it.Folder != "" {
			remove = append(remove, it.WorkshopID)
		}
	}
	removed := map[string]steamworks.Item{}
	if len(remove) > 0 {
		rep, err := a.runSteam(steamworks.Unsubscribe, remove)
		if err != nil {
			return nil, a.unavailable(err, op.Items)
		}
		for _, it := range rep.Items {
			removed[strconv.FormatUint(it.ID, 10)] = it
		}
	}
	var add []string
	for k := range op.Items {
		it := &op.Items[k]
		it.Result = "timeout"
		if it.Folder != "" {
			if r, ok := removed[it.WorkshopID]; ok {
				it.Result, it.State = r.Result, r.State
			}
			if it.Result != "ok" {
				continue
			}
			if err := a.recycleWorkshopFolder(it.Folder); err != nil {
				it.Result = "failed:folder cleanup: " + err.Error()
				continue
			}
		}
		it.Result = "timeout"
		add = append(add, it.WorkshopID)
	}
	if len(add) > 0 {
		rep, err := a.runSteam(steamworks.Subscribe, add)
		if err != nil {
			return nil, a.unavailable(err, op.Items)
		}
		for _, r := range rep.Items {
			for k := range op.Items {
				it := &op.Items[k]
				if it.WorkshopID == strconv.FormatUint(r.ID, 10) {
					it.Result, it.State = r.Result, r.State
				}
			}
		}
	}
	var failed []string
	for _, it := range op.Items {
		if it.Result != "ok" {
			failed = append(failed, fmt.Sprintf("%s (%s)", it.Name, it.Result))
		}
	}
	if len(failed) > 0 {
		return op, fmt.Errorf("could not resubscribe: %s", strings.Join(failed, ", "))
	}
	return op, nil
}

// Steam may leave edited or untracked files behind, or still be deleting the folder.
// Use the shared recycle-bin allow-list and backend, but keep incoming links in place:
// the download will return at this path with the same recorded identity.
func (a *App) recycleWorkshopFolder(path string) error {
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("cannot check %s: %w", path, err)
		}
		if source.IsLink(path) {
			return fmt.Errorf("%s is a link; resubscribe to the original Workshop folder", path)
		}
		err := a.recycleTarget(path)
		if _, checkErr := os.Lstat(path); os.IsNotExist(checkErr) {
			return nil // Steam may have removed it even when recycling failed.
		} else if checkErr != nil {
			return fmt.Errorf("cannot verify removal of %s: %w", path, checkErr)
		}
		if errors.Is(err, ErrNotInTrashList) {
			return err
		}
		if !time.Now().Before(deadline) {
			if err != nil {
				return fmt.Errorf("%s still exists after waiting for the recycle bin; not subscribed: %w", path, err)
			}
			return fmt.Errorf("%s still exists after waiting for the recycle bin; not subscribed", path)
		}
		time.Sleep(100 * time.Millisecond) // retry transient sharing violations from Steam
	}
}

// CanResubscribe checks the source location before resolving links: ModDir points at
// the target of a junction, which would hide that the Workshop copy itself is a link.
func (ix *Index) CanResubscribe(m *store.Mod) bool {
	if m == nil || m.Kind != source.Workshop || !isDigits(m.WorkshopID()) || m.Origin != "" {
		return false
	}
	path := ix.ModDir(m)
	if src := ix.Source(m.SourceID); src != nil {
		path = filepath.Join(src.Root, m.Rel)
	}
	return !source.IsLink(path)
}

// WorkshopState asks Steam for live state; when unavailable it reports only local folder
// presence, with subscription and update state explicitly unknown.
func (a *App) WorkshopState(refs []string) (*WorkshopOp, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	op := &WorkshopOp{Action: steamworks.State}
	var ids []string
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		ws := m.WorkshopID()
		if ws == "" {
			return nil, fmt.Errorf("%s has no Workshop id", st.Ix.Ref(m.ID))
		}
		op.Items = append(op.Items, WorkshopItem{ID: m.ID, Name: m.DisplayName(), WorkshopID: ws})
		ids = append(ids, ws)
	}
	if len(ids) == 0 {
		return op, nil
	}
	rep, err := a.runSteam(steamworks.State, ids)
	if err != nil {
		if !errors.Is(err, ErrSteamUnavailable) {
			return nil, err
		}
		op.Local, op.Note = true, err.Error()
		for k := range op.Items {
			it := &op.Items[k]
			m := st.Ix.ByID[it.ID]
			it.Folder = st.Ix.ModDir(m)
			it.Installed = m.Status == identity.Live && dirExists(it.Folder)
			it.Result = "local; subscription and update state unknown"
		}
		return op, nil
	}
	by := map[string]uint32{}
	for _, x := range rep.Items {
		by[strconv.FormatUint(x.ID, 10)] = x.State
	}
	for k := range op.Items {
		op.Items[k].State = by[op.Items[k].WorkshopID]
	}
	return op, nil
}

func (a *App) unavailable(err error, items []WorkshopItem) error {
	var u *SteamUnavailable
	if !errors.As(err, &u) {
		return err
	}
	for _, it := range items {
		u.URLs = append(u.URLs, "https://steamcommunity.com/sharedfiles/filedetails/?id="+it.WorkshopID)
	}
	return u
}

// runSteam prepares the runtime folder and runs the worker (SteamWorker in tests), 50 items
// a call.
func (a *App) runSteam(action string, ids []string) (*steamworks.Report, error) {
	dir, err := a.SteamRuntime()
	if err != nil {
		return nil, &SteamUnavailable{Why: err.Error()}
	}
	run := a.SteamWorker
	if run == nil {
		run = steamWorkerProcess
	}
	all := &steamworks.Report{}
	for i := 0; i < len(ids); i += 50 {
		out, err := run(dir, append([]string{action}, ids[i:min(i+50, len(ids))]...))
		r := steamworks.Parse(out)
		if r.Error != "" {
			return nil, &SteamUnavailable{Why: r.Error}
		}
		if err != nil || !r.Done {
			why := strings.TrimSpace(out)
			if err != nil {
				why = err.Error()
			}
			return nil, &SteamUnavailable{Why: "the Steam call did not finish: " + why}
		}
		all.Items = append(all.Items, r.Items...)
	}
	return all, nil
}

// SteamRuntime is the worker's folder, %LOCALAPPDATA%\RimArcho\steam: the game's
// steam_api64.dll copied in (again when the game's changes) and steam_appid.txt.
func (a *App) SteamRuntime() (string, error) {
	e, err := a.Env()
	if err != nil {
		return "", err
	}
	var src string
	// next to the game's executable (1.7 here), or with Unity's plugins
	for _, p := range []string{
		filepath.Join(e.GameRoot, "steam_api64.dll"),
		filepath.Join(e.GameRoot, "RimWorldWin64_Data", "Plugins", "x86_64", "steam_api64.dll"),
		filepath.Join(e.GameRoot, "RimWorldWin64_Data", "Plugins", "steam_api64.dll"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			src = p
			break
		}
	}
	if src == "" {
		return "", fmt.Errorf("the game install has no steam_api64.dll")
	}
	dir := filepath.Join(a.LocalDir(), "steam")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "steam_appid.txt"), []byte(strconv.Itoa(steamworks.AppID)), 0o644); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "steam_api64.dll")
	if s, d := statOf(src), statOf(dst); d == nil || s.Size() != d.Size() || s.ModTime().After(d.ModTime()) {
		os.Remove(dst)
		if err := copyFile(src, dst, s); err != nil {
			return "", fmt.Errorf("copying steam_api64.dll: %w", err)
		}
	}
	return dir, nil
}

func statOf(p string) os.FileInfo {
	fi, err := os.Stat(p)
	if err != nil {
		return nil
	}
	return fi
}

// steamWorkerProcess starts `<this executable> steam-worker …` in dir, with a time limit.
func steamWorkerProcess(dir string, args []string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, append([]string{"steam-worker"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SteamAppId="+strconv.Itoa(steamworks.AppID), "SteamGameId="+strconv.Itoa(steamworks.AppID))
	hideWindow(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return string(out), fmt.Errorf("the Steam call took longer than 40 s")
	}
	return string(out), err
}
