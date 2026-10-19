// Spike 1: read HarmonyPatch custom-attribute arguments from real mod DLLs with
// saferwall/pe + a small blob decoder, and compare with ilspycmd's decompiled output.
//
//	go run ./spikes/dotnetmeta -n 20            # pick 20 DLLs from the active mod set
//	go run ./spikes/dotnetmeta -n 0 -noilspy    # reader over every candidate DLL (speed/error rate)
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/spikes/internal/rwenv"
)

var skipDLL = map[string]bool{
	"0harmony.dll": true, "hugslib.dll": true, "harmony.dll": true, "0multiplayerapi.dll": true,
}

func main() {
	n := flag.Int("n", 20, "number of DLLs to compare (0 = all candidates)")
	noIL := flag.Bool("noilspy", false, "skip ilspycmd comparison")
	verbose := flag.Bool("v", false, "print every mismatch")
	flag.Parse()

	env, err := rwenv.Detect()
	must(err)
	mc, err := rwenv.ReadModsConfig(env.ModsConfigPath())
	must(err)
	reg := env.Scan()

	var harmonyDir string
	var cands []string
	seen := map[string]bool{}
	for _, id := range mc.ActiveMods {
		m := reg.Resolve(id)
		if m == nil {
			continue
		}
		filepath.WalkDir(m.Dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".dll") {
				return nil
			}
			base := strings.ToLower(d.Name())
			if base == "0harmony.dll" && harmonyDir == "" {
				harmonyDir = filepath.Dir(p)
			}
			if skipDLL[base] || !strings.EqualFold(filepath.Base(filepath.Dir(p)), "Assemblies") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil || !bytes.Contains(b, []byte("HarmonyPatch\x00")) {
				return nil
			}
			key := fmt.Sprintf("%s|%d", base, len(b))
			if !seen[key] {
				seen[key] = true
				cands = append(cands, p)
			}
			return nil
		})
	}
	sort.Slice(cands, func(i, j int) bool { return fsize(cands[i]) < fsize(cands[j]) })
	fmt.Printf("candidate DLLs referencing HarmonyPatch: %d (harmony ref dir: %s)\n", len(cands), harmonyDir)

	picked := cands
	if *n > 0 && *n < len(cands) {
		picked = nil
		for i := 0; i < *n; i++ { // spread across the size distribution
			picked = append(picked, cands[i*(len(cands)-1)/(*n-1)])
		}
	}

	// Reader pass.
	t0 := time.Now()
	type res struct {
		path   string
		attrs  []string
		owners []Attr
		errs   int
		manual bool
		err    error
	}
	results := make([]res, len(picked))
	totalAttrs, totalErrs := 0, 0
	for i, p := range picked {
		a, err := Open(p)
		if err != nil {
			results[i] = res{path: p, err: err}
			continue
		}
		hp := a.Attributes(func(t string) bool {
			return t == "HarmonyLib.HarmonyPatch" || t == "Harmony.HarmonyPatch"
		})
		r := res{path: p, owners: hp, manual: a.HasMemberRef("HarmonyLib.Harmony", "Patch")}
		for _, at := range hp {
			if at.Errors != "" {
				r.errs++
			}
			r.attrs = append(r.attrs, canonReader(at))
		}
		totalAttrs += len(hp)
		totalErrs += r.errs
		results[i] = r
	}
	el := time.Since(t0)
	fmt.Printf("reader: %d DLLs, %d HarmonyPatch attributes, %d decode errors, %v total (%.1f ms/DLL)\n",
		len(picked), totalAttrs, totalErrs, el.Round(time.Millisecond), float64(el.Microseconds())/1000/float64(max(1, len(picked))))
	if *noIL {
		for _, r := range results {
			if r.err != nil {
				fmt.Printf("  open error %s: %v\n", r.path, r.err)
			}
		}
		return
	}

	// ilspycmd pass (parallel).
	refs := []string{env.GameDir}
	if harmonyDir != "" {
		refs = append(refs, harmonyDir)
	}
	ilOut := make([][]string, len(picked))
	ilErr := make([]error, len(picked))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	t1 := time.Now()
	for i, p := range picked {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ilOut[i], ilErr[i] = ilspyAttrs(p, append(refs, filepath.Dir(p)))
		}(i, p)
	}
	wg.Wait()
	fmt.Printf("ilspycmd: %v\n\n", time.Since(t1).Round(time.Second))

	agree, total := 0, 0
	for i, r := range results {
		rel, _ := filepath.Rel(env.WorkshopDir, r.path)
		if strings.HasPrefix(rel, "..") {
			rel = r.path
		}
		if r.err != nil || ilErr[i] != nil {
			fmt.Printf("%-70s ERROR reader=%v ilspy=%v\n", rel, r.err, ilErr[i])
			continue
		}
		onlyR, onlyI, same := diff(r.attrs, ilOut[i])
		total += same + len(onlyR) + len(onlyI)
		agree += same
		fmt.Printf("%-70s attrs=%-4d ilspy=%-4d match=%-4d manual=%v\n", trunc(rel, 70), len(r.attrs), len(ilOut[i]), same, r.manual)
		if *verbose || len(onlyR)+len(onlyI) > 0 {
			for j, s := range onlyR {
				if j >= 5 && !*verbose {
					fmt.Printf("    … %d more reader-only\n", len(onlyR)-j)
					break
				}
				fmt.Printf("    reader-only: %s\n", s)
			}
			for j, s := range onlyI {
				if j >= 5 && !*verbose {
					fmt.Printf("    … %d more ilspy-only\n", len(onlyI)-j)
					break
				}
				fmt.Printf("    ilspy-only:  %s\n", s)
			}
		}
	}
	fmt.Printf("\nagreement: %d / %d = %.2f%%\n", agree, total, 100*float64(agree)/float64(max(1, total)))
}

