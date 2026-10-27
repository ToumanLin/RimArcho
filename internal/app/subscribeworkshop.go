package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
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

func (a *App) subscribeWorkshop(inputs []string, forceSteamCMD, dryRun bool, w *workshopTransfer) (*SubscribeOp, error) {
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
		if w != nil && w.ctx.Err() != nil {
			return op, w.ctx.Err()
		}
		report := &steamworks.Report{}
		var err error
		for start := 0; start < len(ids); start += 50 {
			if w != nil && w.ctx.Err() != nil {
				break
			}
			part := ids[start:min(start+50, len(ids))]
			if w != nil {
				w.line("Downloading item " + part[0])
			}
			var got *steamworks.Report
			got, err = a.runSteam(steamworks.Subscribe, part)
			if err != nil {
				break
			}
			report.Items = append(report.Items, got.Items...)
			if w != nil {
				for _, it := range got.Items {
					if it.Result == "ok" {
						w.line("Success. Downloaded item " + strconv.FormatUint(it.ID, 10))
					}
				}
			}
		}
		if len(report.Items) > 0 {
			err = nil
		}
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
				if strings.HasPrefix(result, "failed:") {
					code, _ := strconv.Atoi(strings.TrimPrefix(result, "failed:"))
					op.Items[i].Result = "failed: " + WorkshopResultText(code)
					if code == 9 {
						op.Items[i].FailureKind = "missing"
					} else if code == 15 || code == 24 || code == 26 || code == 41 {
						op.Items[i].FailureKind = "permission"
					}
					op.Items[i].Permanent = workshopPermanent(op.Items[i].FailureKind)
				}
			}
			op.Notes = append(op.Notes, "Steam downloads it; it appears after the next scan.")
			return op, errors.Join(failures...)
		}
		if !errors.Is(err, ErrSteamUnavailable) {
			return nil, err
		}
		if w != nil && w.ctx.Err() != nil {
			return op, w.ctx.Err()
		}
	}
	account, err := a.SteamCMDAccount("")
	if err != nil {
		return nil, err
	}
	return a.downloadWorkshopGroup(ids, "", account, false, true, w)
}

// downloadWorkshop uses the same runner/account policy as game downloads. root is
// the existing source during updates; new subscriptions use the configured root.
func (a *App) downloadWorkshop(ids []string, root string, dryRun bool) (result *SubscribeOp, downloadErr error) {
	account, err := a.SteamCMDAccount("")
	if err != nil {
		return nil, err
	}
	return a.downloadWorkshopGroup(ids, root, account, dryRun, true)
}

