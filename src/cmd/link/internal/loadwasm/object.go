package loadwasm

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Section struct {
	ID   byte
	Name string
	Data []byte
}

type Symbol struct {
	Kind   byte
	Flags  uint32
	Index  uint32
	Name   string
	Offset uint32
	Size   uint32
}

type Relocation struct {
	Type      byte
	Offset    uint32
	Symbol    Symbol
	TypeIndex uint32
	Addend    int32
}

type DataSegment struct {
	Name    string
	Address uint32
	Offset  uint32
	Data    []byte
	Align   uint32
	Flags   uint32
}

type FuncType struct {
	Params  []byte
	Results []byte
}

type FunctionImport struct {
	Module    string
	Name      string
	TypeIndex uint32
}

type Function struct {
	Index      uint32
	Type       FuncType
	Body       []byte
	BodyOffset uint32
	Name       string
}

type ElementSegment struct {
	Offset    uint32
	Functions []uint32
}

type Object struct {
	Sections        []Section
	Symbols         []Symbol
	CodeRelocations []Relocation
	DataRelocations []Relocation
	Types           []FuncType
	FunctionImports []FunctionImport
	Functions       []Function
	DataSegments    []DataSegment
	ElementSegments []ElementSegment
}

type reader struct {
	data   []byte
	offset int
}

func (input *reader) byte() (byte, error) {
	if input.offset == len(input.data) {
		return 0, fmt.Errorf("truncated Wasm object")
	}
	value := input.data[input.offset]
	input.offset++
	return value, nil
}

func (input *reader) unsigned() (uint32, error) {
	value, size := binary.Uvarint(input.data[input.offset:])
	if size <= 0 || value > uint64(^uint32(0)) {
		return 0, fmt.Errorf("invalid Wasm unsigned integer")
	}
	input.offset += size
	return uint32(value), nil
}

func (input *reader) signed() (int32, error) {
	var value int64
	for shift := uint(0); shift < 35; shift += 7 {
		current, err := input.byte()
		if err != nil {
			return 0, err
		}
		value |= int64(current&0x7f) << shift
		if current&0x80 == 0 {
			if current&0x40 != 0 {
				value |= ^int64(0) << (shift + 7)
			}
			if value < -1<<31 || value > 1<<31-1 {
				return 0, fmt.Errorf("Wasm signed integer out of range")
			}
			return int32(value), nil
		}
	}
	return 0, fmt.Errorf("invalid Wasm signed integer")
}

func (input *reader) bytes() ([]byte, error) {
	length, err := input.unsigned()
	if err != nil || uint64(length) > uint64(len(input.data)-input.offset) {
		return nil, fmt.Errorf("truncated Wasm bytes")
	}
	value := input.data[input.offset : input.offset+int(length)]
	input.offset += int(length)
	return value, nil
}

func (input *reader) name() (string, error) {
	value, err := input.bytes()
	return string(value), err
}

func (input *reader) section() (reader, error) {
	value, err := input.bytes()
	return reader{data: value}, err
}

func (input *reader) done() error {
	if input.offset != len(input.data) {
		return fmt.Errorf("unexpected bytes in Wasm object")
	}
	return nil
}

