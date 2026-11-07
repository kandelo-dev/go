// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

// compileRecursive is the original traversal, retained only as a test oracle.
// Keeping instruction construction shared isolates changes in traversal order.
func (c *compiler) compileRecursive(re *Regexp) frag {
	switch re.Op {
	case OpNoMatch:
		return c.fail()
	case OpEmptyMatch:
		return c.nop()
	case OpLiteral:
		if len(re.Rune) == 0 {
			return c.nop()
		}
		var f frag
		for j := range re.Rune {
			f1 := c.rune(re.Rune[j:j+1], re.Flags)
			if j == 0 {
				f = f1
			} else {
				f = c.cat(f, f1)
			}
		}
		return f
	case OpCharClass:
		return c.rune(re.Rune, re.Flags)
	case OpAnyCharNotNL:
		return c.rune(anyRuneNotNL, 0)
	case OpAnyChar:
		return c.rune(anyRune, 0)
	case OpBeginLine:
		return c.empty(EmptyBeginLine)
	case OpEndLine:
		return c.empty(EmptyEndLine)
	case OpBeginText:
		return c.empty(EmptyBeginText)
	case OpEndText:
		return c.empty(EmptyEndText)
	case OpWordBoundary:
		return c.empty(EmptyWordBoundary)
	case OpNoWordBoundary:
		return c.empty(EmptyNoWordBoundary)
	case OpCapture:
		bra := c.cap(uint32(re.Cap << 1))
		sub := c.compileRecursive(re.Sub[0])
		ket := c.cap(uint32(re.Cap<<1 | 1))
		return c.cat(c.cat(bra, sub), ket)
	case OpStar:
		return c.star(c.compileRecursive(re.Sub[0]), re.Flags&NonGreedy != 0)
	case OpPlus:
		return c.plus(c.compileRecursive(re.Sub[0]), re.Flags&NonGreedy != 0)
	case OpQuest:
		return c.quest(c.compileRecursive(re.Sub[0]), re.Flags&NonGreedy != 0)
	case OpConcat:
		if len(re.Sub) == 0 {
			return c.nop()
		}
		var f frag
		for i, sub := range re.Sub {
			if i == 0 {
				f = c.compileRecursive(sub)
			} else {
				f = c.cat(f, c.compileRecursive(sub))
			}
		}
		return f
	case OpAlternate:
		var f frag
		for _, sub := range re.Sub {
			f = c.alt(f, c.compileRecursive(sub))
		}
		return f
	}
	panic("regexp: unhandled case in compile")
}

func TestCompileTraversalEquivalent(t *testing.T) {
	random := rand.New(rand.NewPCG(0x5eed, 0xc0ffee))
	atoms := []string{"a", "[a-c]", "[^x]", ".", "(?:)", `\pL`, `\d`}
	var expression func(int) string
	expression = func(depth int) string {
		if depth == 0 {
			return atoms[random.IntN(len(atoms))]
		}
		child := expression(depth - 1)
		switch random.IntN(9) {
		case 0:
			return "(" + child + ")"
		case 1:
			return "(?:" + child + ")?"
		case 2:
			return "(?:" + child + ")+?"
		case 3:
			return "(?:" + child + ")*"
		case 4:
			return "(?:" + child + "){0,4}?"
		case 5:
			return "(?:" + child + "|" + expression(depth-1) + ")"
		case 6:
			return child + expression(depth-1)
		case 7:
			return "(?i:" + child + ")"
		default:
			return "(?s:" + child + ")"
		}
	}
	// Keep the recursive oracle shallow so the test itself also works on
	// targets with a limited native call stack. Deep programs have separate
	// nonrecursive checks.
	patterns := []string{
		`^www\.[-a-zA-Z0-9@:%._+~#=]{1,16}\.[a-z]+$`,
		`(a*?){0,4}?`, `(a+?){2,4}`, `((a|b)*?){0,3}?`,
		`\A(?:\b[a-z]+\B|\d+)$`, `(?m)^[^\n]*?$`,
	}
	for _, tt := range compileTests {
		patterns = append(patterns, tt.Regexp)
	}
	for range 2000 {
		patterns = append(patterns, expression(4))
	}
	for _, pattern := range patterns {
		re, err := Parse(pattern, Perl)
		if err != nil {
			t.Fatalf("parse %q: %v", pattern, err)
		}
		re = re.Simplify()
		var reference compiler
		reference.init()
		f := reference.compileRecursive(re)
		f.out.patch(reference.p, reference.inst(InstMatch).i)
		reference.p.Start = int(f.i)
		got, err := Compile(re)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, reference.p) {
			t.Fatalf("bytecode changed for %q\nold:\n%s\nnew:\n%s", pattern, reference.p, got)
		}
	}
	t.Logf("identical programs for %d patterns", len(patterns))
}
