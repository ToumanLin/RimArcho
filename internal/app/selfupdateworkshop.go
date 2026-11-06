package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const UpdateWorkshopItemEnv = "RIMARCHO_UPDATE_WORKSHOP_ITEM"

func updateWorkshopItem() string {
	if id, set := os.LookupEnv(UpdateWorkshopItemEnv); set {
		return strings.TrimSpace(id)
	}
	return selfupdate.WorkshopItem
}

func (a *App) workshopSelfRelease(ctx context.Context) (*selfupdate.Release, error) {
	id := updateWorkshopItem()
	if valid, err := workshopIDs([]string{id}); err != nil || len(valid) != 1 || valid[0] != id {
		return nil, errors.New("Workshop update item is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	v, title, notes := "", "", ""
	details, err := a.postWorkshopDetails(ctx, []string{id})
	switch d := details[id]; {
	case err != nil:
	case d == nil || d.Result != 1:
		err = errors.New("Workshop update item is unavailable")
	default:
		v, title, notes = selfupdate.WorkshopVersion(d.Tags), d.Title, d.Description
		if v == "" {
			err = errors.New("Workshop update item has no stable version tag")
		}
	}
	// The anonymous Web API answers 9 for an unlisted item; a subscriber's folder still
	// names the version it holds.
	local := ""
	if v == "" {
		for _, root := range env.WorkshopFolders(a.Detect) {
			if v = selfupdate.SubscribedVersion(filepath.Join(root, id), runtime.GOOS, runtime.GOARCH); v != "" {
				local, err = filepath.Join(root, id), nil
				break
			}
		}
	}
	if v == "" {
		return nil, err
	}
	r := &selfupdate.Release{Version: v, Tag: "v" + v, Name: title, Notes: notes,
		Page: workshopURL(id), WorkshopItem: id}
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		r.Zip.Name = selfupdate.ZipName(v, "windows", "amd64")
		r.Sum.Name = r.Zip.Name + ".sha256"
		if st, err := os.Stat(filepath.Join(local, r.Zip.Name)); local != "" && err == nil {
			r.Zip.Size = st.Size() // shown only; the checksum verifies the copy
		}
	}
	return r, nil
}

func (a *App) selfWorkshopBase() string {
	return filepath.Join(a.LocalDir(), "steamcmd-self")
}

func (a *App) cleanSelfWorkshop() {
	base := a.selfWorkshopBase()
	unlock := accounts.Lock(base)
	defer unlock()
	_ = os.RemoveAll(base)
}

func (a *App) downloadSelfWorkshop(ctx context.Context, r *selfupdate.Release, dir string) (string, error) {
	for _, root := range env.WorkshopFolders(a.Detect) {
		if zip, err := selfupdate.CopyLocal(ctx, r, filepath.Join(root, r.WorkshopItem), dir); err == nil {
			return zip, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	exe, ok := a.findTool("steamcmd")
	if !ok || exe == "" {
		return "", errors.New("SteamCMD is not installed")
	}
	account, err := a.SteamCMDAccount("anonymous")
	if err != nil {
		return "", err
	}
	base := a.selfWorkshopBase()
	unlock := accounts.Lock(base)
	defer unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	// runWorkshopSteamCMD holds the same instance lock used by Mod downloads.
	outcomes, err := a.runWorkshopBatches(exe, base, account, []string{r.WorkshopItem}, &workshopTransfer{ctx: ctx})
	if err != nil {
		return "", err
	}
	if o := outcomes[r.WorkshopItem]; !o.confirmed {
		return "", fmt.Errorf("SteamCMD item %s: %s", r.WorkshopItem, o.why())
	}
	return selfupdate.CopyLocal(ctx, r, filepath.Join(base, "steamapps", "workshop", "content", env.WorkshopAppID, r.WorkshopItem), dir)
}

func (a *App) downloadSelfUpdate(ctx context.Context, r *selfupdate.Release, dir string, step func(SelfUpdateStep)) (string, error) {
	if r.WorkshopItem != "" {
		step(SelfUpdateStep{Phase: "download"})
		zip, err := a.downloadSelfWorkshop(ctx, r, dir)
		if err == nil {
			return zip, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Resolve the checked version, even if GitHub now has a newer release.
		fallback, lookupErr := a.selfClient().Version(ctx, r.Version)
		if lookupErr != nil {
			return "", fmt.Errorf("Workshop: %w; GitHub fallback: %w", err, lookupErr)
		}
		r = fallback
	}
	step(SelfUpdateStep{Phase: "download", Total: r.Zip.Size})
	return a.selfClient().Download(ctx, r, dir, func(done, total int64) {
		step(SelfUpdateStep{Phase: "download", Done: done, Total: total})
	})
}
