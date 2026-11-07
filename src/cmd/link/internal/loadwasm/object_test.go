package loadwasm

import (
	"bytes"
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

func testObjectWithInit(symbolIndex uint64) []byte {
	data := []byte("\x00asm\x01\x00\x00\x00")
	data = appendSection(data, 1, []byte{1, 0x60, 0, 0})
	data = appendSection(data, 3, []byte{1, 0})
	data = appendSection(data, 10, []byte{1, 2, 0, 0x0b})
	symbols := appendName([]byte{1, 0, 0, 0}, "constructor")
	initializers := appendUnsigned([]byte{1}, 65535)
	initializers = appendUnsigned(initializers, symbolIndex)
	linking := appendUnsigned(nil, 2)
	linking = appendSection(linking, 6, initializers)
	linking = appendSection(linking, 8, symbols)
	return appendSection(data, 0, append(appendName(nil, "linking"), linking...))
}

func TestInitFunctions(t *testing.T) {
	object, err := Parse(testObjectWithInit(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(object.InitFunctions) != 1 || object.InitFunctions[0].Priority != 65535 || object.InitFunctions[0].Symbol.Name != "constructor" {
		t.Fatalf("unexpected Wasm initializer: %+v", object.InitFunctions)
	}
	if _, err := Parse(testObjectWithInit(1)); err == nil {
		t.Fatal("accepted initializer with missing symbol")
	}
}

func testObjectWithData() []byte {
	data := []byte("\x00asm\x01\x00\x00\x00")
	data = appendSection(data, 1, []byte{1, 0x60, 0, 0})
	data = appendSection(data, 3, []byte{1, 0})
	data = appendSection(data, 10, []byte{1, 9, 0, 0x41, 0x80, 0x80, 0x80, 0x80, 0, 0x1a, 0x0b})
	data = appendSection(data, 11, []byte{1, 0, 0x41, 0, 0x0b, 4, 't', 'e', 's', 't'})
	segment := appendName([]byte{1}, ".rodata")
	segment = append(segment, 2, 0)
	symbols := appendName([]byte{2, 0, 0, 0}, "local")
	symbols = appendName(append(symbols, 1, 2), "message")
	symbols = append(symbols, 0, 0, 4)
	linking := appendUnsigned(nil, 2)
	linking = append(linking, 5)
	linking = appendUnsigned(linking, uint64(len(segment)))
	linking = append(linking, segment...)
	linking = append(linking, 8)
	linking = appendUnsigned(linking, uint64(len(symbols)))
	linking = append(linking, symbols...)
	data = appendSection(data, 0, append(appendName(nil, "linking"), linking...))
	return appendSection(data, 0, append(appendName(nil, "reloc.CODE"), []byte{2, 1, 11, 4, 1, 0x7d}...))
}

func TestParseDataAndMemoryRelocation(t *testing.T) {
	object, err := Parse(testObjectWithData())
	if err != nil {
		t.Fatal(err)
	}
	if len(object.DataSegments) != 1 || object.DataSegments[0].Name != ".rodata" || object.DataSegments[0].Align != 4 || string(object.DataSegments[0].Data) != "test" {
		t.Fatalf("unexpected data segments: %+v", object.DataSegments)
	}
	if len(object.Symbols) != 2 || object.Symbols[1].Name != "message" || object.Symbols[1].Index != 0 || object.Symbols[1].Size != 4 {
		t.Fatalf("unexpected data symbol: %+v", object.Symbols)
	}
	if len(object.CodeRelocations) != 1 || object.CodeRelocations[0].Type != 11 || object.CodeRelocations[0].Addend != -3 || object.CodeRelocations[0].Symbol.Name != "message" {
		t.Fatalf("unexpected memory relocation: %+v", object.CodeRelocations)
	}
	patched, err := object.RelocateFunction(object.Functions[0], nil, nil, map[string]uint32{"message": 0x1234}, nil, nil, nil)
	if err != nil || !bytes.Equal(patched[2:7], []byte{0xb1, 0xa4, 0x80, 0x80, 0}) {
		t.Fatalf("unexpected relocated memory address %x: %v", patched, err)
	}
	if _, err := object.RelocateFunction(object.Functions[0], nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("accepted memory relocation without a memory layout")
	}
}

func TestParseRejectsMalformedData(t *testing.T) {
	data := testObjectWithData()
	for _, invalid := range [][]byte{
		data[:len(data)-1],
		bytes.Replace(data, []byte("\x0b\x04test"), []byte("\x0b\x05test"), 1),
	} {
		if _, err := Parse(invalid); err == nil {
			t.Fatalf("accepted malformed data object %x", invalid)
		}
	}
}

func TestParse(t *testing.T) {
	data := testObjectWithRelocation(0)
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.FunctionImports) != 1 || object.FunctionImports[0] != (FunctionImport{Module: "env", Name: "c_func", TypeIndex: 0}) {
		t.Fatalf("unexpected function imports: %+v", object.FunctionImports)
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
	patched, err := object.RelocateFunction(object.Functions[0], map[string]uint32{"c_func": 42}, nil, nil, nil, nil, nil)
	if err != nil || patched[2] != 42 || object.Functions[0].Body[2] != 0 {
		t.Fatalf("unexpected relocated body %x: %v", patched, err)
	}
}

func TestTypeAndTableRelocations(t *testing.T) {
	for _, kind := range []byte{6, 20} {
		object, err := Parse(testObjectWithRelocation(kind))
		if err != nil {
			t.Fatal(err)
		}
		patched, err := object.RelocateFunction(object.Functions[0], nil, nil, nil, map[string]uint32{"c_func": 42}, nil, map[uint32]uint32{1: 42})
		if err != nil || patched[2] != 42 {
			t.Fatalf("relocation kind %d = %x: %v", kind, patched, err)
		}
	}
}

func TestTableFunctionPointerRelocations(t *testing.T) {
	for _, kind := range []byte{1, 2, 12} {
		object := &Object{CodeRelocations: []Relocation{{Type: kind, Symbol: Symbol{Name: "c_func"}}}}
		body := []byte{0x80, 0x80, 0x80, 0x80, 0}
		if kind == 2 {
			body = make([]byte, 4)
		}
		patched, err := object.RelocateFunction(Function{Body: body}, nil, nil, nil, nil, map[string]uint32{"c_func": 4096}, nil)
		if err != nil {
			t.Fatalf("relocation kind %d: %v", kind, err)
		}
		if kind == 2 {
			if binary.LittleEndian.Uint32(patched) != 4096 {
				t.Fatalf("relocation kind %d = %x", kind, patched)
			}
		} else {
			value := reader{data: patched}
			if index, err := value.signed(); err != nil || index != 4096 {
				t.Fatalf("relocation kind %d = %d: %v", kind, index, err)
			}
		}
	}
}

func TestTagRelocation(t *testing.T) {
	object := &Object{CodeRelocations: []Relocation{{Type: 10, Symbol: Symbol{Kind: 4, Name: "__c_longjmp"}}}}
	function := Function{Body: []byte{0x80, 0x80, 0x80, 0x80, 0x00}}
	patched, err := object.RelocateFunction(function, nil, nil, nil, nil, nil, nil, nil, map[string]uint32{"__c_longjmp": 1})
	if err != nil || patched[0] != 0x81 {
		t.Fatalf("tag relocation = %x: %v", patched, err)
	}
	if function.Body[0] != 0x80 {
		t.Fatal("relocation modified the source body")
	}
}

func TestElementSegments(t *testing.T) {
	functions := []Function{{Index: 1, Name: "local"}}
	segments, err := elementSegments([]byte{1, 0, 0x41, 1, 0x0b, 1, 1}, 1, functions)
	if err != nil || len(segments) != 1 || segments[0].Offset != 1 || len(segments[0].Functions) != 1 || segments[0].Functions[0] != 1 {
		t.Fatalf("element segments = %+v: %v", segments, err)
	}
	imported, err := elementSegments([]byte{1, 0, 0x41, 1, 0x0b, 1, 0}, 1, functions)
	if err != nil || len(imported) != 1 || len(imported[0].Functions) != 1 || imported[0].Functions[0] != 0 {
		t.Fatalf("imported element segments = %+v: %v", imported, err)
	}
	for _, data := range [][]byte{
		{1, 1, 0x41, 1, 0x0b, 1, 1},
		{1, 0, 0x41, 1, 0x0b, 1, 2},
		{1, 0, 0x41, 1, 0x0b, 1},
	} {
		if _, err := elementSegments(data, 1, functions); err == nil {
			t.Fatalf("accepted invalid element segment %x", data)
		}
	}
}

func TestParseRejectsInvalidObject(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("\x00asm\x01\x00\x00\x00\x00\xff"), testObject()[:9], testObjectWithRelocation(21)} {
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
	patched, err := object.RelocateFunction(function, map[string]uint32{"_cgo_topofstack": 42}, nil, nil, nil, nil, nil)
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

func TestSDKRuntimeCgoData(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_RUNTIME_CGO_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_RUNTIME_CGO_OBJECT to a runtime/cgo C object")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.DataSegments) != 2 || object.DataSegments[0].Name != ".rodata..L.str" || len(object.DataSegments[0].Data) != 44 || object.DataSegments[1].Name != ".rodata._cgo_yield" || object.DataSegments[1].Align != 4 {
		t.Fatalf("unexpected runtime/cgo data segments: %+v", object.DataSegments)
	}
	found := false
	for _, relocation := range object.CodeRelocations {
		if relocation.Type == 11 && relocation.Symbol.Name == ".L.str" && relocation.Addend == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing runtime/cgo memory relocation: %+v", object.CodeRelocations)
	}
}

func TestSDKRuntimeCgoContext(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_RUNTIME_CGO_CONTEXT_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_RUNTIME_CGO_CONTEXT_OBJECT to a runtime/cgo context object")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	foundType := false
	foundTable := false
	for _, relocation := range object.CodeRelocations {
		foundType = foundType || relocation.Type == 6
		foundTable = foundTable || relocation.Type == 20 && relocation.Symbol.Name == "__indirect_function_table"
	}
	if !foundType || !foundTable {
		t.Fatalf("missing type or table relocation: %+v", object.CodeRelocations)
	}
}

func TestSDKRuntimeCgoElement(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_RUNTIME_CGO_ELEMENT_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_RUNTIME_CGO_ELEMENT_OBJECT to a runtime/cgo object with a table element")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.ElementSegments) != 1 || len(object.ElementSegments[0].Functions) != 1 {
		t.Fatalf("unexpected element segments: %+v", object.ElementSegments)
	}
	functionIndex := object.ElementSegments[0].Functions[0]
	found := false
	for _, function := range object.Functions {
		found = found || function.Index == functionIndex && function.Name == "pthread_key_destructor"
	}
	if !found {
		t.Fatalf("missing table function %d", functionIndex)
	}
	for _, relocation := range object.CodeRelocations {
		if relocation.Type != 12 || relocation.Symbol.Name != "pthread_key_destructor" {
			continue
		}
		for _, function := range object.Functions {
			if relocation.Offset < function.BodyOffset || uint64(relocation.Offset) >= uint64(function.BodyOffset)+uint64(len(function.Body)) {
				continue
			}
			isolated := *object
			isolated.CodeRelocations = []Relocation{relocation}
			patched, err := isolated.RelocateFunction(function, nil, nil, nil, nil, map[string]uint32{"pthread_key_destructor": 4096}, nil)
			if err != nil {
				t.Fatal(err)
			}
			offset := relocation.Offset - function.BodyOffset
			value := reader{data: patched[offset:]}
			if index, err := value.signed(); err != nil || index != 4096 {
				t.Fatalf("table slot = %d: %v", index, err)
			}
			return
		}
	}
	t.Fatal("missing table-index relocation")
}

func TestSDKDataRelocations(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_DATA_RELOCATION_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_DATA_RELOCATION_OBJECT to an SDK object with DATA relocations")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.DataRelocations) != 1 || object.DataRelocations[0].Type != 5 || object.DataRelocations[0].Symbol.Name != "lock" {
		t.Fatalf("unexpected DATA relocations: %+v", object.DataRelocations)
	}
}

func TestSDKTLSSegment(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_TLS_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_TLS_OBJECT to an SDK object with a TLS segment")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(object.DataSegments) != 2 || object.DataSegments[1].Name != ".tbss.__wasm_thread_pointer" || object.DataSegments[1].Flags&2 == 0 {
		t.Fatalf("missing C TLS segment: %+v", object.DataSegments)
	}
}

func TestSDKTLSRelocation(t *testing.T) {
	path := os.Getenv("KANDELO_WASM_TLS_RELOCATION_OBJECT")
	if path == "" {
		t.Skip("set KANDELO_WASM_TLS_RELOCATION_OBJECT to an SDK object with a TLS CODE relocation")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, relocation := range object.CodeRelocations {
		if relocation.Type == 21 && relocation.Symbol.Name == "__wasm_thread_pointer" && relocation.Symbol.Flags&0x100 != 0 && relocation.Addend == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing TLS relocation: %+v", object.CodeRelocations)
	}
}
