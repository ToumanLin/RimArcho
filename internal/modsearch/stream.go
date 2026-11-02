package modsearch

import (
	"context"
	"path/filepath"
)

// Stream visits effective files one Mod at a time. Unlike Search it retains no
// result history and publishes each matching file immediately. Callbacks may stop
// traversal by returning an error. CLI/MCP paging continues to use Search.
func Stream(ctx context.Context, mods []Mod, o Options, found func(Mod, FileResult) error, finished func(Mod), note func(string)) error {
	if o.Kinds == 0 {
		o.Kinds = DefaultKinds
	}
	if o.FS == nil {
		o.FS = OS
	}
	if o.PerFile == 0 {
		o.PerFile = 5000
	}
	matcher, err := newMatcher(o)
	if err != nil {
		return err
	}
	var scratch []byte
	for _, mod := range mods {
		if err := ctx.Err(); err != nil {
			return err
		}
		listed := listFiles(ctx, mod, o)
		if listed.err != nil {
			return listed.err
		}
		if listed.firstError != "" && note != nil {
			note(listed.firstError)
		}
		for _, j := range listed.files {
			if err := ctx.Err(); err != nil {
				return err
			}
			hit := matcher.search(ctx, j, o, &scratch)
			if hit.err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if note != nil {
					note(errText(j.path, hit.err))
				}
				continue
			}
			if len(hit.matches) > 0 {
				if err := found(mod, FileResult{Rel: j.rel, Folder: j.folder, Kind: j.kind, Count: hit.count, Matches: hit.matches}); err != nil {
					return err
				}
			}
		}
		if finished != nil {
			finished(mod)
		}
	}
	return ctx.Err()
}

// TextFile searches one generated source file with the same matcher as Mod files.
func TextFile(ctx context.Context, path string, o Options) ([]Match, error) {
	if o.FS == nil {
		o.FS = OS
	}
	if o.PerFile == 0 {
		o.PerFile = 5000
	}
	m, err := newMatcher(o)
	if err != nil {
		return nil, err
	}
	var scratch []byte
	hit := m.search(ctx, fileJob{path: path, rel: filepath.Base(path), kind: About}, o, &scratch)
	return hit.matches, hit.err
}
