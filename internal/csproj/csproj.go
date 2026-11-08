// Package csproj reads a Mod's C# build files as data: no MSBuild, no dotnet, no network.
// It parses a .csproj (and the Directory.Build.props, Directory.Packages.props and local
// imports it names) far enough to say what a build needs and where its output lands, and
// finds where a Mod keeps its source (05 §5.5). Property functions and conditions it does
// not understand are left as written and listed in Project.Unresolved.
package csproj

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Package is a PackageReference.
type Package struct {
	Name    string
	Version string // empty when no Version attribute, element or central version exists
}

// Reference is a <Reference> to an assembly outside NuGet.
type Reference struct {
	Name      string
	HintPath  string // expanded, as written otherwise
	Publicize bool   // Publicize="true" (BepInEx.AssemblyPublicizer.MSBuild)
	Missing   bool   // HintPath is fully resolved and nothing is there
}

// Project is one .csproj as far as a static read goes.
type Project struct {
	File string // absolute path of the .csproj
	Dir  string // its folder
	Name string // file name without extension
	Err  string // the file could not be read or parsed

	SDK          string // the Sdk attribute ("" for an old-style project)
	Legacy       bool   // old style: no Sdk attribute, every source file listed in the project
	Frameworks   []string
	AssemblyName string
	OutputType   string
	LangVersion  string

	// Output is the folder the DLL lands in as the project says it (after the SDK adds the
	// framework folder); OutputAbs is its absolute path, empty when the project names it through
	// something unresolved. OutputDefault: the project sets none (bin/Release/<framework>/).
	Output        string
	OutputAbs     string
	OutputDefault bool
	TFMAppended   bool // the SDK adds the framework folder to Output

	Packages       []Package
	Publicizer     bool     // a publicizer package or item is present
	PublicizerPkg  string   // the package, when there is one
	Publicize      []string // Publicize items, "Assembly-CSharp" or "Assembly-CSharp:Verse.Pawn.health"
	PublicizeAll   bool
	References     []Reference
	ProjectRefs    []string // ProjectReference, relative to Dir as written
	Sources        int      // .cs files the project compiles
	Unresolved     []string // $(Property) names the read could not give a value
	PackagesConfig bool     // packages.config beside the project (restore needs nuget)
	Inherited      []string // Directory.Build.props and imports that contributed, relative to Dir
}

