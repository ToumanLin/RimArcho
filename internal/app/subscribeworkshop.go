package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func (a *App) findTool(name string) (string, bool) {
	if a.FindTool != nil {
		return a.FindTool(name)
	}
	if name == "steamcmd" {
		p := a.SteamCMDPath()
		return p, p != ""
	}
	return "", false
}

func (a *App) SteamCMDModsDir() string {
	if p := a.Store.Meta("cfg:steamcmd-mods.dir"); p != "" {
		return p
	}
	return filepath.Join(a.LocalDir(), "steamcmd-mods")
}

func (a *App) steamCMDContentRoot() string {
	return filepath.Join(a.SteamCMDModsDir(), "steamapps", "workshop", "content", env.WorkshopAppID)
}

func workshopIDs(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("give at least one Workshop ID or URL")
	}
	var ids []string
	seen := map[string]bool{}
	for _, input := range inputs {
		id := strings.TrimSpace(input)
		if u, err := url.Parse(id); err == nil && u.Host != "" {
			if !strings.EqualFold(u.Hostname(), "steamcommunity.com") || (u.Scheme != "https" && u.Scheme != "http") {
				return nil, fmt.Errorf("not a Workshop URL: %s", input)
			}
			id = u.Query().Get("id")
		}
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("invalid Workshop ID %q", input)
		}
		id = strconv.FormatUint(n, 10)
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func workshopURL(id string) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + id
}

func (a *App) SubscribeWorkshop(inputs []string, forceSteamCMD, dryRun bool) (*SubscribeOp, error) {
	ids, err := workshopIDs(inputs)
	if err != nil {
		return nil, err
	}
	if _, err := a.Env(); err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "workshop", DryRun: dryRun}
	for _, id := range ids {
		op.Items = append(op.Items, SubscribeItem{Name: "Workshop " + id, Record: &subscribe.Record{WorkshopID: id}})
	}
	if dryRun {
		if forceSteamCMD {
			return a.downloadWorkshop(ids, "", true)
		}
		op.Notes = append(op.Notes, "Would subscribe through Steam; use SteamCMD only if Steam is unavailable.")
		return op, nil
	}
	if !forceSteamCMD {
		report, err := a.runSteam(steamworks.Subscribe, ids)
		if err == nil {
			byID := map[string]string{}
			for _, item := range report.Items {
				byID[strconv.FormatUint(item.ID, 10)] = item.Result
			}
			var failures []error
			for i, id := range ids {
				result := byID[id]
				if result != "ok" {
					failures = append(failures, fmt.Errorf("Workshop %s: Steam did not confirm subscription (%s); %s", id, result, workshopURL(id)))
				}
				op.Items[i].Result = result
			}
			op.Notes = append(op.Notes, "Steam downloads it; it appears after the next scan.")
			return op, errors.Join(failures...)
		}
		if !errors.Is(err, ErrSteamUnavailable) {
			return nil, err
		}
	}
	return a.downloadWorkshop(ids, "", false)
}

var workshopSuccess = regexp.MustCompile(`Success\. Downloaded item (\d+)`)
var workshopFailure = regexp.MustCompile(`ERROR! Download item (\d+) failed(?: \(([^\r\n]*)\))?`)

