// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import (
	"sync"
	"unicode"
)

// A patchList is a list of instruction pointers that need to be filled in (patched).
// Because the pointers haven't been filled in yet, we can reuse their storage
// to hold the list. It's kind of sleazy, but works well in practice.
// See https://swtch.com/~rsc/regexp/regexp1.html for inspiration.
//
// These aren't really pointers: they're integers, so we can reinterpret them
// this way without using package unsafe. A value l.head denotes
// p.inst[l.head>>1].Out (l.head&1==0) or .Arg (l.head&1==1).
// head == 0 denotes the empty list, okay because we start every program
// with a fail instruction, so we'll never want to point at its output link.
type patchList struct {
	head, tail uint32
}

func makePatchList(n uint32) patchList {
	return patchList{n, n}
}

func (l patchList) patch(p *Prog, val uint32) {
	head := l.head
	for head != 0 {
		i := &p.Inst[head>>1]
		if head&1 == 0 {
			head = i.Out
			i.Out = val
		} else {
			head = i.Arg
			i.Arg = val
		}
	}
}

func (l1 patchList) append(p *Prog, l2 patchList) patchList {
	if l1.head == 0 {
		return l2
	}
	if l2.head == 0 {
		return l1
	}

	i := &p.Inst[l1.tail>>1]
	if l1.tail&1 == 0 {
		i.Out = l2.head
	} else {
		i.Arg = l2.head
	}
	return patchList{l1.head, l2.tail}
}

// A frag represents a compiled program fragment.
type frag struct {
	i        uint32    // index of first instruction
	out      patchList // where to record end instruction
	nullable bool      // whether fragment can match empty string
}

type compiler struct {
	p *Prog
}

// Compile compiles the regexp into a program to be executed.
// The regexp should have been simplified already (returned from re.Simplify).
func Compile(re *Regexp) (*Prog, error) {
	var c compiler
	c.init()
	f := c.compile(re)
	f.out.patch(c.p, c.inst(InstMatch).i)
	c.p.Start = int(f.i)
	return c.p, nil
}

func (c *compiler) init() {
	c.p = new(Prog)
	c.p.NumCap = 2 // implicit ( and ) for whole match $0
	c.inst(InstFail)
}

var anyRuneNotNL = []rune{0, '\n' - 1, '\n' + 1, unicode.MaxRune}
var anyRune = []rune{0, unicode.MaxRune}

type compileFrame struct {
	re   *Regexp
	next int
	f    frag
}

type compileStack struct {
	frames []compileFrame
}

var compileStackPool sync.Pool

func (c *compiler) compileLeaf(re *Regexp) frag {
	var f frag
	switch re.Op {
	case OpNoMatch:
		f = c.fail()
	case OpEmptyMatch:
		f = c.nop()
	case OpLiteral:
		if len(re.Rune) == 0 {
			f = c.nop()
			break
		}
		for j := range re.Rune {
			f1 := c.rune(re.Rune[j:j+1], re.Flags)
			if j == 0 {
				f = f1
			} else {
				f = c.cat(f, f1)
			}
		}
	case OpCharClass:
		f = c.rune(re.Rune, re.Flags)
	case OpAnyCharNotNL:
		f = c.rune(anyRuneNotNL, 0)
	case OpAnyChar:
		f = c.rune(anyRune, 0)
	case OpBeginLine:
		f = c.empty(EmptyBeginLine)
	case OpEndLine:
		f = c.empty(EmptyEndLine)
	case OpBeginText:
		f = c.empty(EmptyBeginText)
	case OpEndText:
		f = c.empty(EmptyEndText)
	case OpWordBoundary:
		f = c.empty(EmptyWordBoundary)
	case OpNoWordBoundary:
		f = c.empty(EmptyNoWordBoundary)
	default:
		panic("regexp: unhandled case in compile")
	}
	return f
}

func (c *compiler) compile(re *Regexp) frag {
	// Operations before OpCapture have no children and need no traversal stack.
	if re.Op < OpCapture {
		return c.compileLeaf(re)
	}
	// Simplify can create deep expressions from counted repetitions. Use an
	// explicit stack so compiling them does not require deep call stacks.
	var inline [8]compileFrame
	stack := inline[:0]
	var scratch *compileStack
	var f frag
compile:
	for {
		var parent compileFrame
		switch re.Op {
		case OpCapture:
			opening := c.cap(uint32(re.Cap << 1))
			parent = compileFrame{re: re, f: opening}
			goto descend
		case OpStar, OpPlus, OpQuest:
			parent = compileFrame{re: re}
			goto descend
		case OpConcat, OpAlternate:
			if len(re.Sub) == 0 {
				if re.Op == OpConcat {
					f = c.nop()
				} else {
					f = c.fail()
				}
				break
			}
			parent = compileFrame{re: re, next: 1}
			goto descend
		default:
			f = c.compileLeaf(re)
		}

		// f is the completed child. Resume its parent, preserving the
		// recursive compiler's instruction emission and patching order.
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			switch frame.re.Op {
			case OpCapture:
				closing := c.cap(uint32(frame.re.Cap<<1 | 1))
				f = c.cat(c.cat(frame.f, f), closing)
			case OpStar:
				f = c.star(f, frame.re.Flags&NonGreedy != 0)
			case OpPlus:
				f = c.plus(f, frame.re.Flags&NonGreedy != 0)
			case OpQuest:
				f = c.quest(f, frame.re.Flags&NonGreedy != 0)
			case OpConcat, OpAlternate:
				if frame.re.Op == OpAlternate {
					frame.f = c.alt(frame.f, f)
				} else if frame.next == 1 {
					frame.f = f
				} else {
					frame.f = c.cat(frame.f, f)
				}
				if frame.next < len(frame.re.Sub) {
					re = frame.re.Sub[frame.next]
					frame.next++
					continue compile
				}
				f = frame.f
			}
			// A pooled stack must not keep the expression tree alive.
			frame.re = nil
			stack = stack[:len(stack)-1]
		}
		if scratch != nil && cap(stack) <= 4096 {
			// Do not retain oversized buffers from exceptional hand-built trees.
			scratch.frames = scratch.frames[:0]
			compileStackPool.Put(scratch)
		}
		return f

	descend:
		if len(stack) == cap(stack) && scratch == nil {
			// Shallow expressions use the inline buffer. Reuse larger stacks
			// instead of allocating one at every compile of a deep expression.
			scratch, _ = compileStackPool.Get().(*compileStack)
			if scratch == nil {
				scratch = &compileStack{frames: make([]compileFrame, 0, 2*len(stack))}
			}
			scratch.frames = append(scratch.frames[:0], stack...)
			stack = scratch.frames
		}
		if scratch == nil {
			stack = append(stack, parent)
		} else {
			// Keep the pooled slice separate so the inline array stays on the
			// goroutine stack rather than escaping through the pool.
			scratch.frames = append(scratch.frames[:len(stack)], parent)
			stack = scratch.frames
		}
		re = re.Sub[0]
	}
}

