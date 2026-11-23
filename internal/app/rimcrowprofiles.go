package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// RCProfile is one profile preview, grouped by its physical installation path (03 §6.4).
type RCProfile struct {
	DisplayStatus                                       string `json:",omitempty"`
	ID, Name, Game, UserData, Args, Environment, Status string
	Tokens                                              []string
	Unresolved                                          []string
	ConfigHash                                          string
	list                                                *ImportedList
	launch                                              string
	key                                                 string
	resume                                              *env.Environment // an interrupted import of this profile registered it
}

func crowEnvironmentName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(":#/\\", r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	name = strings.Trim(name, "-.")
	if name == "" {
		return "rimcrow"
	}
	return name
}

// crowProfilePreview is what the profile previews share: the loadout state per game, and
// the Game Config names and installation/user data pairs already taken.
type crowProfilePreview struct {
	states map[string]*LoadoutState
	names  map[string]bool
	pairs  map[string]bool
}

func (a *App) importCrowProfiles(db *sql.DB, opt RimCrowOptions, rep *RimCrowReport) error {
	var exists int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='gameprofile'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	if err := readCrowProfiles(db, rep); err != nil {
		return err
	}
	pv := &crowProfilePreview{states: map[string]*LoadoutState{}, names: map[string]bool{}, pairs: map[string]bool{}}
	for i := range rep.Profiles {
		if err := a.previewCrowProfile(opt, rep, i, pv); err != nil {
			return err
		}
	}
	if opt.DryRun {
		return nil
	}
	scanned := map[string]bool{}
	for i := range rep.Profiles {
		p := &rep.Profiles[i]
		if p.Status != "ready" {
			continue
		}
		if err := a.applyCrowProfile(p, scanned); err != nil {
			return err
		}
	}
	return nil
}

