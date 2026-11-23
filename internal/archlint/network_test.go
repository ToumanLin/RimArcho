package archlint

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// transportDirs build their own http.Transport: netcfg wraps the default one (17 §2.2),
// lanzou needs its own TLS and redirect handling for the mirror, testkit is offline.
var transportDirs = map[string]bool{"internal/netcfg": true, "internal/lanzou": true, "internal/testkit": true}

// listenFuncs open a network port.
var listenFuncs = map[string]map[string]bool{
	"net":               {"Listen": true, "ListenTCP": true, "ListenUDP": true, "ListenUnix": true, "ListenPacket": true, "ListenIP": true, "ListenMulticastUDP": true, "ListenUnixgram": true},
	"net/http":          {"ListenAndServe": true, "ListenAndServeTLS": true, "Serve": true, "ServeTLS": true},
	"net/http/httptest": {"NewServer": true, "NewTLSServer": true, "NewUnstartedServer": true},
}

// listenAllowed: non-test code that opens a port. Key: "file function pkg.Name".
var listenAllowed = map[string]string{}

// networkProblems: requests go through netcfg's transport so the proxy and mirror apply
// (17 §2.2); RimArcho opens no network port (01 §3; MCP is stdio only, 14 §1; the GUI has
// no TCP port, 16 §8.3).
func networkProblems(files []*goFile, allow map[string]string, used map[string]bool) []string {
	var out []string
	for _, g := range files {
		if g.test {
			continue
		}
		imp := g.imports()
		if !transportDirs[g.dir] {
			ast.Inspect(g.f, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if s, ok := cl.Type.(*ast.SelectorExpr); ok && s.Sel.Name == "Transport" {
					if id, ok := s.X.(*ast.Ident); ok && imp[id.Name] == "net/http" {
						out = append(out, fmt.Sprintf("17 §2.2: %s builds an http.Transport; use the default client (netcfg.Install wraps http.DefaultTransport) so the proxy and GitHub mirror apply", loc(g.fset.Position(cl.Pos()), g.path)))
					}
				}
				return true
			})
		}
		for _, r := range g.refs() {
			if !listenFuncs[r.pkg][r.name] {
				continue
			}
			k := writeKey(g, r)
			if allow[k] != "" {
				used[k] = true
				continue
			}
			out = append(out, fmt.Sprintf("01 §3, 14 §1, 16 §8.3: %s in %s: %s opens a network port; RimArcho opens none (MCP is stdio only, the GUI serves WebView2 without TCP)", loc(r.pos, g.path), r.fn, r))
		}
	}
	sort.Strings(out)
	return out
}

func TestArchNetwork(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	report(t, networkProblems(repoSource(t), listenAllowed, used))
	report(t, staleProblems("listenAllowed", listenAllowed, used))
}

func TestArchNetworkDetection(t *testing.T) {
	t.Parallel()
	src := `package p
import (
	"net"
	"net/http"
	"net/http/httptest"
)
var c = &http.Client{Transport: &http.Transport{}}
var tr = http.DefaultTransport.(*http.Transport).Clone()
func serve() { l, _ := net.Listen("tcp", ":0"); http.Serve(l, nil); http.ListenAndServe(":80", nil); httptest.NewServer(nil); net.Dial("tcp", "x") }
`
	expect(t, "app", networkProblems(synthetic(t, map[string]string{"internal/app/n.go": src}), nil, map[string]bool{}), 5)
	expect(t, "netcfg", networkProblems(synthetic(t, map[string]string{"internal/netcfg/n.go": src}), nil, map[string]bool{}), 4)
	expect(t, "test file", networkProblems(synthetic(t, map[string]string{"internal/app/n_test.go": src}), nil, map[string]bool{}), 0)
	allow := map[string]string{"internal/app/n.go serve net.Listen": "x", "internal/app/n.go gone net.Listen": "y"}
	used := map[string]bool{}
	expect(t, "allowed listen", networkProblems(synthetic(t, map[string]string{"internal/app/n.go": src}), allow, used), 4)
	expect(t, "stale listen exception", staleProblems("listenAllowed", allow, used), 1)
}

// trustFields would replace what Windows already does for every request: the system root
// store (a game accelerator's local CA, a company's) and the system resolver (the hosts
// file, an accelerator's virtual adapter). RimArcho pins no certificate (17 §2.2).
var trustFields = map[string]bool{"RootCAs": true, "InsecureSkipVerify": true, "VerifyPeerCertificate": true,
	"VerifyConnection": true, "PreferGo": true}

// trustProblems: no file sets the TLS roots, verification hooks or a Go-only resolver, or
// imports the bundled fallback roots.
func trustProblems(files []*goFile) []string {
	var out []string
	for _, g := range files {
		if g.test {
			continue
		}
		for _, spec := range g.f.Imports {
			if strings.Trim(spec.Path.Value, `"`) == "golang.org/x/crypto/x509roots/fallback" {
				out = append(out, fmt.Sprintf("17 §2.2: %s imports the bundled roots; RimArcho trusts the system root store", g.path))
			}
		}
		ast.Inspect(g.f, func(n ast.Node) bool {
			var id *ast.Ident
			switch x := n.(type) {
			case *ast.KeyValueExpr:
				id, _ = x.Key.(*ast.Ident)
			case *ast.SelectorExpr:
				id = x.Sel
			}
			if id != nil && trustFields[id.Name] {
				out = append(out, fmt.Sprintf("17 §2.2: %s sets %s; requests use the system root store and resolver, and no certificate is pinned", loc(g.fset.Position(id.Pos()), g.path), id.Name))
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

func TestArchNetworkTrust(t *testing.T) {
	t.Parallel()
	report(t, trustProblems(repoSource(t)))
	root, _ := repoRoot(t)
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"netdns", "x509usefallbackroots"} {
		if strings.Contains(string(mod), k) {
			t.Errorf("17 §2.2: go.mod sets the GODEBUG %s; RimArcho uses the system resolver and root store", k)
		}
	}
}

func TestArchNetworkTrustDetection(t *testing.T) {
	t.Parallel()
	src := `package p
import (
	"crypto/tls"
	"net"
	_ "golang.org/x/crypto/x509roots/fallback"
)
var c = &tls.Config{InsecureSkipVerify: true}
var r = &net.Resolver{PreferGo: true}
func f(c *tls.Config) { c.RootCAs = nil }
`
	expect(t, "trust", trustProblems(synthetic(t, map[string]string{"internal/app/n.go": src})), 4)
}
