package loadwasm

import (
	"encoding/binary"
	"os"
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
	importPayload := appendName([]byte{1}, "env")
	importPayload = appendName(importPayload, "c_func")
	importPayload = append(importPayload, 0, 0)
	data = appendSection(data, 2, importPayload)
	data = appendSection(data, 10, []byte{1, 2, 0, 0x0b})
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
	relocations := []byte{1, 1, kind, 0, 1}
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
}
