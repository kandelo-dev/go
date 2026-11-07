// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ld

import (
	"bytes"
	"cmd/internal/bio"
	"os"
	"path/filepath"
	"testing"
)

func TestEmptyBSDArchive(t *testing.T) {
	// llvm-ar's actual object-free Darwin archive, including its padded name.
	original := []byte("!<arch>\n#1/12           0           0     0     0       20        `\n__.SYMDEF\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	mutate := func(offset int, value byte) []byte {
		data := bytes.Clone(original)
		data[offset] = value
		return data
	}
	for _, test := range []struct {
		name string
		data []byte
		want bool
	}{
		{"empty index", original, true},
		{"nonzero ranlib entries", mutate(80, 8), false},
		{"nonzero string table", mutate(84, 1), false},
		{"unknown member name", mutate(68, 'x'), false},
		{"invalid extended name", mutate(11, '-'), false},
		{"invalid header trailer", mutate(66, '!'), false},
		{"truncated index", original[:len(original)-1], false},
		{"additional member", append(bytes.Clone(original), []byte("object.o/       0           0     0     0       4         `\nwasm")...), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.a")
			if err := os.WriteFile(path, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			input, err := bio.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			var header ArHdr
			if nextar(input, SARMAG, &header) <= 0 {
				t.Fatal("missing fixture header")
			}
			if got := emptyBSDArchive(input, header, int64(len(test.data))); got != test.want {
				t.Fatalf("object-free=%v, want %v, header=%#v, length=%d", got, test.want, header, len(test.data))
			}
		})
	}
}
