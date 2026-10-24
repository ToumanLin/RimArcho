package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TargetState string

const (
	TargetPresent TargetState = "present"
	TargetMissing TargetState = "missing"
	TargetUnknown TargetState = "unknown"
)

// LinkInfo retains the entry and parent identities for a later checked removal.
// RawTarget is read without following the link, even when its destination is gone.
type LinkInfo struct {
	Path           string      `json:"path"`
	Kind           string      `json:"kind"`
	RawTarget      string      `json:"target"`
	Target         string      `json:"resolved_target"`
	State          TargetState `json:"state"`
	Reason         string      `json:"reason,omitempty"`
	entry          os.FileInfo
	parent         os.FileInfo
	physicalParent string
}

func ReadLink(path string) (*LinkInfo, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	l := &LinkInfo{Path: p, Kind: "unknown", State: TargetUnknown}
	unknown := func(err error) (*LinkInfo, error) {
		l.Reason = err.Error()
		return l, nil
	}
	l.physicalParent, err = RealPath(filepath.Dir(p))
	if err != nil {
		return unknown(err)
	}
	l.parent, err = os.Stat(l.physicalParent)
	if err != nil {
		return unknown(err)
	}
	l.entry, err = os.Lstat(p)
	if err != nil {
		return unknown(err)
	}
	// Windows FileInfo may load its file ID lazily. Freeze both IDs now, while
	// taking the preview, so a later SameFile cannot inspect the replacement.
	if !os.SameFile(l.parent, l.parent) || !os.SameFile(l.entry, l.entry) {
		return unknown(fmt.Errorf("cannot read link or parent identity: %s", p))
	}
	l.Kind, l.RawTarget, err = rawLink(p)
	if err != nil {
		l.Kind = "unknown"
		l.Reason = err.Error()
		return l, nil
	}
	if l.Kind != "junction" && l.Kind != "symlink" {
		return l, nil
	}
	l.Target = l.RawTarget
	if !filepath.IsAbs(l.Target) {
		l.Target = filepath.Join(filepath.Dir(p), l.Target)
	}
	l.State, l.Reason = ProbeTarget(l.Target)
	return l, nil
}

// ProbeTarget only calls a target missing if its volume and closest surviving
// ancestor can be read. An offline drive or denied access is not deletion evidence.
func ProbeTarget(path string) (TargetState, string) {
	return probeTarget(path, map[string]bool{})
}

func probeTarget(path string, seen map[string]bool) (TargetState, string) {
	path = filepath.Clean(path)
	if seen[path] || len(seen) >= 64 {
		return TargetUnknown, "target link cycle: " + path
	}
	seen[path] = true
	if _, err := os.Stat(path); err == nil {
		return TargetPresent, ""
	} else if !os.IsNotExist(err) {
		return TargetUnknown, err.Error()
	}
	// A surviving ancestor can itself point into an offline volume. Resolve that
	// raw link before using its local parent as evidence that a target was deleted.
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		if IsLink(ancestor) {
			kind, target, err := rawLink(ancestor)
			if err != nil {
				return TargetUnknown, err.Error()
			}
			if kind != "junction" && kind != "symlink" {
				return TargetUnknown, "unknown target reparse type: " + ancestor
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(ancestor), target)
			}
			suffix, err := filepath.Rel(ancestor, path)
			if err != nil {
				return TargetUnknown, err.Error()
			}
			return probeTarget(filepath.Join(target, suffix), seen)
		}
		if ancestor != path {
			if _, err := os.ReadDir(ancestor); err == nil {
				return TargetMissing, "target does not exist; ancestor is accessible: " + ancestor
			} else if !os.IsNotExist(err) {
				return TargetUnknown, err.Error()
			}
		}
		if filepath.Dir(ancestor) == ancestor {
			return TargetUnknown, "target volume is unavailable: " + ancestor
		}
	}
}

func (l *LinkInfo) verify() error {
	if l == nil || l.entry == nil || (l.Kind != "junction" && l.Kind != "symlink") {
		return fmt.Errorf("refusing to remove an unverified link")
	}
	n, err := ReadLink(l.Path)
	if err != nil {
		return err
	}
	if n.Kind != l.Kind || n.RawTarget != l.RawTarget || n.State != l.State || n.physicalParent != l.physicalParent || !os.SameFile(n.parent, l.parent) || !os.SameFile(n.entry, l.entry) {
		return fmt.Errorf("link or parent changed since preview: %s", l.Path)
	}
	return nil
}

// RemoveLinkChecked removes exactly the inspected entry, never its destination.
func RemoveLinkChecked(l *LinkInfo) error { return removeLinkChecked(l) }

func VerifyLink(l *LinkInfo) error { return l.verify() }

func SameLink(a, b *LinkInfo) bool {
	return a != nil && b != nil && a.parent != nil && b.parent != nil && a.entry != nil && b.entry != nil && a.Kind == b.Kind && a.RawTarget == b.RawTarget && a.physicalParent == b.physicalParent && os.SameFile(a.parent, b.parent) && os.SameFile(a.entry, b.entry)
}

// CheckModsLinks is the low-level test runner's read-only preflight. Application
// callers use InspectGameLinks for the full ownership and game-level report.
func CheckModsLinks(mods string) ([]*LinkInfo, error) {
	entries, err := os.ReadDir(mods)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := []*LinkInfo{}
	for _, de := range entries {
		name := strings.ToLower(de.Name())
		if !strings.HasPrefix(name, "_archo_") && !strings.HasPrefix(name, "!archopack_") {
			continue
		}
		l, err := ReadLink(filepath.Join(mods, de.Name()))
		if err != nil {
			return nil, err
		}
		if l.Kind != "plain" && l.State != TargetPresent {
			out = append(out, l)
		}
	}
	return out, nil
}
