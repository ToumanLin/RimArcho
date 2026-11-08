package gametest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// resolvedPath also resolves existing parents of a directory not yet created.
func resolvedPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	real, err = resolvedPath(parent)
	return filepath.Join(real, filepath.Base(path)), err
}

func within(root, path string) bool {
	rel, err := filepath.Rel(env.PathKey(root), env.PathKey(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func preparePaths(o *Options) error {
	for _, p := range []*string{&o.GameRoot, &o.DataDir, &o.RunDir} {
		if *p == "" {
			return fmt.Errorf("gametest: GameRoot, DataDir and RunDir must be set")
		}
		real, err := resolvedPath(*p)
		if err != nil {
			return fmt.Errorf("gametest: resolve %q: %w", *p, err)
		}
		*p = real
	}
	st, err := os.Stat(o.GameRoot)
	if err != nil {
		return fmt.Errorf("gametest: game root %q: %w", o.GameRoot, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("gametest: game root %q is not a directory", o.GameRoot)
	}
	for _, dir := range []string{o.DataDir, o.RunDir} {
		if within(o.GameRoot, dir) {
			return fmt.Errorf("gametest: directory %q must be outside game root %q", dir, o.GameRoot)
		}
	}
	for _, item := range []struct{ dir, name string }{
		{o.DataDir, "Config"}, {o.DataDir, filepath.Join("Config", "Knowledge.xml")}, {o.DataDir, "Saves"},
		{o.RunDir, "Player.log"}, {o.RunDir, "stdout.log"}, {o.RunDir, "result.json"},
	} {
		path := filepath.Join(item.dir, item.name)
		real, err := resolvedPath(path)
		if err != nil {
			return fmt.Errorf("gametest: resolve %q: %w", path, err)
		}
		if !within(item.dir, real) || within(o.GameRoot, real) {
			return fmt.Errorf("gametest: path %q resolves outside isolated directory %q", path, item.dir)
		}
	}
	return nil
}

func executable(o Options) (string, bool) {
	_, dllErr := os.Stat(filepath.Join(o.GameRoot, "Waylith.dll"))
	mm := env.MajorMinor(o.GameVersion)
	waylith := dllErr == nil || mm != "" && !env.VersionLess(mm, "1.7")
	first, second := "RimWorldWin64.exe", "RimWorld.exe"
	if waylith {
		first, second = second, first
	}
	path := filepath.Join(o.GameRoot, first)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fallback := filepath.Join(o.GameRoot, second)
		if st, err := os.Stat(fallback); err == nil && !st.IsDir() {
			path = fallback
		}
	}
	return path, waylith
}
