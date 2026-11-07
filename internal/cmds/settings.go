package cmds

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/render"
)

// The database and the caches (16 §2.14): what the settings page shows, as commands.
func init() {
	// where packs are kept (10); "" is <data>/packs
	freeKeys["pack.dir"] = struct {
		def   string
		check func(string) error
	}{"", func(v string) error {
		if v == "" || filepath.IsAbs(v) {
			return nil
		}
		return fmt.Errorf("pack.dir must be an absolute folder path")
	}}
	register(&Command{Name: "db status", Summary: "the database: path, size, backups", MaxArgs: 0, Run: dbStatusCmd})
	register(&Command{Name: "db backup", Summary: "write a consistent copy of the database to <data>/backups", MaxArgs: 0, Run: dbBackupCmd})
	register(&Command{Name: "db vacuum", Summary: "clean old file lists and compact the database", Flags: []Flag{{Name: "dry-run", Help: "preview old file cleanup without changing the database"}}, MaxArgs: 0, Run: dbVacuumCmd})
	register(&Command{Name: "cache status", Summary: "the caches and their sizes", MaxArgs: 0, Run: cacheStatusCmd})
	register(&Command{Name: "cache clear", Args: "<kind>", Summary: "empty one cache (previews, workshop, images, textures, logs); rebuilt on demand", MinArgs: 1, MaxArgs: 1, Run: cacheClearCmd})
}

func mb(n int64) string { return render.Size(n) }

func dbStatusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	s, err := c.App.DBInfo()
	if err != nil {
		return nil, err
	}
	d, root := render.New("database " + mb(s.Bytes))
	root.KV("path", s.Path)
	n := root.KV("backups", fmt.Sprint(len(s.Backups)))
	for _, b := range s.Backups {
		n.Add(b)
	}
	return d, nil
}

func dbBackupCmd(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := c.App.DBBackup()
	if err != nil {
		return nil, err
	}
	d, _ := render.New("backup written to " + p)
	return d, nil
}

func dbVacuumCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("dry-run") {
		r, err := c.App.Maintain(context.Background(), false, true)
		if err != nil {
			return nil, err
		}
		d, _ := render.New(fmt.Sprintf("database maintenance: would remove %d old file records and compact the database", r.Files))
		return d, nil
	}
	before, after, err := c.App.DBVacuum()
	if err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("database compacted: freed %s (now %s)", mb(max(0, before-after)), mb(after)))
	return d, nil
}

func cacheStatusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	d, root := render.New("caches")
	var total int64
	for _, k := range c.App.Caches() {
		root.KV(k.Kind, fmt.Sprintf("%s, %d items (%s)", mb(k.Bytes), k.Files, k.Desc))
		total += k.Bytes
	}
	root.KV("total", mb(total))
	return d, nil
}

func cacheClearCmd(c *Ctx, a *Args) (*render.Doc, error) {
	k, err := c.App.ClearCache(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("cleared %s: %s, %d items", k.Kind, mb(k.Bytes), k.Files))
	return d, nil
}
