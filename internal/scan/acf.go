package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// ACFItem is the Steam client's record of one workshop item.
type ACFItem struct {
	Manifest    string
	TimeUpdated int64
	Size        int64
	Subscribed  bool
}

// ACF is the parsed appworkshop_294100.acf of one workshop content root.
type ACF struct {
	Path  string
	Items map[string]*ACFItem
}

// ACFPathFor returns the ACF file belonging to a content root
// (…/steamapps/workshop/content/294100 → …/steamapps/workshop/appworkshop_294100.acf).
func ACFPathFor(contentRoot string) string {
	app := filepath.Base(filepath.Clean(contentRoot))
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Clean(contentRoot))), "appworkshop_"+app+".acf")
}

// ReadACF loads the workshop manifest; it returns nil if the file is absent or unreadable.
// An item counts as subscribed when it is listed in WorkshopItemsInstalled or
// WorkshopItemDetails; the Steam client drops unsubscribed items from both lists while the
// folder may stay behind as residue.
func ReadACF(contentRoot string) *ACF {
	a, _ := readACF(contentRoot)
	return a
}

// errACF reports a manifest that exists but has no AppWorkshop section (Steam may be rewriting it).
var errACF = errors.New("workshop manifest has no AppWorkshop section")

// readACF is ReadACF that tells an absent manifest (nil, nil) from an unreadable one (nil, error).
func readACF(contentRoot string) (*ACF, error) {
	p := ACFPathFor(contentRoot)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n, err := vdf.Parse(b)
	if err != nil {
		return nil, err
	}
	root := n.Get("AppWorkshop")
	if root == nil {
		return nil, errACF
	}
	a := &ACF{Path: p, Items: map[string]*ACFItem{}}
	item := func(id string) *ACFItem {
		it := a.Items[id]
		if it == nil {
			it = &ACFItem{}
			a.Items[id] = it
		}
		return it
	}
	for _, c := range kids(root.Get("WorkshopItemsInstalled")) {
		it := item(c.Key)
		it.Subscribed = true
		it.Manifest = c.Str("manifest")
		it.TimeUpdated, _ = strconv.ParseInt(c.Str("timeupdated"), 10, 64)
		it.Size, _ = strconv.ParseInt(c.Str("size"), 10, 64)
	}
	for _, c := range kids(root.Get("WorkshopItemDetails")) {
		it := item(c.Key)
		it.Subscribed = true
		if it.Manifest == "" {
			it.Manifest = c.Str("manifest")
		}
		if it.TimeUpdated == 0 {
			it.TimeUpdated, _ = strconv.ParseInt(c.Str("timeupdated"), 10, 64)
		}
	}
	return a, nil
}

func kids(n *vdf.Node) []*vdf.Node {
	if n == nil {
		return nil
	}
	return n.Children
}
