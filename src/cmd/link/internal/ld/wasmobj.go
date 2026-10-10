package ld

import (
	"cmd/internal/bio"
	"cmd/internal/objabi"
	"cmd/link/internal/loader"
	"cmd/link/internal/loadwasm"
	"cmd/link/internal/sym"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
)

type WasmInitFunction struct {
	Priority uint32
	Target   loader.Sym
}

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
	localFunctionNames := make(map[uint32]string)
	for _, symbol := range object.Symbols {
		if symbol.Kind == 0 && symbol.Flags&2 != 0 && symbol.Flags&0x10 == 0 {
			localFunctionNames[symbol.Index] = fmt.Sprintf("%s$%d", symbol.Name, version)
		}
	}
	for index := range object.Symbols {
		if symbol := &object.Symbols[index]; symbol.Kind == 0 && symbol.Flags&2 != 0 && symbol.Flags&0x10 == 0 {
			symbol.Name = localFunctionNames[symbol.Index]
		}
	}
	for index := range object.Functions {
		if name, ok := localFunctionNames[object.Functions[index].Index]; ok {
			object.Functions[index].Name = name
		}
	}
	for index := range object.CodeRelocations {
		if symbol := &object.CodeRelocations[index].Symbol; symbol.Kind == 0 && symbol.Flags&2 != 0 && symbol.Flags&0x10 == 0 {
			symbol.Name = localFunctionNames[symbol.Index]
		}
	}
	for index := range object.DataRelocations {
		if symbol := &object.DataRelocations[index].Symbol; symbol.Kind == 0 && symbol.Flags&2 != 0 && symbol.Flags&0x10 == 0 {
			symbol.Name = localFunctionNames[symbol.Index]
		}
	}
	for index := range object.InitFunctions {
		if symbol := &object.InitFunctions[index].Symbol; symbol.Kind == 0 && symbol.Flags&2 != 0 && symbol.Flags&0x10 == 0 {
			symbol.Name = localFunctionNames[symbol.Index]
		}
	}
	if ctxt.WasmDataSymbols == nil {
		ctxt.WasmDataSymbols = make(map[string]loader.Sym)
		ctxt.WasmWeakDataSymbols = make(map[loader.Sym]bool)
	}
	dataSymbols := make(map[string]loader.Sym)
	localTLSSymbols := make(map[string]uint32)
	segmentBuilders := make([]*loader.SymbolBuilder, len(object.DataSegments))
	if ctxt.WasmTLSSymbols == nil {
		ctxt.WasmTLSSymbols = make(map[string]uint32)
	}
	tlsSegmentOffsets := make(map[uint32]uint32)
	for index, segment := range object.DataSegments {
		if (strings.HasPrefix(segment.Name, ".init_array") || strings.HasPrefix(segment.Name, ".fini_array")) && len(segment.Data) != 0 {
			Errorf("%s: Wasm constructor and destructor arrays are not yet supported: %s", name, segment.Name)
			return
		}
		if segment.Flags&2 != 0 {
			if segment.Align == 0 || segment.Align > 65536 || segment.Align&(segment.Align-1) != 0 {
				Errorf("%s: invalid Wasm TLS alignment for %s", name, segment.Name)
				return
			}
			offset := (uint32(len(ctxt.WasmTLSTemplate)) + segment.Align - 1) &^ (segment.Align - 1)
			if uint64(offset)+uint64(len(segment.Data)) > 65536 {
				Errorf("%s: Wasm TLS exceeds the per-thread 64 KiB control page", name)
				return
			}
			ctxt.WasmTLSTemplate = append(ctxt.WasmTLSTemplate, make([]byte, int(offset)-len(ctxt.WasmTLSTemplate))...)
			ctxt.WasmTLSTemplate = append(ctxt.WasmTLSTemplate, segment.Data...)
			tlsSegmentOffsets[uint32(index)] = offset
			if segment.Align > ctxt.WasmTLSAlign {
				ctxt.WasmTLSAlign = segment.Align
			}
			continue
		}
		segmentName := fmt.Sprintf("%s(data[%d])", name, index)
		builder := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport(segmentName, version))
		builder.SetType(sym.SNOPTRDATA)
		builder.SetData(segment.Data)
		builder.SetSize(int64(len(segment.Data)))
		builder.SetAlign(int32(segment.Align))
		builder.SetExternal(true)
		ctxt.loader.SetAttrReachable(builder.Sym(), true)
		segmentBuilders[index] = builder
	}
	for _, symbol := range object.Symbols {
		if symbol.Kind != 1 || symbol.Flags&0x10 != 0 {
			continue
		}
		if symbol.Flags&0x100 != 0 {
			offset, ok := tlsSegmentOffsets[symbol.Index]
			if !ok {
				Errorf("%s: Wasm TLS symbol %s has no TLS data segment", name, symbol.Name)
				return
			}
			localTLSSymbols[symbol.Name] = offset + symbol.Offset
			if symbol.Flags&2 == 0 {
				if _, exists := ctxt.WasmTLSSymbols[symbol.Name]; exists {
					Errorf("%s: duplicate Wasm TLS symbol %s", name, symbol.Name)
					return
				}
				ctxt.WasmTLSSymbols[symbol.Name] = offset + symbol.Offset
			}
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
		defined := builder.Type() != 0 && builder.Type() != sym.SXREF && builder.Type() != sym.SHOSTOBJ
		if defined {
			if symbol.Flags&1 != 0 {
				dataSymbols[symbol.Name] = builder.Sym()
				continue
			}
			if !ctxt.WasmWeakDataSymbols[builder.Sym()] {
				Errorf("%s: duplicate Wasm data symbol %s (%s)", name, symbol.Name, builder.Type())
				return
			}
		}
		builder.SetType(sym.SNOPTRDATA)
		builder.SetValue(int64(symbol.Offset))
		builder.SetSize(int64(symbol.Size))
		builder.SetExternal(true)
		if defined {
			ctxt.loader.MoveInteriorSym(segmentBuilders[symbol.Index].Sym(), builder.Sym())
		} else {
			segmentBuilders[symbol.Index].AddInteriorSym(builder.Sym())
		}
		ctxt.loader.SetAttrReachable(builder.Sym(), true)
		dataSymbols[symbol.Name] = builder.Sym()
		ctxt.WasmWeakDataSymbols[builder.Sym()] = symbol.Flags&1 != 0
		if symbol.Flags&2 == 0 {
			ctxt.WasmDataSymbols[symbol.Name] = builder.Sym()
		}
	}
	for _, builder := range segmentBuilders {
		if builder != nil {
			builder.SortSub()
		}
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
			for _, segment := range object.DataSegments {
				if segment.Flags&2 != 0 && relocation.Offset >= segment.Offset && uint64(relocation.Offset)+4 <= uint64(segment.Offset)+uint64(len(segment.Data)) {
					Errorf("%s: Wasm relocation in TLS initializer %s is not yet supported", name, segment.Name)
					return
				}
			}
			Errorf("%s: invalid Wasm DATA relocation offset %d", name, relocation.Offset)
			return
		}
		if relocation.Type == 2 {
			localVersion := 0
			if relocation.Symbol.Flags&2 != 0 {
				localVersion = version
			}
			target := ctxt.loader.LookupOrCreateCgoExport(relocation.Symbol.Name, localVersion)
			if ctxt.loader.SymType(target) == 0 {
				ctxt.loader.MakeSymbolUpdater(target).SetType(sym.SXREF)
			}
			edge, _ := builder.AddRel(objabi.R_CONST)
			edge.SetOff(int32(offset))
			edge.SetSiz(4)
			edge.SetSym(target)
			ctxt.loader.SetAttrReachable(target, true)
			ctxt.WasmDataTableRelocs = append(ctxt.WasmDataTableRelocs, WasmDataTableReloc{Segment: builder.Sym(), Offset: offset, Target: target})
			continue
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
	if len(tlsSegmentOffsets) != 0 {
		template := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport("runtime.cgoTLSTemplate", 0))
		template.SetType(sym.SNOPTRDATA)
		template.SetData(ctxt.WasmTLSTemplate)
		template.SetSize(int64(len(ctxt.WasmTLSTemplate)))
		template.SetAlign(int32(ctxt.WasmTLSAlign))
		template.SetExternal(true)
		ctxt.loader.SetAttrReachable(template.Sym(), true)
		ctxt.WasmTLSTemplateSym = template.Sym()
		mainBlock := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport("runtime.cgoTLSMain", 0))
		mainBlock.SetType(sym.SNOPTRBSS)
		mainBlock.SetSize(int64(len(ctxt.WasmTLSTemplate)))
		mainBlock.SetAlign(int32(ctxt.WasmTLSAlign))
		mainBlock.SetExternal(true)
		ctxt.loader.SetAttrReachable(mainBlock.Sym(), true)
		ctxt.WasmTLSMainSym = mainBlock.Sym()
	}
	if ctxt.WasmHostFunctions == nil {
		ctxt.WasmHostFunctions = make(map[loader.Sym]WasmHostFunction)
		ctxt.WasmWeakFunctions = make(map[loader.Sym]bool)
	}
	functionSymbols := make(map[uint32]loader.Sym)
	functionSymbolsByName := make(map[string]loader.Sym)
	functionsToLoad := append([]loadwasm.Function(nil), object.Functions...)
	for _, function := range object.Functions {
		for _, symbol := range object.Symbols {
			if symbol.Kind == 0 && symbol.Flags&0x10 == 0 && symbol.Index == function.Index && symbol.Name != function.Name {
				alias := function
				alias.Name = symbol.Name
				functionsToLoad = append(functionsToLoad, alias)
			}
		}
	}
	skippedFunctions := make(map[string]bool)
	for _, function := range functionsToLoad {
		if function.Name == "" {
			Errorf("%s: unnamed Wasm function %d", name, function.Index)
			return
		}
		localVersion := 0
		weak := false
		for _, symbol := range object.Symbols {
			if symbol.Kind == 0 && symbol.Index == function.Index && symbol.Name == function.Name {
				if symbol.Flags&2 != 0 {
					localVersion = version
				}
				weak = symbol.Flags&1 != 0
				break
			}
		}
		builder := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport(function.Name, localVersion))
		defined := builder.Type() != 0 && builder.Type() != sym.SXREF && builder.Type() != sym.SHOSTOBJ
		if defined && weak {
			functionSymbols[function.Index] = builder.Sym()
			functionSymbolsByName[function.Name] = builder.Sym()
			skippedFunctions[function.Name] = true
			continue
		}
		if defined && !ctxt.WasmWeakFunctions[builder.Sym()] {
			Errorf("%s: duplicate Wasm function %s (%s)", name, function.Name, builder.Type())
			return
		}
		if defined {
			builder.ResetRelocs()
		}
		builder.SetType(sym.STEXT)
		builder.SetData(function.Body)
		builder.SetSize(int64(len(function.Body)))
		builder.SetExternal(true)
		if !defined {
			ctxt.Textp = append(ctxt.Textp, builder.Sym())
		}
		functionSymbols[function.Index] = builder.Sym()
		functionSymbolsByName[function.Name] = builder.Sym()
		ctxt.WasmHostFunctions[builder.Sym()] = WasmHostFunction{Object: object, Function: function, DataSymbols: dataSymbols, TLSSymbols: localTLSSymbols}
		ctxt.WasmWeakFunctions[builder.Sym()] = weak
	}
	for _, function := range functionsToLoad {
		if skippedFunctions[function.Name] {
			continue
		}
		builder := ctxt.loader.MakeSymbolUpdater(functionSymbolsByName[function.Name])
		for _, relocation := range object.CodeRelocations {
			if relocation.Offset < function.BodyOffset || uint64(relocation.Offset) >= uint64(function.BodyOffset)+uint64(len(function.Body)) {
				continue
			}
			target := loader.Sym(0)
			if (relocation.Type == 0 || relocation.Type == 1 || relocation.Type == 2 || relocation.Type == 12) && relocation.Symbol.Flags&2 != 0 && relocation.Symbol.Flags&0x10 == 0 {
				target = functionSymbolsByName[relocation.Symbol.Name]
				if target == 0 {
					target = functionSymbols[relocation.Symbol.Index]
				}
			} else if relocation.Type == 0 || relocation.Type == 1 || relocation.Type == 2 || relocation.Type == 12 {
				target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
			} else if relocation.Type == 3 || relocation.Type == 4 || relocation.Type == 5 || relocation.Type == 11 {
				if relocation.Symbol.Flags&0x11 == 0x11 {
					continue
				}
				target = dataSymbols[relocation.Symbol.Name]
				if target == 0 {
					target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
				}
			} else if relocation.Type == 7 && (relocation.Symbol.Kind == 1 || relocation.Symbol.Kind == 2) && relocation.Symbol.Name != "__memory_base" && relocation.Symbol.Name != "__table_base" && relocation.Symbol.Name != "__stack_pointer" && relocation.Symbol.Name != "__tls_base" && relocation.Symbol.Name != "__channel_base" {
				target = dataSymbols[relocation.Symbol.Name]
				if target == 0 {
					target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
				}
			} else if relocation.Type == 7 && relocation.Symbol.Kind == 0 {
				target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
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
			edge.SetSiz(4)
			edge.SetSym(target)
			if edgeType == objabi.R_CALL || relocation.Type == 7 && relocation.Symbol.Kind == 0 {
				ctxt.loader.SetAttrReachable(target, true)
			}
		}
	}
	for _, initializer := range object.InitFunctions {
		target := functionSymbolsByName[initializer.Symbol.Name]
		if target == 0 || ctxt.loader.SymType(target) != sym.STEXT {
			Errorf("%s: invalid Wasm constructor target %s", name, initializer.Symbol.Name)
			return
		}
		if len(initializer.Symbol.Name) == 0 {
			Errorf("%s: unnamed Wasm constructor", name)
			return
		}
		ctxt.WasmInitFunctions = append(ctxt.WasmInitFunctions, WasmInitFunction{Priority: initializer.Priority, Target: target})
	}
}

