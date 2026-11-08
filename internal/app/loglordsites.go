package app

// Lord.AddPawn call sites (06 §2, MCP eval h8 G7): for "Tried to add pawn X to lord with lord
// job T but this pawn is already a member of lord …", the source Mod's code calls
// Verse.AI.Group.Lord.AddPawn without checking the pawn's current lord. The cluster lists each
// call site in the source Mod's assemblies and whether a GetLord() call comes before it in the
// same method (an IL window check, not flow analysis); next: opens the unchecked ones first.

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/dotnet"
)

const lordType = "Verse.AI.Group.Lord"

func (cx *attrCtx) lordSites(lc *LogCluster) {
	if !lordAddRe.MatchString(lc.First.Message) {
		return
	}
	var first []string
	for _, at := range lc.Attr {
		if at.Role != RoleSource || cx.official(at.Instance) {
			continue
		}
		m := cx.st.Ix.ByID[at.Instance]
		if m == nil {
			continue
		}
		ps, err := cx.a.modDLLPaths(cx.st, m)
		if err != nil {
			continue
		}
		ref := cx.st.Ix.Ref(at.Instance)
		var items []string
		for _, p := range ps {
			a := cx.openDLL(p)
			if a == nil {
				continue
			}
			sites := a.CallSitesOf(0, "AddPawn", func(mb dotnet.Member) bool {
				return strings.ReplaceAll(mb.Owner, "+", ".") == lordType && mb.Params == 1
			})
			for _, s := range sites {
				check := "GetLord() called before it in the method"
				if !getLordBefore(a, s.Method, s.Off) {
					check = "no GetLord() check before it in the method"
					typ := a.MethodDeclType(s.Method)
					first = append(first, fmt.Sprintf("archo source %s --type %s", ref, outerType(strings.ReplaceAll(typ, "+", "."))))
				}
				items = append(items, fmt.Sprintf("%s IL_%04x (%s)", s.Caller, s.Off, check))
			}
		}
		switch {
		case len(items) > 8:
			items = append(items[:8], fmt.Sprintf("… %d more", len(items)-8))
		case len(items) == 0:
			continue
		}
		lc.Details = append(lc.Details, fmt.Sprintf("Lord.AddPawn call sites in %s: %s", ref, strings.Join(items, "; ")))
	}
	if len(first) > 0 {
		lc.Next = uniqStr(append(first, lc.Next...))
	}
}

// getLordBefore reports a call of a method named GetLord earlier in the method than off.
func getLordBefore(a *dotnet.Assembly, row uint32, off int) bool {
	for _, in := range dotnet.DecodeIL(a.Body(row)) {
		if in.Off >= off {
			break
		}
		if in.Op == 0x28 || in.Op == 0x6f {
			if m, ok := a.Member(uint32(in.Arg)); ok && m.Name == "GetLord" {
				return true
			}
		}
	}
	return false
}
