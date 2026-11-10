package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

const (
	CodeLinkDangling = "mods.link-dangling"
	CodeLinkForeign  = "mods.link-foreign"
	CodeLinkUnknown  = "mods.link-check-failed"
	CodeLinkOccupied = "mods.link-occupied"
	CodeLinkRecovery = "mods.link-recovery"
)

// GameLink is a fresh first-level filesystem observation, independent of mod caches.
type GameLink struct {
	Game     string             `json:"game"`
	Mods     string             `json:"mods"`
	Path     string             `json:"path"`
	Target   string             `json:"target"`
	Kind     string             `json:"kind"`
	State    source.TargetState `json:"state"`
	Code     string             `json:"code,omitempty"`
	Reason   string             `json:"reason"`
	Evidence string             `json:"evidence"`
	Owned    bool               `json:"owned"`
	Action   string             `json:"action"`
	snapshot *source.LinkInfo
	Instance string `json:"instance,omitempty"`
	Pack     string `json:"pack,omitempty"`
	// Copy: a real folder under the link's name that holds the same files as the folder
	// RimArcho links there (Action "recycle copy").
	Copy     string `json:"copy,omitempty"`
	recovery *linkRecovery
}

// sameFiles reports whether two different folders hold the same relative files with the same
// sizes, links refused: the cheap look of a copy. A repair compares the contents
// (treeDigest) before it moves anything.
func sameFiles(a, b string) bool {
	ra, rb := realOrAbs(a), realOrAbs(b)
	if env.PathKey(ra) == env.PathKey(rb) || source.Within(ra, rb) || source.Within(rb, ra) {
		return false
	}
	list := func(root string) (map[string]int64, bool) {
		out := map[string]int64{}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeType != 0 && !d.IsDir() {
				return fmt.Errorf("not a plain file: %s", p)
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			out[strings.ToLower(filepath.ToSlash(rel))] = info.Size()
			return nil
		})
		return out, err == nil && len(out) > 0
	}
	fa, ok := list(a)
	if !ok {
		return false
	}
	fb, ok := list(b)
	if !ok || len(fa) != len(fb) {
		return false
	}
	for k, n := range fa {
		if m, ok := fb[k]; !ok || m != n {
			return false
		}
	}
	return true
}

// gameMods is a game's Mods directory. past: known only from finished link operations,
// so an unreadable one (a drive taken offline) is skipped rather than blocking.
type gameMods struct {
	game, mods string
	past       bool
}

// knownGameMods also serves target-lifecycle callers: it never uses scan aliases.
func (a *App) knownGameMods() ([]gameMods, error) {
	es, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	roots := []gameMods{}
	for _, e := range es {
		if e.GameRoot != "" {
			roots = append(roots, gameMods{game: e.GameRoot, mods: filepath.Join(e.GameRoot, "Mods")})
		}
	}
	for _, s := range ss {
		if s.Kind == source.Local && s.Status != source.StatusRetired && s.Root != "" {
			game := filepath.Dir(s.Root)
			for _, e := range es {
				if e.ID == s.EnvID && e.GameRoot != "" {
					game = e.GameRoot
					break
				}
			}
			roots = append(roots, gameMods{game: game, mods: s.Root})
		}
	}
	ledger, err := a.Store.ManagedLinks()
	if err != nil {
		return nil, err
	}
	for _, l := range ledger {
		past := store.LinkFinished(l.Phase)
		roots = append(roots, gameMods{filepath.Dir(l.Mods), l.Mods, past})
		if l.NextPath != "" {
			roots = append(roots, gameMods{filepath.Dir(filepath.Dir(l.NextPath)), filepath.Dir(l.NextPath), past})
		}
	}
	seen := map[string]int{}
	out := []gameMods{}
	for _, r := range roots {
		r.mods = realOrAbs(r.mods)
		key := env.PathKey(r.mods)
		if i, ok := seen[key]; ok {
			out[i].past = out[i].past && r.past
			continue
		}
		seen[key] = len(out)
		r.game = realOrAbs(r.game)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mods < out[j].mods })
	return out, nil
}

