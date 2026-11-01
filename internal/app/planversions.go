package app

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A plan's game versions (04 §1.6): the game versions the list is meant to be played on.
// The default is worked out each time from the list: the versions every mod lists, plus the
// game the plan was made on, as one run from the lowest to the highest. A player's choice
// is stored and stays until they go back to the default.

// PlanVersions is what `plan versions` and the list management page show.
type PlanVersions struct {
	Plan    *store.Plan
	Allowed []string // the versions in force: Chosen, or Default
	Default []string
	Custom  bool     // the player chose Allowed
	Common  []string // the versions every counted mod lists (may be empty)
	Created string   // the game the plan was made on (major.minor; "" unknown)
	Game    string   // the environment's game now (major.minor)
	// Shown are the versions an interface offers, oldest first: 1.0 up to the newest of the
	// game, the plan's and the mods' versions.
	Shown []string
	// Missing counts, per version, the counted mods that do not list it; Counted is how many
	// mods were counted (official content, missing entries and mods without
	// supportedVersions are not).
	Missing map[string]int
	Muted   map[string]int // missing declarations the player chose not to warn about
	Counted int
}

// PlanVersionsOf works out a plan's game versions in st's environment.
func (a *App) PlanVersionsOf(st *LoadoutState, p *store.Plan) (*PlanVersions, error) {
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	pv := &PlanVersions{Plan: p, Created: env.MajorMinor(p.CreatedGame), Game: st.GameMM(), Missing: map[string]int{}, Muted: map[string]int{}}
	mutes, err := a.Store.AllVersionMutes()
	if err != nil {
		return nil, err
	}
	var sets []map[string]bool
	var packages []string
	for _, e := range ps.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || m.Kind == source.Official || strings.TrimSpace(m.SupportedVersions) == "" {
			continue
		}
		vs := map[string]bool{}
		for _, v := range strings.Fields(strings.ReplaceAll(m.SupportedVersions, ",", " ")) {
			if v = env.MajorMinor(v); v != "" {
				vs[v] = true
			}
		}
		if len(vs) > 0 {
			sets = append(sets, vs)
			packages = append(packages, store.VersionMuteKey(m.PackageID))
		}
	}
	pv.Counted = len(sets)
	if len(sets) > 0 {
		for v := range sets[0] {
			every := true
			for _, s := range sets[1:] {
				if !s[v] {
					every = false
					break
				}
			}
			if every {
				pv.Common = append(pv.Common, v)
			}
		}
	}
	sortVersions(pv.Common)
	made := pv.Created
	if made == "" {
		made = pv.Game
	}
	top := []string{pv.Game, made}
	top = append(top, p.Versions...)
	top = append(top, pv.Common...)
	pv.Shown = versionsUpTo(top)
	pv.Default = defaultVersions(pv.Common, made, pv.Shown)
	if len(p.Versions) > 0 {
		pv.Allowed, pv.Custom = append([]string(nil), p.Versions...), true
	} else {
		pv.Allowed = pv.Default
	}
	for _, v := range pv.Shown {
		n := 0
		for i, s := range sets {
			if !s[v] {
				if mutes[v][packages[i]] {
					pv.Muted[v]++
				} else {
					n++
				}
			}
		}
		pv.Missing[v] = n
	}
	return pv, nil
}

