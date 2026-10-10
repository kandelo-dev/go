package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func wasmTestSection(sectionID byte, payload []byte) []byte {
	section := []byte{sectionID}
	section = binary.AppendUvarint(section, uint64(len(payload)))
	return append(section, payload...)
}

func TestReadWasmDebugModule(t *testing.T) {
	moduleBytes := []byte("\x00asm\x01\x00\x00\x00")
	custom := binary.AppendUvarint(nil, uint64(len(".debug_str")))
	custom = append(custom, ".debug_str"...)
	custom = append(custom, "sample"...)
	moduleBytes = append(moduleBytes, wasmTestSection(0, custom)...)
	moduleBytes = append(moduleBytes, wasmTestSection(6, []byte{1, 0x7f, 0, 0x41, 32, 0x0b})...)
	moduleBytes = append(moduleBytes, wasmTestSection(7, []byte{1, 5, 'v', 'a', 'l', 'u', 'e', 3, 0})...)
	moduleBytes = append(moduleBytes, wasmTestSection(11, []byte{1, 0, 0x41, 32, 0x0b, 4, 1, 2, 3, 4})...)
	path := filepath.Join(t.TempDir(), "module.wasm")
	if err := os.WriteFile(path, moduleBytes, 0600); err != nil {
		t.Fatal(err)
	}
	module, err := readWasmDebugModule(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := module.sections[".debug_str"]; !bytes.Equal(got, []byte("sample")) {
		t.Fatalf("debug section = %q", got)
	}
	contents, err := module.memory("value", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte{1, 2, 3, 4}) {
		t.Fatalf("memory = %v", contents)
	}
	if _, err := module.memory("value", 5); err == nil {
		t.Fatal("out-of-bounds memory read succeeded")
	}
}

func TestReadWasmDebugModuleRejectsTruncatedSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.wasm")
	contents := append([]byte("\x00asm\x01\x00\x00\x00"), 0, 5, 0)
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWasmDebugModule(path); err == nil {
		t.Fatal("truncated Wasm section was accepted")
	}
}

func TestReadWasmDebugModuleWithImportedGlobal(t *testing.T) {
	moduleBytes := []byte("\x00asm\x01\x00\x00\x00")
	imports := []byte{1, 3, 'e', 'n', 'v', 3, 't', 'l', 's', 3, 0x7f, 1}
	moduleBytes = append(moduleBytes, wasmTestSection(2, imports)...)
	moduleBytes = append(moduleBytes, wasmTestSection(6, []byte{1, 0x7f, 0, 0x41, 32, 0x0b})...)
	moduleBytes = append(moduleBytes, wasmTestSection(7, []byte{1, 5, 'v', 'a', 'l', 'u', 'e', 3, 1})...)
	moduleBytes = append(moduleBytes, wasmTestSection(11, []byte{1, 0, 0x41, 32, 0x0b, 4, 1, 2, 3, 4})...)
	path := filepath.Join(t.TempDir(), "imported-global.wasm")
	if err := os.WriteFile(path, moduleBytes, 0600); err != nil {
		t.Fatal(err)
	}
	module, err := readWasmDebugModule(path)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := module.memory("value", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte{1, 2, 3, 4}) {
		t.Fatalf("memory = %v", contents)
	}
}
