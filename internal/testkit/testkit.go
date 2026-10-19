// Package testkit builds synthetic RimWorld environments in a temporary directory for the
// identity acceptance tests (02 §9) and the CLI golden tests. Everything it writes lives
// under t.TempDir(); it never touches a real game or workshop folder.
package testkit

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/source"
)

// Start is the fake clock's initial time.
var Start = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// Env is a synthetic Steam install with RimWorld, a workshop folder, an optional SteamCMD
// folder and a user data directory, plus an opened RimArcho data directory.
type Env struct {
	T        testing.TB
	Root     string
	Steam    string // Steam client root (= first library)
	Game     string // <Steam>/steamapps/common/RimWorld
	Workshop string // <Steam>/steamapps/workshop/content/294100
	SteamCMD string // <Root>/steamcmd/steamapps/workshop/content/294100
	UserData string
	Home     string
	App      *app.App

	now        time.Time
	subscribed map[string]bool
	manifests  map[string]int
}

// New creates the fixture layout: Core in Data/, empty Mods/, an empty workshop library with
// an ACF, and user data with a ModsConfig.xml. The app is opened with a fake clock and a
// seeded ID source so output is reproducible.
func New(t testing.TB) *Env {
	t.Helper()
	root := t.TempDir()
	e := &Env{T: t, Root: root, now: Start, subscribed: map[string]bool{}, manifests: map[string]int{}}
	e.Steam = filepath.Join(root, "Steam")
	e.Game = filepath.Join(e.Steam, "steamapps", "common", "RimWorld")
	e.Workshop = filepath.Join(e.Steam, "steamapps", "workshop", "content", env.WorkshopAppID)
	e.SteamCMD = filepath.Join(root, "steamcmd", "steamapps", "workshop", "content", env.WorkshopAppID)
	e.UserData = filepath.Join(root, "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
	e.Home = filepath.Join(root, "ArchoHome")
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64.exe"), "")
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.6.4633 rev1261\n")
	e.MkdirAll(filepath.Join(e.Game, "Mods"))
	e.CopyTemplate("core", filepath.Join(e.Game, "Data", "Core"))
	e.MkdirAll(e.Workshop)
	e.writeACF()
	e.WriteFile(filepath.Join(e.Steam, "steamapps", "libraryfolders.vdf"), fmt.Sprintf(
		"\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t%q\n\t\t\"apps\"\n\t\t{\n\t\t\t\"294100\"\t\t\"1\"\n\t\t}\n\t}\n}\n",
		strings.ReplaceAll(e.Steam, `\`, `\\`)))
	e.WriteFile(filepath.Join(e.UserData, "Config", "ModsConfig.xml"),
		"<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModsConfigData>\n  <version>1.6.4633 rev1261</version>\n  <activeMods>\n    <li>ludeon.rimworld</li>\n  </activeMods>\n</ModsConfigData>\n")
	e.Open()
	return e
}

// Open (re)opens the RimArcho data directory with deterministic clock and IDs.
func (e *Env) Open() {
	e.T.Helper()
	if e.App != nil {
		e.App.Close()
	}
	a, err := app.Open(e.Home)
	if err != nil {
		e.T.Fatal(err)
	}
	a.Now = func() time.Time { return e.now }
	a.Rand = rand.NewChaCha8([32]byte{'r', 'i', 'm', 'a', 'r', 'c', 'h', 'o'})
	a.Loc = time.UTC
	a.Detect = env.DetectOptions{SteamRoots: []string{e.Steam}, ManualRoots: []string{}, Home: e.Root, GOOS: "windows", SkipRegistry: true}
	e.App = a
	e.T.Cleanup(func() { a.Close() })
}

// Advance moves the fake clock.
func (e *Env) Advance(d time.Duration) { e.now = e.now.Add(d) }

// Now is the fake clock's current time.
func (e *Env) Now() time.Time { return e.now }

// Run executes an archo command line.
func (e *Env) Run(args ...string) (string, int) { return cmds.Execute(e.App, args) }

// MustRun executes a command and fails the test on a non-zero exit code.
func (e *Env) MustRun(args ...string) string {
	e.T.Helper()
	out, code := e.Run(args...)
	if code != 0 {
		e.T.Fatalf("archo %s: exit %d\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

// AddEnv registers the fixture as environment "main" (game, user data, workshop and,
// with steamcmd, the SteamCMD folder) and makes it current.
func (e *Env) AddEnv(steamcmd bool) string {
	e.T.Helper()
	args := []string{"env", "add", "main", "--game", e.Game, "--user-data", e.UserData, "--workshop", e.Workshop}
	if steamcmd {
		e.MkdirAll(e.SteamCMD)
		args = append(args, "--steamcmd", e.SteamCMD)
	}
	return e.MustRun(args...)
}

// Scrub makes output path-independent: the temp root becomes $ROOT and separators "/".
func (e *Env) Scrub(s string) string {
	// The temp dir may be spelled with 8.3 short names (TOUMAN~1) or expanded ones.
	if real, err := source.RealPath(e.Root); err == nil {
		s = strings.ReplaceAll(s, real, "$ROOT")
	}
	s = strings.ReplaceAll(s, e.Root, "$ROOT")
	return strings.ReplaceAll(s, `\`, "/")
}

// ---- files ----

// MkdirAll creates a directory.
func (e *Env) MkdirAll(p string) {
	e.T.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		e.T.Fatal(err)
	}
}

// WriteFile writes a file, creating parent directories.
func (e *Env) WriteFile(p, content string) {
	e.T.Helper()
	e.MkdirAll(filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.T.Fatal(err)
	}
}

// TemplateDir is testdata/mods, located relative to this source file.
func TemplateDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "mods")
}

// CopyTemplate copies testdata/mods/<name> to dst.
func (e *Env) CopyTemplate(name, dst string) {
	e.T.Helper()
	e.CopyTree(filepath.Join(TemplateDir(), name), dst)
}

// CopyTree copies a directory recursively (regular files only), like a player copying a
// folder in Explorer: mtimes are reset.
func (e *Env) CopyTree(src, dst string) {
	e.T.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		e.T.Fatal(err)
	}
}

// Move renames a file or directory.
func (e *Env) Move(src, dst string) {
	e.T.Helper()
	e.MkdirAll(filepath.Dir(dst))
	if err := os.Rename(src, dst); err != nil {
		e.T.Fatal(err)
	}
}

// Remove deletes a directory tree.
func (e *Env) Remove(p string) {
	e.T.Helper()
	if err := os.RemoveAll(p); err != nil {
		e.T.Fatal(err)
	}
}

// ---- mods ----

// Mod describes a synthetic mod.
type Mod struct {
	PackageID string
	Name      string
	Author    string
	Supported []string
	Deps      []string
	Workshop  string            // About/PublishedFileId.txt
	Files     map[string]string // extra files by relative path (Defs, Patches, textures …)
}

// AboutXML renders an About.xml for m.
func (m Mod) AboutXML() string {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModMetaData>\n")
	if m.PackageID != "" {
		fmt.Fprintf(&sb, "  <packageId>%s</packageId>\n", m.PackageID)
	}
	fmt.Fprintf(&sb, "  <name>%s</name>\n", m.Name)
	if m.Author != "" {
		fmt.Fprintf(&sb, "  <author>%s</author>\n", m.Author)
	}
	sup := m.Supported
	if sup == nil {
		sup = []string{"1.6"}
	}
	sb.WriteString("  <supportedVersions>\n")
	for _, v := range sup {
		fmt.Fprintf(&sb, "    <li>%s</li>\n", v)
	}
	sb.WriteString("  </supportedVersions>\n")
	if len(m.Deps) > 0 {
		sb.WriteString("  <modDependencies>\n")
		for _, d := range m.Deps {
			fmt.Fprintf(&sb, "    <li>\n      <packageId>%s</packageId>\n      <displayName>%s</displayName>\n    </li>\n", d, d)
		}
		sb.WriteString("  </modDependencies>\n")
	}
	sb.WriteString("</ModMetaData>\n")
	return sb.String()
}

// WriteMod writes m into dir (About/About.xml, PublishedFileId.txt and files).
func (e *Env) WriteMod(dir string, m Mod) {
	e.T.Helper()
	e.WriteFile(filepath.Join(dir, "About", "About.xml"), m.AboutXML())
	if m.Workshop != "" {
		e.WriteFile(filepath.Join(dir, "About", "PublishedFileId.txt"), m.Workshop)
	}
	keys := make([]string, 0, len(m.Files))
	for k := range m.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e.WriteFile(filepath.Join(dir, filepath.FromSlash(k)), m.Files[k])
	}
}

// Sample returns a mod with a few defs and patches unique to name.
func Sample(pkg, name string) Mod {
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "")
	return Mod{
		PackageID: pkg, Name: name, Author: "Tester",
		Files: map[string]string{
			"Defs/ThingDefs/" + slug + ".xml":          fmt.Sprintf("<Defs><ThingDef><defName>%s_Thing</defName></ThingDef></Defs>\n", slug),
			"Defs/RecipeDefs/" + slug + ".xml":         fmt.Sprintf("<Defs><RecipeDef><defName>%s_Recipe</defName></RecipeDef></Defs>\n", slug),
			"Patches/" + slug + ".xml":                 fmt.Sprintf("<Patch><Operation Class=\"PatchOperationAdd\"><xpath>Defs/ThingDef[defName=\"%s\"]</xpath></Operation></Patch>\n", slug),
			"Textures/Things/" + slug + "/a.png":       strings.Repeat("x", 64),
			"Languages/English/Keyed/" + slug + ".xml": "<LanguageData/>\n",
		},
	}
}

// ---- workshop ----

// AddWorkshop writes a workshop item into the Steam library and subscribes it.
func (e *Env) AddWorkshop(id string, m Mod) string {
	e.T.Helper()
	dir := filepath.Join(e.Workshop, id)
	e.WriteMod(dir, m)
	e.Subscribe(id)
	return dir
}

// Subscribe lists id in the ACF.
func (e *Env) Subscribe(id string) { e.subscribed[id] = true; e.writeACF() }

// Unsubscribe removes id from the ACF (the folder may stay behind as residue).
func (e *Env) Unsubscribe(id string) { delete(e.subscribed, id); e.writeACF() }

func (e *Env) writeACF() {
	ids := make([]string, 0, len(e.subscribed))
	for id := range e.subscribed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var inst, det strings.Builder
	for i, id := range ids {
		fmt.Fprintf(&inst, "\t\t%q\n\t\t{\n\t\t\t\"size\"\t\t\"100\"\n\t\t\t\"timeupdated\"\t\t\"%d\"\n\t\t\t\"manifest\"\t\t\"%d\"\n\t\t}\n", id, 1790000000+i, e.manifest(id, i))
		fmt.Fprintf(&det, "\t\t%q\n\t\t{\n\t\t\t\"manifest\"\t\t\"%d\"\n\t\t\t\"subscribedby\"\t\t\"1\"\n\t\t}\n", id, e.manifest(id, i))
	}
	e.WriteFile(filepath.Join(e.Steam, "steamapps", "workshop", "appworkshop_294100.acf"), fmt.Sprintf(
		"\"AppWorkshop\"\n{\n\t\"appid\"\t\t\"294100\"\n\t\"WorkshopItemsInstalled\"\n\t{\n%s\t}\n\t\"WorkshopItemDetails\"\n\t{\n%s\t}\n}\n", inst.String(), det.String()))
}

// SetManifest overrides the ACF manifest ID of an item (Steam re-downloaded it).
func (e *Env) SetManifest(id string, manifest int) { e.manifests[id] = manifest; e.writeACF() }

func (e *Env) manifest(id string, i int) int {
	if m, ok := e.manifests[id]; ok {
		return m
	}
	return 9000 + i
}
