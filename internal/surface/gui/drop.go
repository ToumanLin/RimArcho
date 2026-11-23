//go:build windows || cgo

package gui

import "github.com/ToumanLin/RimArcho/internal/subscribe"

// DropFolderKind says what a dropped folder is (16 §2.18 "Dropping files"): "mod" when it holds
// one Mod (About/About.xml at its root or inside one top-level folder: it goes to add a
// subscription, to be installed as a local Mod), else "folder" (another manager's data).
func (s *API) DropFolderKind(path string) string {
	if subscribe.Probe(path) == nil {
		return "mod"
	}
	return "folder"
}

// dropData is what "files-dropped" carries: the paths, which of them are folders, and a link
// (Windows only; WebKitGTK hands over files, not links).
type dropData struct {
	Paths []string `json:"paths"`
	Dirs  []bool   `json:"dirs"`
	Text  string   `json:"text,omitempty"`
}
