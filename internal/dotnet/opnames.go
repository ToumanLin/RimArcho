package dotnet

import "fmt"

// Opcode mnemonics (ECMA-335 §III) for trace and diff text.
var opNames1 = map[uint16]string{
	0x00: "nop", 0x01: "break", 0x02: "ldarg.0", 0x03: "ldarg.1", 0x04: "ldarg.2", 0x05: "ldarg.3",
	0x06: "ldloc.0", 0x07: "ldloc.1", 0x08: "ldloc.2", 0x09: "ldloc.3", 0x0a: "stloc.0", 0x0b: "stloc.1",
	0x0c: "stloc.2", 0x0d: "stloc.3", 0x0e: "ldarg.s", 0x0f: "ldarga.s", 0x10: "starg.s", 0x11: "ldloc.s",
	0x12: "ldloca.s", 0x13: "stloc.s", 0x14: "ldnull", 0x15: "ldc.i4.m1", 0x16: "ldc.i4.0", 0x17: "ldc.i4.1",
	0x18: "ldc.i4.2", 0x19: "ldc.i4.3", 0x1a: "ldc.i4.4", 0x1b: "ldc.i4.5", 0x1c: "ldc.i4.6", 0x1d: "ldc.i4.7",
	0x1e: "ldc.i4.8", 0x1f: "ldc.i4.s", 0x20: "ldc.i4", 0x21: "ldc.i8", 0x22: "ldc.r4", 0x23: "ldc.r8",
	0x25: "dup", 0x26: "pop", 0x27: "jmp", 0x28: "call", 0x29: "calli", 0x2a: "ret", 0x2b: "br.s",
	0x2c: "brfalse.s", 0x2d: "brtrue.s", 0x2e: "beq.s", 0x2f: "bge.s", 0x30: "bgt.s", 0x31: "ble.s",
	0x32: "blt.s", 0x33: "bne.un.s", 0x34: "bge.un.s", 0x35: "bgt.un.s", 0x36: "ble.un.s", 0x37: "blt.un.s",
	0x38: "br", 0x39: "brfalse", 0x3a: "brtrue", 0x3b: "beq", 0x3c: "bge", 0x3d: "bgt", 0x3e: "ble",
	0x3f: "blt", 0x40: "bne.un", 0x41: "bge.un", 0x42: "bgt.un", 0x43: "ble.un", 0x44: "blt.un",
	0x45: "switch", 0x46: "ldind.i1", 0x47: "ldind.u1", 0x48: "ldind.i2", 0x49: "ldind.u2", 0x4a: "ldind.i4",
	0x4b: "ldind.u4", 0x4c: "ldind.i8", 0x4d: "ldind.i", 0x4e: "ldind.r4", 0x4f: "ldind.r8", 0x50: "ldind.ref",
	0x51: "stind.ref", 0x52: "stind.i1", 0x53: "stind.i2", 0x54: "stind.i4", 0x55: "stind.i8", 0x56: "stind.r4",
	0x57: "stind.r8", 0x58: "add", 0x59: "sub", 0x5a: "mul", 0x5b: "div", 0x5c: "div.un", 0x5d: "rem",
	0x5e: "rem.un", 0x5f: "and", 0x60: "or", 0x61: "xor", 0x62: "shl", 0x63: "shr", 0x64: "shr.un",
	0x65: "neg", 0x66: "not", 0x67: "conv.i1", 0x68: "conv.i2", 0x69: "conv.i4", 0x6a: "conv.i8",
	0x6b: "conv.r4", 0x6c: "conv.r8", 0x6d: "conv.u4", 0x6e: "conv.u8", 0x6f: "callvirt", 0x70: "cpobj",
	0x71: "ldobj", 0x72: "ldstr", 0x73: "newobj", 0x74: "castclass", 0x75: "isinst", 0x76: "conv.r.un",
	0x79: "unbox", 0x7a: "throw", 0x7b: "ldfld", 0x7c: "ldflda", 0x7d: "stfld", 0x7e: "ldsfld",
	0x7f: "ldsflda", 0x80: "stsfld", 0x81: "stobj", 0x8c: "box", 0x8d: "newarr", 0x8e: "ldlen",
	0x8f: "ldelema", 0x90: "ldelem.i1", 0x91: "ldelem.u1", 0x92: "ldelem.i2", 0x93: "ldelem.u2",
	0x94: "ldelem.i4", 0x95: "ldelem.u4", 0x96: "ldelem.i8", 0x97: "ldelem.i", 0x98: "ldelem.r4",
	0x99: "ldelem.r8", 0x9a: "ldelem.ref", 0x9b: "stelem.i", 0x9c: "stelem.i1", 0x9d: "stelem.i2",
	0x9e: "stelem.i4", 0x9f: "stelem.i8", 0xa0: "stelem.r4", 0xa1: "stelem.r8", 0xa2: "stelem.ref",
	0xa3: "ldelem", 0xa4: "stelem", 0xa5: "unbox.any", 0xc2: "refanyval", 0xc3: "ckfinite", 0xc6: "mkrefany",
	0xd0: "ldtoken", 0xdc: "endfinally", 0xdd: "leave", 0xde: "leave.s", 0xdf: "stind.i", 0xe0: "conv.u",
	0xfe00: "arglist", 0xfe01: "ceq", 0xfe02: "cgt", 0xfe03: "cgt.un", 0xfe04: "clt", 0xfe05: "clt.un",
	0xfe06: "ldftn", 0xfe07: "ldvirtftn", 0xfe09: "ldarg", 0xfe0a: "ldarga", 0xfe0b: "starg", 0xfe0c: "ldloc",
	0xfe0d: "ldloca", 0xfe0e: "stloc", 0xfe0f: "localloc", 0xfe11: "endfilter", 0xfe12: "unaligned.",
	0xfe13: "volatile.", 0xfe14: "tail.", 0xfe15: "initobj", 0xfe16: "constrained.", 0xfe17: "cpblk",
	0xfe18: "initblk", 0xfe19: "no.", 0xfe1a: "rethrow", 0xfe1c: "sizeof", 0xfe1d: "refanytype", 0xfe1e: "readonly.",
}