var (
	propRe   = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_.\-]*)\)`)
	skipDirs = map[string]bool{"obj": true, "bin": true, ".git": true, ".vs": true, "node_modules": true, ".rimarcho": true}
)

// node is an XML element, read without a schema: MSBuild files are free-form.
type node struct {
	name  string
	attrs map[string]string // lower-case attribute names
	text  string
	kids  []*node
}

func parseXML(b []byte) (*node, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if root != nil {
				break // keep what was read
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return root, nil
}

// child returns the text of a child element (or attribute) named name.
func (n *node) val(name string) string {
	if v, ok := n.attrs[strings.ToLower(name)]; ok {
		return strings.TrimSpace(v)
	}
	for _, k := range n.kids {
		if strings.EqualFold(k.name, name) {
			return strings.TrimSpace(k.text)
		}
	}
	return ""
}

// eval holds the state of one project's evaluation: properties in order, items as read.
type eval struct {
	dir       string
	cur       string // the folder of the file being read: imports are relative to it
	root      string // the Mod's folder: files outside are not followed
	props     map[string]string
	pkgs      []Package
	central   map[string]string
	publicize []string
	refs      []Reference
	projRefs  []string
	compInc   []string
	compRem   []string
	seen      map[string]bool
	inherited []string
}

func (e *eval) set(name, value string) { e.props[strings.ToLower(name)] = value }
func (e *eval) get(name string) string { return e.props[strings.ToLower(name)] }

// expand replaces $(Name) with the property's value; an unknown one stays as written in a
// value, and becomes empty in a condition, as MSBuild reads it.
func (e *eval) expand(s string, cond bool) string {
	if !strings.Contains(s, "$(") {
		return s
	}
	return propRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-1]
		if v, ok := e.props[strings.ToLower(name)]; ok {
			return v
		}
		if cond {
			return ""
		}
		return m
	})
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' && s[len(s)-1] == '\'' || s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

// condition evaluates the small part of MSBuild conditions that decides which property
// groups apply: ==, !=, And, Or, Exists(...). Anything else counts as true.
func (e *eval) condition(c string) bool {
	c = strings.TrimSpace(c)
	if c == "" {
		return true
	}
	c = e.expand(c, true)
	return e.cond(c)
}

func splitWord(s, word string) (string, string, bool) {
	low := strings.ToLower(s)
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		switch {
		case inQuote != 0:
			if s[i] == inQuote {
				inQuote = 0
			}
		case s[i] == '\'' || s[i] == '"':
			inQuote = s[i]
		case i > 0 && i+len(word)+2 <= len(s) && s[i-1] == ' ' && low[i:i+len(word)] == word && s[i+len(word)] == ' ':
			return s[:i-1], s[i+len(word)+1:], true
		}
	}
	return "", "", false
}

func (e *eval) cond(c string) bool {
	if l, r, ok := splitWord(c, "or"); ok {
		return e.cond(l) || e.cond(r)
	}
	if l, r, ok := splitWord(c, "and"); ok {
		return e.cond(l) && e.cond(r)
	}
	c = strings.TrimSpace(c)
	if strings.HasPrefix(strings.ToLower(c), "exists(") && strings.HasSuffix(c, ")") {
		p := unquote(c[7 : len(c)-1])
		_, err := os.Stat(e.abs(p))
		return err == nil
	}
	if strings.HasPrefix(c, "!") {
		return !e.cond(c[1:])
	}
	for _, op := range []string{"==", "!="} {
		if i := strings.Index(c, op); i >= 0 {
			eq := strings.EqualFold(unquote(c[:i]), unquote(c[i+2:]))
			return eq == (op == "==")
		}
	}
	return true
}

func (e *eval) abs(p string) string {
	p = filepath.FromSlash(strings.ReplaceAll(p, `\`, "/"))
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.dir, p)
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// file applies one MSBuild file's property groups and item groups, following local imports.
func (e *eval) file(path string, depth int) {
	key := strings.ToLower(path)
	if e.seen[key] || depth > 4 {
		return
	}
	e.seen[key] = true
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	root, err := parseXML(b)
	if err != nil {
		return
	}
	prev, prevCur := e.get("MSBuildThisFileDirectory"), e.cur
	e.cur = filepath.Dir(path)
	e.set("MSBuildThisFileDirectory", e.cur+string(filepath.Separator))
	defer func() { e.set("MSBuildThisFileDirectory", prev); e.cur = prevCur }()
	e.walk(root, depth)
}

func (e *eval) walk(parent *node, depth int) {
	for _, n := range parent.kids {
		switch strings.ToLower(n.name) {
		case "propertygroup":
			if !e.condition(n.attrs["condition"]) {
				continue
			}
			for _, p := range n.kids {
				if !e.condition(p.attrs["condition"]) {
					continue
				}
				e.set(p.name, e.expand(strings.TrimSpace(p.text), false))
			}
		case "itemgroup":
			if !e.condition(n.attrs["condition"]) {
				continue
			}
			for _, it := range n.kids {
				if e.condition(it.attrs["condition"]) {
					e.item(it)
				}
			}
		case "choose":
			for _, w := range n.kids {
				name := strings.ToLower(w.name)
				if name == "when" && e.condition(w.attrs["condition"]) || name == "otherwise" {
					e.walk(w, depth)
					break
				}
			}
		case "import":
			p := e.expand(n.attrs["project"], false)
			if p == "" || strings.Contains(p, "$(") || !e.condition(n.attrs["condition"]) {
				continue
			}
			p = filepath.FromSlash(strings.ReplaceAll(p, `\`, "/"))
			if !filepath.IsAbs(p) {
				p = filepath.Join(e.cur, p)
			}
			if within(e.root, p) {
				e.inherit(p)
				e.file(p, depth+1)
			}
		}
	}
}

func (e *eval) inherit(p string) {
	if rel, err := filepath.Rel(e.dir, p); err == nil {
		e.inherited = append(e.inherited, filepath.ToSlash(rel))
	}
}

func (e *eval) item(it *node) {
	inc := e.expand(it.attrs["include"], false)
	switch strings.ToLower(it.name) {
	case "packagereference":
		name := inc
		if name == "" {
			name = e.expand(it.attrs["update"], false)
		}
		if name == "" {
			return
		}
		e.pkgs = append(e.pkgs, Package{Name: name, Version: e.expand(it.val("Version"), false)})
	case "packageversion":
		if inc != "" {
			e.central[strings.ToLower(inc)] = e.expand(it.val("Version"), false)
		}
	case "publicize":
		for _, p := range strings.Split(inc, ";") {
			if p = strings.TrimSpace(p); p != "" {
				e.publicize = append(e.publicize, p)
			}
		}
	case "reference":
		name := inc
		if i := strings.Index(name, ","); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}
		r := Reference{Name: name, HintPath: e.expand(it.val("HintPath"), false),
			Publicize: strings.EqualFold(it.val("Publicize"), "true")}
		if r.HintPath != "" && !strings.Contains(r.HintPath, "$(") {
			if _, err := os.Stat(e.abs(r.HintPath)); err != nil {
				r.Missing = true
			}
		}
		e.refs = append(e.refs, r)
	case "projectreference":
		if inc != "" {
			e.projRefs = append(e.projRefs, filepath.ToSlash(strings.ReplaceAll(inc, `\`, "/")))
		}
	case "compile":
		if inc != "" {
			e.compInc = append(e.compInc, inc)
		}
		if rm := e.expand(it.attrs["remove"], false); rm != "" {
			e.compRem = append(e.compRem, rm)
		}
	}
}

// Read reads one .csproj. root is the Mod's folder: Directory.Build.props above the project are
// read up to it, and imports outside it are not followed. It never writes and never runs anything.
func Read(file, root string) *Project {
	file, _ = filepath.Abs(file)
	p := &Project{File: file, Dir: filepath.Dir(file), Name: strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))}
	b, err := os.ReadFile(file)
	if err != nil {
		p.Err = err.Error()
		return p
	}
	top, err := parseXML(b)
	if err != nil || !strings.EqualFold(top.name, "Project") {
		p.Err = "not an MSBuild project file"
		return p
	}
	if root == "" {
		root = p.Dir
	}
	e := &eval{dir: p.Dir, root: root, props: map[string]string{}, central: map[string]string{}, seen: map[string]bool{}}
	e.set("Configuration", "Release")
	e.set("Platform", "AnyCPU")
	e.set("MSBuildProjectName", p.Name)
	e.set("MSBuildProjectDirectory", p.Dir)
	e.set("MSBuildProjectFile", filepath.Base(file))
	e.set("MSBuildThisFileDirectory", p.Dir+string(filepath.Separator))
	e.set("ProjectDir", p.Dir+string(filepath.Separator))
	e.set("SolutionDir", p.Dir+string(filepath.Separator))
	p.SDK = top.attrs["sdk"]
	if p.SDK == "" {
		for _, k := range top.kids {
			if strings.EqualFold(k.name, "Sdk") {
				p.SDK = k.attrs["name"]
			}
		}
	}
	p.Legacy = p.SDK == "" && !hasSDKImport(top)

	// Directory.Build.props from the Mod's folder down to the project's, then the project.
	var chain []string
	for d := p.Dir; within(root, d); d = filepath.Dir(d) {
		chain = append([]string{d}, chain...)
		if d == root || filepath.Dir(d) == d {
			break
		}
	}
	for _, d := range chain {
		pp := filepath.Join(d, "Directory.Build.props")
		if _, err := os.Stat(pp); err == nil {
			e.inherit(pp)
			e.file(pp, 0)
		}
	}
	for i := len(chain) - 1; i >= 0; i-- { // the nearest Directory.Packages.props wins
		pp := filepath.Join(chain[i], "Directory.Packages.props")
		if _, err := os.Stat(pp); err == nil {
			e.inherit(pp)
			e.file(pp, 0)
			break
		}
	}
	e.seen[strings.ToLower(file)] = true
	e.cur = p.Dir
	e.walk(top, 0)

	p.fill(e)
	return p
}

func hasSDKImport(top *node) bool {
	for _, k := range top.kids {
		if strings.EqualFold(k.name, "Import") && strings.EqualFold(k.attrs["sdk"], "Microsoft.NET.Sdk") {
			return true
		}
	}
	return false
}

func (p *Project) fill(e *eval) {
	if v := e.get("TargetFrameworks"); v != "" {
		p.Frameworks = splitList(v)
	} else if v := e.get("TargetFramework"); v != "" {
		p.Frameworks = []string{v}
	} else if v := e.get("TargetFrameworkVersion"); v != "" {
		p.Frameworks = []string{v}
	}
	p.AssemblyName = e.get("AssemblyName")
	if p.AssemblyName == "" {
		p.AssemblyName = p.Name
	}
	p.OutputType = e.get("OutputType")
	p.LangVersion = e.get("LangVersion")

	for _, pk := range e.pkgs {
		if pk.Version == "" {
			pk.Version = e.central[strings.ToLower(pk.Name)]
		}
		dup := false
		for i := range p.Packages {
			if strings.EqualFold(p.Packages[i].Name, pk.Name) {
				dup = true
				if pk.Version != "" {
					p.Packages[i].Version = pk.Version
				}
			}
		}
		if !dup {
			p.Packages = append(p.Packages, pk)
		}
		if strings.Contains(strings.ToLower(pk.Name), "publicizer") {
			p.Publicizer, p.PublicizerPkg = true, pk.Name
		}
	}
	p.Publicize = e.publicize
	p.PublicizeAll = strings.EqualFold(e.get("PublicizeAll"), "true")
	p.References = e.refs
	for _, r := range p.References {
		if r.Publicize {
			p.Publicizer = true
		}
	}
	if len(p.Publicize) > 0 || p.PublicizeAll {
		p.Publicizer = true
	}
	p.ProjectRefs = e.projRefs
	p.noteUnresolved()
	p.Inherited = e.inherited
	if _, err := os.Stat(filepath.Join(p.Dir, "packages.config")); err == nil {
		p.PackagesConfig = true
	}

	// Where the DLL lands. OutDir wins over OutputPath; the SDK appends the framework folder
	// to OutputPath unless AppendTargetFrameworkToOutputPath is false.
	out := e.get("OutDir")
	if out == "" {
		out = e.get("OutputPath")
		if out != "" && !p.Legacy && len(p.Frameworks) == 1 && !strings.EqualFold(e.get("AppendTargetFrameworkToOutputPath"), "false") {
			out = strings.TrimRight(out, `\/`) + string(filepath.Separator) + strings.ToLower(p.Frameworks[0])
			p.TFMAppended = true
		}
	}
	if out == "" {
		p.OutputDefault = true
		out = filepath.Join("bin", "Release")
		if len(p.Frameworks) == 1 && !p.Legacy {
			out = filepath.Join(out, strings.ToLower(p.Frameworks[0]))
		}
	}
	p.Output = out
	if !strings.Contains(out, "$(") {
		p.OutputAbs = filepath.Clean(e.abs(out))
	}
	p.Sources = p.countSources(e)
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// DLL is the file name the build produces.
func (p *Project) DLL() string {
	if strings.EqualFold(p.OutputType, "Exe") || strings.EqualFold(p.OutputType, "WinExe") {
		return p.AssemblyName + ".exe"
	}
	return p.AssemblyName + ".dll"
}

// noteUnresolved lists the $(Properties) left in what the build summary reports.
func (p *Project) noteUnresolved() {
	set := map[string]bool{}
	note := func(s string) {
		for _, m := range propRe.FindAllStringSubmatch(s, -1) {
			set[m[1]] = true
		}
	}
	note(p.AssemblyName)
	note(p.Output)
	for _, f := range p.Frameworks {
		note(f)
	}
	for _, pk := range p.Packages {
		note(pk.Version)
	}
	for _, r := range p.References {
		note(r.HintPath)
	}
	for n := range set {
		p.Unresolved = append(p.Unresolved, n)
	}
	sort.Strings(p.Unresolved)
}
