package app

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
			if w != nil && !w.wait {
				w.line("Downloading item " + part[0])
			}
			var got *steamworks.Report
			got, err = a.runSteam(steamworks.Subscribe, part)
			if err != nil {
				break
			}
			report.Items = append(report.Items, got.Items...)
			if w != nil && !w.wait {
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
					why := result
					if code, err := strconv.Atoi(strings.TrimPrefix(result, "failed:")); err == nil {
						why = WorkshopResultText(code)
					}
					failures = append(failures, fmt.Errorf("Workshop %s: Steam did not confirm subscription (%s); %s", id, why, workshopURL(id)))
				}
				op.Items[i].Result = result
				if strings.HasPrefix(result, "failed:") {
					code, _ := strconv.Atoi(strings.TrimPrefix(result, "failed:"))
					op.Items[i].Result = "failed: " + WorkshopResultText(code)
					if code == 9 {
						op.Items[i].FailureKind = "missing"
					} else if code == 3 {
						op.Items[i].FailureKind = "offline"
					} else if code == 15 || code == 24 || code == 26 || code == 41 {
						op.Items[i].FailureKind = "permission"
					}
					op.Items[i].Permanent = workshopPermanent(op.Items[i].FailureKind)
				}
			}
			if w != nil && w.wait {
				return op, errors.Join(append(failures, a.waitInstalled(op, w)...)...)
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
	login, err := a.workshopLogin("")
	if err != nil {
		return nil, err
	}
	return a.downloadWorkshopGroup(ids, "", login, false, true, w)
}

// downloadWorkshop uses the same runner/account policy as game downloads. root is
// the existing source during updates; new subscriptions use the configured root.
func (a *App) downloadWorkshop(ids []string, root string, dryRun bool) (result *SubscribeOp, downloadErr error) {
	login, err := a.workshopLogin("")
	if err != nil {
		return nil, err
	}
	return a.downloadWorkshopGroup(ids, root, login, dryRun, true)
}

// workshopAccount is how a Workshop download through SteamCMD signs in (11 §3).
type workshopAccount struct {
	account string // the resolved account; "anonymous" when none
	// anonymousFirst: the account is the saved one. The download runs anonymously and
	// signs in only for the items anonymous download is denied.
	anonymousFirst bool
}

// workshopLogin resolves a Workshop download's account like SteamCMDAccount. An explicit
// account (anonymous included) is used as it is; a saved one signs in only for the items
// that anonymous download is denied.
func (a *App) workshopLogin(override string) (workshopAccount, error) {
	account, err := a.SteamCMDAccount(override)
	if err != nil {
		return workshopAccount{}, err
	}
	return workshopAccount{account, strings.TrimSpace(override) == "" && account != "anonymous"}, nil
}

// workshopNeedsAccount: anonymous download was refused an item for want of an account
// (access denied, not owned), so the saved account may get it. File Not Found (removed),
// disk and network failures never sign in.
func workshopNeedsAccount(o workshopOutcome) bool {
	return !o.confirmed && o.err == nil && (o.kind == "permission" || o.kind == "ownership")
}

// workshopDownload is one downloadWorkshopGroup run: where SteamCMD downloads, where the
// items land and how they are registered, and what has happened so far.
type workshopDownload struct {
	a                *App
	ids              []string
	op               *SubscribeOp
	root             string // the content root the items land in
	base             string // SteamCMD's force_install_dir
	relocate         bool   // download into staging, then replace the item at its original location
	existing         bool   // an existing source is updated
	checkDir         string
	registrationRoot string
	activeSource     bool // the content root is a registered, active source
	errs             []error
	changed          bool
	settled          map[string]bool
}

