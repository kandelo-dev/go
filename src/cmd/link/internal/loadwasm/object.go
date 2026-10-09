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
	Kind  byte
	Flags uint32
	Index uint32
	Name  string
}

type Relocation struct {
	Type   byte
	Offset uint32
	Symbol Symbol
}

type Object struct {
	Sections        []Section
	Symbols         []Symbol
	CodeRelocations []Relocation
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

func imports(data []byte) (map[byte][]string, error) {
	result := make(map[byte][]string)
	input := reader{data: data}
	count, err := input.unsigned()
	if err != nil {
		return nil, err
	}
	if uint64(count) > uint64(len(data)) {
		return nil, fmt.Errorf("invalid Wasm import count")
	}
	for range count {
		if _, err = input.name(); err != nil {
			return nil, err
		}
		name, err := input.name()
		if err != nil {
			return nil, err
		}
		kind, err := input.byte()
		if err != nil {
			return nil, err
		}
		result[kind] = append(result[kind], name)
		switch kind {
		case 0, 4:
			if kind == 4 {
				if _, err = input.byte(); err != nil {
					return nil, err
				}
			}
			_, err = input.unsigned()
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
			return nil, fmt.Errorf("unsupported Wasm import kind %d", kind)
		}
		if err != nil {
			return nil, err
		}
	}
	return result, input.done()
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
						_, err = subsection.unsigned()
					}
					if err == nil {
						_, err = subsection.unsigned()
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
		if err != nil || int(symbolIndex) >= len(symbols) {
			return nil, fmt.Errorf("Wasm CODE relocation symbol index out of range")
		}
		relocation.Symbol = symbols[symbolIndex]
		switch relocation.Type {
		case 0, 7, 12:
		default:
			return nil, fmt.Errorf("unsupported Wasm CODE relocation type %d", relocation.Type)
		}
		result = append(result, relocation)
	}
	return result, input.done()
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
	var linking []byte
	var relocCode []byte
	for _, section := range object.Sections {
		switch {
		case section.ID == 2:
			var err error
			imported, err = imports(section.Data)
			if err != nil {
				return nil, err
			}
		case section.Name == "linking":
			linking = section.Data
		case section.Name == "reloc.CODE":
			relocCode = section.Data
		}
	}
	if linking == nil {
		return nil, fmt.Errorf("Wasm object has no linking section")
	}
	var err error
	object.Symbols, err = symbolTable(linking, object.Sections, imported)
	if err != nil {
		return nil, err
	}
	if relocCode != nil {
		object.CodeRelocations, err = codeRelocations(relocCode, object.Sections, object.Symbols)
		if err != nil {
			return nil, err
		}
	}
	return object, nil
}
