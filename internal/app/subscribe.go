package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

const SubscribedLabel = "subscribed"

func (a *App) SubscribedDir() string {
	if v := a.Store.Meta("cfg:subscribed.dir"); v != "" {
		return v
	}
	return filepath.Join(a.LocalDir(), "subscribed")
}

type Subscription struct {
	Mod    *store.Mod
	Ref    string
	Folder string
	Record *subscribe.Record
}

type SubscribeOp struct {
	Action string
	DryRun bool
	Items  []SubscribeItem
	Notes  []string
}

type SubscribeItem struct {
	ID, Ref, Name, Folder string
	From, To, Result      string
	Record                *subscribe.Record
	UsedBy                []string
}

type subscriptionCacheEntry struct {
	mtime  time.Time
	size   int64
	record *subscribe.Record
}

func subscriptionSource(m *store.Mod) bool {
	return m != nil && m.Status == identity.Live && (m.Kind == source.SteamCMD || m.Kind == source.Extra && m.SourceLabel == SubscribedLabel)
}

func (a *App) subscriptionRecord(m *store.Mod, dir string) (*subscribe.Record, error) {
	if !subscriptionSource(m) {
		return nil, nil
	}
	st, err := os.Stat(filepath.Join(dir, subscribe.File))
	if errors.Is(err, os.ErrNotExist) {
		delete(a.subscriptionCache, dir)
		if m.Kind == source.SteamCMD {
			return &subscribe.Record{Kind: "steamcmd", WorkshopID: filepath.Base(dir)}, nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c, ok := a.subscriptionCache[dir]; ok && c.mtime.Equal(st.ModTime()) && c.size == st.Size() {
		r := *c.record
		return &r, nil
	}
	r, err := subscribe.Read(dir)
	if err != nil {
		return nil, fmt.Errorf("subscription %s: %w", dir, err)
	}
	if a.subscriptionCache == nil {
		a.subscriptionCache = map[string]subscriptionCacheEntry{}
	}
	if len(a.subscriptionCache) >= 256 {
		clear(a.subscriptionCache)
	}
	a.subscriptionCache[dir] = subscriptionCacheEntry{st.ModTime(), st.Size(), r}
	copy := *r
	return &copy, nil
}

// SubscriptionOf reads provenance only for live subscriptions. A damaged record is
// surfaced by Subscriptions; library rows can still be drawn without provenance.
func (a *App) SubscriptionOf(m *store.Mod) *subscribe.Record {
	if !subscriptionSource(m) {
		return nil
	}
	r, _ := a.subscriptionRecord(m, m.RealPath)
	return r
}

func (a *App) Subscriptions() ([]Subscription, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	var out []Subscription
	for _, m := range ix.Mods {
		if !subscriptionSource(m) {
			continue
		}
		dir := ix.ModDir(m)
		r, err := a.subscriptionRecord(m, dir)
		if err != nil {
			return nil, err
		}
		if r != nil {
			out = append(out, Subscription{m, ix.Ref(m.ID), dir, r})
		}
	}
	return out, nil
}

func (a *App) subscribedRootReady() error {
	dir := a.SubscribedDir()
	if err := a.managedRootReady(SubscribedLabel, dir); err != nil {
		return err
	}
	d, err := a.managedDirectory(SubscribedLabel)
	if err != nil {
		return err
	}
	return a.validateDirectory(d, dir)
}

func (a *App) SubscribeGit(url, refType, ref string, dryRun bool) (*SubscribeOp, error) {
	if refType == "" {
		refType = "branch"
	}
	if refType == "commit" {
		ref = strings.ToLower(ref)
	}
	if err := subscribe.Validate(url, refType, ref); err != nil {
		return nil, err
	}
	if _, err := a.Env(); err != nil {
		return nil, err
	}
	if err := a.subscribedRootReady(); err != nil {
		return nil, err
	}
	dir := a.SubscribedDir()
	entries, _, err := source.List(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		r, err := subscribe.Read(e.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("subscription %s: %w", e.Path, err)
		}
		if r.Kind == "git" && subscribe.NormalizeRepo(r.GitURL()) == subscribe.NormalizeRepo(url) && r.RefType == refType && r.Ref == ref {
			return nil, fmt.Errorf("%s (%s %q) is subscribed already at %s", url, refType, ref, e.Path)
		}
	}
	name := subscribe.FolderName(url)
	target, err := subscriptionTarget(dir, name)
	if err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "git", DryRun: dryRun, Items: []SubscribeItem{{Name: name, Folder: target}}}
	if dryRun {
		return op, nil
	}
	if _, err := a.sharedSource(SubscribedLabel, dir); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(dir, ".archo-tmp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	commit, err := subscribe.Clone(context.Background(), url, refType, ref, tmp)
	if err != nil {
		return nil, err
	}
	if source.AboutState(tmp) != source.AboutPresent {
		var mods []string
		if err := filepath.WalkDir(tmp, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if d.Name() == ".git" || d.Name() == ".rimarcho" {
				return filepath.SkipDir
			}
			if p != tmp && source.AboutState(p) == source.AboutPresent {
				rel, _ := filepath.Rel(tmp, p)
				mods = append(mods, rel)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("repository root has no About/About.xml; mod subdirectories: %s", strings.Join(mods, ", "))
	}
	r := &subscribe.Record{Kind: "git", URL: url, RefType: refType, Ref: ref, Installed: commit, InstalledAt: a.Now()}
	if err := subscribe.Write(tmp, r); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(tmp, ".gitmodules")); err == nil {
		op.Notes = append(op.Notes, "Git submodules are not downloaded.")
	}
	if b, _ := os.ReadFile(filepath.Join(tmp, ".gitattributes")); strings.Contains(string(b), "filter=lfs") {
		op.Notes = append(op.Notes, "Git LFS content is not downloaded; LFS files remain pointers.")
	}
	target, err = subscriptionTarget(dir, name)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return nil, err
	}
	op.Items[0].Folder, op.Items[0].To, op.Items[0].Record = target, commit, r
	if _, err := a.Scan(false); err != nil {
		return op, err
	}
	ix, err := a.Index()
	if err != nil {
		return op, err
	}
	for _, m := range ix.Mods {
		if subscriptionSource(m) && samePath(ix.ModDir(m), target) {
			op.Items[0].ID, op.Items[0].Ref, op.Items[0].Name = m.ID, ix.Ref(m.ID), m.DisplayName()
			op.Items[0].Result = "installed"
			return op, nil
		}
	}
	return op, fmt.Errorf("installed at %s but scan did not find a live instance", target)
}

func subscriptionTarget(dir, name string) (string, error) {
	for n := 1; ; n++ {
		base := name
		if n > 1 {
			base = fmt.Sprintf("%s-%d", name, n)
		}
		p := filepath.Join(dir, base)
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (a *App) selectSubscriptions(refs []string) ([]Subscription, error) {
	all, err := a.Subscriptions()
	if err != nil || len(refs) == 0 {
		return all, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	byID := map[string]Subscription{}
	for _, s := range all {
		byID[s.Mod.ID] = s
	}
	var out []Subscription
	seen := map[string]bool{}
	for _, ref := range refs {
		m, err := ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		s, ok := byID[m.ID]
		if !ok {
			return nil, fmt.Errorf("%s is not a subscription", ref)
		}
		if !seen[m.ID] {
			out = append(out, s)
			seen[m.ID] = true
		}
	}
	return out, nil
}

func subscriptionItem(s Subscription) SubscribeItem {
	return SubscribeItem{ID: s.Mod.ID, Ref: s.Ref, Name: s.Mod.DisplayName(), Folder: s.Folder, From: s.Record.Installed, To: s.Record.Latest, Record: s.Record}
}

func (a *App) CheckSubscriptions(refs []string) (*SubscribeOp, error) {
	ss, err := a.selectSubscriptions(refs)
	if err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "check"}
	if len(refs) > 0 {
		for _, s := range ss {
			if s.Record.Kind != "git" && s.Record.Kind != "page" {
				return nil, fmt.Errorf("%s: update check unavailable offline for %s", s.Ref, s.Record.Kind)
			}
		}
	}
	var failures []error
	for _, s := range ss {
		it := subscriptionItem(s)
		r := s.Record
		if r.Kind == "git" || r.Kind == "page" {
			var commit string
			var checkErr error
			if r.Kind == "git" {
				commit, checkErr = subscribe.RemoteCommit(context.Background(), r.GitURL(), r.RefType, r.Ref)
			} else {
				var page *subscribe.PageInfo
				page, checkErr = a.readSubscriptionPage(r.URL)
				if checkErr == nil {
					commit = page.Stamp()
				}
			}
			r.CheckedAt, r.CheckError = a.Now(), ""
			if checkErr != nil {
				r.CheckError = checkErr.Error()
				if r.Kind == "git" {
					failures = append(failures, fmt.Errorf("%s: %w", s.Ref, checkErr))
				}
			} else {
				r.Latest = commit
			}
			if err := subscribe.Write(s.Folder, r); err != nil {
				return op, err
			}
			delete(a.subscriptionCache, s.Folder)
			it.To, it.Result = r.Latest, "up to date"
			if r.HasUpdate() {
				it.Result = "update available"
			}
			if checkErr != nil {
				it.Result = "check failed: " + checkErr.Error()
			}
		} else {
			it.Result = "skipped: update check unavailable offline for " + r.Kind
		}
		op.Items = append(op.Items, it)
	}
	return op, errors.Join(failures...)
}

func (a *App) UpdateSubscriptions(refs []string, dryRun bool) (*SubscribeOp, error) {
	ss, err := a.selectSubscriptions(refs)
	if err != nil {
		return nil, err
	}
	if run, err := a.GameRunning(); err != nil {
		return nil, err
	} else if run != nil {
		return nil, fmt.Errorf("the game is running (pid %d); close it before updating subscriptions", run.PID)
	}
	op := &SubscribeOp{Action: "update", DryRun: dryRun}
	for _, s := range ss {
		it := subscriptionItem(s)
		if s.Record.Kind != "git" && s.Record.Kind != "steamcmd" {
			if len(refs) > 0 {
				return nil, fmt.Errorf("%s: updating %s requires --from <zip|folder>", s.Ref, s.Record.Kind)
			}
			it.Result = "skipped: update requires --from <zip|folder>"
			op.Items = append(op.Items, it)
			continue
		}
		if dryRun && s.Record.Kind == "git" {
			it.To, err = subscribe.RemoteCommit(context.Background(), s.Record.GitURL(), s.Record.RefType, s.Record.Ref)
			if err != nil {
				return op, err
			}
		}
		if s.Record.Kind == "steamcmd" {
			it.Result = "would download Workshop " + s.Record.WorkshopID + " with SteamCMD"
		}
		op.Items = append(op.Items, it)
	}
	if dryRun {
		return op, nil
	}
	// Rescan successful updates even if a later repository fails.
	changed := false
	for i, s := range ss {
		if s.Record.Kind != "git" && s.Record.Kind != "steamcmd" {
			continue
		}
		if s.Record.Kind == "steamcmd" {
			result, downloadErr := a.downloadWorkshop([]string{s.Record.WorkshopID}, filepath.Dir(s.Folder), false)
			if result != nil && len(result.Items) > 0 {
				op.Items[i].To, op.Items[i].Result = result.Items[0].To, result.Items[0].Result
				op.Items[i].Record = result.Items[0].Record
			}
			if downloadErr != nil {
				if changed {
					_, err = a.Scan(false)
				}
				return op, errors.Join(downloadErr, err)
			}
			continue
		}
		commit, updateErr := subscribe.Update(context.Background(), s.Folder, s.Record)
		if updateErr != nil {
			if changed {
				_, err = a.Scan(false)
			}
			return op, errors.Join(updateErr, err)
		}
		changed = true
		r := s.Record
		r.Installed, r.Latest, r.InstalledAt, r.CheckedAt, r.CheckError = commit, commit, a.Now(), a.Now(), ""
		op.Items[i].To, op.Items[i].Result = commit, "updated"
		if err := subscribe.Write(s.Folder, r); err != nil {
			_, scanErr := a.Scan(false)
			return op, errors.Join(err, scanErr)
		}
		delete(a.subscriptionCache, s.Folder)
	}
	if changed {
		_, err = a.Scan(false)
	}
	return op, err
}

func (a *App) RemoveSubscriptions(refs []string, dryRun bool) (*SubscribeOp, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("give at least one subscription reference")
	}
	ss, err := a.selectSubscriptions(refs)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "remove", DryRun: dryRun}
	for _, s := range ss {
		it := subscriptionItem(s)
		if it.UsedBy, err = a.usedBy(st, s.Mod.ID); err != nil {
			return nil, err
		}
		if !samePath(filepath.Dir(s.Folder), a.SubscribedDir()) && s.Mod.Kind != source.SteamCMD {
			return nil, fmt.Errorf("%s is not directly inside a subscription source", s.Ref)
		}
		op.Items = append(op.Items, it)
	}
	if dryRun {
		return op, nil
	}
	for i := range op.Items {
		if err := a.ToTrash(op.Items[i].Folder); err != nil {
			_, scanErr := a.Scan(false)
			return op, errors.Join(err, scanErr)
		}
		op.Items[i].Result = "moved to recycle bin"
		delete(a.subscriptionCache, op.Items[i].Folder)
	}
	_, err = a.Scan(false)
	return op, err
}