// InspectGameLinks reads every known game Mods directory afresh. Healthy links
// are included so other lifecycle operations can reuse the same observations.
func (a *App) InspectGameLinks() ([]*GameLink, error) {
	roots, err := a.knownGameMods()
	if err != nil {
		return nil, err
	}
	li, err := a.linkInstances()
	if err != nil {
		return nil, err
	}
	// A link's target is matched against the real path its instance's scan recorded and
	// confirmed on disk; only a miss resolves every instance's path again (one file-system
	// call per mod, thousands with a large library).
	var resolved map[string]*store.LinkMod
	liveTarget := func(target string) *store.LinkMod {
		key := env.PathKey(realOrAbs(target))
		if m := li.recorded[key]; m != nil && env.PathKey(realOrAbs(m.RealPath)) == key {
			return m
		}
		if resolved == nil {
			resolved = make(map[string]*store.LinkMod, len(li.live))
			for _, m := range li.live {
				resolved[env.PathKey(realOrAbs(m.RealPath))] = m
			}
		}
		return resolved[key]
	}
	// Packs' expected targets and evidence; a pack wins over an instance of the same name.
	expected, evidence, packIDs := map[string]string{}, map[string]string{}, map[string]string{}
	ledger, err := a.Store.ManagedLinks()
	if err != nil {
		return nil, err
	}
	registered := map[string]store.ManagedLink{}
	for _, l := range ledger {
		if l.Phase == "complete" && l.NextTarget != "" {
			delete(registered, env.PathKey(filepath.Join(l.Mods, l.Name)))
			registered[env.PathKey(l.NextPath)] = l
		} else if l.Phase == "rolled-back" && l.Target != "" {
			l.NextTarget = l.Target
			registered[env.PathKey(filepath.Join(l.Mods, l.Name))] = l
		} else if store.LinkFinished(l.Phase) {
			delete(registered, env.PathKey(filepath.Join(l.Mods, l.Name)))
		}
	}
	packs, err := os.ReadDir(a.PacksDir())
	packEvidenceError := ""
	if err != nil && !os.IsNotExist(err) {
		packEvidenceError = "cannot read pack ownership evidence: " + err.Error()
	}
	for _, p := range packs {
		if !p.IsDir() {
			continue
		}
		pack, err := a.ReadPack(p.Name())
		if err != nil || pack.Light {
			continue
		}
		for _, e := range pack.Entries {
			if !e.Official && e.Folder != "" {
				key := strings.ToLower(packLinkName(p.Name(), e.Folder))
				expected[key] = filepath.Join(a.packPath(p.Name()), "mods", e.Folder)
				packIDs[key] = p.Name()
				evidence[key] = "known pack " + p.Name() + "; expected target: " + expected[key]
			}
		}
	}
	out := []*GameLink{}
	for _, root := range roots {
		entries, err := os.ReadDir(root.mods)
		if err != nil {
			if os.IsNotExist(err) {
				if state, _ := source.ProbeTarget(root.mods); state == source.TargetMissing {
					continue
				}
			}
			if root.past {
				continue
			}
			out = append(out, &GameLink{Game: root.game, Mods: root.mods, Path: root.mods, State: source.TargetUnknown, Code: CodeLinkUnknown, Reason: err.Error(), Action: "keep"})
			continue
		}
		for _, de := range entries {
			name := de.Name()
			if !managedLink(name) && !packLink(name) {
				continue
			}
			l := &GameLink{Game: root.game, Mods: root.mods, Path: filepath.Join(root.mods, name), Action: "keep", State: source.TargetUnknown, Evidence: "prefix only; ownership is unproven"}
			if packLink(name) && packEvidenceError != "" {
				l.Evidence += "; " + packEvidenceError
			}
			info, err := source.ReadLink(l.Path)
			if err != nil {
				l.Code, l.Reason = CodeLinkUnknown, err.Error()
				out = append(out, l)
				continue
			}
			l.snapshot, l.Target, l.Kind, l.State = info, info.RawTarget, info.Kind, info.State
			lname := strings.ToLower(name)
			exp, ev := "", ""
			if m := li.byName[lname]; m != nil {
				l.Instance, exp, ev = m.ID, m.RealPath, "known instance "+m.ID+"; expected target: "+m.RealPath
			}
			if p, ok := expected[lname]; ok {
				l.Pack, exp, ev = packIDs[lname], p, evidence[lname]
			}
			if exp != "" {
				l.Owned, l.Evidence = true, ev
			}
			if row, ok := registered[env.PathKey(l.Path)]; ok {
				exp, l.Owned, l.Instance, l.Pack = row.NextTarget, true, row.Instance, row.Pack
				l.Evidence = "managed link ledger; operation " + row.Operation + "; expected target: " + exp
				if row.Kind != info.Kind {
					l.Owned = false
					l.Evidence += "; reparse type differs from ledger"
				}
			}
			// A surviving target proves ownership even after the name's ID was retired.
			if (info.Kind == "junction" || info.Kind == "symlink") && info.State == source.TargetPresent {
				if m := liveTarget(info.Target); m != nil {
					exp, l.Owned, l.Instance, l.Pack = m.RealPath, true, m.ID, ""
					l.Evidence = "target is known instance " + m.ID
				}
			}
			switch {
			case info.Kind == "plain":
				l.Code, l.Reason = CodeLinkOccupied, "real directory or file uses a RimArcho prefix; rename it yourself"
				// A game folder copied in Explorer turns RimArcho's links into folders holding
				// their targets' files (09 §3): such a copy may go to the recycle bin, so the
				// link can come back. Anything else under the prefix stays the player's.
				if exp != "" && sameFiles(l.Path, exp) {
					l.Copy, l.Action = exp, "recycle copy"
					l.Reason = "a copy of the folder RimArcho links here (the game folder was copied?); it can go to the recycle bin and the link is made again"
					l.Evidence += "; same files and sizes as " + exp
				}
			case info.Kind != "junction" && info.Kind != "symlink" || info.State == source.TargetUnknown:
				l.Code, l.Reason = CodeLinkUnknown, info.Reason
			case info.State == source.TargetMissing:
				l.Code, l.Reason = CodeLinkDangling, info.Reason
			case !l.Owned:
				l.Code, l.Reason = CodeLinkForeign, "no matching instance or pack"
			case env.PathKey(realOrAbs(info.Target)) != env.PathKey(realOrAbs(exp)):
				l.Code, l.Reason = CodeLinkForeign, "target differs from known instance or pack"
			default:
				l.Reason = "target exists"
			}
			// Environment links are installed local mods; a present target always stays.
			if l.Code == CodeLinkDangling || l.Code == CodeLinkForeign && !(envLink(name) && l.State == source.TargetPresent) {
				if l.Owned {
					l.Action = "remove link"
				} else {
					l.Action = "include explicitly to remove link"
				}
			}
			out = append(out, l)
		}
	}
	recoveries, err := a.linkRecoveries(ledger)
	if err != nil {
		return nil, err
	}
	for _, r := range recoveries {
		for _, l := range out {
			if env.PathKey(l.Path) == env.PathKey(r.Path) {
				l.Action = "keep"
			}
		}
	}
	return append(out, recoveries...), nil
}

