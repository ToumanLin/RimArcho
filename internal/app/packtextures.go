package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/textures"
)

// A pack's textures (08 §5.2): after the copy, each mod's PNGs are converted to DDS in the
// pack and the PNGs deleted — preconvert at full size, resize when scaled. The copy is the pack's
// own, so the game version is known and nothing needs recording: what could not be
// converted keeps its PNG.

// packTextureEstimate is what converting one mod's textures would take, as Measure sees it
// before anything is copied: the mod's folders without the version folders left out.
func packTextureEstimate(root string, trimmed []string, o textures.Options, gen map[string]bool) (textures.Estimate, int) {
	var folders []string
	for _, f := range textures.AllFolders(root) {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		out := false
		for _, t := range trimmed {
			if strings.EqualFold(top, t) {
				out = true
			}
		}
		if !out {
			folders = append(folders, f)
		}
	}
	set, err := textures.Collect(root, folders)
	if err != nil {
		return textures.Estimate{}, 0
	}
	// RimArcho's DDS are not copied: their keys are converted like any other
	ours := func(p string) bool {
		rel, err := filepath.Rel(root, p)
		return err == nil && gen[strings.ToLower(filepath.ToSlash(rel))]
	}
	p := textures.MakePlan(set, o, textures.Owned{Ours: ours}, false)
	return p.Est, p.Skipped[textures.SkipOwnDDS] + p.Skipped[textures.SkipOtherFmt]
}

// genSize is the size of the files RimArcho wrote into a mod folder (not copied to packs).
func genSize(root string, gen map[string]bool) int64 {
	var n int64
	for rel := range gen {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			n += fi.Size()
		}
	}
	return n
}

// packTextures converts the textures of every copy in the pack being assembled in tmp.
func (a *App) packTextures(r *PackBuild, tmp string, enc textures.Encoder, bs *buildState) error {
	o := *r.TexOptions
	pt := r.Pack.Textures
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { // the stop file (twice a second)
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if bs.stopRequested() != nil {
					cancel()
					return
				}
			}
		}
	}()
	done := 0
	for i := range r.Pack.Entries {
		pe := &r.Pack.Entries[i]
		if pe.Official || pe.Folder == "" {
			continue
		}
		bs.update(func(p *BuildProgress) { p.Mod = pe.Name })
		dst := filepath.Join(tmp, "mods", pe.Folder)
		set, err := textures.Collect(dst, textures.AllFolders(dst))
		if err != nil {
			return err
		}
		plan := textures.MakePlan(set, o, textures.Owned{}, false)
		pt.Kept += plan.Skipped[textures.SkipOwnDDS] + plan.Skipped[textures.SkipOtherFmt]
		base := done
		res, err := textures.Run(ctx, enc, plan.Jobs, o, nil, nil, func(x textures.Progress) {
			bs.update(func(p *BuildProgress) { p.Files = base + x.Done })
		})
		if ctx.Err() != nil {
			return errStopped
		}
		if err != nil {
			return err
		}
		done += len(plan.Jobs)
		// A key that failed anywhere keeps its PNGs everywhere and loses the DDS written for
		// it (a DDS shadows the PNG of the same name in every load folder).
		failed := map[string]bool{}
		for _, x := range res {
			if !x.OK {
				failed[x.Key] = true
			}
		}
		for _, x := range res {
			switch {
			case failed[x.Key]:
				if x.OK {
					os.Remove(x.DDS)
				} else {
					pt.Failed++
				}
				pt.Kept++
			default:
				if err := os.Remove(x.Path); err != nil {
					return err
				}
				pt.Converted++
				pt.PNGBytes += x.Size
				pt.DDSBytes += x.DDSSize
				pe.Bytes += x.DDSSize - x.Size
			}
		}
		bs.update(func(p *BuildProgress) { p.Mods++ })
	}
	return nil
}