// defaultVersions is the run from the lowest to the highest of common and made.
func defaultVersions(common []string, made string, shown []string) []string {
	set := append([]string(nil), common...)
	if made != "" {
		set = append(set, made)
	}
	if len(set) == 0 {
		return nil
	}
	sortVersions(set)
	lo, hi := set[0], set[len(set)-1]
	var out []string
	for _, v := range shown {
		if !env.VersionLess(v, lo) && !env.VersionLess(hi, v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []string{lo}
	}
	return out
}

// versionsUpTo lists 1.0, 1.1, … up to the newest of vs (same major; other majors are
// added as they are).
func versionsUpTo(vs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range vs {
		maj, min, ok := splitMM(v)
		if !ok {
			continue
		}
		for i := 0; i <= min; i++ {
			add(fmt.Sprintf("%d.%d", maj, i))
		}
	}
	sortVersions(out)
	return out
}

func splitMM(v string) (maj, min int, ok bool) {
	mm := env.MajorMinor(v)
	i := strings.IndexByte(mm, '.')
	if i < 0 {
		return 0, 0, false
	}
	maj, e1 := strconv.Atoi(mm[:i])
	min, e2 := strconv.Atoi(mm[i+1:])
	return maj, min, e1 == nil && e2 == nil && min < 100
}

func sortVersions(vs []string) {
	sort.Slice(vs, func(i, j int) bool { return env.VersionLess(vs[i], vs[j]) })
}

// FormatVersions writes versions as runs: "1.5–1.7", "1.3, 1.5–1.6", "—" when empty.
// Runs follow the minor numbers of one major.
func FormatVersions(vs []string) string {
	vs = append([]string(nil), vs...)
	sortVersions(vs)
	var out []string
	for i := 0; i < len(vs); i++ {
		j := i
		for j+1 < len(vs) && nextVersion(vs[j], vs[j+1]) {
			j++
		}
		if i == j {
			out = append(out, vs[i])
		} else {
			out = append(out, vs[i]+"–"+vs[j])
		}
		i = j
	}
	if len(out) == 0 {
		return "—"
	}
	return strings.Join(out, ", ")
}

func nextVersion(a, b string) bool {
	am, an, ok1 := splitMM(a)
	bm, bn, ok2 := splitMM(b)
	return ok1 && ok2 && am == bm && bn == an+1
}

var versionTokRe = regexp.MustCompile(`^(\d+\.\d+)(?:\s*[-–~]\s*(\d+\.\d+))?$`)

// ParseVersions reads "1.5 1.6", "1.5-1.7", "1.4,1.6–1.7" into sorted, distinct versions.
func ParseVersions(args []string) ([]string, error) {
	text := strings.Join(args, " ")
	text = strings.NewReplacer(",", " ", "\uff0c", " ", "\u3001", " ", " - ", "-", " – ", "–").Replace(text)
	seen := map[string]bool{}
	var out []string
	for _, tok := range strings.Fields(text) {
		m := versionTokRe.FindStringSubmatch(tok)
		if m == nil {
			return nil, fmt.Errorf("%q is not a game version: write 1.6, 1.5-1.7 or default", tok)
		}
		lo, hi := m[1], m[1]
		if m[2] != "" {
			hi = m[2]
		}
		lm, ln, _ := splitMM(lo)
		hm, hn, _ := splitMM(hi)
		if lm != hm || hn < ln {
			return nil, fmt.Errorf("%q is not a range of one major version", tok)
		}
		for i := ln; i <= hn; i++ {
			v := fmt.Sprintf("%d.%d", lm, i)
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no game version given: write 1.6, 1.5-1.7 or default")
	}
	sortVersions(out)
	return out, nil
}

// PlanVersionsResult is what SetPlanVersions did.
type PlanVersionsResult struct {
	Before, After *PlanVersions
}

// SetPlanVersions sets a plan's game versions; args "default" (or none of them chosen)
// goes back to the default. A choice equal to the default is stored as the default, so the
// plan keeps following its mods. Not a list edit: nothing enters the undo stack.
func (a *App) SetPlanVersions(name string, args []string) (*PlanVersionsResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(name)
	if err != nil {
		return nil, err
	}
	before, err := a.PlanVersionsOf(st, p)
	if err != nil {
		return nil, err
	}
	var chosen []string
	if !(len(args) == 1 && strings.EqualFold(strings.TrimSpace(args[0]), "default")) {
		if chosen, err = ParseVersions(args); err != nil {
			return nil, err
		}
		if equalVersions(chosen, before.Default) {
			chosen = nil
		}
	}
	if err := a.Store.SetPlanVersions(p.ID, chosen); err != nil {
		return nil, err
	}
	p.Versions = chosen
	after, err := a.PlanVersionsOf(st, p)
	if err != nil {
		return nil, err
	}
	return &PlanVersionsResult{Before: before, After: after}, nil
}

func equalVersions(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// PlanAllowedVersions is the versions in force for a plan (nil when they cannot be worked out).
func (a *App) PlanAllowedVersions(st *LoadoutState, p *store.Plan) []string {
	if len(p.Versions) > 0 {
		return p.Versions
	}
	if pv, err := a.PlanVersionsOf(st, p); err == nil {
		return pv.Allowed
	}
	return nil
}

// adoptVersions stores versions read from a RimArcho list for a new plan (04 §1.6, §9.3).
func (a *App) adoptVersions(p *store.Plan, vs []string) {
	if len(vs) == 0 {
		return
	}
	if parsed, err := ParseVersions(vs); err == nil {
		a.Store.SetPlanVersions(p.ID, parsed)
		p.Versions = parsed
	}
}
