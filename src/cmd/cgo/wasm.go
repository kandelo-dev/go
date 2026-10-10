package main

import (
	"bytes"
	"debug/dwarf"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
)

type wasmDataSegment struct {
	address uint64
	data    []byte
}

type wasmDebugModule struct {
	sections        map[string][]byte
	importedGlobals uint64
	globalValues    []uint64
	globalExports   map[string]uint64
	dataSegments    []wasmDataSegment
}

func wasmUnsigned(data []byte, offset *int) (uint64, error) {
	if *offset >= len(data) {
		return 0, fmt.Errorf("truncated Wasm integer")
	}
	value, length := binary.Uvarint(data[*offset:])
	if length <= 0 {
		return 0, fmt.Errorf("invalid Wasm integer")
	}
	*offset += length
	return value, nil
}

func wasmSigned(data []byte, offset *int) (int64, error) {
	var value int64
	var shift uint
	for {
		if *offset >= len(data) || shift >= 64 {
			return 0, fmt.Errorf("invalid signed Wasm integer")
		}
		current := data[*offset]
		*offset++
		value |= int64(current&0x7f) << shift
		shift += 7
		if current&0x80 == 0 {
			if current&0x40 != 0 && shift < 64 {
				value |= ^int64(0) << shift
			}
			return value, nil
		}
	}
}

func wasmBytes(data []byte, offset *int) ([]byte, error) {
	length, err := wasmUnsigned(data, offset)
	if err != nil {
		return nil, err
	}
	if length > uint64(len(data)-*offset) {
		return nil, fmt.Errorf("truncated Wasm bytes")
	}
	start := *offset
	*offset += int(length)
	return data[start:*offset], nil
}

func wasmConstExpression(data []byte, offset *int) (uint64, error) {
	if *offset >= len(data) || data[*offset] != 0x41 {
		return 0, fmt.Errorf("unsupported Wasm constant expression")
	}
	*offset++
	value, err := wasmSigned(data, offset)
	if err != nil {
		return 0, err
	}
	if value < 0 || *offset >= len(data) || data[*offset] != 0x0b {
		return 0, fmt.Errorf("invalid Wasm constant expression")
	}
	*offset++
	return uint64(value), nil
}

func readWasmDebugModule(path string) (*wasmDebugModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 || !bytes.Equal(data[:8], []byte("\x00asm\x01\x00\x00\x00")) {
		return nil, fmt.Errorf("not a WebAssembly module")
	}
	module := &wasmDebugModule{
		sections:      make(map[string][]byte),
		globalExports: make(map[string]uint64),
	}
	var importSection, globalSection, exportSection, dataSection []byte
	for offset := 8; offset < len(data); {
		sectionID := data[offset]
		offset++
		length, err := wasmUnsigned(data, &offset)
		if err != nil || length > uint64(len(data)-offset) {
			return nil, fmt.Errorf("invalid Wasm section size")
		}
		section := data[offset : offset+int(length)]
		offset += int(length)
		switch sectionID {
		case 0:
			nameOffset := 0
			name, err := wasmBytes(section, &nameOffset)
			if err != nil {
				return nil, err
			}
			module.sections[string(name)] = section[nameOffset:]
		case 2:
			importSection = section
		case 6:
			globalSection = section
		case 7:
			exportSection = section
		case 11:
			dataSection = section
		}
	}
	if err := module.readImports(importSection); err != nil {
		return nil, err
	}
	if err := module.readGlobals(globalSection); err != nil {
		return nil, err
	}
	if err := module.readExports(exportSection); err != nil {
		return nil, err
	}
	if err := module.readData(dataSection); err != nil {
		return nil, err
	}
	return module, nil
}

func wasmLimits(data []byte, offset *int) error {
	flags, err := wasmUnsigned(data, offset)
	if err != nil || flags > 3 {
		return fmt.Errorf("unsupported Wasm limits")
	}
	if _, err := wasmUnsigned(data, offset); err != nil {
		return err
	}
	if flags&1 != 0 {
		if _, err := wasmUnsigned(data, offset); err != nil {
			return err
		}
	}
	return nil
}

