package logs

import (
	"regexp"
	"strings"
)

// ilPrefix is an optional label ("IL_001a:") or method name ("AdjustParms - ") before the
// opcode of an IL listing line.
var ilPrefix = regexp.MustCompile("^(?:IL_[0-9a-fA-F]{4}:\\s*|[A-Za-z_][\\w.<>`]* - )")

// ilOps are CIL opcodes without their dotted suffixes (ldarg.0, stloc.s, conv.i4).
var ilOps = map[string]bool{}

func init() {
	for _, op := range strings.Fields(`ldarg ldarga ldloc ldloca ldfld ldflda ldsfld ldsflda ldc ldstr ldnull ldelem
		ldelema ldlen ldind ldftn ldvirtftn ldtoken ldobj starg stloc stfld stsfld stelem stind stobj call
		callvirt calli newobj newarr ret nop pop dup br brtrue brfalse beq bne bge bgt ble blt conv add sub mul
		div rem and or xor shl shr neg not ceq cgt clt box unbox castclass isinst switch leave throw rethrow
		endfinally initobj constrained sizeof localloc jmp`) {
		ilOps[op] = true
	}
}

// ILDump reports whether a message is a line of an IL listing that a transpiler printed
// (MCP review P13): it says nothing about the error next to it.
func ILDump(message string) bool {
	line, _, _ := strings.Cut(message, "\n")
	line = strings.TrimSpace(line)
	prefix := ilPrefix.FindString(line)
	line = line[len(prefix):]
	op, rest, _ := strings.Cut(line, " ")
	base, suffix, dotted := strings.Cut(op, ".")
	if !ilOps[base] {
		return false
	}
	if prefix != "" || dotted && suffix != "" {
		return true
	}
	// Without a prefix or suffix, an opcode that is also an English word ("call", "and",
	// "box") is IL only alone without operands, or with an operand that is not prose.
	if rest == "" {
		return base == "ret" || base == "nop" || base == "pop" || base == "dup" || base == "endfinally"
	}
	first, _, _ := strings.Cut(rest, " ")
	return first == "static" || first == "instance" || first == "class" || first == "valuetype" ||
		strings.ContainsAny(first, ".:()[]0123456789")
}
