package loadwasm

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

func appendUnsigned(data []byte, value uint64) []byte {
	return binary.AppendUvarint(data, value)
}

func appendName(data []byte, name string) []byte {
	data = appendUnsigned(data, uint64(len(name)))
	return append(data, name...)
}

func appendSection(data []byte, kind byte, payload []byte) []byte {
	data = append(data, kind)
	data = appendUnsigned(data, uint64(len(payload)))
	return append(data, payload...)
}

func testObject() []byte {
	data := []byte("\x00asm\x01\x00\x00\x00")
	types := []byte{2, 0x60, 0, 1, 0x7f, 0x60, 1, 0x7f, 0}
	data = appendSection(data, 1, types)
	importPayload := appendName([]byte{1}, "env")
	importPayload = appendName(importPayload, "c_func")
	importPayload = append(importPayload, 0, 0)
	data = appendSection(data, 2, importPayload)
	data = appendSection(data, 3, []byte{1, 1})
	data = appendSection(data, 10, []byte{1, 4, 0, 0x10, 0, 0x0b})
	symbols := []byte{2, 0, 0, 1}
	symbols = appendName(symbols, "local")
	symbols = append(symbols, 0, 0x10, 0)
	linking := appendUnsigned(nil, 2)
	linking = append(linking, 8)
	linking = appendUnsigned(linking, uint64(len(symbols)))
	linking = append(linking, symbols...)
	data = appendSection(data, 0, append(appendName(nil, "linking"), linking...))
	return data
}

func testObjectWithRelocation(kind byte) []byte {
	data := testObject()
	relocations := []byte{3, 1, kind, 4, 1}
	return appendSection(data, 0, append(appendName(nil, "reloc.CODE"), relocations...))
}

func TestParse(t *testing.T) {
	data := testObjectWithRelocation(0)
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.Symbols) != 2 || object.Symbols[0].Name != "local" || object.Symbols[1].Name != "c_func" {
		t.Fatalf("unexpected symbols: %+v", object.Symbols)
	}
	if len(object.CodeRelocations) != 1 || object.CodeRelocations[0].Symbol.Name != "c_func" {
		t.Fatalf("unexpected relocations: %+v", object.CodeRelocations)
	}
	if len(object.Functions) != 1 || object.Functions[0].Name != "local" || object.Functions[0].Type.Params[0] != 0x7f {
		t.Fatalf("unexpected functions: %+v", object.Functions)
	}
	patched, err := object.RelocateFunction(object.Functions[0], map[string]uint32{"c_func": 42}, nil)
	if err != nil || patched[2] != 42 || object.Functions[0].Body[2] != 0 {
		t.Fatalf("unexpected relocated body %x: %v", patched, err)
	}
}

func TestParseRejectsInvalidObject(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("\x00asm\x01\x00\x00\x00\x00\xff"), testObject()[:9], testObjectWithRelocation(6)} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("accepted invalid object %x", data)
		}
	}
}

func TestSDKObject(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_OBJECT to a compiled SDK C object")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.Symbols) == 0 || len(object.CodeRelocations) == 0 {
		t.Fatalf("missing symbols or CODE relocations: %+v", object)
	}
	for _, relocation := range object.CodeRelocations {
		if relocation.Symbol.Name == "" {
			t.Fatalf("unresolved CODE relocation symbol: %+v", relocation)
		}
		t.Logf("type=%d offset=%#x symbol=%s", relocation.Type, relocation.Offset, relocation.Symbol.Name)
	}
	for _, function := range object.Functions {
		t.Logf("function=%s index=%d params=%x results=%x body=%d bytes", function.Name, function.Index, function.Type.Params, function.Type.Results, len(function.Body))
	}
}

func TestSDKCgoCallRelocation(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_CGO_CALL_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_CGO_CALL_OBJECT to the C.abs cgo shim object")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.Functions) != 1 || !strings.HasSuffix(object.Functions[0].Name, "_Cfunc_abs") {
		t.Fatalf("unexpected C-call functions: %+v", object.Functions)
	}
	function := object.Functions[0]
	patched, err := object.RelocateFunction(function, map[string]uint32{"_cgo_topofstack": 42}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.CodeRelocations) != 2 {
		t.Fatalf("unexpected C-call relocations: %+v", object.CodeRelocations)
	}
	for _, relocation := range object.CodeRelocations {
		offset := int(relocation.Offset - function.BodyOffset)
		value, width := binary.Uvarint(patched[offset:])
		if value != 42 || width != 5 || function.Body[offset] == patched[offset] {
			t.Fatalf("relocation at %#x not applied: %x", relocation.Offset, patched[offset:offset+5])
		}
	}
}