// downloadWorkshop uses the same runner/account policy as game downloads. root is
// the existing source during updates; new subscriptions use the configured root.
func (a *App) downloadWorkshop(ids []string, root string, dryRun bool) (*SubscribeOp, error) {
	if err := a.refuseSubscriptionGameRunning(); err != nil {
		return nil, err
	}
	exe, ok := a.findTool("steamcmd")
	if !ok || exe == "" {
		urls := make([]string, len(ids))
		for i, id := range ids {
			urls[i] = workshopURL(id)
		}
		return nil, fmt.Errorf("install SteamCMD in Settings; Workshop pages: %s", strings.Join(urls, " "))
	}
	account, err := a.SteamCMDAccount("")
	if err != nil {
		return nil, err
	}
	base := a.SteamCMDModsDir()
	relocate := false
	existing := root != ""
	if root == "" {
		root = a.steamCMDContentRoot()
	} else {
		base = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(root))))
		// folder add steamcmd also accepts content roots with an arbitrary name.
		// Download those into staging, then replace the item at its original location.
		if !samePath(root, filepath.Join(base, "steamapps", "workshop", "content", env.WorkshopAppID)) {
			relocate = true
			base = filepath.Join(a.SteamCMDModsDir(), ".archo-tmp-download")
		}
	}
	checkDir := base
	if existing && !relocate {
		// A game's own SteamCMD content source lives beside Data/ and Mods/.
		// The download changes workshop content, not those sibling sources.
		checkDir = root
	}
	if err := a.checkRoots(checkDir); err != nil {
		return nil, err
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	for _, s := range ss {
		if s.Status == source.StatusRetired {
			continue
		}
		if s.Kind == source.SteamCMD && samePath(root, s.Root) {
			continue
		}
		if directoriesOverlap(checkDir, s.Root) {
			return nil, fmt.Errorf("SteamCMD download directory overlaps src:%s (%s)", s.Label, s.Root)
		}
	}
	if directoriesOverlap(checkDir, a.SubscribedDir()) {
		return nil, fmt.Errorf("SteamCMD download directory overlaps subscriptions")
	}
	op := &SubscribeOp{Action: "workshop", DryRun: dryRun}
	for _, id := range ids {
		op.Items = append(op.Items, SubscribeItem{Name: "Workshop " + id, Folder: filepath.Join(root, id), Record: &subscribe.Record{Kind: "steamcmd", WorkshopID: id}, Result: "would download with SteamCMD"})
	}
	if dryRun {
		return op, nil
	}
	if relocate {
		if err := os.MkdirAll(a.SteamCMDModsDir(), 0o755); err != nil {
			return nil, err
		}
		base, err = os.MkdirTemp(a.SteamCMDModsDir(), ".archo-tmp-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(base)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	args := []string{"+force_install_dir", base, "+login", account}
	for _, id := range ids {
		args = append(args, "+workshop_download_item", env.WorkshopAppID, id)
	}
	args = append(args, "+quit")
	out, runErr := a.runWorkshopSteamCMD(exe, args, account != "anonymous")
	okIDs := map[string]bool{}
	for _, m := range workshopSuccess.FindAllStringSubmatch(out, -1) {
		okIDs[m[1]] = true
	}
	failures := map[string]string{}
	for _, m := range workshopFailure.FindAllStringSubmatch(out, -1) {
		failures[m[1]] = m[2]
		delete(okIDs, m[1])
	}
	var errs []error
	if runErr != nil {
		errs = append(errs, runErr)
	}
	changed := false
	for i, id := range ids {
		it := &op.Items[i]
		if !okIDs[id] {
			why := failures[id]
			if why == "" {
				why = "download was not confirmed"
			}
			it.Result = "failed: " + why
			errs = append(errs, fmt.Errorf("SteamCMD item %s: %s; %s", id, why, workshopURL(id)))
			continue
		}
		if relocate {
			downloaded := filepath.Join(base, "steamapps", "workshop", "content", env.WorkshopAppID, id)
			if err := a.replaceWorkshopImport(downloaded, it.Folder); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		if source.AboutState(it.Folder) != source.AboutPresent {
			errs = append(errs, fmt.Errorf("SteamCMD item %s has no About/About.xml at %s", id, it.Folder))
			continue
		}
		now := a.Now()
		it.Record.Installed, it.Record.InstalledAt = now.Format(time.RFC3339), now
		if err := subscribe.Write(it.Folder, it.Record); err != nil {
			errs = append(errs, err)
			continue
		}
		delete(a.subscriptionCache, it.Folder)
		it.To, it.Result = it.Record.Installed, "downloaded with SteamCMD"
		changed = true
	}
	if changed {
		if _, err := a.FolderAdd(source.SteamCMD, root); err != nil {
			return op, errors.Join(append(errs, err)...)
		}
		if _, err := a.Scan(false); err != nil {
			return op, errors.Join(append(errs, err)...)
		}
		ix, err := a.Index()
		if err != nil {
			return op, err
		}
		for i := range op.Items {
			for _, m := range ix.Mods {
				if subscriptionSource(m) && samePath(ix.ModDir(m), op.Items[i].Folder) {
					op.Items[i].ID, op.Items[i].Ref = m.ID, ix.Ref(m.ID)
				}
			}
		}
	}
	return op, errors.Join(errs...)
}

func (a *App) replaceWorkshopImport(from, target string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".archo-tmp-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmp)
		}
	}()
	mod, err := subscribe.Import(from, tmp)
	if err != nil {
		return err
	}
	if err := a.ToTrash(target); err != nil {
		return err
	}
	if err := os.Rename(mod, target); err != nil {
		cleanup = false
		return fmt.Errorf("old folder is in the recycle bin; replacement remains at %s: %w", mod, err)
	}
	return nil
}

func (a *App) runWorkshopSteamCMD(exe string, args []string, console bool) (string, error) {
	if !console {
		return a.steamCMD().run(exe, args)
	}
	log := filepath.Join(filepath.Dir(exe), "logs", "console_log.txt")
	var off int64
	if st, err := os.Stat(log); err == nil {
		off = st.Size()
	}
	p, err := a.steamCMD().start(exe, args, true)
	if err != nil {
		return "", err
	}
	err = p.Wait()
	tail := &logTail{path: log, off: off}
	lines := tail.lines()
	if tail.partial != "" {
		lines = append(lines, tail.partial)
	}
	return strings.Join(lines, "\n"), err
}