func imports(data []byte) (map[byte][]string, []FunctionImport, error) {
	result := make(map[byte][]string)
	var functionImports []FunctionImport
	input := reader{data: data}
	count, err := input.unsigned()
	if err != nil {
		return nil, nil, err
	}
	if uint64(count) > uint64(len(data)) {
		return nil, nil, fmt.Errorf("invalid Wasm import count")
	}
	for range count {
		module, err := input.name()
		if err != nil {
			return nil, nil, err
		}
		name, err := input.name()
		if err != nil {
			return nil, nil, err
		}
		kind, err := input.byte()
		if err != nil {
			return nil, nil, err
		}
		result[kind] = append(result[kind], name)
		switch kind {
		case 0, 4:
			if kind == 4 {
				if _, err = input.byte(); err != nil {
					return nil, nil, err
				}
			}
			var typeIndex uint32
			typeIndex, err = input.unsigned()
			if kind == 0 && err == nil {
				functionImports = append(functionImports, FunctionImport{Module: module, Name: name, TypeIndex: typeIndex})
			}
		case 1:
			_, err = input.byte()
			if err == nil {
				err = limits(&input)
			}
		case 2:
			err = limits(&input)
		case 3:
			_, err = input.byte()
			if err == nil {
				_, err = input.byte()
			}
		default:
			return nil, nil, fmt.Errorf("unsupported Wasm import kind %d", kind)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	return result, functionImports, input.done()
}

func limits(input *reader) error {
	flags, err := input.unsigned()
	if err != nil {
		return err
	}
	if _, err = input.unsigned(); err != nil {
		return err
	}
	if flags&1 != 0 {
		_, err = input.unsigned()
	}
	return err
}

func symbolTable(data []byte, sections []Section, imported map[byte][]string) ([]Symbol, error) {
	input := reader{data: data}
	version, err := input.unsigned()
	if err != nil || version != 2 {
		return nil, fmt.Errorf("unsupported Wasm linking version %d", version)
	}
	var symbols []Symbol
	for input.offset < len(input.data) {
		kind, err := input.byte()
		if err != nil {
			return nil, err
		}
		subsection, err := input.section()
		if err != nil {
			return nil, err
		}
		if kind != 8 {
			continue
		}
		count, err := subsection.unsigned()
		if err != nil {
			return nil, err
		}
		if uint64(count) > uint64(len(subsection.data)) {
			return nil, fmt.Errorf("invalid Wasm symbol count")
		}
		for range count {
			var symbol Symbol
			if symbol.Kind, err = subsection.byte(); err != nil {
				return nil, err
			}
			if symbol.Flags, err = subsection.unsigned(); err != nil {
				return nil, err
			}
			switch symbol.Kind {
			case 0, 2, 4, 5:
				if symbol.Index, err = subsection.unsigned(); err != nil {
					return nil, err
				}
				if symbol.Flags&0x10 == 0 || symbol.Flags&0x40 != 0 {
					symbol.Name, err = subsection.name()
				} else {
					importKind := symbol.Kind
					if importKind == 2 {
						importKind = 3
					} else if importKind == 5 {
						importKind = 1
					}
					if int(symbol.Index) >= len(imported[importKind]) {
						return nil, fmt.Errorf("Wasm symbol import index %d out of range", symbol.Index)
					}
					symbol.Name = imported[importKind][symbol.Index]
				}
			case 1:
				symbol.Name, err = subsection.name()
				if err == nil && symbol.Flags&0x10 == 0 {
					if symbol.Index, err = subsection.unsigned(); err == nil {
						symbol.Offset, err = subsection.unsigned()
					}
					if err == nil {
						symbol.Size, err = subsection.unsigned()
					}
				}
			case 3:
				if symbol.Index, err = subsection.unsigned(); err == nil {
					if int(symbol.Index) >= len(sections) {
						return nil, fmt.Errorf("Wasm section symbol index %d out of range", symbol.Index)
					}
					symbol.Name = sections[symbol.Index].Name
				}
			default:
				return nil, fmt.Errorf("unsupported Wasm symbol kind %d", symbol.Kind)
			}
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, symbol)
		}
		if err = subsection.done(); err != nil {
			return nil, err
		}
	}
	return symbols, nil
}

func dataSegments(data []byte) ([]DataSegment, error) {
	input := reader{data: data}
	count, err := input.unsigned()
	if err != nil || uint64(count) > uint64(len(data)) {
		return nil, fmt.Errorf("invalid Wasm data segment count")
	}
	segments := make([]DataSegment, count)
	for index := range segments {
		mode, err := input.unsigned()
		if err != nil {
			return nil, err
		}
		switch mode {
		case 0:
		case 2:
			memoryIndex, err := input.unsigned()
			if err != nil || memoryIndex != 0 {
				return nil, fmt.Errorf("unsupported Wasm data memory index %d", memoryIndex)
			}
		default:
			return nil, fmt.Errorf("unsupported Wasm data segment mode %d", mode)
		}
		opcode, err := input.byte()
		if err != nil || opcode != 0x41 {
			return nil, fmt.Errorf("unsupported Wasm data offset expression")
		}
		address, err := input.signed()
		if err != nil || address < 0 {
			return nil, fmt.Errorf("invalid Wasm data offset")
		}
		end, err := input.byte()
		if err != nil || end != 0x0b {
			return nil, fmt.Errorf("invalid Wasm data offset expression end")
		}
		segments[index].Address = uint32(address)
		segments[index].Align = 1
		if segments[index].Data, err = input.bytes(); err != nil {
			return nil, err
		}
		segments[index].Offset = uint32(input.offset - len(segments[index].Data))
	}
	return segments, input.done()
}

func elementSegments(data []byte, importedFunctions uint32, functions []Function) ([]ElementSegment, error) {
	input := reader{data: data}
	count, err := input.unsigned()
	if err != nil || uint64(count) > uint64(len(data)) {
		return nil, fmt.Errorf("invalid Wasm element segment count")
	}
	segments := make([]ElementSegment, count)
	for index := range segments {
		mode, err := input.unsigned()
		if err != nil || mode != 0 {
			return nil, fmt.Errorf("unsupported Wasm element segment mode %d", mode)
		}
		opcode, err := input.byte()
		if err != nil || opcode != 0x41 {
			return nil, fmt.Errorf("unsupported Wasm element offset expression")
		}
		offset, err := input.signed()
		if err != nil || offset < 0 {
			return nil, fmt.Errorf("invalid Wasm element offset")
		}
		end, err := input.byte()
		if err != nil || end != 0x0b {
			return nil, fmt.Errorf("invalid Wasm element offset expression end")
		}
		segments[index].Offset = uint32(offset)
		functionCount, err := input.unsigned()
		if err != nil || uint64(functionCount) > uint64(len(data)-input.offset) {
			return nil, fmt.Errorf("invalid Wasm element function count")
		}
		for range functionCount {
			functionIndex, err := input.unsigned()
			if err != nil {
				return nil, err
			}
			if functionIndex >= importedFunctions+uint32(len(functions)) {
				return nil, fmt.Errorf("unsupported Wasm element function index %d", functionIndex)
			}
			segments[index].Functions = append(segments[index].Functions, functionIndex)
		}
	}
	return segments, input.done()
}

func segmentInfo(data []byte, segments []DataSegment) error {
	input := reader{data: data}
	version, err := input.unsigned()
	if err != nil || version != 2 {
		return fmt.Errorf("unsupported Wasm linking version %d", version)
	}
	for input.offset < len(input.data) {
		kind, err := input.byte()
		if err != nil {
			return err
		}
		subsection, err := input.section()
		if err != nil {
			return err
		}
		if kind != 5 {
			continue
		}
		count, err := subsection.unsigned()
		if err != nil || int(count) != len(segments) {
			return fmt.Errorf("Wasm segment info count does not match data segments")
		}
		for index := range segments {
			if segments[index].Name, err = subsection.name(); err != nil {
				return err
			}
			exponent, err := subsection.unsigned()
			if err != nil || exponent > 30 {
				return fmt.Errorf("invalid Wasm data segment alignment")
			}
			segments[index].Align = 1 << exponent
			if segments[index].Flags, err = subsection.unsigned(); err != nil {
				return err
			}
		}
		if err = subsection.done(); err != nil {
			return err
		}
	}
	return nil
}

func codeRelocations(data []byte, sections []Section, symbols []Symbol) ([]Relocation, error) {
	input := reader{data: data}
	sectionIndex, err := input.unsigned()
	if err != nil || int(sectionIndex) >= len(sections) || sections[sectionIndex].ID != 10 {
		return nil, fmt.Errorf("invalid Wasm CODE relocation section index")
	}
	count, err := input.unsigned()
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(len(data))/3 {
		return nil, fmt.Errorf("invalid Wasm CODE relocation count")
	}
	result := make([]Relocation, 0, count)
	for range count {
		var relocation Relocation
		if relocation.Type, err = input.byte(); err != nil {
			return nil, err
		}
		if relocation.Offset, err = input.unsigned(); err != nil {
			return nil, err
		}
		if int(relocation.Offset) >= len(sections[sectionIndex].Data) {
			return nil, fmt.Errorf("Wasm CODE relocation offset %d out of range", relocation.Offset)
		}
		symbolIndex, err := input.unsigned()
		if err != nil {
			return nil, err
		}
		if relocation.Type == 6 {
			relocation.TypeIndex = symbolIndex
		} else {
			if int(symbolIndex) >= len(symbols) {
				return nil, fmt.Errorf("Wasm CODE relocation symbol index out of range")
			}
			relocation.Symbol = symbols[symbolIndex]
		}
		switch relocation.Type {
		case 0, 1, 2, 6, 7, 12, 20:
		case 3, 4, 5, 11, 21:
			relocation.Addend, err = input.signed()
		default:
			return nil, fmt.Errorf("unsupported Wasm CODE relocation type %d", relocation.Type)
		}
		if err != nil {
			return nil, err
		}
		if relocation.Type == 21 && (relocation.Symbol.Kind != 1 || relocation.Symbol.Flags&0x100 == 0) {
			return nil, fmt.Errorf("Wasm TLS relocation requires a TLS data symbol")
		}
		result = append(result, relocation)
	}
	return result, input.done()
}

func dataRelocations(data []byte, sections []Section, symbols []Symbol, segments []DataSegment) ([]Relocation, error) {
	input := reader{data: data}
	sectionIndex, err := input.unsigned()
	if err != nil || int(sectionIndex) >= len(sections) || sections[sectionIndex].ID != 11 {
		return nil, fmt.Errorf("invalid Wasm DATA relocation section index")
	}
	count, err := input.unsigned()
	if err != nil || uint64(count) > uint64(len(data))/3 {
		return nil, fmt.Errorf("invalid Wasm DATA relocation count")
	}
	result := make([]Relocation, 0, count)
	for range count {
		var relocation Relocation
		if relocation.Type, err = input.byte(); err != nil {
			return nil, err
		}
		if relocation.Type != 2 && relocation.Type != 5 {
			return nil, fmt.Errorf("unsupported Wasm DATA relocation type %d", relocation.Type)
		}
		if relocation.Offset, err = input.unsigned(); err != nil {
			return nil, err
		}
		symbolIndex, err := input.unsigned()
		if err != nil || int(symbolIndex) >= len(symbols) {
			return nil, fmt.Errorf("Wasm DATA relocation symbol index out of range")
		}
		relocation.Symbol = symbols[symbolIndex]
		if (relocation.Type == 5 && relocation.Symbol.Kind != 1) || (relocation.Type == 2 && relocation.Symbol.Kind != 0) {
			return nil, fmt.Errorf("unsupported Wasm DATA relocation symbol kind %d", relocation.Symbol.Kind)
		}
		if relocation.Type == 5 {
			if relocation.Addend, err = input.signed(); err != nil {
				return nil, err
			}
		}
		covered := false
		for _, segment := range segments {
			if relocation.Offset >= segment.Offset && uint64(relocation.Offset)+4 <= uint64(segment.Offset)+uint64(len(segment.Data)) {
				covered = true
				break
			}
		}
		if !covered {
			return nil, fmt.Errorf("Wasm DATA relocation at offset %d is outside segment data", relocation.Offset)
		}
		result = append(result, relocation)
	}
	return result, input.done()
}

func valueTypes(input *reader) ([]byte, error) {
	count, err := input.unsigned()
	if err != nil || uint64(count) > uint64(len(input.data)-input.offset) {
		return nil, fmt.Errorf("invalid Wasm value type count")
	}
	values := make([]byte, count)
	for index := range values {
		values[index], err = input.byte()
		if err != nil {
			return nil, err
		}
		switch values[index] {
		case 0x7f, 0x7e, 0x7d, 0x7c, 0x7b:
		default:
			return nil, fmt.Errorf("unsupported Wasm value type %#x", values[index])
		}
	}
	return values, nil
}

func functionTypes(data []byte) ([]FuncType, error) {
	input := reader{data: data}
	count, err := input.unsigned()
	if err != nil || uint64(count) > uint64(len(data))/3 {
		return nil, fmt.Errorf("invalid Wasm function type count")
	}
	types := make([]FuncType, count)
	for index := range types {
		form, err := input.byte()
		if err != nil || form != 0x60 {
			return nil, fmt.Errorf("invalid Wasm function type")
		}
		if types[index].Params, err = valueTypes(&input); err != nil {
			return nil, err
		}
		if types[index].Results, err = valueTypes(&input); err != nil {
			return nil, err
		}
	}
	return types, input.done()
}

func definedFunctions(typeData, codeData []byte, types []FuncType, symbols []Symbol, importedFunctions uint32) ([]Function, error) {
	declarations := reader{data: typeData}
	code := reader{data: codeData}
	count, err := declarations.unsigned()
	if err != nil || uint64(count) > uint64(len(typeData)) {
		return nil, fmt.Errorf("invalid Wasm function count")
	}
	codeCount, err := code.unsigned()
	if err != nil || count != codeCount {
		return nil, fmt.Errorf("Wasm function and CODE counts differ")
	}
	functions := make([]Function, count)
	for index := range functions {
		typeIndex, err := declarations.unsigned()
		if err != nil || int(typeIndex) >= len(types) {
			return nil, fmt.Errorf("Wasm function type index out of range")
		}
		functions[index].Index = importedFunctions + uint32(index)
		functions[index].Type = types[typeIndex]
		body, err := code.section()
		if err != nil {
			return nil, err
		}
		functions[index].BodyOffset = uint32(code.offset - len(body.data))
		functions[index].Body = body.data
	}
	if err = declarations.done(); err != nil {
		return nil, err
	}
	if err = code.done(); err != nil {
		return nil, err
	}
	for _, symbol := range symbols {
		if symbol.Kind != 0 || symbol.Flags&0x10 != 0 {
			continue
		}
		if symbol.Index < importedFunctions || int(symbol.Index-importedFunctions) >= len(functions) {
			return nil, fmt.Errorf("Wasm function symbol index %d out of range", symbol.Index)
		}
		functions[symbol.Index-importedFunctions].Name = symbol.Name
	}
	return functions, nil
}

func (object *Object) RelocateFunction(function Function, functionIndices, globalIndices, memoryAddresses, tableNumbers, tableSlots map[string]uint32, typeIndices map[uint32]uint32, tlsOffsets ...map[string]uint32) ([]byte, error) {
	result := append([]byte(nil), function.Body...)
	for _, relocation := range object.CodeRelocations {
		if relocation.Offset < function.BodyOffset || uint64(relocation.Offset) >= uint64(function.BodyOffset)+uint64(len(result)) {
			continue
		}
		var target uint32
		var ok bool
		switch relocation.Type {
		case 0:
			target, ok = functionIndices[relocation.Symbol.Name]
		case 6:
			target, ok = typeIndices[relocation.TypeIndex]
		case 7:
			target, ok = globalIndices[relocation.Symbol.Name]
		case 3, 4, 5, 11:
			target, ok = memoryAddresses[relocation.Symbol.Name]
		case 21:
			if len(tlsOffsets) != 0 {
				target, ok = tlsOffsets[0][relocation.Symbol.Name]
			}
		case 20:
			target, ok = tableNumbers[relocation.Symbol.Name]
		case 1, 2, 12:
			target, ok = tableSlots[relocation.Symbol.Name]
		default:
			return nil, fmt.Errorf("unsupported Wasm function relocation type %d for %s", relocation.Type, function.Name)
		}
		if !ok {
			return nil, fmt.Errorf("unresolved Wasm relocation target %s (type index %d)", relocation.Symbol.Name, relocation.TypeIndex)
		}
		offset := int(relocation.Offset - function.BodyOffset)
		_, width := binary.Uvarint(result[offset:])
		if relocation.Type == 2 || relocation.Type == 5 {
			width = 4
		}
		if width <= 0 || width > 5 || offset+width > len(result) {
			return nil, fmt.Errorf("invalid Wasm relocation width for %s", relocation.Symbol.Name)
		}
		value := int64(target) + int64(relocation.Addend)
		if relocation.Type == 1 || relocation.Type == 4 || relocation.Type == 11 || relocation.Type == 12 || relocation.Type == 21 {
			if value < -1<<31 || value > 1<<31-1 {
				return nil, fmt.Errorf("Wasm memory address out of range for %s", relocation.Symbol.Name)
			}
			var encoded byte
			for index := 0; index < width; index++ {
				encoded = byte(value & 0x7f)
				value >>= 7
				if index < width-1 {
					encoded |= 0x80
				}
				result[offset+index] = encoded
			}
			if (value != 0 || encoded&0x40 != 0) && (value != -1 || encoded&0x40 == 0) {
				return nil, fmt.Errorf("Wasm memory address exceeds encoded width for %s", relocation.Symbol.Name)
			}
			continue
		}
		if value < 0 || value > 1<<32-1 {
			return nil, fmt.Errorf("Wasm relocation value out of range for %s", relocation.Symbol.Name)
		}
		if relocation.Type == 2 || relocation.Type == 5 {
			binary.LittleEndian.PutUint32(result[offset:], uint32(value))
			continue
		}
		target = uint32(value)
		for index := 0; index < width; index++ {
			value := byte(target & 0x7f)
			target >>= 7
			if index < width-1 {
				value |= 0x80
			}
			result[offset+index] = value
		}
		if target != 0 {
			return nil, fmt.Errorf("Wasm relocation value exceeds encoded width")
		}
	}
	return result, nil
}

func Parse(data []byte) (*Object, error) {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte("\x00asm\x01\x00\x00\x00")) {
		return nil, fmt.Errorf("not a Wasm object")
	}
	object := new(Object)
	input := reader{data: data[8:]}
	for input.offset < len(input.data) {
		sectionID, err := input.byte()
		if err != nil {
			return nil, err
		}
		payload, err := input.section()
		if err != nil {
			return nil, err
		}
		section := Section{ID: sectionID, Data: payload.data}
		if sectionID == 0 {
			section.Name, err = payload.name()
			if err != nil {
				return nil, err
			}
			section.Data = payload.data[payload.offset:]
		}
		object.Sections = append(object.Sections, section)
	}
	var imported map[byte][]string
	var typeSection []byte
	var functionSection []byte
	var codeSection []byte
	var dataSection []byte
	var elementSection []byte
	var linking []byte
	var relocCode []byte
	var relocData []byte
	for _, section := range object.Sections {
		switch {
		case section.ID == 1:
			typeSection = section.Data
		case section.ID == 2:
			var err error
			imported, object.FunctionImports, err = imports(section.Data)
			if err != nil {
				return nil, err
			}
		case section.ID == 3:
			functionSection = section.Data
		case section.ID == 10:
			codeSection = section.Data
		case section.ID == 9:
			elementSection = section.Data
		case section.ID == 11:
			dataSection = section.Data
		case section.Name == "linking":
			linking = section.Data
		case section.Name == "reloc.CODE":
			relocCode = section.Data
		case section.Name == "reloc.DATA":
			relocData = section.Data
		}
	}
	if linking == nil {
		return nil, fmt.Errorf("Wasm object has no linking section")
	}
	var err error
	if dataSection != nil {
		object.DataSegments, err = dataSegments(dataSection)
		if err != nil {
			return nil, err
		}
	}
	if err = segmentInfo(linking, object.DataSegments); err != nil {
		return nil, err
	}
	object.Symbols, err = symbolTable(linking, object.Sections, imported)
	if err != nil {
		return nil, err
	}
	for _, symbol := range object.Symbols {
		if symbol.Kind != 1 || symbol.Flags&0x10 != 0 {
			continue
		}
		if int(symbol.Index) >= len(object.DataSegments) || uint64(symbol.Offset)+uint64(symbol.Size) > uint64(len(object.DataSegments[symbol.Index].Data)) {
			return nil, fmt.Errorf("Wasm data symbol %s is outside its segment", symbol.Name)
		}
	}
	if relocCode != nil {
		object.CodeRelocations, err = codeRelocations(relocCode, object.Sections, object.Symbols)
		if err != nil {
			return nil, err
		}
	}
	if relocData != nil {
		object.DataRelocations, err = dataRelocations(relocData, object.Sections, object.Symbols, object.DataSegments)
		if err != nil {
			return nil, err
		}
	}
	if typeSection != nil {
		object.Types, err = functionTypes(typeSection)
		if err != nil {
			return nil, err
		}
	}
	for _, importedFunction := range object.FunctionImports {
		if int(importedFunction.TypeIndex) >= len(object.Types) {
			return nil, fmt.Errorf("Wasm function import %s.%s type index %d out of range", importedFunction.Module, importedFunction.Name, importedFunction.TypeIndex)
		}
	}
	for _, relocation := range object.CodeRelocations {
		if relocation.Type == 6 && int(relocation.TypeIndex) >= len(object.Types) {
			return nil, fmt.Errorf("Wasm CODE relocation type index %d out of range", relocation.TypeIndex)
		}
	}
	if functionSection != nil || codeSection != nil {
		if functionSection == nil || codeSection == nil {
			return nil, fmt.Errorf("Wasm object has incomplete function sections")
		}
		object.Functions, err = definedFunctions(functionSection, codeSection, object.Types, object.Symbols, uint32(len(imported[0])))
		if err != nil {
			return nil, err
		}
	}
	if elementSection != nil {
		object.ElementSegments, err = elementSegments(elementSection, uint32(len(imported[0])), object.Functions)
		if err != nil {
			return nil, err
		}
	}
	for _, relocation := range object.CodeRelocations {
		covered := false
		for _, function := range object.Functions {
			if relocation.Offset >= function.BodyOffset && uint64(relocation.Offset) < uint64(function.BodyOffset)+uint64(len(function.Body)) {
				covered = true
				break
			}
		}
		if !covered {
			return nil, fmt.Errorf("Wasm CODE relocation at offset %d is outside a function body", relocation.Offset)
		}
	}
	return object, nil
}
