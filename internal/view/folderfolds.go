package view

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// FolderFold is one fold of Settings › Folders (16 §2.14): the rows under one root. Only the
// GUI draws it; the TUI, CLI and MCP list the rows as they are.
type FolderFold struct {
	Key   string `json:"key"`             // its entry in FoldsKey
	Root  string `json:"root,omitempty"`  // the common root; "" when the rows share none
	Title string `json:"title,omitempty"` // the heading when Root is ""
	Size  string `json:"size,omitempty"`  // the recorded sizes of its rows; nothing is walked
	// Disk: the drive, once for the fold, when all its rows are on one; otherwise each row
	// keeps its own meter.
	Disk  *DiskMeter `json:"disk,omitempty"`
	Rows  []int      `json:"rows"`  // indexes into the group's Items
	Count string     `json:"count"` // "5 folders", said folded or not
	// Attention: a row is missing, low on space or warns; such a fold starts open.
	Attention bool   `json:"attention,omitempty"`
	Open      bool   `json:"open"` // the player's choice, else Attention
	Hover     string `json:"hover,omitempty"`
}

// FoldsKey keeps the player's folds: a JSON object of fold key → open (16 §2.14).
const FoldsKey = "ui.gui.settings.folders"

// Fold keys of the fixed roots; any other parent folder is "at-" and a hash of its path.
const (
	foldGame  = "game"
	foldLocal = "local"
	foldData  = "data"
)

// folderFolds groups the shown directories by root: the game and Workshop folders,
// RimArcho's local folder, its data folder, then each other parent folder (a folder the
// player moved or added elsewhere), in that order. items and dirs are in the same order.
func folderFolds(a *app.App, items []SettingItem, dirs []app.ManagedDirectory) []FolderFold {
	saved := map[string]bool{}
	_ = json.Unmarshal([]byte(a.Store.Meta(FoldsKey)), &saved)
	// recorded sizes only (16 §10): the live Mods' scanned sizes per source root, and the
	// source library's accounting
	byRoot := map[string]int64{}
	if sizes, err := a.Store.SourceBytes(); err == nil && len(sizes) > 0 {
		if ss, err := a.Store.AllSources(); err == nil {
			for _, s := range ss {
				byRoot[env.PathKey(s.Root)] += sizes[s.ID]
			}
		}
	}
	type fold struct {
		FolderFold
		paths []string
		bytes int64
	}
	var order []string
	folds := map[string]*fold{}
	local, data := filepath.Clean(a.LocalDir()), filepath.Clean(a.Home)
	for i, d := range dirs {
		key, root := "", ""
		switch {
		case within(d.Path, local):
			key, root = foldLocal, local
		case within(d.Path, data):
			key, root = foldData, data
		case d.Kind == "game" || d.Kind == "workshop":
			key = foldGame
		default:
			root = filepath.Dir(filepath.Clean(d.Path))
			h := fnv.New32a()
			h.Write([]byte(env.PathKey(root)))
			key = fmt.Sprintf("at-%08x", h.Sum32())
		}
		f := folds[key]
		if f == nil {
			f = &fold{FolderFold: FolderFold{Key: key, Root: root}}
			folds[key] = f
			order = append(order, key)
		}
		f.Rows = append(f.Rows, i)
		f.paths = append(f.paths, d.Path)
		if d.ID == "source" {
			f.bytes += a.Store.SourceLibraryBytes()
		} else {
			f.bytes += byRoot[env.PathKey(d.Path)]
		}
	}
	rank := map[string]int{foldGame: 0, foldLocal: 1, foldData: 2}
	var out []FolderFold
	for r := 0; r < 4; r++ {
		for _, k := range order {
			if n, ok := rank[k]; ok && n != r || !ok && r != 3 {
				continue
			}
			f := folds[k]
			if k == foldGame {
				f.Root = commonParent(f.paths)
				if f.Root == "" {
					f.Title = T("game and Workshop")
				}
			}
			var disk *DiskMeter
			one := true
			for _, i := range f.Rows {
				it := &items[i]
				it.Rel = relName(f.Root, it.Value)
				if DirectoryNeedsLocation(dirs[i]) || it.Disk != nil && it.Disk.Low {
					f.Attention = true
				}
				switch {
				case it.Disk == nil:
				case disk == nil:
					disk = it.Disk
				case disk.Total != it.Disk.Total || disk.Free != it.Disk.Free:
					one = false
				}
			}
			if one {
				f.Disk = disk
			}
			if f.bytes > 0 {
				f.Size = SizeText(f.bytes)
				f.Hover = T("what RimArcho has recorded there: the Mods' sizes from the last scan and the generated code")
			}
			f.Count = Tf("%d folders", len(f.Rows))
			if len(f.Rows) == 1 {
				f.Count = T("1 folder")
			}
			f.Open = f.Attention
			if v, ok := saved[k]; ok {
				f.Open = v
			}
			out = append(out, f.FolderFold)
		}
	}
	return out
}

// within: path is root or inside it.
func within(path, root string) bool {
	if root == "" || path == "" {
		return false
	}
	r, err := filepath.Rel(env.PathKey(root), env.PathKey(path))
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// commonParent is the deepest folder holding every path's parent, or "" when that is only a
// drive's root (rows on different drives, or nothing else shared).
func commonParent(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	c := filepath.Dir(filepath.Clean(paths[0]))
	for _, p := range paths[1:] {
		for !within(p, c) {
			up := filepath.Dir(c)
			if up == c {
				return ""
			}
			c = up
		}
	}
	if filepath.Dir(c) == c {
		return ""
	}
	return c
}

// relName is the row's folder under root; the full path when there is no root.
func relName(root, path string) string {
	if root == "" {
		return path
	}
	if r, err := filepath.Rel(root, path); err == nil && r != "." && !strings.HasPrefix(r, "..") {
		return r
	}
	return filepath.Base(path)
}