func diff(a, b []string) (onlyA, onlyB []string, same int) {
	cnt := map[string]int{}
	for _, s := range b {
		cnt[s]++
	}
	for _, s := range a {
		if cnt[s] > 0 {
			cnt[s]--
			same++
		} else {
			onlyA = append(onlyA, s)
		}
	}
	for _, s := range b {
		if cnt[s] > 0 {
			cnt[s]--
			onlyB = append(onlyB, s)
		}
	}
	return
}

// ---- canonical form -------------------------------------------------------

var methodType = []string{"Normal", "Getter", "Setter", "Constructor", "StaticConstructor", "Enumerator", "Async"}
var argumentType = []string{"Normal", "Ref", "Out", "Pointer"}

var keywords = map[string]string{
	"System.Boolean": "bool", "System.Byte": "byte", "System.SByte": "sbyte", "System.Char": "char",
	"System.Int16": "short", "System.UInt16": "ushort", "System.Int32": "int", "System.UInt32": "uint",
	"System.Int64": "long", "System.UInt64": "ulong", "System.Single": "float", "System.Double": "double",
	"System.String": "string", "System.Object": "object", "System.Decimal": "decimal", "System.Void": "void",
	"System.IntPtr": "IntPtr", "System.UIntPtr": "UIntPtr",
}

func canonReader(at Attr) string {
	if at.Errors != "" {
		return "HarmonyPatch(<error: " + at.Errors + ">)"
	}
	var parts []string
	for _, v := range at.Fixed {
		parts = append(parts, canonValue(v))
	}
	for _, nv := range at.Named {
		parts = append(parts, nv.Name+"="+canonValue(nv.V))
	}
	return "HarmonyPatch(" + strings.Join(parts, ",") + ")"
}