// OpName is an opcode's mnemonic.
func OpName(op uint16) string {
	if n, ok := opNames1[op]; ok {
		return n
	}
	return fmt.Sprintf("op_%x", op)
}

// tokenOperand reports opcodes whose operand is a metadata token.
func tokenOperand(op uint16) bool {
	switch op {
	case 0x27, 0x28, 0x29, 0x6f, 0x70, 0x71, 0x73, 0x74, 0x75, 0x79, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f, 0x80, 0x81,
		0x8c, 0x8d, 0x8f, 0xa3, 0xa4, 0xa5, 0xc2, 0xc6, 0xd0, 0xfe06, 0xfe07, 0xfe15, 0xfe16, 0xfe1c:
		return true
	}
	return false
}

// TokenText names a metadata token: Owner::Name for members, the type for types, a string
// literal for ldstr.
func (a *Assembly) TokenText(tok uint32) string {
	switch tok >> 24 {
	case 0x01, 0x02, 0x1B:
		return a.TypeToken(tok)
	case 0x70:
		return fmt.Sprintf("%q", a.UserString(tok))
	case 0x04, 0x06, 0x0A, 0x2B:
		if m, ok := a.Member(tok); ok {
			return m.Owner + "::" + m.Name
		}
	}
	return fmt.Sprintf("token 0x%08x", tok)
}

// InstrText renders one instruction: "ldarg.3", "ldfld Verse.Pawn::genes", "ldnull".
func (a *Assembly) InstrText(in Instr) string {
	n := OpName(in.Op)
	switch {
	case tokenOperand(in.Op) || in.Op == 0x72:
		return n + " " + a.TokenText(uint32(in.Arg))
	case in.Op == 0x0e || in.Op == 0x0f || in.Op == 0x10 || in.Op == 0x11 || in.Op == 0x12 || in.Op == 0x13 ||
		in.Op >= 0xfe09 && in.Op <= 0xfe0e || in.Op == 0x1f || in.Op == 0x20 || in.Op == 0x21:
		return fmt.Sprintf("%s %d", n, in.Arg)
	case IsBranch(in.Op):
		return fmt.Sprintf("%s IL_%04x", n, in.Arg)
	}
	return n
}

// instrAt returns the instruction at an offset of MethodDef row's body.
func (a *Assembly) instrAt(row uint32, off int) (Instr, bool) {
	for _, in := range DecodeIL(a.Body(row)) {
		if in.Off == off {
			return in, true
		}
	}
	return Instr{}, false
}
