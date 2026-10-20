package scan

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/ToumanLin/RimArcho/internal/textenc"
)

// Dependency is one <modDependencies> entry.
type Dependency struct {
	PackageID        string `json:"package_id"`
	DisplayName      string `json:"display_name,omitempty"`
	SteamWorkshopURL string `json:"steam_workshop_url,omitempty"`
	DownloadURL      string `json:"download_url,omitempty"`
}

// Decl is the normalised declaration of a mod: About.xml, PublishedFileId.txt and
// LoadFolders.xml (02 §4.2). It is stored as snapshot.decl_blob for the analysis layer.
type Decl struct {
	PackageIDRaw      string   `json:"package_id_raw,omitempty"`
	PackageID         string   `json:"package_id,omitempty"` // lower case
	PackageIDDerived  bool     `json:"package_id_derived,omitempty"`
	Name              string   `json:"name,omitempty"`
	Authors           []string `json:"authors,omitempty"`
	ModVersion        string   `json:"mod_version,omitempty"`
	SupportedVersions []string `json:"supported_versions,omitempty"`

	Dependencies          []Dependency            `json:"dependencies,omitempty"`
	DependenciesByVersion map[string][]Dependency `json:"dependencies_by_version,omitempty"`
	LoadBefore            []string                `json:"load_before,omitempty"`
	LoadAfter             []string                `json:"load_after,omitempty"`
	ForceLoadBefore       []string                `json:"force_load_before,omitempty"`
	ForceLoadAfter        []string                `json:"force_load_after,omitempty"`
	IncompatibleWith      []string                `json:"incompatible_with,omitempty"`
	LoadBeforeByVersion   map[string][]string     `json:"load_before_by_version,omitempty"`
	LoadAfterByVersion    map[string][]string     `json:"load_after_by_version,omitempty"`
	IncompatibleByVersion map[string][]string     `json:"incompatible_with_by_version,omitempty"`
	DescriptionsByVersion map[string]string       `json:"descriptions_by_version,omitempty"`

	PublishedFileID string `json:"published_file_id,omitempty"`
	LoadFolders     string `json:"load_folders,omitempty"` // LoadFolders.xml verbatim
	ParseError      string `json:"parse_error,omitempty"`
}

// node is a minimal XML element tree; names are lower-cased for tolerant lookup.
type node struct {
	name     string
	text     strings.Builder
	children []*node
}

func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *node) str(name string) string {
	c := n.child(name)
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.text.String())
}

// list returns the trimmed text of every child element (normally <li>).
func (n *node) list(name string) []string {
	c := n.child(name)
	if c == nil {
		return nil
	}
	var out []string
	for _, li := range c.children {
		if s := strings.TrimSpace(li.text.String()); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseXML(b []byte) (*node, error) {
	b = textenc.UTF8(b)
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	root := &node{}
	stack := []*node{root}
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(root.children) > 0 {
				return root.children[0], err
			}
			return nil, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			n := &node{name: strings.ToLower(t.Name.Local)}
			top := stack[len(stack)-1]
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			stack[len(stack)-1].text.Write(t)
		}
	}
	if len(root.children) == 0 {
		return &node{}, nil
	}
	return root.children[0], nil
}

// ReadDecl reads a mod folder's declarations. official marks Data/ sources, whose missing
// packageId falls back to ludeon.rimworld.<folder>. Errors are recorded in ParseError, never
// returned: a broken About.xml still yields an Instance.
func ReadDecl(dir string, official bool) Decl {
	var d Decl
	aboutPath := findCI(dir, "About", "About.xml")
	if _, err := os.Stat(aboutPath); err != nil {
		if p := findCI(dir, "About", "About.xml.disabled"); p != "" {
			if _, err := os.Stat(p); err == nil {
				aboutPath = p // RimCrow-disabled: the declaration is still valid (02 §4.1)
			}
		}
	}
	b, err := os.ReadFile(aboutPath)
	var root *node
	if err == nil {
		root, err = parseXML(b)
	}
	if err != nil {
		d.ParseError = err.Error()
	}
	var description, author string
	if root != nil {
		d.PackageIDRaw = root.str("packageid")
		d.Name = root.str("name")
		author = root.str("author")
		d.Authors = root.list("authors")
		d.ModVersion = root.str("modversion")
		d.SupportedVersions = root.list("supportedversions")
		description = root.str("description")
		d.Dependencies = deps(root.child("moddependencies"))
		if bv := root.child("moddependenciesbyversion"); bv != nil {
			d.DependenciesByVersion = map[string][]Dependency{}
			for _, v := range bv.children {
				d.DependenciesByVersion[v.name] = deps(v)
			}
		}
		d.LoadBefore = root.list("loadbefore")
		d.LoadAfter = root.list("loadafter")
		d.ForceLoadBefore = root.list("forceloadbefore")
		d.ForceLoadAfter = root.list("forceloadafter")
		d.IncompatibleWith = root.list("incompatiblewith")
		d.LoadBeforeByVersion = byVersion(root.child("loadbeforebyversion"))
		d.LoadAfterByVersion = byVersion(root.child("loadafterbyversion"))
		d.IncompatibleByVersion = byVersion(root.child("incompatiblewithbyversion"))
		if dv := root.child("descriptionsbyversion"); dv != nil {
			d.DescriptionsByVersion = map[string]string{}
			for _, v := range dv.children {
				d.DescriptionsByVersion[v.name] = strings.TrimSpace(v.text.String())
			}
		}
	}
	if len(d.Authors) == 0 && author != "" {
		d.Authors = splitAuthors(author)
	}
	d.PackageID = strings.ToLower(d.PackageIDRaw)
	if d.PackageID == "" {
		folder := filepath.Base(dir)
		if official {
			d.PackageID = "ludeon.rimworld." + strings.ToLower(folder)
			if strings.EqualFold(folder, "Core") {
				d.PackageID = "ludeon.rimworld"
			}
		} else {
			if author == "" {
				author = "Anonymous" // ModMetaDataInternal.author default
			}
			d.PackageID = strings.ToLower(DerivePackageID(author, d.Name, description))
		}
		d.PackageIDDerived = true
	}
	if b, err := os.ReadFile(findCI(dir, "About", "PublishedFileId.txt")); err == nil {
		d.PublishedFileID = strings.TrimSpace(strings.TrimPrefix(string(b), string([]byte{0xEF, 0xBB, 0xBF})))
	}
	if b, err := os.ReadFile(findCI(dir, "LoadFolders.xml")); err == nil {
		d.LoadFolders = string(b)
	}
	return d
}

