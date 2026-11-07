package menubg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type expansion struct {
	Name  string `xml:"defName"`
	Label string `xml:"label"`
	Mod   string `xml:"linkedMod"`
	Path  string `xml:"backgroundPath"`
}

// Item uses the old official package key so saved choices and stars keep working
// when a Preview.png is replaced by the actual menu background (16 §2.14).
type Item struct {
	Key, Label, Full, Thumb string
}

type entry struct {
	def  expansion
	file *zip.File
}

func catalogDefs(game string, defs []expansion, files []*zip.File) []entry {
	var out []entry
	for _, d := range defs {
		prefix := "resources/textures/" + strings.ToLower(strings.Trim(d.Path, "/")) + "."
		var matches []*zip.File
		var candidates []string
		for _, f := range files {
			name := strings.ToLower(f.Name)
			if strings.HasPrefix(name, prefix) {
				candidates = append(candidates, f.Name)
				if strings.HasSuffix(name, ".dds") || strings.HasSuffix(name, ".dds.split") {
					matches = append(matches, f)
				}
			}
		}
		if len(matches) != 1 {
			// Do not guess between raw and split entries for one resource.
			log.Printf("menu backgrounds: %s in %s: expected one .dds or BC7 .dds.split entry for %s, found %d supported candidates (%s); using About/Preview.png if available", d.Name, game, d.Path, len(matches), strings.Join(candidates, ", "))
			continue
		}
		out = append(out, entry{d, matches[0]})
	}
	return out
}

func expansions(game, lang string) ([]expansion, error) {
	b, err := os.ReadFile(defsPath(game))
	if err != nil {
		return nil, err
	}
	var defs struct {
		Expansions []expansion `xml:"ExpansionDef"`
	}
	if err := xml.Unmarshal(b, &defs); err != nil {
		return nil, err
	}
	labels := map[string]string{}
	if lang == "zh" {
		labels = chineseLabels(game)
	}
	var out []expansion
	for _, d := range defs.Expansions {
		if d.Name == "" || d.Path == "" || strings.ContainsAny(d.Name, `/\`) || d.Name == "." || d.Name == ".." {
			continue
		}
		if d.Name == "Core" {
			d.Mod = "ludeon.rimworld"
		}
		if d.Mod == "" {
			d.Mod = "ludeon.rimworld." + d.Name
		}
		if lang == "zh" {
			d.Label = labels[d.Name]
		}
		if strings.TrimSpace(d.Label) == "" {
			d.Label = d.Name
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name == "Core" && out[j].Name != "Core" })
	return out, nil
}

// Both unpacked translations and RimWorld 1.7's language tar files are read in
// place. Translation XML is small; no language archive is extracted (16 §2.14).
func chineseLabels(game string) map[string]string {
	labels := map[string]string{}
	data, _ := os.ReadDir(filepath.Join(game, "Data"))
	for _, content := range data {
		if !content.IsDir() {
			continue
		}
		root := filepath.Join(game, "Data", content.Name(), "Languages")
		languages, _ := os.ReadDir(root)
		for _, language := range languages {
			if !strings.HasPrefix(language.Name(), "ChineseSimplified") {
				continue
			}
			path := filepath.Join(root, language.Name())
			if language.IsDir() {
				filepath.WalkDir(filepath.Join(path, "DefInjected", "ExpansionDef"), func(p string, d fs.DirEntry, err error) error {
					if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".xml") {
						f, err := os.Open(p)
						if err == nil {
							readLabels(f, labels)
							f.Close()
						}
					}
					return nil
				})
			} else if strings.EqualFold(filepath.Ext(path), ".tar") {
				f, err := os.Open(path)
				if err != nil {
					continue
				}
				tr := tar.NewReader(f)
				for {
					h, err := tr.Next()
					if err != nil {
						break
					}
					name := strings.ToLower(strings.ReplaceAll(h.Name, `\`, "/"))
					if h.Typeflag == tar.TypeReg && strings.Contains("/"+name, "/definjected/expansiondef/") && strings.HasSuffix(name, ".xml") {
						readLabels(tr, labels)
					}
				}
				f.Close()
			}
		}
	}
	return labels
}

func readLabels(r io.Reader, labels map[string]string) {
	b, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return
	}
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		t, err := dec.Token()
		if err != nil {
			return
		}
		if se, ok := t.(xml.StartElement); ok && strings.HasSuffix(se.Name.Local, ".label") {
			var label string
			if dec.DecodeElement(&label, &se) == nil && strings.TrimSpace(label) != "" {
				labels[strings.TrimSuffix(se.Name.Local, ".label")] = strings.TrimSpace(label)
			}
		}
	}
}

// Stamp guards extraction against source changes. It is never part of a shared
// cache key and is only checked while filling missing pairs.
func Stamp(game string) string {
	path, _ := filepath.Abs(filepath.Join(game, "Assets.zip"))
	stamp := fileStamp(path)
	if _, err := os.Stat(path); err == nil {
		return stamp
	}
	for _, name := range []string{"resources.assets", "resources.assets.resS", "globalgamemanagers"} {
		stamp += "|" + fileStamp(filepath.Join(filepath.Dir(path), "RimWorldWin64_Data", name))
	}
	return stamp
}

func fileStamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return path + "|missing"
	}
	return fmt.Sprintf("%s|%d|%d", path, st.Size(), st.ModTime().UnixNano())
}
