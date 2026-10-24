package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func (a *App) readSubscriptionPage(url string) (*subscribe.PageInfo, error) {
	return subscribe.ReadPage(context.Background(), a.HTTP, url)
}

func (a *App) SubscribePage(pageURL string, useGit bool, from string, dryRun bool) (*SubscribeOp, error) {
	if useGit && from != "" {
		return nil, fmt.Errorf("choose --git or --from, not both")
	}
	p, err := a.readSubscriptionPage(pageURL)
	if err != nil {
		return nil, err
	}
	if useGit {
		repo := p.Main
		if repo == "" && len(p.Repos) == 1 {
			repo = p.Repos[0]
		}
		if repo == "" {
			return nil, fmt.Errorf("cannot tell which linked repository is the mod (%d: %s); subscribe to it with archo subscribe git <repository>", len(p.Repos), strings.Join(p.Repos, ", "))
		}
		if a.pageRepo != nil {
			repo = a.pageRepo(repo)
		}
		op, err := a.SubscribeGit(repo, "branch", "", dryRun)
		if err != nil || dryRun {
			return op, err
		}
		r := op.Items[0].Record
		r.URL, r.Repo = pageURL, repo
		if err := subscribe.Write(op.Items[0].Folder, r); err != nil {
			return op, err
		}
		delete(a.subscriptionCache, op.Items[0].Folder)
		return op, nil
	}
	op := &SubscribeOp{Action: "page", DryRun: dryRun || from == "", Notes: []string{"Page: " + pageURL, "Version: " + p.Stamp()}}
	for _, repo := range p.Repos {
		op.Notes = append(op.Notes, "Repository: "+repo)
	}
	if from == "" {
		op.Items = []SubscribeItem{{Name: p.Name, To: p.Stamp()}}
		op.Notes = append(op.Notes, "Use --git to subscribe to a linked repository, or download in your browser and use --from <zip|folder>.")
		return op, nil
	}
	if _, err := a.Env(); err != nil {
		return nil, err
	}
	if err := a.subscribedRootReady(); err != nil {
		return nil, err
	}
	ss, err := a.Subscriptions()
	if err != nil {
		return nil, err
	}
	for _, s := range ss {
		if s.Record.Kind == "page" && s.Record.URL == pageURL {
			return nil, fmt.Errorf("page is subscribed already: %s", s.Ref)
		}
	}
	name := subscribe.FolderName(strings.ReplaceAll(p.Name, "/", "-"))
	target, err := subscriptionTarget(a.SubscribedDir(), name)
	if err != nil {
		return nil, err
	}
	op.Items = []SubscribeItem{{Name: p.Name, Folder: target, To: p.Stamp()}}
	if dryRun {
		return op, nil
	}
	if _, err := a.sharedSource(SubscribedLabel, a.SubscribedDir()); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(a.SubscribedDir(), ".archo-tmp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	mod, err := subscribe.Import(from, tmp)
	if err != nil {
		return nil, err
	}
	r := &subscribe.Record{Kind: "page", URL: pageURL, Installed: p.Stamp(), InstalledAt: a.Now()}
	if err := subscribe.Write(mod, r); err != nil {
		return nil, err
	}
	if err := os.Rename(mod, target); err != nil {
		return nil, err
	}
	op.Items[0].Record = r
	if err := a.scanSubscriptionItem(&op.Items[0]); err != nil {
		return op, err
	}
	op.Items[0].Result = "installed"
	return op, nil
}

func (a *App) scanSubscriptionItem(it *SubscribeItem) error {
	if _, err := a.Scan(false); err != nil {
		return err
	}
	ix, err := a.Index()
	if err != nil {
		return err
	}
	for _, m := range ix.Mods {
		if subscriptionSource(m) && samePath(ix.ModDir(m), it.Folder) {
			it.ID, it.Ref, it.Name = m.ID, ix.Ref(m.ID), m.DisplayName()
			return nil
		}
	}
	return fmt.Errorf("scan did not find subscription at %s", it.Folder)
}

func (a *App) refuseSubscriptionGameRunning() error {
	if run, err := a.GameRunning(); err != nil {
		return err
	} else if run != nil {
		return fmt.Errorf("the game is running (pid %d); close it before updating subscriptions", run.PID)
	}
	return nil
}

func (a *App) UpdateSubscriptionFrom(ref, from string, dryRun bool) (*SubscribeOp, error) {
	ss, err := a.selectSubscriptions([]string{ref})
	if err != nil {
		return nil, err
	}
	s := ss[0]
	if s.Record.Kind != "page" {
		return nil, fmt.Errorf("%s is not a page subscription; --from is only for page subscriptions", ref)
	}
	if err := a.refuseSubscriptionGameRunning(); err != nil {
		return nil, err
	}
	if err := a.subscribedRootReady(); err != nil {
		return nil, err
	}
	r := s.Record
	stamp := r.Latest
	if stamp == "" {
		p, err := a.readSubscriptionPage(r.URL)
		if err != nil {
			return nil, err
		}
		stamp = p.Stamp()
	}
	op := &SubscribeOp{Action: "update", DryRun: dryRun, Items: []SubscribeItem{subscriptionItem(s)}}
	op.Items[0].To = stamp
	op.Items[0].Links, err = a.PlanIncomingLinks(s.Folder, s.Folder)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return op, nil
	}
	tmp, err := os.MkdirTemp(a.SubscribedDir(), ".archo-tmp-")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmp)
		}
	}()
	mod, err := subscribe.Import(from, tmp)
	if err != nil {
		return nil, err
	}
	r.Installed, r.Latest, r.InstalledAt, r.CheckError = stamp, stamp, a.Now(), ""
	if err := subscribe.Write(mod, r); err != nil {
		return nil, err
	}
	if err := a.withIncomingLinks(s.Folder, s.Folder, func() error {
		if err := a.recycleTarget(s.Folder); err != nil {
			return err
		}
		if err := os.Rename(mod, s.Folder); err != nil {
			cleanup = false
			return fmt.Errorf("old folder is in the recycle bin; replacement remains at %s: %w", mod, err)
		}
		return a.scanSubscriptionItem(&op.Items[0])
	}); err != nil {
		return op, err
	}
	delete(a.subscriptionCache, s.Folder)
	op.Items[0].Result = "updated"
	return op, nil
}
