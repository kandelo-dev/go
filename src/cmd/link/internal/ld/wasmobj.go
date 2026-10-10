package ld

import (
	"cmd/internal/bio"
	"cmd/internal/objabi"
	"cmd/link/internal/loader"
	"cmd/link/internal/loadwasm"
	"cmd/link/internal/sym"
	"fmt"
	"io"
)

func loadwasmobj(ctxt *Link, input *bio.Reader, _ string, length int64, name string) {
	if length < 0 || length > 1<<30 {
		Errorf("%s: invalid Wasm object length", name)
		return
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(input, data); err != nil {
		Errorf("%s: reading Wasm object: %v", name, err)
		return
	}
	object, err := loadwasm.Parse(data)
	if err != nil {
		Errorf("%s: parsing Wasm object: %v", name, err)
		return
	}
	for _, relocation := range object.CodeRelocations {
		if relocation.Type == 21 {
			Errorf("%s: Wasm TLS relocation for %s requires per-thread TLS linking", name, relocation.Symbol.Name)
			return
		}
	}
	for _, section := range object.Sections {
		if section.ID == 6 {
			Errorf("%s: Wasm object section %d is not yet supported by internal linking", name, section.ID)
			return
		}
		if section.Name == "reloc.ELEM" {
			Errorf("%s: Wasm ELEM relocations are not yet supported by internal linking", name)
			return
		}
	}
	version := ctxt.IncVersion()
	dataSymbols := make(map[string]loader.Sym)
	segmentBuilders := make([]*loader.SymbolBuilder, len(object.DataSegments))
	for index, segment := range object.DataSegments {
		if segment.Flags&2 != 0 {
			Errorf("%s: Wasm TLS data segment %s requires per-thread TLS linking", name, segment.Name)
			return
		}
		segmentName := fmt.Sprintf("%s(data[%d])", name, index)
		builder := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport(segmentName, version))
		builder.SetType(sym.SNOPTRDATA)
		builder.SetData(segment.Data)
		builder.SetSize(int64(len(segment.Data)))
		builder.SetAlign(int32(segment.Align))
		builder.SetExternal(true)
		segmentBuilders[index] = builder
	}
	for _, symbol := range object.Symbols {
		if symbol.Kind != 1 || symbol.Flags&0x10 != 0 {
			continue
		}
		if _, exists := dataSymbols[symbol.Name]; exists {
			Errorf("%s: duplicate Wasm data symbol name %s", name, symbol.Name)
			return
		}
		localVersion := 0
		if symbol.Flags&2 != 0 {
			localVersion = version
		}
		builder := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport(symbol.Name, localVersion))
		if builder.Type() != 0 && builder.Type() != sym.SXREF && builder.Type() != sym.SHOSTOBJ {
			Errorf("%s: duplicate Wasm data symbol %s (%s)", name, symbol.Name, builder.Type())
			return
		}
		builder.SetType(sym.SNOPTRDATA)
		builder.SetValue(int64(symbol.Offset))
		builder.SetSize(int64(symbol.Size))
		builder.SetExternal(true)
		segmentBuilders[symbol.Index].AddInteriorSym(builder.Sym())
		dataSymbols[symbol.Name] = builder.Sym()
	}
	for _, builder := range segmentBuilders {
		builder.SortSub()
	}
	for _, relocation := range object.DataRelocations {
		var builder *loader.SymbolBuilder
		var offset uint32
		for index, segment := range object.DataSegments {
			if relocation.Offset >= segment.Offset && uint64(relocation.Offset)+4 <= uint64(segment.Offset)+uint64(len(segment.Data)) {
				builder = segmentBuilders[index]
				offset = relocation.Offset - segment.Offset
				break
			}
		}
		if builder == nil {
			Errorf("%s: invalid Wasm DATA relocation offset %d", name, relocation.Offset)
			return
		}
		target := dataSymbols[relocation.Symbol.Name]
		if target == 0 {
			target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
		}
		if ctxt.loader.SymType(target) == 0 {
			ctxt.loader.MakeSymbolUpdater(target).SetType(sym.SXREF)
		}
		edge, _ := builder.AddRel(objabi.R_ADDR)
		edge.SetOff(int32(offset))
		edge.SetSiz(4)
		edge.SetAdd(int64(relocation.Addend))
		edge.SetSym(target)
	}
	if ctxt.WasmHostFunctions == nil {
		ctxt.WasmHostFunctions = make(map[loader.Sym]WasmHostFunction)
	}
	functionSymbols := make(map[uint32]loader.Sym)
	for _, function := range object.Functions {
		if function.Name == "" {
			Errorf("%s: unnamed Wasm function %d", name, function.Index)
			return
		}
		localVersion := 0
		for _, symbol := range object.Symbols {
			if symbol.Kind == 0 && symbol.Index == function.Index && symbol.Flags&2 != 0 {
				localVersion = version
				break
			}
		}
		builder := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport(function.Name, localVersion))
		if builder.Type() != 0 && builder.Type() != sym.SXREF && builder.Type() != sym.SHOSTOBJ {
			Errorf("%s: duplicate Wasm function %s (%s)", name, function.Name, builder.Type())
			return
		}
		builder.SetType(sym.STEXT)
		builder.SetData(function.Body)
		builder.SetSize(int64(len(function.Body)))
		builder.SetExternal(true)
		ctxt.Textp = append(ctxt.Textp, builder.Sym())
		functionSymbols[function.Index] = builder.Sym()
		if _, exists := ctxt.WasmHostFunctions[builder.Sym()]; exists {
			Errorf("%s: duplicate Wasm host function %s", name, function.Name)
			return
		}
		ctxt.WasmHostFunctions[builder.Sym()] = WasmHostFunction{Object: object, Function: function, DataSymbols: dataSymbols}
	}
	for _, function := range object.Functions {
		builder := ctxt.loader.MakeSymbolUpdater(functionSymbols[function.Index])
		for _, relocation := range object.CodeRelocations {
			if relocation.Offset < function.BodyOffset || uint64(relocation.Offset) >= uint64(function.BodyOffset)+uint64(len(function.Body)) {
				continue
			}
			target := loader.Sym(0)
			if (relocation.Type == 0 || relocation.Type == 1 || relocation.Type == 2 || relocation.Type == 12) && relocation.Symbol.Flags&2 != 0 && relocation.Symbol.Flags&0x10 == 0 {
				target = functionSymbols[relocation.Symbol.Index]
			} else if relocation.Type == 0 || relocation.Type == 1 || relocation.Type == 2 || relocation.Type == 12 {
				target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
			} else if relocation.Type == 3 || relocation.Type == 4 || relocation.Type == 5 || relocation.Type == 11 {
				target = dataSymbols[relocation.Symbol.Name]
				if target == 0 {
					target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
				}
			} else {
				continue
			}
			if target == 0 {
				Errorf("%s: missing Wasm relocation target %s", name, relocation.Symbol.Name)
				return
			}
			if ctxt.loader.SymType(target) == 0 {
				ctxt.loader.MakeSymbolUpdater(target).SetType(sym.SXREF)
			}
			edgeType := objabi.R_CALL
			if relocation.Type != 0 && relocation.Type != 1 && relocation.Type != 2 && relocation.Type != 12 {
				edgeType = objabi.R_ADDR
			}
			edge, _ := builder.AddRel(edgeType)
			edge.SetOff(int32(relocation.Offset - function.BodyOffset))
			edge.SetSiz(0)
			edge.SetSym(target)
		}
	}
}
