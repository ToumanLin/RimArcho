// vmstate is test infrastructure: deterministic fixture generation and read-only snapshots.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
	_ "modernc.org/sqlite"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func write(p, s string) {
	check(os.MkdirAll(filepath.Dir(p), 0755))
	check(os.WriteFile(p, []byte(s), 0644))
}
func run(a *app.App, args ...string) {
	out, code := cmds.Execute(a, args)
	if code != 0 {
		panic(out)
	}
}
func fixture(root string) {
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		panic("fixture destination must not exist")
	}
	home := filepath.Join(root, "home")
	mod := func(p, pkg, name, deps string) {
		write(filepath.Join(p, "About", "About.xml"), `<ModMetaData><packageId>`+pkg+`</packageId><name>`+name+`</name><author>VM fixture</author><supportedVersions><li>1.6</li></supportedVersions>`+deps+`</ModMetaData>`)
		write(filepath.Join(p, "Defs", "Fixture.xml"), `<Defs/>`)
	}
	for _, g := range []string{"game-a", "game-b"} {
		base := filepath.Join(root, g)
		write(filepath.Join(base, "RimWorldWin64.exe"), "")
		version := "1.6.4633 rev1"
		if g == "game-b" {
			version = "1.7.0000 rev1"
		}
		write(filepath.Join(base, "Version.txt"), version)
		mod(filepath.Join(base, "Data", "Core"), "ludeon.rimworld", "Core", "")
		check(os.MkdirAll(filepath.Join(base, "Mods"), 0755))
		write(filepath.Join(root, g+"-userdata", "Config", "ModsConfig.xml"), `<ModsConfigData><version>1.6.4633</version><activeMods><li>ludeon.rimworld</li><li>vm.dependent</li><li>vm.old</li><li>vm.absent</li><li>vm.installed</li></activeMods></ModsConfigData>`)
	}
	mod(filepath.Join(root, "workshop", "2710270001"), "vm.workshop", "000-workshop", "")
	for i := 0; i < 28; i++ {
		name := fmt.Sprintf("mod-%02d", i)
		mod(filepath.Join(root, "library", name), "vm."+name, name, "")
	}
	for _, name := range []string{"old", "new", "installed", "dependency"} {
		mod(filepath.Join(root, "library", name), "vm."+name, name, "")
	}
	mod(filepath.Join(root, "library", "dependent"), "vm.dependent", "dependent", `<modDependencies><li><packageId>vm.old</packageId></li><li><packageId>vm.dependency</packageId></li><li><packageId>vm.remote</packageId><steamWorkshopUrl>steam://url/CommunityFilePage/123456</steamWorkshopUrl></li><li><packageId>vm.unknown</packageId></li></modDependencies>`)
	write(filepath.Join(home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"vm.old","newPackageId":"vm.new","newName":"new","newVersions":["1.6"]}]}`)
	a, err := app.Open(home)
	check(err)
	defer a.Close()
	a.Now = func() time.Time { return time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC) }
	a.Rand = mathrand.NewChaCha8([32]byte{2, 7, 1, 0, 2, 7})
	a.MonitorBundle = app.MonitorNone
	run(a, "config", "set", "monitor.default", "off")
	run(a, "config", "set", "ui.lang", "en")
	for _, g := range []string{"game-a", "game-b"} {
		run(a, "env", "add", g, "--game", filepath.Join(root, g), "--user-data", filepath.Join(root, g+"-userdata"))
	}
	run(a, "folder", "add", "extra", filepath.Join(root, "library"))
	run(a, "folder", "add", "workshop", filepath.Join(root, "workshop"))
	run(a, "env", "use", "game-a")
	run(a, "scan")
	// Read first: extra-folder copies stay unresolved until explicitly bound with UseInstalled.
	_, err = a.Loadout()
	check(err)
	run(a, "loadout", "use-installed", "vm.old", "vm.dependent")
	st, err := a.Loadout()
	check(err)
	check(a.Store.ClearSteps(st.Plan.ID, "undo"))
	check(a.Store.ClearSteps(st.Plan.ID, "redo"))
	run(a, "plan", "new", "alternate", "--from", "current")
	// Materialize both environments before navigation tests; opening the second game
	// must not be mistaken for a spontaneous plan creation during a read-only scenario.
	a.EnvName = "game-b"
	run(a, "scan")
	_, err = a.Loadout()
	check(err)
	a.EnvName = ""
	write(filepath.Join(root, "fixture.json"), `{"schema":1,"seed":271027,"env":"game-a","plan":"default","alternate":"alternate","toggle":"vm.mod-00"}`)
}
func rows(db *sql.DB, q string, args ...any) []map[string]any {
	r, err := db.Query(q, args...)
	check(err)
	defer r.Close()
	cols, err := r.Columns()
	check(err)
	out := []map[string]any{}
	for r.Next() {
		vals := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		check(r.Scan(ptr...))
		m := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = hex.EncodeToString(b)
			}
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	check(r.Err())
	return out
}
func snapshot(home string, openPlan ...string) any {
	// Opening App would perform startup recovery. This observer opens SQLite read-only instead.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(home, "archo.db"))+"?mode=ro")
	check(err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec("BEGIN")
	check(err)
	defer db.Exec("ROLLBACK")
	// Real baselines may predate staging history; observation must not migrate them.
	stageColumn := "NULL AS staging"
	for _, col := range rows(db, "PRAGMA table_info(workspace_undo)") {
		if col["name"] == "staging" {
			stageColumn = "staging"
			break
		}
	}
	envs := rows(db, "SELECT id,name,game_root,user_data_root,game_version,current_plan FROM environment ORDER BY name")
	sources := rows(db, "SELECT id,environment_id,kind,label,root,status,pending_root FROM source ORDER BY kind,label")
	plans := rows(db, "SELECT id,name,created_game,versions FROM plan ORDER BY name")
	ids := map[string]string{}
	roots := map[string]string{home: "HOME"}
	for _, e := range envs {
		ids[fmt.Sprint(e["id"])] = "env:" + fmt.Sprint(e["name"])
		roots[fmt.Sprint(e["game_root"])] = "GAME:" + fmt.Sprint(e["name"])
		roots[fmt.Sprint(e["user_data_root"])] = "USERDATA:" + fmt.Sprint(e["name"])
	}
	for _, s := range sources {
		ids[fmt.Sprint(s["id"])] = fmt.Sprint(s["kind"]) + ":" + fmt.Sprint(s["label"]) + ":" + ids[fmt.Sprint(s["environment_id"])]
		roots[fmt.Sprint(s["root"])] = "SOURCE:" + ids[fmt.Sprint(s["id"])]
	}
	for _, p := range plans {
		ids["plan:"+fmt.Sprint(p["id"])] = "plan:" + fmt.Sprint(p["name"])
	}
	instances := rows(db, `SELECT i.id,i.lineage_id,i.environment_id,i.source_id,i.rel_path,i.status,i.residue,i.unloadable,s.package_id,s.snapshot_hash FROM instance i LEFT JOIN snapshot s ON s.id=i.current_snapshot_id ORDER BY i.source_id,i.rel_path`)
	for _, m := range instances {
		key := ids[fmt.Sprint(m["source_id"])] + "/" + fmt.Sprint(m["rel_path"]) + "|" + fmt.Sprint(m["package_id"]) + "|" + fmt.Sprint(m["snapshot_hash"])
		ids[fmt.Sprint(m["id"])] = key
	}
	lineageMembers := map[string][]string{}
	for _, m := range instances {
		k := fmt.Sprint(m["lineage_id"])
		lineageMembers[k] = append(lineageMembers[k], ids[fmt.Sprint(m["id"])])
	}
	for k, v := range lineageMembers {
		sort.Strings(v)
		ids[k] = "lineage:" + strings.Join(v, "+")
	}

	keys := make([]string, 0, len(ids)+len(roots))
	all := map[string]string{}
	for k, v := range ids {
		all[k] = v
		keys = append(keys, k)
	}
	for k, v := range roots {
		if k != "" {
			all[k] = v
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pairs := []string{}
	for _, k := range keys {
		pairs = append(pairs, k, all[k])
	}
	replacer := strings.NewReplacer(pairs...)
	replace := replacer.Replace

	// Normalize recursively, including IDs embedded in JSON entry/staging payloads.
	var norm func(any) any
	norm = func(v any) any {
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, "[") || strings.HasPrefix(x, "{") {
				var decoded any
				if json.Unmarshal([]byte(x), &decoded) == nil {
					b, err := json.Marshal(norm(decoded))
					check(err)
					return string(b)
				}
			}
			return replace(x)
		case []any:
			for i := range x {
				x[i] = norm(x[i])
			}
		case []map[string]any:
			for _, m := range x {
				norm(m)
			}
		case map[string]any:
			// Problem ownership maps use instance IDs as keys, not only values.
			next := make(map[string]any, len(x))
			for k, v := range x {
				next[replace(k)] = norm(v)
			}
			clear(x)
			for k, v := range next {
				x[k] = v
			}
		}
		return v
	}
	for _, e := range envs {
		e["current_plan"] = ids["plan:"+fmt.Sprint(e["current_plan"])]
	}
	out := map[string]any{"schema": 1, "environments": norm(envs), "sources": norm(sources), "instances": norm(instances)}
	for _, p := range plans {
		id := p["id"]
		p["id"] = "plan:" + fmt.Sprint(p["name"])
		w := rows(db, "SELECT entries,origin_version,origin_kind,conflict_version FROM workspace WHERE plan_id=?", id)
		p["workspace"] = norm(w)
		if len(w) > 0 {
			es := loadout.Decode(fmt.Sprint(w[0]["entries"]))
			enabled := []string{}
			for _, e := range es {
				enabled = append(enabled, e.Key())
			}
			sort.Strings(enabled)
			p["enabled"] = enabled
		}
		for _, stack := range []string{"undo", "redo"} {
			r := rows(db, "SELECT entries,op,"+stageColumn+" FROM workspace_undo WHERE plan_id=? AND stack=? ORDER BY id DESC", id, stack)
			p[stack] = norm(r)
			p[stack+"Depth"] = len(r)
		}
		p["versionsHistory"] = norm(rows(db, "SELECT n,source,entries,game_version,note FROM loadout_version WHERE plan_id=? ORDER BY n", id))
		p["staging"] = norm(rows(db, "SELECT value FROM meta WHERE key=?", fmt.Sprintf("stage:%v", id)))
	}
	sort.Slice(instances, func(i, j int) bool { return fmt.Sprint(instances[i]["id"]) < fmt.Sprint(instances[j]["id"]) })
	out["plans"] = plans
	for _, table := range []string{"lineage_data", "lineage_tag", "instance_note", "plan_collection", "user_rule"} {
		out[table] = norm(rows(db, "SELECT * FROM "+table))
	}
	out["problemCounts"] = norm(rows(db, "SELECT a.environment_id,f.severity,count(*) AS count FROM finding f JOIN analysis a ON a.id=f.analysis_id GROUP BY a.environment_id,f.severity ORDER BY a.environment_id,f.severity"))
	configs := map[string]any{}
	configContents := map[string]any{}
	links := map[string]any{}
	for _, e := range rows(db, "SELECT name,user_data_root FROM environment ORDER BY name") {
		b, err := os.ReadFile(filepath.Join(fmt.Sprint(e["user_data_root"]), "Config", "ModsConfig.xml"))
		if err == nil {
			configs[fmt.Sprint(e["name"])] = fmt.Sprintf("%x", sha256.Sum256(b))
			var projection struct {
				Version    string   `xml:"version"`
				Active     []string `xml:"activeMods>li"`
				Expansions []string `xml:"knownExpansions>li"`
			}
			check(xml.Unmarshal(b, &projection))
			configContents[fmt.Sprint(e["name"])] = map[string]any{"xml": string(b), "projection": projection}
		} else {
			configs[fmt.Sprint(e["name"])] = err.Error()
		}
	}
	out["modsConfigHashes"] = configs
	out["modsConfig"] = configContents
	for _, e := range rows(db, "SELECT name,game_root FROM environment ORDER BY name") {
		path := filepath.Join(fmt.Sprint(e["game_root"]), "Mods")
		items, err := os.ReadDir(path)
		check(err)
		entries := []map[string]any{}
		for _, item := range items {
			full := filepath.Join(path, item.Name())
			info, err := os.Lstat(full)
			check(err)
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
				target, err := filepath.EvalSymlinks(full)
				entries = append(entries, map[string]any{"name": item.Name(), "kind": info.Mode().String(), "target": replace(target), "exists": err == nil})
			}
		}
		links[fmt.Sprint(e["name"])] = entries
	}
	out["links"] = links
	// Compute declaration + stored-analysis problems through the normal app/view code,
	// with a read-only DB and persisted-workspace mode: no startup/import/recovery writes.
	a := &app.App{Home: home, Store: &store.Store{DB: db}, Now: time.Now, Rand: rand.Reader, Loc: time.UTC, LazyLog: true, LogCacheOnly: true, MonitorBundle: app.MonitorNone}
	if len(openPlan) > 0 {
		a.PlanName = openPlan[0]
	}
	a.WorkspaceSnapshot()
	ps, err := a.Problems(false, nil)
	if err != nil {
		out["problems"] = map[string]any{"error": err.Error()}
	} else {
		b, err := json.Marshal(view.ProblemsOf(view.BuildNeeds(a, ps.State, ps)))
		check(err)
		var value any
		check(json.Unmarshal(b, &value))
		out["problems"] = norm(value)
		handles := map[string]string{}
		eligible := map[string]bool{}
		kinds := map[string]string{}
		for _, m := range ps.State.Ix.Mods {
			handles[m.ID] = m.PackageID
			kinds[m.ID] = string(m.Kind)
			winner := ps.State.Reg.Resolve(m.PackageID).Instance
			eligible[m.ID] = !view.ModItem(ps.State, m).Dim && (winner == "" || winner == m.ID)
		}
		out["observation"] = map[string]any{"entries": ps.State.List, "environment": ps.State.Ix.Env.ID, "plan": ps.State.Plan.ID, "packages": handles, "eligible": eligible, "kinds": kinds}
	}
	return out
}

func audit(home, root string) {
	root, err := filepath.Abs(root)
	check(err)
	within := func(path string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(home, "archo.db"))+"?mode=ro")
	check(err)
	defer db.Close()
	queries := []string{"SELECT game_root AS path FROM environment UNION SELECT user_data_root AS path FROM environment", "SELECT root AS path FROM source UNION SELECT pending_root AS path FROM source", "SELECT real_path AS path FROM instance WHERE status='live'", "SELECT value AS path FROM meta WHERE key IN ('cfg:lock.dir','cfg:monitor.dir','cfg:pack.dir','cfg:subscribed.dir')"}
	for _, q := range queries {
		for _, r := range rows(db, q) {
			path := fmt.Sprint(r["path"])
			if path == "" {
				continue
			}
			if !within(path) {
				panic("unsafe baseline path: " + path)
			}
			if resolved, err := filepath.EvalSymlinks(path); err == nil && !within(resolved) {
				panic("baseline link escapes isolation: " + path + " -> " + resolved)
			}
		}
	}
	fmt.Println("isolated paths verified")
}
func main() {
	mode := flag.String("mode", "snapshot", "fixture or snapshot")
	home := flag.String("home", "", "isolated home")
	root := flag.String("root", "", "new fixture root")
	plan := flag.String("plan", "", "open plan for problem snapshot")
	flag.Parse()
	if *mode == "audit" {
		audit(*home, *root)
		return
	}
	if *mode == "fixture" {
		p, err := filepath.Abs(*root)
		check(err)
		fixture(p)
		fmt.Println(p)
		return
	}
	if *home == "" {
		panic("--home is required")
	}
	b, err := json.MarshalIndent(snapshot(*home, *plan), "", "  ")
	check(err)
	fmt.Println(string(b))
}
