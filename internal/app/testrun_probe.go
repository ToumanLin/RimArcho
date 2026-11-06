package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// A probe run adds a probe Mod to the tested list and runs a test map until its last
// "[Probe]" line (09 §7.1). Any folder that follows the line contract works.

// TestRunProbe is the probe part of a test run's report.
type TestRunProbe struct {
	Folder    string `json:"folder"`
	PackageID string `json:"package_id"`
	Position  int    `json:"position"` // 1-based in the tested load order, official content included
	// Outcome: result, exception, or none when no last line arrived.
	Outcome     string   `json:"outcome"`
	Result      string   `json:"result,omitempty"`
	Exception   string   `json:"exception,omitempty"`
	Lines       []string `json:"lines"`
	LinesTotal  int      `json:"lines_total"`
	Screenshot  string   `json:"screenshot,omitempty"`
	MissingDeps []string `json:"missing_dependencies,omitempty"`
}

const (
	probeLinkPrefix = DeployPrefix + "probe_"
	probeShot       = "probe.png"
	probeLineMax    = 300  // characters of each listed line
	probeResultMax  = 2000 // characters of result and exception
	probeLinesHead  = 10   // beyond 40 lines: the first 10 and the last 30
	probeLinesTail  = 30
	probeStackLines = 8 // stack lines after an EXCEPTION line
)

var probeLinkChars = regexp.MustCompile(`[^A-Za-z0-9.-]`)

// probeSetup is a checked probe: what to link and where it goes in the test ModsConfig.xml.
type probeSetup struct {
	report *TestRunProbe
	index  int         // insertion index into the test run's tokens
	link   *LinkChange // nil when Mods/ already holds the folder
}

// testRunTokens are the tested list's activeMods identifiers, as prepareTestConfig writes them.
func testRunTokens(st *LoadoutState) []string {
	return st.Reg.Tokens(st.List, func(id string) string {
		if m := st.Ix.ByID[id]; m != nil {
			return m.PackageID
		}
		return ""
	})
}

// tokenIndex finds a packageId among the tokens; a Workshop copy beside a Mods/ copy is <pkg>_steam.
func tokenIndex(tokens []string, pkg string) int {
	return slices.IndexFunc(tokens, func(t string) bool {
		return strings.EqualFold(t, pkg) || strings.EqualFold(t, pkg+"_steam")
	})
}

// checkProbe validates the probe folder and its position before the run links or writes
// anything (09 §7.1).
func (a *App) checkProbe(st *LoadoutState, o TestRunOptions) (*probeSetup, error) {
	if o.Probe == "" {
		if o.ProbeAfter != "" || o.ProbeBefore != "" {
			return nil, fmt.Errorf("test run: probe position given without a probe folder")
		}
		return nil, nil
	}
	if o.ProbeAfter != "" && o.ProbeBefore != "" {
		return nil, fmt.Errorf("test run: give the probe's position after or before a mod, not both")
	}
	folder, err := filepath.Abs(o.Probe)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("test run: probe folder %s not found", folder)
	}
	real, err := source.RealPath(folder)
	if err != nil {
		return nil, err
	}
	if !source.HasAbout(folder) {
		return nil, fmt.Errorf("test run: probe folder %s has no About/About.xml", folder)
	}
	d := scan.ReadDecl(folder, false)
	if d.ParseError != "" {
		return nil, fmt.Errorf("test run: probe About.xml: %s", d.ParseError)
	}
	pkg := d.PackageID
	if pkg == "" || d.PackageIDDerived {
		return nil, fmt.Errorf("test run: probe About.xml in %s has no packageId", folder)
	}
	if strings.HasPrefix(pkg, "ludeon.rimworld") {
		return nil, fmt.Errorf("test run: probe packageId %s is official content", pkg)
	}
	for _, e := range st.List {
		id := e.Missing
		if m := st.Ix.ByID[e.Instance]; m != nil {
			id = m.PackageID
		}
		if strings.EqualFold(id, pkg) {
			return nil, fmt.Errorf("test run: the tested plan already lists %s; a probe needs a packageId of its own", pkg)
		}
	}
	tokens := testRunTokens(st)
	index := len(tokens)
	if ref := o.ProbeAfter; ref != "" {
		if index = tokenIndex(tokens, ref); index < 0 {
			return nil, fmt.Errorf("test run: the tested list has no %s to put the probe after", ref)
		}
		index++
	} else if ref := o.ProbeBefore; ref != "" {
		if index = tokenIndex(tokens, ref); index < 0 {
			return nil, fmt.Errorf("test run: the tested list has no %s to put the probe before", ref)
		}
	}
	mods := modsDir(st.Ix)
	if fi, err := os.Stat(mods); mods == "" || err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("test run: the game has no Mods/ folder to link the probe into")
	}
	p := &probeSetup{index: index, report: &TestRunProbe{Folder: folder, PackageID: pkg, Position: index + 1, Outcome: "none", Lines: []string{}}}
	seen := false // Mods/ already holds the folder itself
	for _, c := range a.ModsCopies(st.Ix) {
		if c.PackageID != pkg {
			continue
		}
		if have, err := source.RealPath(filepath.Join(mods, c.Name)); err == nil && env.PathKey(have) == env.PathKey(real) {
			seen = true
			continue
		}
		return nil, fmt.Errorf("test run: Mods/%s already has packageId %s; the game would load only one", c.Name, pkg)
	}
	if !seen {
		name := probeLinkPrefix + probeLinkChars.ReplaceAllString(pkg, "_")
		if _, err := os.Lstat(filepath.Join(mods, name)); err == nil {
			return nil, fmt.Errorf("test run: Mods/%s exists and is not this probe", name)
		}
		p.link = &LinkChange{Op: "add", Name: name, Target: real}
	}
	// The dependencies the probe declares that the tested list does not have.
	deps := append([]scan.Dependency{}, d.Dependencies...)
	deps = append(deps, d.DependenciesByVersion["v"+st.GameMM()]...)
	for _, dep := range deps {
		id := strings.ToLower(dep.PackageID)
		if tokenIndex(tokens, id) < 0 && !slices.Contains(p.report.MissingDeps, id) {
			p.report.MissingDeps = append(p.report.MissingDeps, id)
		}
	}
	return p, nil
}