// readCrowProfiles reads RimCrow's game profiles into rep, sorted by installation path.
func readCrowProfiles(db *sql.DB, rep *RimCrowReport) error {
	rows, err := db.Query(`SELECT id,name,game_install_path,user_data_path,COALESCE(run_commands,'[]'),prefer_steam_launch,use_workshop_mods,use_self_mods FROM gameprofile ORDER BY game_install_path,id`)
	if err != nil {
		return fmt.Errorf("RimCrow profiles: %w", err)
	}
	for rows.Next() {
		var p RCProfile
		var steam, workshop, self bool
		if err := rows.Scan(&p.ID, &p.Name, &p.Game, &p.UserData, &p.Args, &steam, &workshop, &self); err != nil {
			rows.Close()
			return err
		}
		p.launch = "direct"
		if steam {
			p.launch = "steam"
		}
		rep.Profiles = append(rep.Profiles, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i := range rep.Profiles {
		if filepath.IsAbs(rep.Profiles[i].Game) {
			rep.Profiles[i].Game = realOrAbs(rep.Profiles[i].Game)
		}
	}
	sort.SliceStable(rep.Profiles, func(i, j int) bool { return canonicalRoot(rep.Profiles[i].Game) < canonicalRoot(rep.Profiles[j].Game) })
	return nil
}

// previewCrowProfile decides profile i's status: skipped (and why), already imported, or
// ready to import.
func (a *App) previewCrowProfile(opt RimCrowOptions, rep *RimCrowReport, i int, pv *crowProfilePreview) error {
	p := &rep.Profiles[i]
	if opt.SelectProfiles && !slices.Contains(opt.ProfileIDs, p.ID) {
		p.Status = "skipped: not selected"
		return nil
	}
	p.Environment = crowEnvironmentName(p.Name)
	p.key = fmt.Sprintf("import:rimcrow:profile:%x", sha256.Sum256([]byte(canonicalRoot(opt.DB)+"\x00"+p.ID)))
	if done, err := a.previewMappedCrowProfile(p, rep, pv); err != nil || done {
		return err
	}
	skip, err := a.checkCrowProfile(p, pv)
	if err != nil {
		return err
	}
	if skip == "" && crowUserDataOverlaps(rep, i) {
		skip = "skipped: user data folders overlap; choose separate folders in RimCrow"
	}
	if skip != "" {
		p.Status = skip
		return nil
	}
	if skip := p.readModsConfig(); skip != "" {
		p.Status = skip
		return nil
	}
	if err := a.crowUnresolved(p, pv); err != nil {
		return err
	}
	pv.names[strings.ToLower(p.Environment)] = true
	pv.pairs[canonicalRoot(p.Game)+"\x00"+canonicalRoot(p.UserData)] = true
	p.Status = "ready"
	rep.NotMapped["profile source flags (use_workshop_mods/use_self_mods)"]++
	return nil
}

// previewMappedCrowProfile handles a profile an earlier import mapped to a Game Config. It
// reports whether the profile's status is decided; otherwise it is imported afresh.
func (a *App) previewMappedCrowProfile(p *RCProfile, rep *RimCrowReport, pv *crowProfilePreview) (bool, error) {
	// The mapping is written before the Game Config is registered, and ":complete"
	// records the steps done after it ("args", then "yes").
	mapped := a.Store.Meta(p.key)
	if mapped == "" {
		return false, nil
	}
	known, err := a.Store.EnvByID(mapped)
	if err != nil {
		return false, err
	}
	switch stage := a.Store.Meta(p.key + ":complete"); {
	case stage == "yes" && known == nil:
		p.Status = "previously imported Game Config was removed; not recreated automatically"
		return true, nil
	case stage == "yes":
		p.Status = "already imported; player edits kept"
		p.Environment = known.Name
		return true, nil
	case known != nil:
		// Interrupted after registering: finish the missing steps on the
		// registered Game Config, keeping its folders and the player's changes.
		if a.resumeCrowProfile(p, known, stage) {
			pv.names[strings.ToLower(p.Environment)] = true
			pv.pairs[canonicalRoot(p.Game)+"\x00"+canonicalRoot(p.UserData)] = true
			rep.NotMapped["profile source flags (use_workshop_mods/use_self_mods)"]++
		}
		return true, nil
	}
	// Interrupted before registering (a failed registration is undone): import afresh.
	return false, nil
}

// checkCrowProfile checks a profile's folders, launch, arguments and name; it returns the
// status of a profile that is skipped ("" none).
func (a *App) checkCrowProfile(p *RCProfile, pv *crowProfilePreview) (string, error) {
	if strings.TrimSpace(p.Game) == "" {
		return "skipped: unconfigured game folder", nil
	}
	if !filepath.IsAbs(p.Game) || p.UserData != "" && !filepath.IsAbs(p.UserData) {
		return "skipped: paths must be absolute; fix the profile in RimCrow", nil
	}
	if !env.LooksLikeGameRoot(p.Game) {
		return "skipped: game folder is missing or is not RimWorld", nil
	}
	if p.UserData == "" {
		p.UserData = a.DefaultUserData(p.Game)
	}
	if err := a.checkRoots(p.Game, p.UserData); err != nil {
		return "skipped: " + err.Error(), nil
	}
	if err := a.checkEnvironmentRoot("", p.Game, p.UserData); err != nil {
		return "skipped: " + err.Error(), nil
	}
	if err := a.checkProtonUserData(p.Game, p.UserData); err != nil {
		return "skipped: " + err.Error(), nil
	}
	if env.IsProtonGame(p.Game) && p.launch != "steam" {
		return "skipped: Proton requires Steam launch", nil
	}
	if err := p.normaliseArgs(); err != nil {
		return "skipped: " + err.Error(), nil
	}
	existing, err := a.Store.EnvByName(p.Environment)
	if err != nil {
		return "", err
	}
	if existing != nil || pv.names[strings.ToLower(p.Environment)] {
		return "skipped: Game Config name collision; rename the profile in RimCrow", nil
	}
	if pv.pairs[canonicalRoot(p.Game)+"\x00"+canonicalRoot(p.UserData)] {
		return "skipped: another profile uses this installation and user data folder", nil
	}
	return "", nil
}

// crowUserDataOverlaps reports whether profile i's user data folder lies inside, or holds,
// that of a profile before it that is ready.
func crowUserDataOverlaps(rep *RimCrowReport, i int) bool {
	p := &rep.Profiles[i]
	overlap := false
	for j := 0; j < i; j++ {
		o := &rep.Profiles[j]
		if o.Status != "ready" {
			continue
		}
		r, q := canonicalRoot(p.UserData), canonicalRoot(o.UserData)
		if r != q && (underDir(r, q) || underDir(q, r)) {
			overlap = true
		}
	}
	return overlap
}

// readModsConfig reads the profile's Config/ModsConfig.xml as its list; it returns the
// status of a profile that is skipped ("" none).
func (p *RCProfile) readModsConfig() string {
	raw, err := os.ReadFile(filepath.Join(p.UserData, "Config", "ModsConfig.xml"))
	if err != nil {
		return "skipped: missing or unreadable Config/ModsConfig.xml: " + err.Error()
	}
	p.list, err = ParseList(raw)
	if err != nil {
		return "skipped: unreadable Config/ModsConfig.xml: " + err.Error()
	}
	p.ConfigHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	p.Tokens = append([]string(nil), p.list.Tokens...)
	return ""
}

// crowUnresolved lists the profile's entries that a known Game Config of its game cannot
// resolve.
func (a *App) crowUnresolved(p *RCProfile, pv *crowProfilePreview) error {
	known, err := a.envOfGame(p.Game)
	if err != nil {
		return err
	}
	if known == nil {
		return nil
	}
	st := pv.states[known.GameID]
	if st == nil {
		ix, err := a.loadIndex(known)
		if err != nil {
			return err
		}
		reg, _, _, _, err := a.registry(ix)
		if err != nil {
			return err
		}
		st = &LoadoutState{Ix: ix, Reg: reg}
		pv.states[known.GameID] = st
	}
	entries, _ := ImportedEntries(st, p.list)
	for _, entry := range entries {
		if entry.Missing != "" {
			p.Unresolved = append(p.Unresolved, entry.Missing)
		}
	}
	return nil
}

// applyCrowProfile imports a ready profile: registers its Game Config (or resumes the one an
// interrupted import registered), stores the launch arguments and gives it its list.
func (a *App) applyCrowProfile(p *RCProfile, scanned map[string]bool) error {
	e := p.resume
	if e == nil {
		var err error
		if e, err = a.registerCrowProfile(p); err != nil {
			return err
		}
	}
	if err := testFault(a, "profile-registered"); err != nil {
		return err
	}
	if a.Store.Meta(p.key+":complete") == "" {
		if err := a.SetLaunchArgs(e.ID, p.Args); err != nil {
			return err
		}
		if err := a.Store.SetMeta(p.key+":complete", "args"); err != nil {
			return err
		}
	}
	reader, err := a.ReadSession()
	if err != nil {
		return err
	}
	reader.EnvName = e.Name
	// Read before the first scan below, which may give the Game Config a first list.
	hadPlan := a.Store.CurrentPlanID(e.ID) != 0
	err = func() error {
		defer reader.Close()
		return a.initCrowProfile(reader, p, e, hadPlan, scanned)
	}()
	if err != nil {
		p.Status = "incomplete: " + err.Error()
		return fmt.Errorf("profile %s registered but initialization failed; importing again finishes Game Config %s: %w", p.Name, e.Name, err)
	}
	if err := a.Store.SetMeta(p.key+":complete", "yes"); err != nil {
		return err
	}
	p.Status = "imported"
	return nil
}

// registerCrowProfile records the profile's mapping, then registers its Game Config. An
// interruption leaves either no Game Config with this ID (the next import starts afresh)
// or one the next import resumes.
func (a *App) registerCrowProfile(p *RCProfile) (*env.Environment, error) {
	id := a.NewID()
	if err := a.Store.SetMeta(p.key, id); err != nil {
		return nil, err
	}
	if err := a.Store.SetMeta(p.key+":complete", ""); err != nil {
		return nil, err
	}
	if err := testFault(a, "profile-mapped"); err != nil {
		return nil, err
	}
	e, _, err := a.EnvAdd(p.Environment, EnvSpec{GameRoot: p.Game, UserDataRoot: p.UserData, Launch: p.launch, deferPlan: true, id: id})
	if err != nil {
		return nil, fmt.Errorf("profile %s: %w", p.Name, err)
	}
	return e, nil
}

// initCrowProfile scans the game once if it never was, and, unless the Game Config has a
// list already (hadPlan), creates and selects one from the profile's ModsConfig.xml.
func (a *App) initCrowProfile(reader *App, p *RCProfile, e *env.Environment, hadPlan bool, scanned map[string]bool) error {
	scan, _, err := reader.LastScan(e.GameID)
	if err != nil {
		return err
	}
	if scan == nil && !scanned[e.GameID] {
		if _, err := reader.Scan(false); err != nil {
			return err
		}
		scanned[e.GameID] = true
	}
	ix, err := reader.Index()
	if err != nil {
		return err
	}
	// Resolve without Loadout: reading the adopted root must not synchronize another plan.
	reg, _, _, _, err := reader.registry(ix)
	if err != nil {
		return err
	}
	if hadPlan {
		return nil // the interrupted import, or the player, selected its list already
	}
	st := &LoadoutState{Ix: ix, Reg: reg}
	name, err := reader.siblingPlanName(p.Name)
	if err != nil {
		return err
	}
	list, _ := ImportedEntries(st, p.list)
	p.Unresolved = nil
	for _, entry := range list {
		if entry.Missing != "" {
			p.Unresolved = append(p.Unresolved, entry.Missing)
		}
	}
	plan, err := reader.createPlan(e.ID, name, list, &store.Version{Source: "import", Note: "RimCrow profile " + p.ID, GameVersion: p.list.Game})
	if err != nil {
		return err
	}
	if err := reader.Store.SetCurrentPlan(e.ID, plan.ID); err != nil {
		return err
	}
	return testFault(a, "profile-plan")
}

// normaliseArgs stores RimCrow's run_commands as a JSON argv (03 §6.4) and refuses
// arguments that override the Game Config's folders.
func (p *RCProfile) normaliseArgs() error {
	var argv []string
	if err := json.Unmarshal([]byte(p.Args), &argv); err != nil {
		return err
	}
	if argv == nil {
		argv = []string{}
	}
	data, _ := json.Marshal(argv)
	p.Args = string(data)
	return ValidateLaunchArgs(p.Environment, p.Args)
}

// resumeCrowProfile prepares an interrupted import of p, registered as known, to finish:
// the launch arguments when stage says they were not stored yet, and the list from the
// adopted Config/ModsConfig.xml when the Game Config has no current list. It reports
// whether the profile is ready.
func (a *App) resumeCrowProfile(p *RCProfile, known *env.Environment, stage string) bool {
	p.resume = known
	p.Environment, p.Game, p.UserData = known.Name, known.GameRoot, known.UserDataRoot
	if stage == "" {
		if err := p.normaliseArgs(); err != nil {
			p.Status = "skipped: " + err.Error()
			return false
		}
	}
	if a.Store.CurrentPlanID(known.ID) == 0 {
		raw, err := os.ReadFile(filepath.Join(p.UserData, "Config", "ModsConfig.xml"))
		if err != nil {
			p.Status = "skipped: missing or unreadable Config/ModsConfig.xml: " + err.Error()
			return false
		}
		if p.list, err = ParseList(raw); err != nil {
			p.Status = "skipped: unreadable Config/ModsConfig.xml: " + err.Error()
			return false
		}
		p.ConfigHash = fmt.Sprintf("%x", sha256.Sum256(raw))
		p.Tokens = append([]string(nil), p.list.Tokens...)
	}
	p.Status = "ready"
	return true
}
