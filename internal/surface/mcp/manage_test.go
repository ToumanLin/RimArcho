package mcp_test

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

var freeRe = regexp.MustCompile(`(space: )[0-9.]+ [KMGT]?B`)

var update = flag.Bool("update", false, "rewrite golden files")

// manageEnv is the fixture for the tools of 05 §1.2: a lineage with two copies, a mod that
// comes back under a new package id (an identity question), a hidden subscribed mod, a
// hidden unsubscribed one, a Workshop collection server and a RimCrow database.
func manageEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony", Files: map[string]string{"Assemblies/0Harmony.dll": "MZ"}})
	a := testkit.Sample("tester.a", "A")
	a.Deps = []string{"brrainz.harmony"}
	ws := e.AddWorkshop("1001", a)
	e.CopyTree(ws, filepath.Join(e.Game, "Mods", "A Local"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "B"), testkit.Sample("tester.b", "B"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Leftover"), testkit.Sample("tester.leftover", "Leftover"))
	hidden := e.AddWorkshop("4300", testkit.Sample("tester.alone", "Alone"))
	e.Move(filepath.Join(hidden, "About", "About.xml"), filepath.Join(hidden, "About", "About.xml.disabled"))
	gone := e.AddWorkshop("4200", testkit.Sample("tester.gone", "Gone"))
	e.Move(filepath.Join(gone, "About", "About.xml"), filepath.Join(gone, "About", "About.xml.disabled"))
	e.Unsubscribe("4200")
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a")
	e.AddEnv(false)
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("scan")
	// Leftover goes away and comes back in another folder under another package id.
	e.Advance(time.Hour)
	e.Remove(filepath.Join(e.Game, "Mods", "Leftover"))
	e.MustRun("scan")
	e.Advance(time.Hour)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Leftover Again"), testkit.Sample("tester.leftover2", "Leftover"))

	e.App.TrashFunc = os.RemoveAll
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/collection":
			w.Write([]byte(`{"response":{"result":1,"resultcount":1,"collectiondetails":[{"publishedfileid":"3000000001","result":1,"children":[
{"publishedfileid":"1001","sortorder":2,"filetype":0},{"publishedfileid":"2009463077","sortorder":1,"filetype":0},{"publishedfileid":"5550001","sortorder":3,"filetype":0}]}]}}`))
		case "/api":
			var b []string
			for i := 0; ; i++ {
				id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
				if id == "" {
					break
				}
				b = append(b, `{"publishedfileid":"`+id+`","result":1,"title":"Title `+id+`"}`)
			}
			w.Write([]byte(`{"response":{"publishedfiledetails":[` + strings.Join(b, ",") + `]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"

	crow := filepath.Join(e.Root, "RimCrow", "data")
	os.MkdirAll(crow, 0o755)
	db, err := sql.Open("sqlite", filepath.Join(crow, "mod_manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range []string{
		`CREATE TABLE modasset (path_hash VARCHAR(255) PRIMARY KEY, package_id VARCHAR(255) NOT NULL, path VARCHAR(255) NOT NULL, source VARCHAR(255) NOT NULL, disabled INTEGER NOT NULL)`,
		`CREATE TABLE modinterlock (id VARCHAR(255) PRIMARY KEY, chain TEXT NOT NULL)`,
		`CREATE TABLE usermoddata (mod_id VARCHAR(255) NOT NULL PRIMARY KEY COLLATE NOCASE, alias_name VARCHAR(255), notes TEXT, tags TEXT NOT NULL, sign_color VARCHAR(255), user_mod_type VARCHAR(255), interlock_id VARCHAR(255), ignored_issues TEXT NOT NULL)`,
		`CREATE TABLE groupdata (group_id TEXT PRIMARY KEY, name VARCHAR(255) NOT NULL, color VARCHAR(255) NOT NULL, sort_index INTEGER NOT NULL, is_expanded INTEGER NOT NULL)`,
		`CREATE TABLE groupmod (group_id VARCHAR(255) NOT NULL, mod_id VARCHAR(255) NOT NULL, sort_index INTEGER NOT NULL, PRIMARY KEY (group_id, mod_id))`,
		`INSERT INTO usermoddata VALUES ('tester.b','Bee','a note','[]',NULL,NULL,NULL,'[]')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return e
}

type manageStep struct {
	tool string
	args map[string]any
	cli  []string
}

// manageSteps exercises every new tool and every action. Refs are looked up in e, so the
// MCP fixture and the CLI fixture each get their own (identical) refs.
func manageSteps(e *testkit.Env) []func() manageStep {
	r := func(name string) string { return ref(e.T.(*testing.T), e, name) }
	pick := func(name, label string) string {
		ix, _ := e.App.Index()
		for _, m := range ix.Mods {
			if m.Name == name && m.SourceLabel == label {
				return ix.Ref(m.ID)
			}
		}
		e.T.Fatalf("no %s copy of %s", label, name)
		return ""
	}
	lineage := func(name, label string) string {
		ix, _ := e.App.Index()
		for _, m := range ix.Mods {
			if m.Name == name && m.SourceLabel == label {
				return ix.LRef(m.LineageID)
			}
		}
		e.T.Fatalf("no %s copy of %s", label, name)
		return ""
	}
	problem := func(code string) string {
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			e.T.Fatal(err)
		}
		for _, p := range ps.List {
			if p.Code == code {
				return p.Ref
			}
		}
		e.T.Fatalf("no %s problem", code)
		return ""
	}
	var ignored string
	list := filepath.Join(e.Root, "list.txt")
	light := filepath.Join(e.Root, "first.archopack.json")
	crowDB := filepath.Join(e.Root, "RimCrow", "data", "mod_manager.db")
	s := func(tool string, args map[string]any, cli ...string) func() manageStep {
		return func() manageStep { return manageStep{tool, args, cli} }
	}
	return []func() manageStep{
		s("status", map[string]any{"show": "envs"}, "env", "list"),
		s("status", map[string]any{"show": "settings"}, "config", "get"),
		s("status", map[string]any{"show": "data"}, "data", "status"),
		s("scan", nil, "scan"),
		s("resolve_identity", map[string]any{"action": "list"}, "identity", "list"),
		s("resolve_identity", map[string]any{"action": "resolve", "question": "q:1", "answer": "same"}, "identity", "resolve", "q:1", "same"),
		func() manageStep {
			a := pick("A", "local")
			return manageStep{"resolve_identity", map[string]any{"action": "split", "refs": []string{a}}, []string{"lineage", "split", a}}
		},
		func() manageStep {
			l, a := lineage("A", "workshop"), pick("A", "local")
			return manageStep{"resolve_identity", map[string]any{"action": "merge", "refs": []string{l, a}}, []string{"lineage", "merge", l, a}}
		},
		func() manageStep {
			a, b := r("B"), r("Harmony")
			return manageStep{"manage_rules", map[string]any{"action": "add", "kind": "after", "a": a, "b": b, "note": "test"}, []string{"rule", "add", "after", a, b, "--note", "test"}}
		},
		s("manage_rules", map[string]any{"action": "list"}, "rule", "list"),
		s("add_rule", map[string]any{"action": "remove", "rule": "r:1"}, "rule", "remove", "r:1"),
		func() manageStep {
			ignored = problem("mod.hidden")
			return manageStep{"ignore_problem", map[string]any{"ref": ignored}, []string{"problems", "ignore", ignored}}
		},
		func() manageStep {
			p := ignored
			return manageStep{"ignore_problem", map[string]any{"ref": p, "undo": true}, []string{"problems", "unignore", p}}
		},
		s("exchange_list", map[string]any{"action": "export"}, "loadout", "export"),
		s("exchange_list", map[string]any{"action": "export", "format": "rml", "out": list}, "loadout", "export", "--format", "rml", "--out", list),
		s("exchange_list", map[string]any{"action": "collection", "from": "3000000001"}, "collection", "3000000001"),
		s("exchange_list", map[string]any{"action": "collection", "from": "3000000001", "import": true}, "collection", "3000000001", "--import"),
		s("exchange_list", map[string]any{"action": "import", "from": list}, "loadout", "import", list),
		s("manage_hidden", map[string]any{"action": "unhide", "ref": "ws:4300", "dry_run": true}, "unhide", "ws:4300", "--dry-run"),
		s("manage_hidden", map[string]any{"action": "unhide", "ref": "ws:4300"}, "unhide", "ws:4300"),
		s("manage_hidden", map[string]any{"action": "trash", "ref": "ws:4200", "dry_run": true}, "trash", "ws:4200", "--dry-run"),
		s("manage_hidden", map[string]any{"action": "trash", "ref": "ws:4200"}, "trash", "ws:4200"),
		s("manage_packs", map[string]any{"action": "list"}, "pack", "list"),
		s("manage_packs", map[string]any{"action": "build", "name": "first", "dry_run": true}, "pack", "build", "first", "--dry-run"),
		s("manage_packs", map[string]any{"action": "build", "name": "first"}, "pack", "build", "first"),
		s("manage_packs", map[string]any{"action": "launch", "name": "first", "dry_run": true}, "pack", "launch", "first", "--dry-run"),
		s("manage_packs", map[string]any{"action": "launch", "name": "first"}, "pack", "launch", "first"),
		s("manage_packs", map[string]any{"action": "export", "name": "first", "file": light, "light": true}, "pack", "export", "first", light, "--light"),
		s("manage_packs", map[string]any{"action": "import", "file": light}, "pack", "import", light),
		s("manage_packs", map[string]any{"action": "stop", "name": "first"}, "pack", "stop", "first"),
		s("manage_packs", map[string]any{"action": "remove", "name": "first", "dry_run": true}, "pack", "remove", "first", "--dry-run"),
		s("manage_packs", map[string]any{"action": "remove", "name": "first"}, "pack", "remove", "first"),
		s("manage_packs", map[string]any{"action": "list"}, "pack", "list"),
		s("maintenance", map[string]any{"action": "data_update", "datasets": []string{"rules"}, "dry_run": true}, "data", "update", "rules", "--dry-run"),
		s("maintenance", map[string]any{"action": "import_rimcrow", "db": crowDB, "dry_run": true}, "import", "rimcrow", "--db", crowDB, "--dry-run"),
		s("maintenance", map[string]any{"action": "import_rimcrow", "db": crowDB}, "import", "rimcrow", "--db", crowDB),
		s("maintenance", map[string]any{"action": "gc", "dry_run": true}, "gc", "--dry-run"),
		s("maintenance", map[string]any{"action": "gc"}, "gc"),
		// 0.9: history, going back to a version, plans (I10) and launching.
		func() manageStep {
			h := r("Harmony")
			return manageStep{"edit_loadout", map[string]any{"action": "disable", "refs": []string{h}}, []string{"disable", h}}
		},
		s("plan_history", nil, "loadout", "log"),
		s("plan_history", map[string]any{"action": "steps"}, "history"),
		s("plan_history", map[string]any{"action": "diff"}, "loadout", "diff"),
		s("plan_history", map[string]any{"action": "diff", "from": "v1", "to": "workspace"}, "loadout", "diff", "v1", "workspace"),
		s("edit_loadout", map[string]any{"action": "checkout", "version": "v1"}, "loadout", "checkout", "v1"),
		s("plan_history", map[string]any{"action": "steps"}, "history"),
		s("manage_plans", nil, "plan", "list"),
		s("manage_plans", map[string]any{"action": "new", "name": "copy", "from": "current"}, "plan", "new", "copy", "--from", "current"),
		s("manage_plans", map[string]any{"action": "new", "name": "first", "from": "v1"}, "plan", "new", "first", "--from", "v1"),
		s("manage_plans", map[string]any{"action": "new", "name": "empty"}, "plan", "new", "empty"),
		s("manage_plans", map[string]any{"action": "use", "name": "copy"}, "plan", "use", "copy"),
		s("manage_plans", map[string]any{"action": "rename", "name": "copy", "new_name": "raid night"}, "plan", "rename", "copy", "raid night"),
		s("manage_plans", map[string]any{"action": "remove", "name": "empty"}, "plan", "remove", "empty", "--yes"),
		s("manage_plans", map[string]any{"action": "import", "from": list, "name": "from file"}, "loadout", "import", list, "from file", "--new"),
		s("manage_plans", map[string]any{"action": "list"}, "plan", "list"),
		s("manage_plans", map[string]any{"action": "use", "name": "raid night"}, "plan", "use", "raid night"),
		s("launch_game", map[string]any{"dry_run": true}, "launch", "--dry-run"),
		s("launch_game", nil, "launch"),
		s("plan_history", nil, "loadout", "log"),
		// 06 §2.9: the library, moving a spare copy into the lock library, removing from plans.
		s("list_library", map[string]any{"include": []string{"state:unused"}}, "library", "--include", "state:unused"),
		func() manageStep {
			a := pick("A", "local")
			return manageStep{"manage_library", map[string]any{"action": "move_to_locked", "refs": []string{a}, "dry_run": true}, []string{"library", "move-to-locked", a, "--dry-run"}}
		},
		func() manageStep {
			a := pick("A", "local")
			return manageStep{"manage_library", map[string]any{"action": "move_to_locked", "refs": []string{a}}, []string{"library", "move-to-locked", a}}
		},
		func() manageStep {
			l := lineage("A", "workshop")
			return manageStep{"list_library", map[string]any{"lineage": l}, []string{"library", "--lineage", l}}
		},
		func() manageStep {
			h := r("Harmony")
			return manageStep{"manage_library", map[string]any{"action": "remove_from_plans", "refs": []string{h}}, []string{"library", "remove-from-plans", h}}
		},
		func() manageStep {
			g := r("Gone")
			return manageStep{"manage_library", map[string]any{"action": "subscribe", "refs": []string{g}, "dry_run": true}, []string{"workshop", "subscribe", g, "--dry-run"}}
		},
		func() manageStep {
			a := pick("A", "locked")
			return manageStep{"set_player_data", map[string]any{"action": "copy_note", "refs": []string{a}, "text": "kept for the 1.5 save"}, []string{"note", a, "kept for the 1.5 save", "--copy"}}
		},
		func() manageStep {
			l := lineage("A", "workshop")
			return manageStep{"list_library", map[string]any{"lineage": l, "sort": "-size"}, []string{"library", "--lineage", l, "--sort", "-size"}}
		},
	}
}

// TestGoldenManageTools runs every new tool of 05 §1.2 through MCP on one fixture and the
// matching CLI command on an identical one: the texts must be the same (I1), and the MCP
// transcript is compared with testdata/golden/mcp_manage.txt.
func TestGoldenManageTools(t *testing.T) {
	em, ec := manageEnv(t), manageEnv(t)
	cs := connect(t, em)
	sm, sc := manageSteps(em), manageSteps(ec)
	var sb strings.Builder
	for i := range sm {
		m, c := sm[i](), sc[i]()
		got, isErr := call(t, cs, m.tool, m.args)
		want, code := ec.Run(c.cli...)
		// The free space on the packs drive is the test machine's.
		got, want = freeRe.ReplaceAllString(em.Scrub(got), "$1<free>"), freeRe.ReplaceAllString(ec.Scrub(want), "$1<free>")
		if got != want {
			t.Errorf("%s %v: MCP text differs from `archo %s`:\n--- mcp\n%s\n--- cli\n%s", m.tool, m.args, strings.Join(c.cli, " "), got, want)
		}
		if isErr != (code == cmds.ExitError || code == cmds.ExitUsage) {
			t.Errorf("%s %v: isError=%v, exit %d", m.tool, m.args, isErr, code)
		}
		args, _ := json.Marshal(m.args)
		if m.args == nil {
			args = []byte("{}")
		}
		var unq map[string]any
		json.Unmarshal(args, &unq)
		for k, v := range unq {
			if s, ok := v.(string); ok {
				unq[k] = em.Scrub(s)
			}
		}
		args, _ = json.Marshal(unq)
		fmt.Fprintf(&sb, "> %s %s\n", m.tool, args)
		sb.WriteString(got)
		if isErr {
			sb.WriteString("[isError]\n")
		}
		sb.WriteString("\n")
	}
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "golden", "mcp_manage.txt")
	if *update {
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/surface/mcp -update)", err)
	}
	if want := strings.ReplaceAll(string(b), "\r\n", "\n"); sb.String() != want {
		t.Errorf("mcp_manage.txt differs from golden (go test ./internal/surface/mcp -update):\n%s", sb.String())
	}
}
