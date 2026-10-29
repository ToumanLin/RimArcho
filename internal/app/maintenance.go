package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Maintain is shared by GUI idle maintenance and explicit compact on all surfaces.
// Callers supply cancellation when foreground work starts; no page App is borrowed.
func (a *App) Maintain(ctx context.Context, automatic, dryRun bool) (store.MaintenanceResult, error) {
	var empty store.MaintenanceResult
	if err := a.maintenanceNoGame(); err != nil {
		return empty, err
	}
	free := a.FreeSpace
	if free == nil {
		free = DiskFree
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	return a.Store.Maintain(ctx, store.MaintenanceOptions{
		Now: a.Now(), Automatic: automatic, DryRun: dryRun,
		FreeSpace: func(path string) int64 { return free(filepath.Dir(path)) },
	})
}

// MaintenanceGamesIdle probes captured roots without accessing the database.
func (a *App) MaintenanceGamesIdle(envs []*env.Environment) bool {
	probe := a.probe()
	for _, e := range envs {
		if probe(e.GameRoot) != nil {
			return false
		}
	}
	return true
}

func (a *App) maintenanceNoGame() error {
	envs, err := a.Store.Envs()
	if err != nil {
		return err
	}
	probe := a.probe()
	for _, e := range envs {
		if r := probe(e.GameRoot); r != nil {
			return fmt.Errorf("cannot compact database: game %s is running (pid %d); close the game first", e.Name, r.PID)
		}
	}
	return nil
}