// linkInstances is what the game-link check needs from every game's instances, cached
// like the mod index (Store.IndexKey: instances, sources and environments).
type linkInstances struct {
	key      string
	byName   map[string]*store.LinkMod // lower-cased link name (DeployName, env link) → instance
	recorded map[string]*store.LinkMod // live instances by their recorded real path
	live     []*store.LinkMod
}

func (a *App) linkInstances() (*linkInstances, error) {
	key := a.Store.IndexKey()
	if c := a.linkInst; c != nil && c.key == key {
		return c, nil
	}
	if c := sharedLinks(a.Store, key); c != nil {
		a.linkInst = c
		return c, nil
	}
	ms, err := a.Store.LinkMods()
	if err != nil {
		return nil, err
	}
	li := &linkInstances{key: key, byName: map[string]*store.LinkMod{}, recorded: map[string]*store.LinkMod{}}
	for _, m := range ms {
		if m.Status == identity.Live && m.RealPath != "" {
			li.recorded[env.PathKey(m.RealPath)] = m
			li.live = append(li.live, m)
		}
		li.byName[strings.ToLower(DeployName(&store.Mod{ID: m.ID, SourceLabel: m.SourceLabel, Rel: m.Rel}))] = m
		li.byName[strings.ToLower(DeployPrefix+"env_"+m.ID)] = m
	}
	a.linkInst = li
	shareLinks(a.Store, li)
	return li, nil
}

