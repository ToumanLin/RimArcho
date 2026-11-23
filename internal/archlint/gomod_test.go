package archlint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// forbiddenModules are rejected by 17 §1: cgo SQLite, Bubbles, ORMs and Web frameworks.
var forbiddenModules = map[string]string{
	"github.com/mattn/go-sqlite3":          "cgo SQLite; RimArcho uses pure Go modernc.org/sqlite with CGO_ENABLED=0",
	"github.com/charmbracelet/bubbles":     "the TUI uses Bubble Tea and Lip Gloss without Bubbles",
	"gorm.io/gorm":                         "no ORM; store writes SQL against schema.sql",
	"github.com/jinzhu/gorm":               "no ORM; store writes SQL against schema.sql",
	"entgo.io/ent":                         "no ORM; store writes SQL against schema.sql",
	"github.com/uptrace/bun":               "no ORM; store writes SQL against schema.sql",
	"xorm.io/xorm":                         "no ORM; store writes SQL against schema.sql",
	"github.com/go-xorm/xorm":              "no ORM; store writes SQL against schema.sql",
	"github.com/volatiletech/sqlboiler/v4": "no ORM; store writes SQL against schema.sql",
	"github.com/gin-gonic/gin":             "no Web framework; the GUI has no server (16 §8)",
	"github.com/labstack/echo/v4":          "no Web framework; the GUI has no server (16 §8)",
	"github.com/gofiber/fiber/v2":          "no Web framework; the GUI has no server (16 §8)",
}

// pinnedIn17 maps a module to the words before its version in 17-engineering.md (§1's
// table and §7.1): go.mod and the specification name the same version.
var pinnedIn17 = map[string]string{
	"go":                                     "Go ",
	"github.com/charmbracelet/bubbletea":     "Bubble Tea ",
	"github.com/wailsapp/wails/v3":           "Wails ",
	"modernc.org/sqlite":                     "modernc.org/sqlite ",
	"github.com/antchfx/xpath":               "antchfx/xpath ",
	"github.com/antchfx/xmlquery":            "antchfx/xmlquery ",
	"github.com/saferwall/pe":                "saferwall/pe ",
	"github.com/modelcontextprotocol/go-sdk": "modelcontextprotocol/go-sdk ",
	"github.com/zeebo/xxh3":                  "zeebo/xxh3 ",
	"github.com/klauspost/compress":          "klauspost/compress ",
	"github.com/woozymasta/bcn":              "woozymasta/bcn ",
	"github.com/mattn/go-runewidth":          "mattn/go-runewidth ",
	"github.com/ebitengine/purego":           "ebitengine/purego ",
	"github.com/charmbracelet/x/ansi":        "x/ansi** is at ",
	"github.com/clipperhouse/displaywidth":   "`clipperhouse/displaywidth` is at ",
	"github.com/clipperhouse/uax29/v2":       "uax29 ",
}

// goModVersions reads the go directive and every require line of go.mod.
func goModVersions(gomod string) map[string]string {
	out := map[string]string{}
	block := false
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		switch {
		case line == "":
		case strings.HasPrefix(line, "go "):
			out["go"] = strings.TrimSpace(line[3:])
		case line == "require (":
			block = true
		case block && line == ")":
			block = false
		case block, strings.HasPrefix(line, "require "):
			f := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(f) >= 2 {
				out[f[0]] = f[1]
			}
		}
	}
	return out
}

func goModProblems(gomod, spec17 string) []string {
	var out []string
	mods := goModVersions(gomod)
	for m, why := range forbiddenModules {
		if _, ok := mods[m]; ok {
			out = append(out, fmt.Sprintf("17 §1: go.mod requires %s: %s", m, why))
		}
	}
	for m, words := range pinnedIn17 {
		v, ok := mods[m]
		if !ok {
			out = append(out, fmt.Sprintf("17 §1, §7.1: go.mod no longer requires %s; update 17 and pinnedIn17 in internal/archlint", m))
			continue
		}
		if !strings.Contains(spec17, words+v) {
			out = append(out, fmt.Sprintf("17 §1, §7.1: go.mod has %s %s but docs/specs/17-engineering.md does not say %q; verify the upgrade (17 §7.1) and update the specification in the same commit", m, v, words+v))
		}
	}
	sort.Strings(out)
	return out
}

func TestArchGoMod(t *testing.T) {
	t.Parallel()
	root, _ := repoRoot(t)
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile(filepath.Join(root, "docs", "specs", "17-engineering.md"))
	if err != nil {
		t.Fatal(err)
	}
	report(t, goModProblems(string(gomod), string(spec)))
}

func TestArchGoModDetection(t *testing.T) {
	t.Parallel()
	var spec strings.Builder
	var mod strings.Builder
	mod.WriteString("module x\n\ngo 1.2.3\n\nrequire (\n")
	for m, words := range pinnedIn17 {
		if m == "go" {
			spec.WriteString("Go 1.2.3\n")
			continue
		}
		spec.WriteString(words + "v1.0.0\n")
		mod.WriteString("\t" + m + " v1.0.0 // indirect\n")
	}
	expect(t, "pins match", goModProblems(mod.String()+")\n", spec.String()), 0)
	expect(t, "forbidden and bumped", goModProblems(mod.String()+"\tgithub.com/mattn/go-sqlite3 v1.14.0\n)\nrequire github.com/charmbracelet/bubbles v0.1.0\n", strings.Replace(spec.String(), "Bubble Tea v1.0.0", "Bubble Tea v0.9.0", 1)), 3)
}