func (c *compiler) inst(op InstOp) frag {
	// TODO: impose length limit
	f := frag{i: uint32(len(c.p.Inst)), nullable: true}
	c.p.Inst = append(c.p.Inst, Inst{Op: op})
	return f
}

func (c *compiler) nop() frag {
	f := c.inst(InstNop)
	f.out = makePatchList(f.i << 1)
	return f
}

func (c *compiler) fail() frag {
	return frag{}
}

func (c *compiler) cap(arg uint32) frag {
	f := c.inst(InstCapture)
	f.out = makePatchList(f.i << 1)
	c.p.Inst[f.i].Arg = arg

	if c.p.NumCap < int(arg)+1 {
		c.p.NumCap = int(arg) + 1
	}
	return f
}

func (c *compiler) cat(f1, f2 frag) frag {
	// concat of failure is failure
	if f1.i == 0 || f2.i == 0 {
		return frag{}
	}

	// TODO: elide nop

	f1.out.patch(c.p, f2.i)
	return frag{f1.i, f2.out, f1.nullable && f2.nullable}
}

func (c *compiler) alt(f1, f2 frag) frag {
	// alt of failure is other
	if f1.i == 0 {
		return f2
	}
	if f2.i == 0 {
		return f1
	}

	f := c.inst(InstAlt)
	i := &c.p.Inst[f.i]
	i.Out = f1.i
	i.Arg = f2.i
	f.out = f1.out.append(c.p, f2.out)
	f.nullable = f1.nullable || f2.nullable
	return f
}

func (c *compiler) quest(f1 frag, nongreedy bool) frag {
	f := c.inst(InstAlt)
	i := &c.p.Inst[f.i]
	if nongreedy {
		i.Arg = f1.i
		f.out = makePatchList(f.i << 1)
	} else {
		i.Out = f1.i
		f.out = makePatchList(f.i<<1 | 1)
	}
	f.out = f.out.append(c.p, f1.out)
	return f
}

// loop returns the fragment for the main loop of a plus or star.
// For plus, it can be used after changing the entry to f1.i.
// For star, it can be used directly when f1 can't match an empty string.
// (When f1 can match an empty string, f1* must be implemented as (f1+)?
// to get the priority match order correct.)
func (c *compiler) loop(f1 frag, nongreedy bool) frag {
	f := c.inst(InstAlt)
	i := &c.p.Inst[f.i]
	if nongreedy {
		i.Arg = f1.i
		f.out = makePatchList(f.i << 1)
	} else {
		i.Out = f1.i
		f.out = makePatchList(f.i<<1 | 1)
	}
	f1.out.patch(c.p, f.i)
	return f
}

func (c *compiler) star(f1 frag, nongreedy bool) frag {
	if f1.nullable {
		// Use (f1+)? to get priority match order correct.
		// See golang.org/issue/46123.
		return c.quest(c.plus(f1, nongreedy), nongreedy)
	}
	return c.loop(f1, nongreedy)
}

func (c *compiler) plus(f1 frag, nongreedy bool) frag {
	return frag{f1.i, c.loop(f1, nongreedy).out, f1.nullable}
}

func (c *compiler) empty(op EmptyOp) frag {
	f := c.inst(InstEmptyWidth)
	c.p.Inst[f.i].Arg = uint32(op)
	f.out = makePatchList(f.i << 1)
	return f
}

func (c *compiler) rune(r []rune, flags Flags) frag {
	f := c.inst(InstRune)
	f.nullable = false
	i := &c.p.Inst[f.i]
	i.Rune = r
	flags &= FoldCase // only relevant flag is FoldCase
	if len(r) != 1 || unicode.SimpleFold(r[0]) == r[0] {
		// and sometimes not even that
		flags &^= FoldCase
	}
	i.Arg = uint32(flags)
	f.out = makePatchList(f.i << 1)

	// Special cases for exec machine.
	switch {
	case flags&FoldCase == 0 && (len(r) == 1 || len(r) == 2 && r[0] == r[1]):
		i.Op = InstRune1
	case len(r) == 2 && r[0] == 0 && r[1] == unicode.MaxRune:
		i.Op = InstRuneAny
	case len(r) == 4 && r[0] == 0 && r[1] == '\n'-1 && r[2] == '\n'+1 && r[3] == unicode.MaxRune:
		i.Op = InstRuneAnyNotNL
	}

	return f
}