func deps(n *node) []Dependency {
	if n == nil {
		return nil
	}
	var out []Dependency
	for _, li := range n.children {
		dep := Dependency{
			PackageID:        li.str("packageid"),
			DisplayName:      li.str("displayname"),
			SteamWorkshopURL: li.str("steamworkshopurl"),
			DownloadURL:      li.str("downloadurl"),
		}
		if dep.PackageID == "" && len(li.children) == 0 {
			dep.PackageID = strings.TrimSpace(li.text.String())
		}
		if dep.PackageID != "" {
			out = append(out, dep)
		}
	}
	return out
}

func byVersion(n *node) map[string][]string {
	if n == nil {
		return nil
	}
	m := map[string][]string{}
	for _, v := range n.children {
		var l []string
		for _, li := range v.children {
			if s := strings.TrimSpace(li.text.String()); s != "" {
				l = append(l, s)
			}
		}
		m[v.name] = l
	}
	return m
}

// splitAuthors mirrors ModMetaData.Authors: split <author> on " and ", "&" and ",".
func splitAuthors(s string) []string {
	s = strings.ReplaceAll(s, " and ", ",")
	s = strings.ReplaceAll(s, "&", ",")
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// DerivePackageID reproduces ModMetaDataInternal.TryParsePackageId for mods without
// <packageId> (game 1.7.4982): ConvertToASCII(author + hash3(description)) + "." +
// ConvertToASCII(name). The result is marked as inferred.
func DerivePackageID(author, name, description string) string {
	text := "none"
	if description != "" {
		h := strings.ReplaceAll(strconv.Itoa(int(stableStringHash(description))), "-", "")
		text = h[:min(3, len(h))]
	}
	return convertToASCII(author+text) + "." + convertToASCII(name)
}

// stableStringHash is Verse.GenText.StableStringHash over UTF-16 code units with int32 wrap.
func stableStringHash(s string) int32 {
	n := int32(23)
	for _, c := range utf16.Encode([]rune(s)) {
		n = n*31 + int32(c)
	}
	return n
}

func convertToASCII(s string) string {
	var sb strings.Builder
	for _, c := range utf16.Encode([]rune(s)) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			c = c%25 + 65
		}
		sb.WriteByte(byte(c))
	}
	return sb.String()
}

// findCI joins parts under dir, matching each component case-insensitively when the exact
// spelling does not exist. Returns the exact join if nothing matches.
func findCI(dir string, parts ...string) string {
	exact := filepath.Join(append([]string{dir}, parts...)...)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}
	cur := dir
	for _, p := range parts {
		des, err := os.ReadDir(cur)
		if err != nil {
			return exact
		}
		found := ""
		for _, d := range des {
			if strings.EqualFold(d.Name(), p) {
				found = d.Name()
				break
			}
		}
		if found == "" {
			return exact
		}
		cur = filepath.Join(cur, found)
	}
	return cur
}

// AllDeps returns every dependency package id (base and by-version), lower-cased and sorted.
func (d *Decl) AllDeps() []string {
	set := map[string]bool{}
	for _, x := range d.Dependencies {
		set[strings.ToLower(x.PackageID)] = true
	}
	for _, l := range d.DependenciesByVersion {
		for _, x := range l {
			set[strings.ToLower(x.PackageID)] = true
		}
	}
	return sortedKeys(set)
}

// LoadRulesKey is a canonical string of all ordering/incompatibility rules, for change detection.
func (d *Decl) LoadRulesKey() string {
	var sb strings.Builder
	w := func(tag string, l []string) {
		ll := make([]string, len(l))
		for i, s := range l {
			ll[i] = strings.ToLower(s)
		}
		sort.Strings(ll)
		sb.WriteString(tag + "=" + strings.Join(ll, ",") + ";")
	}
	wm := func(tag string, m map[string][]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			w(tag+"@"+k, m[k])
		}
	}
	w("lb", d.LoadBefore)
	w("la", d.LoadAfter)
	w("flb", d.ForceLoadBefore)
	w("fla", d.ForceLoadAfter)
	w("inc", d.IncompatibleWith)
	wm("lb", d.LoadBeforeByVersion)
	wm("la", d.LoadAfterByVersion)
	wm("inc", d.IncompatibleByVersion)
	return sb.String()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
