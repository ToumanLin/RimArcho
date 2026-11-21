package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// InstallLocalMod copies a Mod folder or extracts a Mod zip into the game's Mods folder, as a
// local Mod with no subscription (16 §2.9: a dropped Mod folder or zip). The folder
// is named after the source (a free name: "Name-2" when "Name" is taken); nothing is
// replaced. The source must have About/About.xml at its root or inside one top-level
// folder. A dry run says where it would go.
func (a *App) InstallLocalMod(from string, dryRun bool) (*SubscribeOp, error) {
	from = strings.Trim(strings.TrimSpace(from), `"'`)
	if from == "" {
		return nil, fmt.Errorf("choose a Mod folder or zip")
	}
	if err := subscribe.Probe(from); err != nil {
		return nil, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	dir := modsDir(ix)
	if dir == "" {
		return nil, fmt.Errorf("the Game Config has no Mods folder")
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("the Mods folder is not there: %s", dir)
	}
	var name string
	if strings.EqualFold(filepath.Ext(from), ".zip") {
		name = subscribe.FolderName(strings.TrimSuffix(filepath.Base(from), filepath.Ext(from)))
	} else {
		name = subscribe.FolderName(filepath.Base(filepath.Clean(from)))
	}
	if name == "" {
		name = "LocalMod"
	}
	target, err := subscriptionTarget(dir, name)
	if err != nil {
		return nil, err
	}
	op := &SubscribeOp{Action: "install local", DryRun: dryRun, Items: []SubscribeItem{{Name: name, Folder: target, From: from}},
		Notes: []string{"A local Mod: no subscription, no update checks."}}
	if dryRun {
		return op, nil
	}
	tmp, err := os.MkdirTemp(dir, ".archo-tmp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	mod, err := subscribe.Import(from, tmp)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(mod, target); err != nil {
		return nil, err
	}
	if _, err := a.Scan(false); err != nil {
		return op, err
	}
	if ix, err := a.Index(); err == nil {
		for _, m := range ix.Mods {
			if m.TombstoneAt.IsZero() && samePath(ix.ModDir(m), target) {
				op.Items[0].ID, op.Items[0].Ref, op.Items[0].Name = m.ID, ix.Ref(m.ID), m.DisplayName()
			}
		}
	}
	op.Items[0].Result = "installed"
	return op, nil
}