func (ctxt *Link) prepareWasmInitFunctions() {
	if len(ctxt.WasmInitFunctions) > 65536 {
		Errorf("too many Wasm constructor functions")
		return
	}
	if ctxt.WasmDataSymbols == nil {
		ctxt.WasmDataSymbols = make(map[string]loader.Sym)
	}
	sort.SliceStable(ctxt.WasmInitFunctions, func(first, second int) bool {
		return ctxt.WasmInitFunctions[first].Priority < ctxt.WasmInitFunctions[second].Priority
	})
	count := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport("__kandelo_cgo_ctor_count", 0))
	count.SetType(sym.SNOPTRDATA)
	count.SetData(binary.LittleEndian.AppendUint32(nil, uint32(len(ctxt.WasmInitFunctions))))
	count.SetSize(4)
	count.SetAlign(4)
	count.SetExternal(true)
	ctxt.loader.SetAttrReachable(count.Sym(), true)
	ctxt.WasmDataSymbols["__kandelo_cgo_ctor_count"] = count.Sym()
	dsoHandle := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport("__dso_handle", 0))
	if dsoHandle.Type() == 0 || dsoHandle.Type() == sym.SXREF {
		dsoHandle.SetType(sym.SNOPTRDATA)
		dsoHandle.SetData(make([]byte, 4))
		dsoHandle.SetSize(4)
		dsoHandle.SetAlign(4)
		dsoHandle.SetExternal(true)
	}
	ctxt.loader.SetAttrReachable(dsoHandle.Sym(), true)
	ctxt.WasmDataSymbols["__dso_handle"] = dsoHandle.Sym()
	table := ctxt.loader.MakeSymbolUpdater(ctxt.loader.LookupOrCreateCgoExport("__kandelo_cgo_ctors", 0))
	table.SetType(sym.SNOPTRDATA)
	table.SetData(make([]byte, max(4, 4*len(ctxt.WasmInitFunctions))))
	table.SetSize(int64(max(4, 4*len(ctxt.WasmInitFunctions))))
	table.SetAlign(4)
	table.SetExternal(true)
	ctxt.loader.SetAttrReachable(table.Sym(), true)
	ctxt.WasmDataSymbols["__kandelo_cgo_ctors"] = table.Sym()
	for index, initializer := range ctxt.WasmInitFunctions {
		edge, _ := table.AddRel(objabi.R_ADDR)
		edge.SetOff(int32(4 * index))
		edge.SetSiz(4)
		edge.SetSym(initializer.Target)
		ctxt.WasmDataTableRelocs = append(ctxt.WasmDataTableRelocs, WasmDataTableReloc{Segment: table.Sym(), Offset: uint32(4 * index), Target: initializer.Target})
	}
}
