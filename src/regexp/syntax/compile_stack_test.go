// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import "testing"

func TestCompileDeepConcat(t *testing.T) {
	const depth = 10000
	literal := &Regexp{Op: OpLiteral, Rune: []rune{'a'}}
	re := literal
	for i := 0; i < depth; i++ {
		// Reuse the same leaf to check that a shared subtree is compiled
		// at each occurrence, rather than being memoized.
		re = &Regexp{Op: OpConcat, Sub: []*Regexp{re, literal}}
	}
	p, err := Compile(re)
	if err != nil {
		t.Fatal(err)
	}
	if p.Start != 1 || p.NumCap != 2 || len(p.Inst) != depth+3 {
		t.Fatalf("start=%d captures=%d instructions=%d", p.Start, p.NumCap, len(p.Inst))
	}
	for i := 1; i < len(p.Inst)-1; i++ {
		inst := &p.Inst[i]
		if inst.Op != InstRune1 || len(inst.Rune) != 1 || inst.Rune[0] != 'a' || inst.Out != uint32(i+1) {
			t.Fatalf("instruction %d: %v", i, inst)
		}
	}
	if p.Inst[0].Op != InstFail || p.Inst[len(p.Inst)-1].Op != InstMatch {
		t.Fatal("incorrect program boundaries")
	}
}

func TestCompileDeepCapture(t *testing.T) {
	const depth = 10000
	re := &Regexp{Op: OpLiteral, Rune: []rune{'a'}}
	for i := 0; i < depth; i++ {
		re = &Regexp{Op: OpCapture, Cap: 1, Sub: []*Regexp{re}}
	}
	p, err := Compile(re)
	if err != nil {
		t.Fatal(err)
	}
	if p.Start != 1 || p.NumCap != 4 || len(p.Inst) != 2*depth+3 {
		t.Fatalf("start=%d captures=%d instructions=%d", p.Start, p.NumCap, len(p.Inst))
	}
	for i := 1; i < len(p.Inst)-1; i++ {
		inst := &p.Inst[i]
		if inst.Out != uint32(i+1) {
			t.Fatalf("instruction %d: incorrect successor %d", i, inst.Out)
		}
		switch {
		case i <= depth:
			if inst.Op != InstCapture || inst.Arg != 2 {
				t.Fatalf("opening instruction %d: %v", i, inst)
			}
		case i == depth+1:
			if inst.Op != InstRune1 {
				t.Fatalf("literal instruction: %v", inst)
			}
		default:
			if inst.Op != InstCapture || inst.Arg != 3 {
				t.Fatalf("closing instruction %d: %v", i, inst)
			}
		}
	}
	if p.Inst[0].Op != InstFail || p.Inst[len(p.Inst)-1].Op != InstMatch {
		t.Fatal("incorrect program boundaries")
	}
}