func (a *App) downloadWorkshopGroup(ids []string, root, account string, dryRun, refresh bool, transfer ...*workshopTransfer) (result *SubscribeOp, downloadErr error) {
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
	var err error
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
	registrationRoot, activeSource := root, false
	for _, s := range ss {
		if s.Kind == source.SteamCMD && samePath(root, s.Root) {
			registrationRoot = s.Root
			activeSource = activeSource || s.Status != source.StatusRetired
			continue
		}
		if s.Status == source.StatusRetired {
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
	var incoming *LinkLifecyclePlan
	if existing {
		incoming, err = a.PlanIncomingLinks(root, root)
		if err != nil {
			return nil, err
		}
		for i := range op.Items {
			op.Items[i].Links = incoming
		}
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
	if incoming != nil && !relocate {
		if err := a.beginLinkLifecycle(incoming); err != nil {
			for i := range op.Items {
				op.Items[i].Result = "failed: " + err.Error()
			}
			return op, err
		}
	}
	finished := false
	defer func() {
		if incoming != nil && !relocate && !finished {
			downloadErr = a.abortLinkLifecycle(incoming, downloadErr)
		}
	}()
	outcomes, runErr := a.runWorkshopBatches(exe, base, account, ids, transfer...)
	var errs []error
	if runErr != nil {
		errs = append(errs, runErr)
	}
	changed := false
	for i, id := range ids {
		it := &op.Items[i]
		if outcome := outcomes[id]; !outcome.confirmed {
			why := outcome.why()
			it.Result = "failed: " + why
			it.FailureKind, it.Permanent = outcome.kind, workshopPermanent(outcome.kind)
			errs = append(errs, fmt.Errorf("SteamCMD item %s: %s; %s", id, why, workshopURL(id)))
			continue
		}
		if relocate {
			downloaded := filepath.Join(base, "steamapps", "workshop", "content", env.WorkshopAppID, id)
			if err := a.replaceWorkshopImport(downloaded, it.Folder); err != nil {
				it.Result = "failed: " + err.Error()
				errs = append(errs, err)
				continue
			}
		}
		if source.AboutState(it.Folder) != source.AboutPresent {
			it.Result = "failed: no About/About.xml"
			errs = append(errs, fmt.Errorf("SteamCMD item %s has no About/About.xml at %s", id, it.Folder))
			continue
		}
		now := a.Now()
		it.Record.Installed, it.Record.InstalledAt = now.Format(time.RFC3339), now
		if err := subscribe.Write(it.Folder, it.Record); err != nil {
			it.Result = "failed: " + err.Error()
			errs = append(errs, err)
			continue
		}
		delete(a.subscriptionCache, it.Folder)
		delete(a.subscriptionCache, realOrAbs(it.Folder))
		delete(a.subscriptionCache, filepath.Join(registrationRoot, id))
		it.To, it.Result = it.Record.Installed, "downloaded with SteamCMD"
		changed = true
	}
	if changed {
		if !activeSource {
			if _, err := a.FolderAdd(source.SteamCMD, registrationRoot); err != nil {
				return op, errors.Join(append(errs, err)...)
			}
		}
		if refresh {
			if err := a.refreshWorkshop(op); err != nil {
				return op, errors.Join(append(errs, err)...)
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return op, err
	}
	if incoming != nil && !relocate {
		finished = true
		if err := a.finishLinkLifecycle(incoming); err != nil {
			return op, err
		}
	}
	return op, nil
}

func (a *App) refreshWorkshop(op *SubscribeOp) error {
	if _, err := a.Scan(false); err != nil {
		return err
	}
	ix, err := a.Index()
	if err != nil {
		return err
	}
	for i := range op.Items {
		for _, m := range ix.Mods {
			if subscriptionSource(m) && samePath(ix.ModDir(m), op.Items[i].Folder) {
				op.Items[i].ID, op.Items[i].Ref = m.ID, ix.Ref(m.ID)
			}
		}
	}
	return nil
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
	return a.withIncomingLinks(target, target, func() error {
		if err := a.recycleTarget(target); err != nil {
			return err
		}
		if err := os.Rename(mod, target); err != nil {
			cleanup = false
			return fmt.Errorf("old folder is in the recycle bin; replacement remains at %s: %w", mod, err)
		}
		return nil
	})
}

func (a *App) runWorkshopSteamCMD(exe string, args []string, console bool, transfer ...*workshopTransfer) (string, error) {
	unlock := accounts.Lock(exe)
	defer unlock()
	w := workshopObserver(transfer)
	if w.ctx.Err() != nil {
		return "", w.ctx.Err()
	}
	log := filepath.Join(filepath.Dir(exe), "logs", "console_log.txt")
	var off int64
	if st, err := os.Stat(log); err == nil {
		off = st.Size()
	}
	// Tests may supply a synchronous runner. Production always tails its own log
	// offset under the same account lock, for hidden and interactive processes alike.
	if !console && a.SteamCMDRun != nil {
		out, err := a.SteamCMDRun(exe, args)
		for _, line := range strings.Split(out, "\n") {
			w.line(line)
		}
		return out, err
	}
	start := startWorkshopCMD
	if a.SteamCMDStart != nil {
		start = a.SteamCMDStart
	}
	p, err := start(exe, args, console)
	if err != nil {
		return "", err
	}
	tail := &logTail{path: log, off: off}
	var lines []string
	read := func() {
		for _, line := range tail.lines() {
			lines = append(lines, line)
			w.line(line)
		}
	}
	wait := make(chan error, 1)
	go func() { wait <- p.Wait() }()
	poll := a.SteamCMDPoll
	if poll <= 0 {
		poll = 200 * time.Millisecond
	}
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case err = <-wait:
			read()
			goto finished
		case <-tick.C:
			read()
		case <-w.ctx.Done():
			if killer, ok := p.(interface{ KillTree() error }); ok {
				killer.KillTree()
			} else if killer, ok := p.(interface{ Kill() error }); ok {
				killer.Kill()
			}
			<-wait
			err = w.ctx.Err()
			read()
			goto finished
		}
	}
finished:
	if tail.partial != "" {
		lines = append(lines, tail.partial)
		w.line(tail.partial)
	}
	return strings.Join(lines, "\n"), err
}
