package main

import (
	"bytes"
	"debug/dwarf"
	"encoding/binary"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKandeloWasmCgoPointerStructFields(t *testing.T) {
	previousOS, previousArch := goos, goarch
	goos, goarch = "kandelo", "wasm32"
	t.Cleanup(func() { goos, goarch = previousOS, previousArch })
	integer := &dwarf.IntType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{Name: "int", ByteSize: 4}}}
	pointer := &dwarf.PtrType{CommonType: dwarf.CommonType{ByteSize: 4}, Type: integer}
	array := &dwarf.ArrayType{Type: pointer, Count: 2}
	converter := new(typeConv)
	converter.Init(4, 4)
	if _, ok := converter.Type(pointer, token.NoPos).Go.(*ast.StarExpr); !ok {
		t.Fatal("C pointers must retain ordinary typed Go pointer fields")
	}
	mapped, ok := converter.Type(array, token.NoPos).Go.(*ast.ArrayType)
	if !ok {
		t.Fatal("C pointer array lost its array type")
	}
	if _, ok := mapped.Elt.(*ast.StarExpr); !ok {
		t.Fatal("C pointer arrays must retain typed pointer elements")
	}
}

func TestKandeloWasmCgoFrameLayout(t *testing.T) {
	previousOS, previousArch := goos, goarch
	goos, goarch = "kandelo", "wasm32"
	t.Cleanup(func() { goos, goarch = previousOS, previousArch })
	integer := &Type{Size: 4, Align: 4, C: c("int"), Go: ast.NewIdent("int32")}
	pointer := &Type{Size: 4, Align: 4, C: c("int*"), Go: &ast.StarExpr{X: ast.NewIdent("int32")}}
	name := &Name{FuncType: &FuncType{Params: []*Type{integer, pointer, integer}, Result: integer}}
	layout, size := (&Package{PtrSize: 4}).structType(name)
	if size != 16 || strings.Contains(layout, "__pad") {
		t.Fatalf("unexpected Go/C frame layout: size %d, struct %s", size, layout)
	}
}

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

func TestWasmStaticImportNamespaces(t *testing.T) {
	for _, name := range []string{"env", "kernel", "native-library"} {
		payload := []byte{1}
		payload = binary.AppendUvarint(payload, uint64(len(name)))
		payload = append(payload, name...)
		payload = append(payload, 1, 'f', 0, 0) // one function import, type index zero
		module := new(wasmDebugModule)
		if err := module.readImports(payload); err != nil {
			t.Fatal(err)
		}
		err := module.checkStaticImports()
		if name == "native-library" {
			if err == nil {
				t.Fatal("unsupported dynamic library import was accepted")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWasmPassiveDataIsNotAddressable(t *testing.T) {
	module := &wasmDebugModule{globalExports: map[string]uint64{"value": 32}}
	// One passive segment; its contents must not be assigned an invented address.
	if err := module.readData([]byte{1, 1, 4, 1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if len(module.dataSegments) != 0 {
		t.Fatal("passive segment became active")
	}
	if _, err := module.memory("value", 4); err == nil {
		t.Fatal("passive constants were treated as initialized memory")
	}
	if err := module.readData([]byte{1, 1, 4, 1}); err == nil {
		t.Fatal("truncated passive bytes were accepted")
	}
}

func TestWasm32FunctionPointersAreNotHeapPointers(t *testing.T) {
	previousArch := goarch
	goarch = "wasm32"
	t.Cleanup(func() { goarch = previousArch })
	function := &dwarf.FuncType{CommonType: dwarf.CommonType{ByteSize: -1}}
	functionPointer := &dwarf.PtrType{CommonType: dwarf.CommonType{ByteSize: 4}, Type: function}
	converter := new(typeConv)
	converter.Init(4, 4)
	mapped := converter.Type(functionPointer, token.NoPos)
	converter.FinishType(token.NoPos)
	pointer, ok := mapped.Go.(*ast.StarExpr)
	if !ok {
		t.Fatalf("function pointer type = %T", mapped.Go)
	}
	opaque, ok := pointer.X.(*ast.Ident)
	if !ok || opaque.Name != incomplete {
		t.Fatalf("function pointer must use cgo's non-heap opaque type: %#v", pointer.X)
	}
	if mapped.Size != 4 {
		t.Fatalf("function pointer size = %d", mapped.Size)
	}
}

func TestWasm32ScalarAlignment(t *testing.T) {
	previousArch := goarch
	goarch = "wasm32"
	t.Cleanup(func() { goarch = previousArch })
	converter := new(typeConv)
	converter.Init(4, 4)
	for _, test := range []struct {
		name  string
		dtype dwarf.Type
		align int64
	}{
		{"int32", &dwarf.IntType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{ByteSize: 4, Name: "int"}}}, 4},
		{"int64", &dwarf.IntType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{ByteSize: 8, Name: "long long int"}}}, 8},
		{"float64", &dwarf.FloatType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{ByteSize: 8, Name: "double"}}}, 8},
		{"complex64", &dwarf.ComplexType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{ByteSize: 8, Name: "complex float"}}}, 4},
		{"complex128", &dwarf.ComplexType{BasicType: dwarf.BasicType{CommonType: dwarf.CommonType{ByteSize: 16, Name: "complex double"}}}, 8},
	} {
		if got := converter.Type(test.dtype, token.NoPos).Align; got != test.align {
			t.Fatalf("%s alignment = %d, want %d", test.name, got, test.align)
		}
	}
	integer := &Type{Size: 4, Align: 4, C: c("int"), Go: ast.NewIdent("int32")}
	wide := &Type{Size: 8, Align: 8, C: c("long long"), Go: ast.NewIdent("int64")}
	call := &Name{FuncType: &FuncType{Params: []*Type{integer, wide, integer}, Result: wide}}
	layout, size := (&Package{PtrSize: 4}).structType(call)
	if size != 32 || !strings.Contains(layout, "__pad4[4]") || !strings.Contains(layout, "__pad20[4]") {
		t.Fatalf("wide frame: %d %s", size, layout)
	}
}