func (a *App) downloadWorkshopGroup(ids []string, root string, login workshopAccount, dryRun, refresh bool, transfer ...*workshopTransfer) (result *SubscribeOp, downloadErr error) {
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
	d := &workshopDownload{a: a, ids: ids, root: root}
	d.resolveDirs()
	if err := a.checkRoots(d.checkDir); err != nil {
		return nil, err
	}
	if err := d.checkSources(); err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "workshop", DryRun: dryRun}
	d.op = op
	for _, id := range ids {
		op.Items = append(op.Items, SubscribeItem{Name: "Workshop " + id, Folder: filepath.Join(d.root, id), Record: &subscribe.Record{Kind: "steamcmd", WorkshopID: id}, Result: "would download with SteamCMD"})
	}
	if a.SteamCMDSignsOutSteam(login.account) {
		note := SteamSignOutNote
		if login.anonymousFirst {
			note = WorkshopSignOutNote // only if an item needs the account
		}
		op.Notes = append(op.Notes, fmt.Sprintf(note, login.account))
	}
	var incoming *LinkLifecyclePlan
	if d.existing {
		incoming, err = a.PlanIncomingLinks(d.root, d.root)
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
	if d.relocate {
		if err := os.MkdirAll(a.SteamCMDModsDir(), 0o755); err != nil {
			return nil, err
		}
		d.base, err = os.MkdirTemp(a.SteamCMDModsDir(), ".archo-tmp-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(d.base)
	}
	if err := os.MkdirAll(d.base, 0o755); err != nil {
		return nil, err
	}
	lifecycle := incoming != nil && !d.relocate
	if lifecycle {
		if err := a.beginLinkLifecycle(incoming); err != nil {
			for i := range op.Items {
				op.Items[i].Result = "failed: " + err.Error()
			}
			return op, err
		}
	}
	finished := false
	defer func() {
		if lifecycle && !finished {
			downloadErr = a.abortLinkLifecycle(incoming, downloadErr)
		}
	}()
	d.run(exe, login, transfer)
	if err := d.register(refresh); err != nil {
		return op, err
	}
	if err := errors.Join(d.errs...); err != nil {
		return op, err
	}
	if lifecycle {
		finished = true
		if err := a.finishLinkLifecycle(incoming); err != nil {
			return op, err
		}
	}
	return op, nil
}

// resolveDirs works out where SteamCMD downloads (base), the content root and the folder
// whose overlaps are checked.
func (d *workshopDownload) resolveDirs() {
	a := d.a
	d.base = a.SteamCMDModsDir()
	d.existing = d.root != ""
	if d.root == "" {
		d.root = a.steamCMDContentRoot()
	} else {
		d.base = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(d.root))))
		// folder add steamcmd also accepts content roots with an arbitrary name.
		// Download those into staging, then replace the item at its original location.
		if !samePath(d.root, filepath.Join(d.base, "steamapps", "workshop", "content", env.WorkshopAppID)) {
			d.relocate = true
			d.base = filepath.Join(a.SteamCMDModsDir(), ".archo-tmp-download")
		}
	}
	d.checkDir = d.base
	if d.existing && !d.relocate {
		// A game's own SteamCMD content source lives beside Data/ and Mods/.
		// The download changes workshop content, not those sibling sources.
		d.checkDir = d.root
	}
}

// checkSources finds the source the content root is registered as, and refuses a download
// directory that overlaps another source or the subscriptions.
func (d *workshopDownload) checkSources() error {
	ss, err := d.a.Store.AllSources()
	if err != nil {
		return err
	}
	d.registrationRoot = d.root
	for _, s := range ss {
		if s.Kind == source.SteamCMD && samePath(d.root, s.Root) {
			d.registrationRoot = s.Root
			d.activeSource = d.activeSource || s.Status != source.StatusRetired
			continue
		}
		if s.Status == source.StatusRetired {
			continue
		}
		if directoriesOverlap(d.checkDir, s.Root) {
			return fmt.Errorf("SteamCMD download directory overlaps src:%s (%s)", s.Label, s.Root)
		}
	}
	if directoriesOverlap(d.checkDir, d.a.SubscribedDir()) {
		return fmt.Errorf("SteamCMD download directory overlaps subscriptions")
	}
	return nil
}

// run downloads the items with SteamCMD and settles each: as soon as SteamCMD confirms
// it, or with its outcome once the run ends. With a saved account the items download
// anonymously first; only those anonymous download is denied are retried signed in, in
// one more run (11 §3).
func (d *workshopDownload) run(exe string, login workshopAccount, transfer []*workshopTransfer) {
	w := workshopObserver(transfer)
	d.settled = map[string]bool{}
	index := make(map[string]int, len(d.ids))
	for i, id := range d.ids {
		index[id] = i
	}
	before := w.confirmed
	w.confirmed = func(id string) {
		if before != nil {
			before(id)
		}
		if i, ok := index[id]; ok && !d.settled[id] {
			d.settle(w, i, workshopOutcome{confirmed: true})
		}
	}
	first := login.account
	if login.anonymousFirst {
		first = "anonymous"
	}
	outcomes, runErr := d.a.runWorkshopBatches(exe, d.base, first, d.ids, w)
	if runErr != nil {
		d.errs = append(d.errs, runErr)
	}
	if first == "anonymous" && w.ctx.Err() == nil {
		d.signInFor(exe, login, outcomes, w)
	}
	w.confirmed = before
	for i, id := range d.ids {
		if !d.settled[id] {
			d.settle(w, i, outcomes[id])
		}
	}
}