func canonValue(v Value) string {
	switch {
	case v.Kind == "null":
		return "null"
	case v.Kind == "type":
		return "typeof(" + shortTypes(clrToCS(v.S)) + ")"
	case v.Kind == "string":
		return fmt.Sprintf("%q", v.S)
	case strings.HasPrefix(v.Kind, "enum:"):
		var idx int
		fmt.Sscan(v.S, &idx)
		switch strings.TrimPrefix(v.Kind, "enum:") {
		case "HarmonyLib.MethodType", "Harmony.MethodType":
			if idx >= 0 && idx < len(methodType) {
				return "MethodType." + methodType[idx]
			}
		case "HarmonyLib.ArgumentType", "Harmony.ArgumentType":
			if idx >= 0 && idx < len(argumentType) {
				return "ArgumentType." + argumentType[idx]
			}
		}
		return "(" + shortTypes(strings.TrimPrefix(v.Kind, "enum:")) + ")" + v.S
	case v.Kind == "array":
		var es []string
		for _, e := range v.Arr {
			es = append(es, canonValue(e))
		}
		return "[" + strings.Join(es, ",") + "]"
	}
	return v.S
}

// clrToCS converts a serialized (assembly-qualified) CLR type name to C# syntax.
func clrToCS(s string) string {
	name, rest := splitTypeName(s)
	arr := ""
	for strings.HasSuffix(name, "[]") {
		arr += "[]"
		name = strings.TrimSuffix(name, "[]")
	}
	byRef := strings.HasSuffix(name, "&")
	name = strings.TrimSuffix(name, "&")
	name = strings.ReplaceAll(name, "+", ".")
	if kw, ok := keywords[name]; ok {
		name = kw
	}
	if i := strings.IndexByte(name, '`'); i >= 0 {
		n := 0
		fmt.Sscan(name[i+1:], &n)
		base := name[:i]
		if rest == "" {
			return base + "<" + strings.Repeat(",", max(0, n-1)) + ">" + arr
		}
		var args []string
		for _, g := range splitGenericArgs(rest) {
			args = append(args, clrToCS(g))
		}
		name = base + "<" + strings.Join(args, ", ") + ">"
	}
	if byRef {
		name += "&"
	}
	return name + arr
}

// splitTypeName splits "Ns.Gen`2[[A, asm],[B, asm]][], asm" into ("Ns.Gen`2[]", "[A, asm],[B, asm]").
func splitTypeName(s string) (string, string) {
	s = strings.TrimSpace(s)
	var name strings.Builder
	depth := 0
	start, end := -1, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '[' && depth == 0 && i+1 < len(s) && s[i+1] == '[':
			start = i + 1
			depth++
		case c == '[' && depth > 0:
			depth++
		case c == ']' && depth > 0:
			depth--
			if depth == 0 {
				end = i
			}
		case c == ',' && depth == 0:
			return name.String(), sliceOr(s, start, end)
		default:
			if depth == 0 {
				name.WriteByte(c)
			}
		}
	}
	return name.String(), sliceOr(s, start, end)
}

func sliceOr(s string, a, b int) string {
	if a < 0 || b < 0 {
		return ""
	}
	return s[a:b]
}

func splitGenericArgs(s string) []string {
	var out []string
	depth := 0
	cur := strings.Builder{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '[':
			if depth > 0 {
				cur.WriteByte(c)
			}
			depth++
		case c == ']':
			depth--
			if depth > 0 {
				cur.WriteByte(c)
			} else {
				out = append(out, cur.String())
				cur.Reset()
			}
		case c == ',' && depth == 0:
		default:
			cur.WriteByte(c)
		}
	}
	return out
}

var nsPrefix = regexp.MustCompile(`\b(?:global::)?(?:[A-Za-z_][\w]*\.)+([A-Za-z_]\w*)`)

// shortTypes strips namespaces/enclosing types so both sides compare by simple names.
func shortTypes(s string) string {
	s = strings.ReplaceAll(nsPrefix.ReplaceAllString(s, "$1"), " ", "")
	return nullable.ReplaceAllString(s, "$1?")
}