// probePackage is a report's probe packageId; "" for a run without a probe.
func probePackage(p *TestRunProbe) string {
	if p == nil {
		return ""
	}
	return p.PackageID
}

// probeTokens puts the probe into the test run's activeMods.
func (p *probeSetup) probeTokens(tokens []string) []string {
	if p == nil {
		return tokens
	}
	return slices.Insert(slices.Clone(tokens), min(p.index, len(tokens)), p.report.PackageID)
}

// clearProbeShot removes an earlier run's screenshot from the reused user data.
func clearProbeShot(data string) error {
	if err := os.Remove(filepath.Join(data, probeShot)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// finishProbe reads the probe's lines from the run's log and moves its screenshot into the run directory.
func finishProbe(p *TestRunProbe, logPath, data, runDir string) error {
	var errs []error
	shot := filepath.Join(data, probeShot)
	if _, err := os.Stat(shot); err == nil {
		dst := filepath.Join(runDir, probeShot)
		if err := os.Rename(shot, dst); err != nil {
			errs = append(errs, err)
		} else {
			p.Screenshot = dst
		}
	}
	if err := readProbeLines(p, logPath); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("test run: probe: %w", errs[0])
	}
	return nil
}

func readProbeLines(p *TestRunProbe, logPath string) error {
	f, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var head, tail []string
	stack := -1 // lines left to add to the exception's text
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		text, ok := gametest.ProbeLine(line)
		if !ok {
			if stack > 0 && probeStackLine(line) {
				p.Exception += "\n" + strings.TrimSpace(line)
				stack--
			} else {
				stack = -1
			}
			continue
		}
		stack = -1
		p.LinesTotal++
		short := cutRunes(text, probeLineMax)
		if len(head) < probeLinesHead {
			head = append(head, short)
		} else {
			tail = append(tail, short)
			if len(tail) > probeLinesTail {
				tail = tail[1:]
			}
		}
		if p.Outcome != "none" {
			continue // the first last line counts
		}
		switch gametest.ProbeEnd(text) {
		case "result":
			p.Outcome = "result"
			p.Result = cutRunes(strings.TrimSpace(strings.TrimPrefix(text, "RESULT")), probeResultMax)
		case "exception":
			p.Outcome = "exception"
			p.Exception = cutRunes(strings.TrimSpace(strings.TrimPrefix(text, "EXCEPTION")), probeResultMax)
			stack = probeStackLines
		}
	}
	p.Lines = append(append([]string{}, head...), tail...)
	return sc.Err()
}

// probeStackLine: a line of the exception's own stack ("at …"), not Unity's logging stack.
func probeStackLine(line string) bool {
	s := strings.TrimSpace(line)
	return strings.HasPrefix(s, "at ") || strings.HasPrefix(s, "--- End of inner exception")
}

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-3]) + "..."
	}
	return s
}
