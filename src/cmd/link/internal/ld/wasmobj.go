package ld

import (
	"cmd/internal/bio"
	"cmd/internal/objabi"
	"cmd/link/internal/loader"
	"cmd/link/internal/loadwasm"
	"cmd/link/internal/sym"
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
	for _, section := range object.Sections {
		if section.ID == 6 || section.ID == 9 || section.ID == 11 {
			Errorf("%s: Wasm object section %d is not yet supported by internal linking", name, section.ID)
			return
		}
	}
	version := ctxt.IncVersion()
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
		ctxt.WasmHostFunctions[builder.Sym()] = WasmHostFunction{Object: object, Function: function}
	}
	for _, function := range object.Functions {
		builder := ctxt.loader.MakeSymbolUpdater(functionSymbols[function.Index])
		for _, relocation := range object.CodeRelocations {
			if relocation.Type != 0 || relocation.Offset < function.BodyOffset || uint64(relocation.Offset) >= uint64(function.BodyOffset)+uint64(len(function.Body)) {
				continue
			}
			target := loader.Sym(0)
			if relocation.Symbol.Flags&2 != 0 && relocation.Symbol.Flags&0x10 == 0 {
				target = functionSymbols[relocation.Symbol.Index]
			} else {
				target = ctxt.loader.LookupOrCreateSym(relocation.Symbol.Name, 0)
			}
			if target == 0 {
				Errorf("%s: missing Wasm relocation target %s", name, relocation.Symbol.Name)
				return
			}
			edge, _ := builder.AddRel(objabi.R_CALL)
			edge.SetOff(int32(relocation.Offset - function.BodyOffset))
			edge.SetSiz(0)
			edge.SetSym(target)
		}
	}
}