var nullable = regexp.MustCompile(`\bNullable<(\w+)>`)

// ---- ilspycmd side --------------------------------------------------------

func ilspyAttrs(dll string, refs []string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	args := []string{dll}
	for _, r := range refs {
		args = append(args, "-r", r)
	}
	out, err := exec.CommandContext(ctx, "ilspycmd", args...).Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return extractAttrs(string(out)), nil
}

var attrStart = regexp.MustCompile(`[\[,]\s*(?:global::)?(?:HarmonyLib\.|Harmony\.)?HarmonyPatch(?:Attribute)?\b`)

func extractAttrs(src string) []string {
	var out []string
	for _, loc := range attrStart.FindAllStringIndex(src, -1) {
		// Must be inside an attribute section: the line's first non-space char is '['.
		ls := strings.LastIndexByte(src[:loc[0]+1], '\n') + 1
		if !strings.HasPrefix(strings.TrimSpace(src[ls:loc[1]]), "[") {
			continue
		}
		i := loc[1]
		for i < len(src) && src[i] == ' ' {
			i++
		}
		if i >= len(src) || src[i] != '(' {
			out = append(out, "HarmonyPatch()")
			continue
		}
		j := matchParen(src, i)
		if j < 0 {
			continue
		}
		out = append(out, canonCS(src[i+1:j]))
	}
	return out
}

func matchParen(s string, i int) int {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '(', '{':
			depth++
		case ')', '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '(', '{', '<', '[':
			depth++
		case ')', '}', '>', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

var (
	castEnum = regexp.MustCompile(`^\((?:HarmonyLib\.)?(MethodType|ArgumentType)\)(-?\d+)$`)
	newArr   = regexp.MustCompile(`(?s)^new\s+[\w.:<>]+\[\d*\]\s*(\{(.*)\})?$`)
	nullCast = regexp.MustCompile(`^\([\w.\[\]<>]+\)null$`)
)

func canonCS(args string) string {
	var parts []string
	for _, a := range splitTop(args) {
		parts = append(parts, canonCSValue(a))
	}
	return "HarmonyPatch(" + strings.Join(parts, ",") + ")"
}

func canonCSValue(a string) string {
	a = strings.TrimSpace(a)
	if i := strings.Index(a, "="); i > 0 && !strings.ContainsAny(a[:i], "\"(") {
		return strings.TrimSpace(a[:i]) + "=" + canonCSValue(a[i+1:])
	}
	switch {
	case a == "null" || nullCast.MatchString(a):
		return "null"
	case strings.HasPrefix(a, "typeof("):
		return "typeof(" + shortTypes(a[7:len(a)-1]) + ")"
	case strings.HasPrefix(a, "\""):
		var s string
		if _, err := fmt.Sscanf(a, "%q", &s); err == nil {
			return fmt.Sprintf("%q", s)
		}
		return a
	case strings.HasPrefix(a, "@\""):
		return fmt.Sprintf("%q", strings.ReplaceAll(a[2:len(a)-1], `""`, `"`))
	}
	if m := castEnum.FindStringSubmatch(a); m != nil {
		var idx int
		fmt.Sscan(m[2], &idx)
		tbl := methodType
		if m[1] == "ArgumentType" {
			tbl = argumentType
		}
		if idx >= 0 && idx < len(tbl) {
			return m[1] + "." + tbl[idx]
		}
		return a
	}
	if m := newArr.FindStringSubmatch(a); m != nil {
		var es []string
		for _, e := range splitTop(m[2]) {
			es = append(es, canonCSValue(e))
		}
		return "[" + strings.Join(es, ",") + "]"
	}
	if strings.HasPrefix(a, "HarmonyLib.") {
		a = strings.TrimPrefix(a, "HarmonyLib.")
	}
	return a
}

// ---- misc -----------------------------------------------------------------

func fsize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n+1:]
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