func (l *GameLink) Finding() *finding.Finding {
	severity := finding.Warning
	if l.Code == CodeLinkDangling || l.Code == CodeLinkUnknown || l.Code == CodeLinkRecovery {
		severity = finding.Error
	}
	return &finding.Finding{Code: l.Code, Severity: severity, Key: env.PathKey(l.Path), Layer: "decl", Confidence: finding.Certain,
		Subjects: []string{"path:" + l.Path, "game:" + l.Game}, Claim: l.Game + ": " + l.Path + ": " + l.Reason,
		Evidence: []finding.Evidence{{Source: finding.Inferred, Ref: l.Path, Text: "target: " + l.Target + "; " + l.Evidence}}, Suggestions: []string{"archo links check --all", "archo links repair --all --dry-run"}}
}

func (a *App) CheckGameLinks(mods string) ([]*GameLink, error) {
	all, err := a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	out := []*GameLink{}
	for _, l := range all {
		if env.PathKey(l.Mods) == env.PathKey(realOrAbs(mods)) && (l.Code == CodeLinkDangling || l.Code == CodeLinkUnknown || l.Code == CodeLinkRecovery) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (a *App) refuseUnhealthyLinks(mods string) error {
	links, err := a.CheckGameLinks(mods)
	if err != nil {
		return err
	}
	if len(links) > 0 {
		return fmt.Errorf("%w: %s: %s; check archo links check and confirm a repair", ErrLaunchBlocked, links[0].Path, links[0].Reason)
	}
	return nil
}

type LinkRepairPlan struct {
	Links   []*GameLink `json:"links"`
	Removed []string    `json:"removed"`
	// Recycled: copies of linked folders moved to the recycle bin (Action "recycle copy").
	Recycled []string `json:"recycled,omitempty"`
}

func (a *App) PlanLinkRepair(all bool, include []string) (*LinkRepairPlan, error) {
	links, err := a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	if !all {
		e, err := a.Env()
		if err != nil {
			return nil, err
		}
		selected[env.PathKey(realOrAbs(filepath.Join(e.GameRoot, "Mods")))] = true
		sources, err := a.Store.Sources(e.ID)
		if err != nil {
			return nil, err
		}
		for _, s := range sources {
			if s.Kind == source.Local && s.Status != source.StatusRetired {
				selected[env.PathKey(realOrAbs(s.Root))] = true
			}
		}
	}
	explicit := map[string]bool{}
	for _, p := range include {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		// Resolving the entry itself fails for a dangling link. Normalize its
		// surviving parent instead, including Windows 8.3 and junction aliases.
		explicit[env.PathKey(filepath.Join(realOrAbs(filepath.Dir(abs)), filepath.Base(abs)))] = true
	}
	matched := map[string]bool{}
	plan := &LinkRepairPlan{Links: []*GameLink{}, Removed: []string{}}
	for _, l := range links {
		if !all && !selected[env.PathKey(l.Mods)] {
			continue
		}
		if explicit[env.PathKey(l.Path)] {
			matched[env.PathKey(l.Path)] = true
		}
		if l.Action == "include explicitly to remove link" && explicit[env.PathKey(l.Path)] {
			l.Action = "remove link"
			l.Evidence += "; explicitly included by player"
		}
		plan.Links = append(plan.Links, l)
	}
	for path := range explicit {
		if !matched[path] {
			return nil, fmt.Errorf("included link is not a prefixed entry in the selected game scope: %s", path)
		}
	}
	return plan, nil
}

// ApplyLinkRepair uses the original preview identities. Preflight the complete
// batch before removing anything; a changed or newly healthy link cancels it. A copy moved
// out of the current game's Mods/ is read again at once, so its instance goes and the
// folder RimArcho links there takes its place (19 §6).
func (a *App) ApplyLinkRepair(plan *LinkRepairPlan) error {
	if err := a.applyLinkRepair(plan); err != nil {
		return err
	}
	if len(plan.Recycled) == 0 {
		return nil
	}
	e, err := a.Env()
	if err != nil {
		return nil
	}
	mods := env.PathKey(realOrAbs(filepath.Join(e.GameRoot, "Mods")))
	for _, p := range plan.Recycled {
		if env.PathKey(realOrAbs(filepath.Dir(p))) == mods {
			if _, _, err := a.ScanWithMonitor(ScanOptions{GameOnly: true}); err != nil {
				return fmt.Errorf("the copies are in the recycle bin, but reading the game's Mods/ again failed (run archo scan): %w", err)
			}
			return nil
		}
	}
	return nil
}

func (a *App) applyLinkRepair(plan *LinkRepairPlan) error {
	if plan == nil {
		return fmt.Errorf("missing link repair preview")
	}
	locked := map[string]bool{}
	var unlocks []func()
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()
	for _, l := range plan.Links {
		if (l.Action == "remove link" || l.Action == "recycle copy") && !locked[l.Mods] {
			unlock, err := lockGameLinks(l.Mods)
			if err != nil {
				return err
			}
			unlocks = append(unlocks, unlock)
			locked[l.Mods] = true
		}
	}
	probe := a.probe()
	for _, l := range plan.Links {
		if l.Action == "recycle copy" {
			if !managedLink(filepath.Base(l.Path)) && !packLink(filepath.Base(l.Path)) || filepath.Dir(l.Path) != l.Mods || l.Copy == "" {
				return fmt.Errorf("invalid repair entry: %s", l.Path)
			}
			if probe(l.Game) != nil {
				return fmt.Errorf("game is running: %s; stop it before repairing links", l.Game)
			}
			if fresh, err := source.ReadLink(l.Path); err != nil {
				return err
			} else if fresh.Kind != "plain" {
				return fmt.Errorf("link changed since preview: %s", l.Path)
			}
			// The preview compared names and sizes; the files themselves must match too.
			have, err := treeDigest(l.Path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", l.Path, err)
			}
			want, err := treeDigest(l.Copy)
			if err != nil {
				return fmt.Errorf("reading %s: %w", l.Copy, err)
			}
			if have != want {
				return fmt.Errorf("%s is not an exact copy of %s; nothing was moved (move it out of Mods/ yourself if you do not need it)", l.Path, l.Copy)
			}
		}
		if l.Action == "remove link" {
			if !managedLink(filepath.Base(l.Path)) && !packLink(filepath.Base(l.Path)) || filepath.Dir(l.Path) != l.Mods || l.snapshot == nil {
				return fmt.Errorf("invalid repair entry: %s", l.Path)
			}
			if probe(l.Game) != nil {
				return fmt.Errorf("game is running: %s; stop it before repairing links", l.Game)
			}
			fresh, err := source.ReadLink(l.Path)
			if err != nil {
				return err
			}
			if fresh.Kind != l.Kind || fresh.RawTarget != l.Target || fresh.State == source.TargetUnknown || l.Code == CodeLinkDangling && fresh.State != source.TargetMissing || envLink(filepath.Base(l.Path)) && fresh.State == source.TargetPresent {
				return fmt.Errorf("link changed since preview: %s", l.Path)
			}
			if err := source.VerifyLink(l.snapshot); err != nil {
				return err
			}
		}
	}
	if err := a.applyLinkRecoveries(plan); err != nil {
		return err
	}
	for _, l := range plan.Links {
		if l.Action == "remove link" {
			if a.probe()(l.Game) != nil {
				return fmt.Errorf("game is running: %s", l.Game)
			}
			state, _ := source.ProbeTarget(l.snapshot.Target)
			if state == source.TargetUnknown || l.Code == CodeLinkDangling && state != source.TargetMissing || envLink(filepath.Base(l.Path)) && state == source.TargetPresent {
				return fmt.Errorf("target changed since preview: %s", l.Path)
			}
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			op := hex.EncodeToString(id[:])
			row := store.ManagedLink{Operation: op, Mods: l.Mods, Name: filepath.Base(l.Path), Kind: l.Kind, Instance: l.Instance, Pack: l.Pack, Target: l.snapshot.Target}
			if err := a.Store.RecordLinkIntent([]store.ManagedLink{row}); err != nil {
				return err
			}
			if err := source.RemoveLinkChecked(l.snapshot); err != nil {
				a.settleFailedLink(op, l.Path, l.snapshot, true)
				return err
			}
			if err := a.Store.LinkPhase(op, "complete"); err != nil {
				return err
			}
			plan.Removed = append(plan.Removed, l.Path)
		}
		if l.Action == "recycle copy" {
			if a.probe()(l.Game) != nil {
				return fmt.Errorf("game is running: %s", l.Game)
			}
			if err := a.ToTrash(l.Path); err != nil {
				return fmt.Errorf("moving %s to the recycle bin: %w", l.Path, err)
			}
			plan.Recycled = append(plan.Recycled, l.Path)
		}
	}
	a.modsCopies = nil
	return nil
}

// RememberLinkRepair keeps filesystem identities on the server, never trusting a
// round trip through GUI JSON to reconstruct permission to delete an entry.
func (a *App) RememberLinkRepair(plan *LinkRepairPlan) string {
	for token, previous := range a.linkRepairPreviews {
		same := len(plan.Links) == len(previous.Links)
		if same {
			for i, l := range plan.Links {
				p := previous.Links[i]
				// Occupied plain entries are informational: this token permits no
				// operation on them. Removable links still require full identities.
				// A copy to recycle is checked again, file by file, when the repair runs.
				occupied := l.Code == CodeLinkOccupied && (l.Action == "keep" || l.Action == "recycle copy" && l.Copy == p.Copy) && l.Kind == "plain" && p.Kind == "plain"
				if l.Path != p.Path || l.Action != p.Action || l.Code != p.Code || !occupied && !source.SameLink(l.snapshot, p.snapshot) {
					same = false
					break
				}
			}
		}
		if same {
			return token
		}
	}
	if len(a.linkRepairPreviews) >= 512 {
		a.linkRepairPreviews = nil
	}
	if a.linkRepairPreviews == nil {
		a.linkRepairPreviews = map[string]*LinkRepairPlan{}
	}
	a.linkRepairSerial++
	token := fmt.Sprint(a.linkRepairSerial)
	a.linkRepairPreviews[token] = plan
	return token
}

func (a *App) ConfirmLinkRepair(token string) error {
	plan := a.linkRepairPreviews[token]
	delete(a.linkRepairPreviews, token)
	if plan == nil {
		return fmt.Errorf("link preview expired; reopen the needs page")
	}
	return a.ApplyLinkRepair(plan)
}

// ShortReason is the shared diagnostic wording; surfaces may localize it.
func (l *GameLink) ShortReason() string {
	switch l.Code {
	case CodeLinkDangling:
		return "target is missing"
	case CodeLinkForeign:
		if !l.Owned {
			return "unknown target; RimArcho ownership is unconfirmed"
		}
		return "target differs from the expected location"
	case CodeLinkUnknown:
		return "cannot read (offline or access denied)"
	case CodeLinkOccupied:
		if l.Action == "recycle copy" {
			return "a copy of the linked folder (the game folder was copied?)"
		}
		return "a real file or folder occupies this name"
	case CodeModsDuplicate:
		return "duplicate entry"
	case CodeLinkRecovery:
		return "link operation needs recovery"
	default:
		return "target exists"
	}
}