func (module *wasmDebugModule) readImports(section []byte) error {
	if section == nil {
		return nil
	}
	offset := 0
	count, err := wasmUnsigned(section, &offset)
	if err != nil {
		return err
	}
	for range count {
		if _, err := wasmBytes(section, &offset); err != nil {
			return err
		}
		if _, err := wasmBytes(section, &offset); err != nil {
			return err
		}
		if offset >= len(section) {
			return fmt.Errorf("truncated Wasm import")
		}
		kind := section[offset]
		offset++
		switch kind {
		case 0:
			_, err = wasmUnsigned(section, &offset)
		case 1:
			if offset >= len(section) {
				return fmt.Errorf("truncated Wasm table import")
			}
			offset++
			err = wasmLimits(section, &offset)
		case 2:
			err = wasmLimits(section, &offset)
		case 3:
			if len(section)-offset < 2 {
				return fmt.Errorf("truncated Wasm global import")
			}
			offset += 2
			module.importedGlobals++
		case 4:
			if offset >= len(section) {
				return fmt.Errorf("truncated Wasm tag import")
			}
			offset++
			_, err = wasmUnsigned(section, &offset)
		default:
			return fmt.Errorf("unsupported Wasm import kind %d", kind)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (module *wasmDebugModule) readGlobals(section []byte) error {
	if section == nil {
		return nil
	}
	offset := 0
	count, err := wasmUnsigned(section, &offset)
	if err != nil {
		return err
	}
	for range count {
		if len(section)-offset < 2 || section[offset] != 0x7f {
			return fmt.Errorf("unsupported Wasm global type")
		}
		offset += 2
		value, err := wasmConstExpression(section, &offset)
		if err != nil {
			return err
		}
		module.globalValues = append(module.globalValues, value)
	}
	return nil
}

func (module *wasmDebugModule) readExports(section []byte) error {
	if section == nil {
		return nil
	}
	offset := 0
	count, err := wasmUnsigned(section, &offset)
	if err != nil {
		return err
	}
	for range count {
		name, err := wasmBytes(section, &offset)
		if err != nil {
			return err
		}
		if offset >= len(section) {
			return fmt.Errorf("truncated Wasm export")
		}
		kind := section[offset]
		offset++
		index, err := wasmUnsigned(section, &offset)
		if err != nil {
			return err
		}
		if kind == 3 {
			if index < module.importedGlobals {
				return fmt.Errorf("unsupported imported Wasm global")
			}
			localIndex := index - module.importedGlobals
			if localIndex >= uint64(len(module.globalValues)) {
				return fmt.Errorf("invalid Wasm global export")
			}
			module.globalExports[string(name)] = module.globalValues[localIndex]
		}
	}
	return nil
}

func (module *wasmDebugModule) readData(section []byte) error {
	if section == nil {
		return nil
	}
	offset := 0
	count, err := wasmUnsigned(section, &offset)
	if err != nil {
		return err
	}
	for range count {
		mode, err := wasmUnsigned(section, &offset)
		if err != nil {
			return err
		}
		if mode == 2 {
			memoryIndex, err := wasmUnsigned(section, &offset)
			if err != nil || memoryIndex != 0 {
				return fmt.Errorf("unsupported Wasm data memory")
			}
		} else if mode != 0 {
			return fmt.Errorf("unsupported Wasm data segment mode %d", mode)
		}
		address, err := wasmConstExpression(section, &offset)
		if err != nil {
			return err
		}
		contents, err := wasmBytes(section, &offset)
		if err != nil {
			return err
		}
		module.dataSegments = append(module.dataSegments, wasmDataSegment{address, contents})
	}
	return nil
}

func (module *wasmDebugModule) memory(name string, length uint64) ([]byte, error) {
	address, ok := module.globalExports[name]
	if !ok {
		return nil, fmt.Errorf("missing Wasm export %s", name)
	}
	for _, segment := range module.dataSegments {
		if address >= segment.address && address-segment.address <= uint64(len(segment.data)) && length <= uint64(len(segment.data))-(address-segment.address) {
			start := int(address - segment.address)
			return segment.data[start : start+int(length)], nil
		}
	}
	return nil, fmt.Errorf("Wasm export %s is not backed by initialized data", name)
}

func (module *wasmDebugModule) dwarf() (*dwarf.Data, error) {
	sections := module.sections
	data, err := dwarf.New(sections[".debug_abbrev"], nil, nil, sections[".debug_info"], sections[".debug_line"], nil, sections[".debug_ranges"], sections[".debug_str"])
	if err != nil {
		return nil, err
	}
	for name, contents := range sections {
		if strings.HasPrefix(name, ".debug_") {
			if err := data.AddSection(name, contents); err != nil {
				return nil, err
			}
		}
	}
	return data, nil
}

func (module *wasmDebugModule) constants(count int) ([]int64, []float64, []string, error) {
	integerBytes, err := module.memory("__cgodebug_ints", uint64(count+1)*8)
	if err != nil {
		return nil, nil, nil, err
	}
	floatBytes, err := module.memory("__cgodebug_floats", uint64(count+1)*8)
	if err != nil {
		return nil, nil, nil, err
	}
	integers := make([]int64, count)
	floats := make([]float64, count)
	strings := make([]string, count)
	for index := range integers {
		integers[index] = int64(binary.LittleEndian.Uint64(integerBytes[index*8:]))
		floats[index] = math.Float64frombits(binary.LittleEndian.Uint64(floatBytes[index*8:]))
		lengthName := fmt.Sprintf("__cgodebug_strlen__%d", index)
		if _, ok := module.globalExports[lengthName]; !ok {
			continue
		}
		lengthBytes, err := module.memory(lengthName, 8)
		if err != nil {
			return nil, nil, nil, err
		}
		length := binary.LittleEndian.Uint64(lengthBytes)
		stringBytes, err := module.memory(fmt.Sprintf("__cgodebug_str__%d", index), length)
		if err != nil {
			return nil, nil, nil, err
		}
		strings[index] = string(stringBytes)
	}
	return integers, floats, strings, nil
}