// signInFor retries, signed in, the items anonymous download was denied, or says they need
// an account when none is saved. outcomes takes the retry's outcomes.
func (d *workshopDownload) signInFor(exe string, login workshopAccount, outcomes map[string]workshopOutcome, w *workshopTransfer) {
	var denied []string
	for _, id := range d.ids {
		if workshopNeedsAccount(outcomes[id]) && !d.settled[id] && !slices.Contains(denied, id) {
			denied = append(denied, id)
		}
	}
	if len(denied) == 0 {
		return
	}
	if !login.anonymousFirst {
		for _, id := range denied {
			outcomes[id] = workshopOutcome{kind: "account"}
			w.outcome(id, outcomes[id])
		}
		return
	}
	log.Printf("workshop: anonymous download was denied %d items; retrying them signed in: %s", len(denied), strings.Join(denied, ", "))
	d.op.Notes = append(d.op.Notes, fmt.Sprintf(WorkshopAccountNote, len(denied), login.account, strings.Join(denied, ", ")))
	if d.a.SteamCMDSignsOutSteam(login.account) {
		d.op.Notes = append(d.op.Notes, fmt.Sprintf(SteamSignOutNote, login.account))
	}
	w.again(denied)
	retried, err := d.a.runWorkshopBatches(exe, d.base, login.account, denied, w)
	if err != nil {
		d.errs = append(d.errs, err)
	}
	for _, id := range denied {
		outcomes[id] = retried[id]
	}
}

// settle finishes one item: its record is written as soon as SteamCMD confirms it, so a
// long run reports each Mod when it is done instead of all of them at the end.
func (d *workshopDownload) settle(w *workshopTransfer, i int, outcome workshopOutcome) {
	a := d.a
	id, it := d.ids[i], &d.op.Items[i]
	d.settled[id] = true
	defer func() {
		if w.settled != nil {
			w.settled(*it)
		}
	}()
	if !outcome.confirmed {
		why := outcome.why()
		it.Result = "failed: " + why
		it.FailureKind, it.Permanent = outcome.kind, workshopPermanent(outcome.kind)
		d.errs = append(d.errs, fmt.Errorf("SteamCMD item %s: %s; %s", id, why, workshopURL(id)))
		return
	}
	if d.relocate {
		downloaded := filepath.Join(d.base, "steamapps", "workshop", "content", env.WorkshopAppID, id)
		if err := a.replaceWorkshopImport(downloaded, it.Folder); err != nil {
			it.Result = "failed: " + err.Error()
			d.errs = append(d.errs, err)
			return
		}
	}
	if source.AboutState(it.Folder) != source.AboutPresent {
		it.Result = "failed: no About/About.xml"
		d.errs = append(d.errs, fmt.Errorf("SteamCMD item %s has no About/About.xml at %s", id, it.Folder))
		return
	}
	now := a.Now()
	it.Record.Installed, it.Record.InstalledAt = now.Format(time.RFC3339), now
	if err := subscribe.Write(it.Folder, it.Record); err != nil {
		it.Result = "failed: " + err.Error()
		d.errs = append(d.errs, err)
		return
	}
	delete(a.subscriptionCache, it.Folder)
	delete(a.subscriptionCache, realOrAbs(it.Folder))
	delete(a.subscriptionCache, filepath.Join(d.registrationRoot, id))
	it.To, it.Result = it.Record.Installed, "downloaded with SteamCMD"
	d.changed = true
}

// register adds the content root as a source when it is not one yet and refreshes the
// Workshop view, once something changed. An error comes joined with the item failures.
func (d *workshopDownload) register(refresh bool) error {
	if !d.changed {
		return nil
	}
	if !d.activeSource {
		if _, err := d.a.FolderAdd(source.SteamCMD, d.registrationRoot); err != nil {
			return errors.Join(append(d.errs, err)...)
		}
	}
	if refresh {
		if err := d.a.refreshWorkshop(d.op); err != nil {
			return errors.Join(append(d.errs, err)...)
		}
	}
	return nil
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
	log := steamCMDConsoleLog(exe)
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
	// console: a signed-in run, which may ask for the password (subscribeworkshoprun.go)
	end := a.steamSignIn(console)
	defer end()
	shared := console && a.SteamCMDShareConsole && a.SteamCMDStart == nil
	if shared {
		start = startWorkshopShared // the CLI: SteamCMD in its own terminal
	}
	p, err := start(exe, args, console)
	if err != nil {
		return "", err
	}
	if console {
		defer a.watchSteamPrompts(p, shared)()
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
