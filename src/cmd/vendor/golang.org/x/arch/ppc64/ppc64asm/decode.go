// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppc64asm

import (
	"encoding/binary"
	"fmt"
	"log"
)

const debugDecode = false

const prefixOpcode = 1

// instFormat is a decoding rule for one specific instruction form.
// an instruction ins matches the rule if ins&Mask == Value
// DontCare bits should be zero, but the machine might not reject
// ones in those bits, they are mainly reserved for future expansion
// of the instruction set.
// The Args are stored in the same order as the instruction manual.
//
// Prefixed instructions are stored as:
//
//	prefix << 32 | suffix,
//
// Regular instructions are:
//
//	inst << 32
type instFormat struct {
	Op       Op
	Mask     uint64
	Value    uint64
	DontCare uint64
	Args     [6]*argField
}

// argField indicate how to decode an argument to an instruction.
// First parse the value from the BitFields, shift it left by Shift
// bits to get the actual numerical value.
type argField struct {
	Type  ArgType
	Shift uint8
	BitFields
}

// Parse parses the Arg out from the given binary instruction i.
func (a argField) Parse(i [2]uint32) Arg {
	switch a.Type {
	default:
		return nil
	case TypeUnknown:
		return nil
	case TypeReg:
		return R0 + Reg(a.BitFields.Parse(i))
	case TypeCondRegBit:
		return Cond0LT + CondReg(a.BitFields.Parse(i))
	case TypeCondRegField:
		return CR0 + CondReg(a.BitFields.Parse(i))
	case TypeFPReg:
		return F0 + Reg(a.BitFields.Parse(i))
	case TypeVecReg:
		return V0 + Reg(a.BitFields.Parse(i))
	case TypeVecSReg:
		return VS0 + Reg(a.BitFields.Parse(i))
	case TypeVecSpReg:
		return VS0 + Reg(a.BitFields.Parse(i))*2
	case TypeMMAReg:
		return A0 + Reg(a.BitFields.Parse(i))
	case TypeSpReg:
		return SpReg(a.BitFields.Parse(i))
	case TypeImmSigned:
		return Imm(a.BitFields.ParseSigned(i) << a.Shift)
	case TypeImmUnsigned:
		return Imm(a.BitFields.Parse(i) << a.Shift)
	case TypePCRel:
		return PCRel(a.BitFields.ParseSigned(i) << a.Shift)
	case TypeLabel:
		return Label(a.BitFields.ParseSigned(i) << a.Shift)
	case TypeOffset:
		return Offset(a.BitFields.ParseSigned(i) << a.Shift)
	case TypeNegOffset:
		// An oddball encoding of offset for hashchk and similar.
		// e.g hashchk offset is 0b1111111000000000 | DX << 8 | D << 3
		off := a.BitFields.ParseSigned(i) << a.Shift
		neg := int64(-1) << (int(a.Shift) + a.BitFields.NumBits())
		return Offset(neg | off)
	}
}

type ArgType int8

const (
	TypeUnknown      ArgType = iota
	TypePCRel                // PC-relative address
	TypeLabel                // absolute address
	TypeReg                  // integer register
	TypeCondRegBit           // conditional register bit (0-31)
	TypeCondRegField         // conditional register field (0-7)
	TypeFPReg                // floating point register
	TypeVecReg               // vector register
	TypeVecSReg              // VSX register
	TypeVecSpReg             // VSX register pair (even only encoding)
	TypeMMAReg               // MMA register
	TypeSpReg                // special register (depends on Op)
	TypeImmSigned            // signed immediate
	TypeImmUnsigned          // unsigned immediate/flag/mask, this is the catch-all type
	TypeOffset               // signed offset in load/store
	TypeNegOffset            // A negative 16 bit value 0b1111111xxxxx000 encoded as 0bxxxxx (e.g in the hashchk instruction)
	TypeLast                 // must be the last one
)

func (t ArgType) String() string {
	switch t {
	default:
		return fmt.Sprintf("ArgType(%d)", int(t))
	case TypeUnknown:
		return "Unknown"
	case TypeReg:
		return "Reg"
	case TypeCondRegBit:
		return "CondRegBit"
	case TypeCondRegField:
		return "CondRegField"
	case TypeFPReg:
		return "FPReg"
	case TypeVecReg:
		return "VecReg"
	case TypeVecSReg:
		return "VecSReg"
	case TypeVecSpReg:
		return "VecSpReg"
	case TypeMMAReg:
		return "MMAReg"
	case TypeSpReg:
		return "SpReg"
	case TypeImmSigned:
		return "ImmSigned"
	case TypeImmUnsigned:
		return "ImmUnsigned"
	case TypePCRel:
		return "PCRel"
	case TypeLabel:
		return "Label"
	case TypeOffset:
		return "Offset"
	case TypeNegOffset:
		return "NegOffset"
	}
}

func (t ArgType) GoString() string {
	s := t.String()
	if t > 0 && t < TypeLast {
		return "Type" + s
	}
	return s
}

var (
	// Errors
	errShort   = fmt.Errorf("truncated instruction")
	errUnknown = fmt.Errorf("unknown instruction")
)

var decoderCover []bool

// Decode decodes the leading bytes in src as a single instruction using
// byte order ord.
func Decode(src []byte, ord binary.ByteOrder) (inst Inst, err error) {
	if len(src) < 4 {
		return inst, errShort
	}
	if decoderCover == nil {
		decoderCover = make([]bool, len(instFormats))
	}
	inst.Len = 4
	ui_extn := [2]uint32{ord.Uint32(src[:inst.Len]), 0}
	ui := uint64(ui_extn[0]) << 32
	inst.Enc = ui_extn[0]
	opcode := inst.Enc >> 26
	if opcode == prefixOpcode {
		// This is a prefixed instruction
		inst.Len = 8
		if len(src) < 8 {
			return inst, errShort
		}
		// Merge the suffixed word.
		ui_extn[1] = ord.Uint32(src[4:inst.Len])
		ui |= uint64(ui_extn[1])
		inst.SuffixEnc = ui_extn[1]
	}
	for i, iform := range instFormats {
		if ui&iform.Mask != iform.Value {
			continue
		}
		if ui&iform.DontCare != 0 {
			if debugDecode {
				log.Printf("Decode(%#x): unused bit is 1 for Op %s", ui, iform.Op)
			}
			// to match GNU objdump (libopcodes), we ignore don't care bits
		}
		for i, argfield := range iform.Args {
			if argfield == nil {
				break
			}
			inst.Args[i] = argfield.Parse(ui_extn)
		}
		inst.Op = iform.Op
		if debugDecode {
			log.Printf("%#x: search entry %d", ui, i)
			continue
		}
		break
	}
	if inst.Op == 0 && inst.Enc != 0 {
		return inst, errUnknown
	}
	return inst, nil
}
