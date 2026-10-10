// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wasm

import (
	"bytes"
	"cmd/internal/obj"
	"cmd/internal/obj/wasm"
	"cmd/internal/objabi"
	"cmd/link/internal/ld"
	"cmd/link/internal/loader"
	"cmd/link/internal/sym"
	"encoding/binary"
	"flag"
	"fmt"
	"internal/abi"
	"internal/buildcfg"
	"io"
	"regexp"
	"sort"
)

const (
	I32 = 0x7F
	I64 = 0x7E
	F32 = 0x7D
	F64 = 0x7C
)

const (
	sectionCustom   = 0
	sectionType     = 1
	sectionImport   = 2
	sectionFunction = 3
	sectionTable    = 4
	sectionMemory   = 5
	sectionGlobal   = 6
	sectionExport   = 7
	sectionStart    = 8
	sectionElement  = 9
	sectionCode     = 10
	sectionData     = 11
)

// funcValueOffset is the offset between the PC_F value of a function and the index of the function in WebAssembly
const funcValueOffset = 0x1000 // TODO(neelance): make function addresses play nice with heap addresses

var kandeloThreadSlots = flag.Int("kandelothreadslots", 32, "number of preallocated Kandelo pthread slots")

func gentext(ctxt *ld.Link, ldr *loader.Loader) {
}

type wasmFunc struct {
	Module string
	Name   string
	Type   uint32
	Code   []byte
}

type wasmFuncType struct {
	Params  []byte
	Results []byte
}

type wasmDataAddressGlobal struct {
	Name    string
	Address uint32
}

func readWasmImport(ldr *loader.Loader, s loader.Sym) obj.WasmImport {
	var wi obj.WasmImport
	wi.Read(ldr.Data(s))
	return wi
}

var wasmFuncTypes = map[string]*wasmFuncType{
	"_rt0_wasm_js":            {Params: []byte{}},                                         //
	"_rt0_wasm_wasip1":        {Params: []byte{}},                                         //
	"_rt0_wasm_wasip1_lib":    {Params: []byte{}},                                         //
	"_rt0_wasm_kandelo":       {Params: []byte{}},                                         //
	"_rt0_wasm_kandelo_lib":   {Params: []byte{}},                                         //
	"wasm_export__start":      {},                                                         //
	"wasm_export_run":         {Params: []byte{I32, I32}},                                 // argc, argv
	"wasm_export_resume":      {Params: []byte{}},                                         //
	"wasm_export_getsp":       {Results: []byte{I32}},                                     // sp
	"wasm_pc_f_loop":          {Params: []byte{}},                                         //
	"wasm_pc_f_loop_export":   {Params: []byte{I32}},                                      // pc_f
	"runtime.wasmDiv":         {Params: []byte{I64, I64}, Results: []byte{I64}},           // x, y -> x/y
	"runtime.wasmTruncS":      {Params: []byte{F64}, Results: []byte{I64}},                // x -> int(x)
	"runtime.wasmTruncU":      {Params: []byte{F64}, Results: []byte{I64}},                // x -> uint(x)
	"gcWriteBarrier":          {Params: []byte{I64}, Results: []byte{I64}},                // #bytes -> bufptr
	"runtime.gcWriteBarrier1": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier2": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier3": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier4": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier5": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier6": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier7": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.gcWriteBarrier8": {Results: []byte{I64}},                                     // -> bufptr
	"runtime.notInitialized":  {},                                                         //
	"cmpbody":                 {Params: []byte{I64, I64, I64, I64}, Results: []byte{I64}}, // a, alen, b, blen -> -1/0/1
	"memeqbody":               {Params: []byte{I64, I64, I64}, Results: []byte{I64}},      // a, b, len -> 0/1
	"memcmp":                  {Params: []byte{I32, I32, I32}, Results: []byte{I32}},      // a, b, len -> <0/0/>0
	"memchr":                  {Params: []byte{I32, I32, I32}, Results: []byte{I32}},      // s, c, len -> index
}

func assignAddress(ldr *loader.Loader, sect *sym.Section, n int, s loader.Sym, va uint64, isTramp bool) (*sym.Section, int, uint64) {
	// WebAssembly functions do not live in the same address space as the linear memory.
	// Instead, WebAssembly automatically assigns indices. Imported functions (section "import")
	// have indices 0 to n. They are followed by native functions (sections "function" and "code")
	// with indices n+1 and following.
	//
	// The following rules describe how wasm handles function indices and addresses:
	//   PC_F = funcValueOffset + WebAssembly function index (not including the imports)
	//   s.Value = PC = PC_F<<16 + PC_B
	//
	// The funcValueOffset is necessary to avoid conflicts with expectations
	// that the Go runtime has about function addresses.
	// The field "s.Value" corresponds to the concept of PC at runtime.
	// However, there is no PC register, only PC_F and PC_B. PC_F denotes the function,
	// PC_B the resume point inside of that function. The entry of the function has PC_B = 0.
	ldr.SetSymSect(s, sect)
	ldr.SetSymValue(s, int64(funcValueOffset+va/abi.MINFUNC)<<16) // va starts at zero
	va += uint64(abi.MINFUNC)
	return sect, n, va
}

type wasmDataSect struct {
	sect *sym.Section
	data []byte
}

var dataSects []wasmDataSect

func asmb(ctxt *ld.Link, ldr *loader.Loader) {
	sections := []*sym.Section{
		ldr.SymSect(ldr.Lookup("runtime.rodata", 0)),
		ldr.SymSect(ldr.Lookup("runtime.typelink", 0)),
		ldr.SymSect(ldr.Lookup("runtime.itablink", 0)),
		ldr.SymSect(ldr.Lookup("runtime.symtab", 0)),
		ldr.SymSect(ldr.Lookup("runtime.pclntab", 0)),
		ldr.SymSect(ldr.Lookup("runtime.noptrdata", 0)),
		ldr.SymSect(ldr.Lookup("runtime.data", 0)),
	}

	dataSects = make([]wasmDataSect, len(sections))
	for i, sect := range sections {
		data := ld.DatblkBytes(ctxt, int64(sect.Vaddr), int64(sect.Length))
		dataSects[i] = wasmDataSect{sect, data}
	}
	for _, relocation := range ctxt.WasmDataTableRelocs {
		address := ldr.SymValue(relocation.Segment) + int64(relocation.Offset)
		target := ldr.SymValue(relocation.Target)
		if target <= 0 || target>>16 < funcValueOffset || target&0xffff != 0 {
			ld.Exitf("invalid Wasm function pointer relocation target %s (value=%d type=%s reachable=%t)", ldr.SymName(relocation.Target), target, ldr.SymType(relocation.Target), ldr.AttrReachable(relocation.Target))
		}
		found := false
		for _, section := range dataSects {
			start := int64(section.sect.Vaddr)
			if address < start || address+4 > start+int64(len(section.data)) {
				continue
			}
			binary.LittleEndian.PutUint32(section.data[address-start:], uint32(target>>16)+uint32(relocation.Addend))
			found = true
			break
		}
		if !found {
			ld.Exitf("Wasm function pointer relocation outside static data for %s", ldr.SymName(relocation.Target))
		}
	}
}

// asmb writes the final WebAssembly module binary.
// Spec: https://webassembly.github.io/spec/core/binary/modules.html
func asmb2(ctxt *ld.Link, ldr *loader.Loader) {
	types := []*wasmFuncType{
		// For normal Go functions, the single parameter is PC_B,
		// the return value is
		// 0 if the function returned normally or
		// 1 if the stack needs to be unwound.
		{Params: []byte{I32}, Results: []byte{I32}},
		{Params: []byte{I32}},
	}

	// collect host imports (functions that get imported from the WebAssembly host, usually JavaScript)
	// we store the import index of each imported function, so the R_WASMIMPORT relocation
	// can write the correct index after a "call" instruction
	// these are added as import statements to the top of the WebAssembly binary
	var hostImports []*wasmFunc
	hostImportMap := make(map[loader.Sym]int64)
	for _, fn := range ctxt.Textp {
		relocs := ldr.Relocs(fn)
		for ri := 0; ri < relocs.Count(); ri++ {
			r := relocs.At(ri)
			if r.Type() == objabi.R_WASMIMPORT {
				if wsym := ldr.WasmImportSym(fn); wsym != 0 {
					wi := readWasmImport(ldr, wsym)
					hostImportMap[fn] = int64(len(hostImports))
					hostImports = append(hostImports, &wasmFunc{
						Module: wi.Module,
						Name:   wi.Name,
						Type: lookupType(&wasmFuncType{
							Params:  fieldsToTypes(wi.Params),
							Results: fieldsToTypes(wi.Results),
						}, &types),
					})
				} else {
					panic(fmt.Sprintf("missing wasm symbol for %s", ldr.SymName(r.Sym())))
				}
			}
		}
	}
	kernelImports := make(map[string]wasmFuncType)
	for _, host := range ctxt.WasmHostFunctions {
		for _, importedFunction := range host.Object.FunctionImports {
			if importedFunction.Module != "kernel" {
				continue
			}
			signature := host.Object.Types[importedFunction.TypeIndex]
			candidate := wasmFuncType{Params: signature.Params, Results: signature.Results}
			if previous, ok := kernelImports[importedFunction.Name]; ok && (!bytes.Equal(previous.Params, candidate.Params) || !bytes.Equal(previous.Results, candidate.Results)) {
				ld.Exitf("conflicting Kandelo kernel import signature for %s", importedFunction.Name)
			}
			kernelImports[importedFunction.Name] = candidate
		}
	}
	var kernelImportNames []string
	for name := range kernelImports {
		kernelImportNames = append(kernelImportNames, name)
	}
	sort.Strings(kernelImportNames)
	for _, name := range kernelImportNames {
		signature := kernelImports[name]
		found := false
		for _, existing := range hostImports {
			if existing.Module == "kernel" && existing.Name == name {
				previous := types[existing.Type]
				if !bytes.Equal(previous.Params, signature.Params) || !bytes.Equal(previous.Results, signature.Results) {
					ld.Exitf("conflicting Go and C kernel import signature for %s", name)
				}
				found = true
				break
			}
		}
		if !found {
			hostImports = append(hostImports, &wasmFunc{Module: "kernel", Name: name, Type: lookupType(&signature, &types)})
		}
	}

	// collect functions with WebAssembly body
	var buildid []byte
	fns := make([]*wasmFunc, len(ctxt.Textp))
	hostFunctionIndices := make(map[string]uint32)
	for index, importedFunction := range hostImports {
		if importedFunction.Module == "kernel" {
			hostFunctionIndices[importedFunction.Name] = uint32(index)
		}
	}
	globalDataAddresses := make(map[string]uint32)
	for name, symbol := range ctxt.WasmDataSymbols {
		address := ldr.SymValue(symbol)
		if address <= 0 || address > 1<<32-1 {
			ld.Exitf("Wasm data symbol %s has invalid address %d", name, address)
		}
		globalDataAddresses[name] = uint32(address)
	}
	hasMemoryBase := false
	hasTableBase := false
	hasStackPointer := false
	hasChannelBase := false
	hasCTLSBase := len(ctxt.WasmTLSTemplate) != 0
	dataAddressGlobalNames := make(map[string]string)
	var cgoTopofstackGoIndex uint32
	for _, fn := range ctxt.Textp {
		if ldr.SymName(fn) == "_cgo_topofstack" {
			cgoTopofstackGoIndex = uint32(len(hostImports)) + uint32(ldr.SymValue(fn)>>16) - funcValueOffset
			hostFunctionIndices["_cgo_topofstack"] = uint32(len(hostImports) + len(ctxt.Textp))
		}
		if host, ok := ctxt.WasmHostFunctions[fn]; ok {
			if _, exists := hostFunctionIndices[host.Function.Name]; exists {
				ld.Exitf("duplicate Wasm host function name %s", host.Function.Name)
			}
			hostFunctionIndices[host.Function.Name] = uint32(len(hostImports)) + uint32(ldr.SymValue(fn)>>16) - funcValueOffset
			for _, relocation := range host.Object.CodeRelocations {
				if relocation.Offset < host.Function.BodyOffset || uint64(relocation.Offset) >= uint64(host.Function.BodyOffset)+uint64(len(host.Function.Body)) {
					continue
				}
				if relocation.Type == 7 {
					switch relocation.Symbol.Name {
					case "__memory_base":
						hasMemoryBase = true
					case "__table_base":
						hasTableBase = true
					case "__stack_pointer":
						hasStackPointer = true
					case "__tls_base":
						hasCTLSBase = true
					case "__channel_base":
						hasChannelBase = true
					default:
						dataAddressGlobalNames[relocation.Symbol.Name] = host.Function.Name
					}
				}
			}
		}
	}
	var dataAddressGlobalNamesSorted []string
	for name := range dataAddressGlobalNames {
		dataAddressGlobalNamesSorted = append(dataAddressGlobalNamesSorted, name)
	}
	sort.Strings(dataAddressGlobalNamesSorted)
	dataAddressGlobals := make([]wasmDataAddressGlobal, 0, len(dataAddressGlobalNamesSorted))
	for _, name := range dataAddressGlobalNamesSorted {
		address, ok := globalDataAddresses[name]
		if !ok {
			function := ldr.Lookup(name, 0)
			if function == 0 || ldr.SymType(function) != sym.STEXT || !ldr.AttrReachable(function) {
				ld.Exitf("unresolved Wasm data-address global %s referenced by %s (symbol=%d type=%v reachable=%t)", name, dataAddressGlobalNames[name], function, ldr.SymType(function), ldr.AttrReachable(function))
			}
			address = uint32(ldr.SymValue(function) >> 16)
		}
		dataAddressGlobals = append(dataAddressGlobals, wasmDataAddressGlobal{Name: name, Address: address})
	}
	for i, fn := range ctxt.Textp {
		if host, ok := ctxt.WasmHostFunctions[fn]; ok {
			globalIndices := make(map[string]uint32)
			if hasMemoryBase {
				globalIndices["__memory_base"] = 10
			}
			nextGlobal := uint32(10)
			if hasMemoryBase {
				nextGlobal++
			}
			if hasTableBase {
				globalIndices["__table_base"] = nextGlobal
				nextGlobal++
			}
			if hasStackPointer {
				globalIndices["__stack_pointer"] = nextGlobal
				nextGlobal++
			}
			if hasCTLSBase {
				globalIndices["__tls_base"] = nextGlobal
				nextGlobal++
			}
			if hasChannelBase {
				globalIndices["__channel_base"] = nextGlobal
				nextGlobal++
			}
			for _, addressGlobal := range dataAddressGlobals {
				globalIndices[addressGlobal.Name] = nextGlobal
				nextGlobal++
			}
			memoryAddresses := make(map[string]uint32, len(globalDataAddresses)+len(host.DataSymbols))
			for name, address := range globalDataAddresses {
				memoryAddresses[name] = address
			}
			for name, symbol := range host.DataSymbols {
				address := ldr.SymValue(symbol)
				if address < 0 || address > 1<<32-1 {
					ld.Exitf("Wasm data symbol %s has invalid address %d", name, address)
				}
				memoryAddresses[name] = uint32(address)
			}
			for _, relocation := range host.Object.CodeRelocations {
				if relocation.Offset < host.Function.BodyOffset || uint64(relocation.Offset) >= uint64(host.Function.BodyOffset)+uint64(len(host.Function.Body)) {
					continue
				}
				if (relocation.Type == 3 || relocation.Type == 4 || relocation.Type == 5 || relocation.Type == 11) && relocation.Symbol.Flags&0x11 == 0x11 {
					if _, defined := memoryAddresses[relocation.Symbol.Name]; !defined {
						memoryAddresses[relocation.Symbol.Name] = 0
					}
				}
			}
			typeIndices := make(map[uint32]uint32)
			for index, signature := range host.Object.Types {
				typeIndices[uint32(index)] = lookupType(&wasmFuncType{Params: signature.Params, Results: signature.Results}, &types)
			}
			tableSlots := make(map[string]uint32, len(hostFunctionIndices))
			for name, index := range hostFunctionIndices {
				tableSlots[name] = funcValueOffset + index - uint32(len(hostImports))
			}
			tlsOffsets := make(map[string]uint32, len(ctxt.WasmTLSSymbols)+len(host.TLSSymbols))
			for name, offset := range ctxt.WasmTLSSymbols {
				tlsOffsets[name] = offset
			}
			for name, offset := range host.TLSSymbols {
				tlsOffsets[name] = offset
			}
			body, err := host.Object.RelocateFunction(host.Function, hostFunctionIndices, globalIndices, memoryAddresses, map[string]uint32{"__indirect_function_table": 0}, tableSlots, typeIndices, tlsOffsets)
			if err != nil {
				ld.Exitf("Wasm host function %s: %v", host.Function.Name, err)
			}
			typeIndex := lookupType(&wasmFuncType{Params: host.Function.Type.Params, Results: host.Function.Type.Results}, &types)
			fns[i] = &wasmFunc{Name: host.Function.Name, Type: typeIndex, Code: body}
			continue
		}
		wfn := new(bytes.Buffer)
		if ldr.SymName(fn) == "go:buildid" {
			writeUleb128(wfn, 0) // number of sets of locals
			writeI32Const(wfn, 0)
			wfn.WriteByte(0x0b) // end
			buildid = ldr.Data(fn)
		} else {
			// Relocations have variable length, handle them here.
			relocs := ldr.Relocs(fn)
			P := ldr.Data(fn)
			off := int32(0)
			for ri := 0; ri < relocs.Count(); ri++ {
				r := relocs.At(ri)
				if r.Siz() == 0 {
					continue // skip marker relocations
				}
				wfn.Write(P[off:r.Off()])
				off = r.Off()
				rs := r.Sym()
				switch r.Type() {
				case objabi.R_ADDR:
					writeSleb128(wfn, ldr.SymValue(rs)+r.Add())
				case objabi.R_CALL:
					writeSleb128(wfn, int64(len(hostImports))+ldr.SymValue(rs)>>16-funcValueOffset)
				case objabi.R_WASMIMPORT:
					writeSleb128(wfn, hostImportMap[rs])
				default:
					ldr.Errorf(fn, "bad reloc type %d (%s)", r.Type(), sym.RelocName(ctxt.Arch, r.Type()))
					continue
				}
			}
			wfn.Write(P[off:])
		}

		typ := uint32(0)
		if sig, ok := wasmFuncTypes[ldr.SymName(fn)]; ok {
			typ = lookupType(sig, &types)
		}
		if s := ldr.WasmTypeSym(fn); s != 0 {
			var o obj.WasmFuncType
			o.Read(ldr.Data(s))
			t := &wasmFuncType{
				Params:  fieldsToTypes(o.Params),
				Results: fieldsToTypes(o.Results),
			}
			typ = lookupType(t, &types)
		}
		if buildcfg.GOOS == "kandelo" && ldr.SymName(fn) == "_cgo_topofstack" {
			typ = 0
		}

		name := nameRegexp.ReplaceAllString(ldr.SymName(fn), "_")
		fns[i] = &wasmFunc{Name: name, Type: typ, Code: wfn.Bytes()}
	}

	if cgoTopofstackGoIndex != 0 {
		var body bytes.Buffer
		writeUleb128(&body, 0)
		writeI32Const(&body, 0)
		body.WriteByte(0x10)
		writeUleb128(&body, uint64(cgoTopofstackGoIndex))
		body.WriteByte(0x0b)
		wrapperType := lookupType(&wasmFuncType{Results: []byte{I32}}, &types)
		fns = append(fns, &wasmFunc{Name: "_cgo_topofstack_cabi", Type: wrapperType, Code: body.Bytes()})
	}

	// abiVersionFuncIdx is the WebAssembly function index of the synthesized
	// __abi_version marker export (GOOS=kandelo only). Zero when not emitted.
	var abiVersionFuncIdx uint32
	if buildcfg.GOOS == "kandelo" {
		// Synthesize a raw () -> i32 function whose body reduces to
		// `i32.const 43; end`. The Kandelo host does NOT call this export; it
		// byte-parses the exported function body and accepts an i32.const that
		// is returned directly (following at most one call wrapper). A normal
		// Go wasm function carries an SP/resume prologue the parser would
		// reject, so the body is emitted directly here.
		//
		// kandeloABIVersion must stay in sync with ABI_VERSION in the Kandelo
		// repository's crates/shared/src/lib.rs. It is hardcoded here; when
		// upstream Kandelo bumps ABI_VERSION, bump this to match (the host
		// hard-rejects a mismatched marker). Last synced: ABI 48.
		const kandeloABIVersion = 48
		abiType := lookupType(&wasmFuncType{Results: []byte{I32}}, &types)
		var body bytes.Buffer
		writeUleb128(&body, 0)                  // local declaration count
		writeI32Const(&body, kandeloABIVersion) // i32.const 48
		body.WriteByte(0x0b)                    // end
		// Module function index: imported functions occupy [0, len(hostImports)),
		// then the defined functions in fns order. This synthesized function is
		// appended at the current end of fns.
		abiVersionFuncIdx = uint32(len(hostImports)) + uint32(len(fns))
		fns = append(fns, &wasmFunc{Name: "__abi_version", Type: abiType, Code: body.Bytes()})
	}

	// threadSlotsFuncIdx is the WebAssembly function index of the synthesized
	// __wasm_posix_thread_slots declaration export (GOOS=kandelo only). Zero
	// when not emitted.
	var threadSlotsFuncIdx uint32
	if buildcfg.GOOS == "kandelo" {
		// Declare how many concurrent pthread control slots this process needs,
		// mirroring the C SDK's __wasm_posix_thread_slots export
		// (libc/glue/channel_syscall.c). The Kandelo host byte-parses this
		// constant-return export (extractThreadSlotDeclaration in
		// host/src/constants.ts) to bound the per-process thread-slot pool. A Go
		// program that can spawn a second M must declare a nonzero, bounded
		// count so the host does not treat it as single-threaded. Emitted as a
		// raw `i32.const N; end` body for the same reason as __abi_version: a
		// normal Go wasm function carries an SP/resume prologue the parser would
		// reject.
		if *kandeloThreadSlots < 1 || *kandeloThreadSlots > 1024 {
			ld.Exitf("-kandelothreadslots must be between 1 and 1024")
		}
		slotsType := lookupType(&wasmFuncType{Results: []byte{I32}}, &types)
		var body bytes.Buffer
		writeUleb128(&body, 0) // local declaration count
		writeI32Const(&body, int32(*kandeloThreadSlots))
		body.WriteByte(0x0b) // end
		threadSlotsFuncIdx = uint32(len(hostImports)) + uint32(len(fns))
		fns = append(fns, &wasmFunc{Name: "__wasm_posix_thread_slots", Type: slotsType, Code: body.Bytes()})
	}
	var preallocateThreadSlotsFuncIdx uint32
	if buildcfg.GOOS == "kandelo" {
		markerType := lookupType(&wasmFuncType{Results: []byte{I32}}, &types)
		var body bytes.Buffer
		writeUleb128(&body, 0)
		writeI32Const(&body, 1)
		body.WriteByte(0x0b)
		preallocateThreadSlotsFuncIdx = uint32(len(hostImports)) + uint32(len(fns))
		fns = append(fns, &wasmFunc{Name: "__wasm_posix_preallocate_thread_slots", Type: markerType, Code: body.Bytes()})
	}
	var initTLSFuncIdx, startTLSFuncIdx uint32
	if buildcfg.GOOS == "kandelo" && len(ctxt.WasmTLSTemplate) != 0 {
		initTLSFuncIdx = uint32(len(hostImports) + len(fns))
		initType := lookupType(&wasmFuncType{Params: []byte{I32}}, &types)
		var body bytes.Buffer
		writeUleb128(&body, 0)
		body.WriteByte(0x20)
		writeUleb128(&body, 0)
		body.WriteByte(0x24)
		globalIndex := uint32(10)
		if hasMemoryBase {
			globalIndex++
		}
		if hasTableBase {
			globalIndex++
		}
		if hasStackPointer {
			globalIndex++
		}
		writeUleb128(&body, uint64(globalIndex))
		body.WriteByte(0x20)
		writeUleb128(&body, 0)
		writeI32Const(&body, int32(ldr.SymValue(ctxt.WasmTLSTemplateSym)))
		writeI32Const(&body, int32(len(ctxt.WasmTLSTemplate)))
		body.Write([]byte{0xfc, 0x0a, 0x00, 0x00, 0x0b})
		fns = append(fns, &wasmFunc{Name: "__wasm_init_tls", Type: initType, Code: body.Bytes()})
		entry := ldr.Lookup("_rt0_wasm_kandelo", 0)
		if entry == 0 {
			ld.Exitf("Go Kandelo entry point not defined")
		}
		entryIndex := uint32(len(hostImports)) + uint32(ldr.SymValue(entry)>>16) - funcValueOffset
		startTLSFuncIdx = uint32(len(hostImports) + len(fns))
		startType := lookupType(&wasmFuncType{}, &types)
		var startBody bytes.Buffer
		writeUleb128(&startBody, 0)
		writeI32Const(&startBody, int32(ldr.SymValue(ctxt.WasmTLSMainSym)))
		startBody.WriteByte(0x10)
		writeUleb128(&startBody, uint64(initTLSFuncIdx))
		startBody.WriteByte(0x10)
		writeUleb128(&startBody, uint64(entryIndex))
		startBody.WriteByte(0x0b)
		fns = append(fns, &wasmFunc{Name: "__kandelo_start_tls", Type: startType, Code: startBody.Bytes()})
	}

	ctxt.Out.Write([]byte{0x00, 0x61, 0x73, 0x6d}) // magic
	ctxt.Out.Write([]byte{0x01, 0x00, 0x00, 0x00}) // version

	// Add any buildid early in the binary:
	if len(buildid) != 0 {
		writeBuildID(ctxt, buildid)
	}

	writeTypeSec(ctxt, types)
	writeImportSec(ctxt, ldr, hostImports)
	writeFunctionSec(ctxt, fns)
	writeTableSec(ctxt, fns)
	if buildcfg.GOOS != "kandelo" {
		// GOOS=kandelo imports its linear memory from env.memory (written by
		// writeImportSec) instead of defining a local memory, so it emits no
		// memory section.
		writeMemorySec(ctxt, ldr)
	}
	writeGlobalSec(ctxt, ldr, hasMemoryBase, hasTableBase, hasStackPointer, hasCTLSBase, hasChannelBase, dataAddressGlobals)
	writeExportSec(ctxt, ldr, len(hostImports), abiVersionFuncIdx, threadSlotsFuncIdx, preallocateThreadSlotsFuncIdx, initTLSFuncIdx, startTLSFuncIdx, hasMemoryBase, hasTableBase, hasStackPointer, hasCTLSBase, hasChannelBase)
	writeElementSec(ctxt, uint64(len(hostImports)), uint64(len(fns)))
	writeCodeSec(ctxt, fns)
	writeDataSec(ctxt)
	writeProducerSec(ctxt)
	if !*ld.FlagS {
		writeNameSec(ctxt, len(hostImports), fns)
	}
}

func lookupType(sig *wasmFuncType, types *[]*wasmFuncType) uint32 {
	for i, t := range *types {
		if bytes.Equal(sig.Params, t.Params) && bytes.Equal(sig.Results, t.Results) {
			return uint32(i)
		}
	}
	*types = append(*types, sig)
	return uint32(len(*types) - 1)
}

func writeSecHeader(ctxt *ld.Link, id uint8) int64 {
	ctxt.Out.WriteByte(id)
	sizeOffset := ctxt.Out.Offset()
	ctxt.Out.Write(make([]byte, 5)) // placeholder for length
	return sizeOffset
}

func writeSecSize(ctxt *ld.Link, sizeOffset int64) {
	endOffset := ctxt.Out.Offset()
	ctxt.Out.SeekSet(sizeOffset)
	writeUleb128FixedLength(ctxt.Out, uint64(endOffset-sizeOffset-5), 5)
	ctxt.Out.SeekSet(endOffset)
}

func writeBuildID(ctxt *ld.Link, buildid []byte) {
	sizeOffset := writeSecHeader(ctxt, sectionCustom)
	writeName(ctxt.Out, "go:buildid")
	ctxt.Out.Write(buildid)
	writeSecSize(ctxt, sizeOffset)
}

// writeTypeSec writes the section that declares all function types
// so they can be referenced by index.
func writeTypeSec(ctxt *ld.Link, types []*wasmFuncType) {
	sizeOffset := writeSecHeader(ctxt, sectionType)

	writeUleb128(ctxt.Out, uint64(len(types)))

	for _, t := range types {
		ctxt.Out.WriteByte(0x60) // functype
		writeUleb128(ctxt.Out, uint64(len(t.Params)))
		for _, v := range t.Params {
			ctxt.Out.WriteByte(byte(v))
		}
		writeUleb128(ctxt.Out, uint64(len(t.Results)))
		for _, v := range t.Results {
			ctxt.Out.WriteByte(byte(v))
		}
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeImportSec writes the section that lists the functions that get
// imported from the WebAssembly host, usually JavaScript.
func writeImportSec(ctxt *ld.Link, ldr *loader.Loader, hostImports []*wasmFunc) {
	sizeOffset := writeSecHeader(ctxt, sectionImport)

	// GOOS=kandelo imports its (shared) linear memory from env.memory rather
	// than defining and exporting a local memory. This memory import adds one
	// entry to the import section but does not affect function-index space.
	importMemory := buildcfg.GOOS == "kandelo"

	numImports := uint64(len(hostImports))
	if importMemory {
		numImports++
	}
	writeUleb128(ctxt.Out, numImports) // number of imports
	for _, fn := range hostImports {
		if fn.Module != "" {
			writeName(ctxt.Out, fn.Module)
		} else {
			writeName(ctxt.Out, wasm.GojsModule) // provided by the import object in wasm_exec.js
		}
		writeName(ctxt.Out, fn.Name)
		ctxt.Out.WriteByte(0x00) // func import
		writeUleb128(ctxt.Out, uint64(fn.Type))
	}

	if importMemory {
		// Import a SHARED linear memory from module "env", field "memory". The
		// Kandelo host provides one shared memory that also backs the syscall
		// channel; a guest-defined memory would be a separate address space.
		// A shared memory must declare a maximum, which the host pins at 16384
		// 64 KiB pages (1 GiB). The initial (min) size matches what
		// writeMemorySec computes from the program layout.
		const wasmPageSize = 64 << 10 // 64KB
		const kandeloMaxPages = 16384 // 16384 * 64 KiB = 1 GiB
		dataEnd := uint64(ldr.SymValue(ldr.Lookup("runtime.end", 0)))
		initialSize := dataEnd + 1<<20 // 1 MB, for runtime init allocating a few pages
		minPages := initialSize / wasmPageSize
		if minPages > kandeloMaxPages {
			// The program's static layout exceeds Kandelo's 1 GiB memory cap.
			// Emitting min > max would produce an invalid module that only
			// fails opaquely at instantiation, so fail loudly here instead.
			ld.Errorf("GOOS=kandelo: initial linear memory %d pages exceeds the maximum of %d pages (1 GiB); program static data is too large", minPages, uint64(kandeloMaxPages))
		}

		writeName(ctxt.Out, "env")
		writeName(ctxt.Out, "memory")
		ctxt.Out.WriteByte(0x02)                // memory import
		ctxt.Out.WriteByte(0x03)                // limits flags: has-max (0x01) | shared (0x02)
		writeUleb128(ctxt.Out, minPages)        // min (initial) pages
		writeUleb128(ctxt.Out, kandeloMaxPages) // max pages
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeFunctionSec writes the section that declares the types of functions.
// The bodies of these functions will later be provided in the "code" section.
func writeFunctionSec(ctxt *ld.Link, fns []*wasmFunc) {
	sizeOffset := writeSecHeader(ctxt, sectionFunction)

	writeUleb128(ctxt.Out, uint64(len(fns)))
	for _, fn := range fns {
		writeUleb128(ctxt.Out, uint64(fn.Type))
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeTableSec writes the section that declares tables. Currently there is only a single table
// that is used by the CallIndirect operation to dynamically call any function.
// The contents of the table get initialized by the "element" section.
func writeTableSec(ctxt *ld.Link, fns []*wasmFunc) {
	sizeOffset := writeSecHeader(ctxt, sectionTable)

	numElements := uint64(funcValueOffset + len(fns))
	writeUleb128(ctxt.Out, 1)           // number of tables
	ctxt.Out.WriteByte(0x70)            // type: anyfunc
	ctxt.Out.WriteByte(0x00)            // no max
	writeUleb128(ctxt.Out, numElements) // min

	writeSecSize(ctxt, sizeOffset)
}

// writeMemorySec writes the section that declares linear memories. Currently one linear memory is being used.
// Linear memory always starts at address zero. More memory can be requested with the GrowMemory instruction.
func writeMemorySec(ctxt *ld.Link, ldr *loader.Loader) {
	sizeOffset := writeSecHeader(ctxt, sectionMemory)

	dataEnd := uint64(ldr.SymValue(ldr.Lookup("runtime.end", 0)))
	var initialSize = dataEnd + 1<<20 // 1 MB, for runtime init allocating a few pages

	const wasmPageSize = 64 << 10 // 64KB

	writeUleb128(ctxt.Out, 1)                        // number of memories
	ctxt.Out.WriteByte(0x00)                         // no maximum memory size
	writeUleb128(ctxt.Out, initialSize/wasmPageSize) // minimum (initial) memory size

	writeSecSize(ctxt, sizeOffset)
}

// writeGlobalSec writes the section that declares global variables.
func writeGlobalSec(ctxt *ld.Link, ldr *loader.Loader, hasMemoryBase, hasTableBase, hasStackPointer, hasCTLSBase, hasChannelBase bool, dataAddressGlobals []wasmDataAddressGlobal) {
	sizeOffset := writeSecHeader(ctxt, sectionGlobal)

	globalRegs := []byte{
		I32, // 0: SP
		I64, // 1: CTXT
		I64, // 2: g
		I64, // 3: RET0
		I64, // 4: RET1
		I64, // 5: RET2
		I64, // 6: RET3
		I32, // 7: PAUSE
	}

	// GOOS=kandelo synthesizes one extra, exported global, __tls_base (global
	// index 8), whose constant value is the linear-memory address of the
	// runtime.kandeloChannelBase data word. The Kandelo host locates this
	// exported global at instantiation, reads its value as an address, and
	// stores the syscall-channel offset into linear memory there (see
	// setupChannelBase in host/src/worker-main.ts). Because we do NOT export
	// __get_channel_base_addr, the host uses a detected offset of 0, i.e. it
	// writes at exactly __tls_base. The runtime then reads the channel offset
	// back from runtime.kandeloChannelBase. The name "__tls_base" mirrors the C
	// glue's TLS-slot mechanism; here it simply points at a reserved word.
	numGlobals := len(globalRegs)
	kandelo := buildcfg.GOOS == "kandelo"
	if kandelo {
		// Two synthesized globals: __tls_base (channel-base receiver, index 8)
		// and __heap_base (static-data end, index 9). See below.
		numGlobals += 2
		if hasMemoryBase {
			numGlobals++
		}
		if hasTableBase {
			numGlobals++
		}
		if hasStackPointer {
			numGlobals++
		}
		if hasCTLSBase {
			numGlobals++
		}
		if hasChannelBase {
			numGlobals++
		}
		numGlobals += len(dataAddressGlobals)
	}

	writeUleb128(ctxt.Out, uint64(numGlobals)) // number of globals

	for _, typ := range globalRegs {
		ctxt.Out.WriteByte(typ)
		ctxt.Out.WriteByte(0x01) // var
		switch typ {
		case I32:
			writeI32Const(ctxt.Out, 0)
		case I64:
			writeI64Const(ctxt.Out, 0)
		}
		ctxt.Out.WriteByte(0x0b) // end
	}

	if kandelo {
		s := ldr.Lookup("runtime.kandeloChannelBase", 0)
		if s == 0 {
			ld.Errorf("GOOS=kandelo: runtime.kandeloChannelBase symbol not defined")
		}
		addr := ldr.SymValue(s)
		if addr <= 0 {
			ld.Errorf("GOOS=kandelo: runtime.kandeloChannelBase has invalid address %d (symbol dead-code-eliminated?)", addr)
		}
		ctxt.Out.WriteByte(I32)  // __tls_base type: i32 (address)
		ctxt.Out.WriteByte(0x00) // immutable (const)
		writeI32Const(ctxt.Out, int32(addr))
		ctxt.Out.WriteByte(0x0b) // end

		// __heap_base (global index 9): an immutable i32 whose value is the end
		// of the module's static data (runtime.end, the same address initBloc
		// uses for the initial break). The Kandelo host reads this exported
		// global (see extractHeapBase in host/src/constants.ts) to place the
		// per-process syscall channel region just above the guest's static data
		// instead of at its fixed 16 MiB fallback. Combined with the runtime
		// starting the Go heap above the channel (see kandeloStartHeapAboveChannel
		// in channel_kandelo.go), this bounds the memory reserved below the heap
		// to roughly the module's own 1 MiB init headroom rather than ~14 MiB.
		// Every C/musl module already exports __heap_base; this makes Go modules
		// match that native-module shape.
		dataEnd := int32(ldr.SymValue(ldr.Lookup("runtime.end", 0)))
		ctxt.Out.WriteByte(I32)  // __heap_base type: i32 (address)
		ctxt.Out.WriteByte(0x00) // immutable (const)
		writeI32Const(ctxt.Out, dataEnd)
		ctxt.Out.WriteByte(0x0b) // end

		if hasMemoryBase {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x01)
			writeI32Const(ctxt.Out, 0)
			ctxt.Out.WriteByte(0x0b)
		}
		if hasTableBase {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x00)
			writeI32Const(ctxt.Out, 0)
			ctxt.Out.WriteByte(0x0b)
		}
		if hasStackPointer {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x01)
			cStack := ldr.Lookup("runtime.cgoCStack", 0)
			if cStack == 0 || !ldr.AttrReachable(cStack) {
				ld.Exitf("GOOS=kandelo: C stack is not linked")
			}
			writeI32Const(ctxt.Out, int32(ldr.SymValue(cStack)+int64(ldr.SymSize(cStack))))
			ctxt.Out.WriteByte(0x0b)
		}
		if hasCTLSBase {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x01)
			writeI32Const(ctxt.Out, 0)
			ctxt.Out.WriteByte(0x0b)
		}
		if hasChannelBase {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x01)
			writeI32Const(ctxt.Out, 0)
			ctxt.Out.WriteByte(0x0b)
		}
		for _, addressGlobal := range dataAddressGlobals {
			ctxt.Out.WriteByte(I32)
			ctxt.Out.WriteByte(0x00)
			writeI32Const(ctxt.Out, int32(addressGlobal.Address))
			ctxt.Out.WriteByte(0x0b)
		}
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeExportSec writes the section that declares exports.
// Exports can be accessed by the WebAssembly host, usually JavaScript.
// The wasm_export_* functions and the linear memory get exported.
func writeExportSec(ctxt *ld.Link, ldr *loader.Loader, lenHostImports int, abiVersionFuncIdx uint32, threadSlotsFuncIdx uint32, preallocateThreadSlotsFuncIdx uint32, initTLSFuncIdx, startTLSFuncIdx uint32, hasMemoryBase, hasTableBase, hasStackPointer, hasCTLSBase, hasChannelBase bool) {
	sizeOffset := writeSecHeader(ctxt, sectionExport)

	switch buildcfg.GOOS {
	case "wasip1":
		// wasip1 exports the runtime entry point and the linear memory.
		writeUleb128(ctxt.Out, uint64(2+len(ldr.WasmExports))) // number of exports
		var entry, entryExpName string
		switch ctxt.BuildMode {
		case ld.BuildModeExe:
			entry = "_rt0_wasm_" + buildcfg.GOOS
			entryExpName = "_start"
		case ld.BuildModeCShared:
			entry = "_rt0_wasm_" + buildcfg.GOOS + "_lib"
			entryExpName = "_initialize"
		}
		s := ldr.Lookup(entry, 0)
		if s == 0 {
			ld.Errorf("export symbol %s not defined", entry)
		}
		idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
		writeName(ctxt.Out, entryExpName)   // the wasi entrypoint
		ctxt.Out.WriteByte(0x00)            // func export
		writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		for _, s := range ldr.WasmExports {
			idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
			writeName(ctxt.Out, ldr.SymName(s))
			ctxt.Out.WriteByte(0x00)            // func export
			writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		}
		writeName(ctxt.Out, "memory") // memory in wasi
		ctxt.Out.WriteByte(0x02)      // mem export
		writeUleb128(ctxt.Out, 0)     // memidx
	case "kandelo":
		// Kandelo native module shape: export the runtime entry point, the
		// __abi_version marker, the __tls_base global (channel-base receiver),
		// the __heap_base global (static-data end), and the single anyfunc
		// table as __indirect_function_table. Linear memory is NOT exported
		// here; it is imported from env.memory (see writeImportSec).
		// The export count is entry(1) + WasmExports + __abi_version(1) +
		// __wasm_posix_thread_slots(1) + preallocation marker(1) +
		// __tls_base(1) + __heap_base(1) +
		// __indirect_function_table(1).
		exportCount := 7 + len(ldr.WasmExports)
		if hasStackPointer {
			exportCount++
		}
		if hasChannelBase {
			exportCount++
		}
		if initTLSFuncIdx != 0 {
			exportCount++
		}
		writeUleb128(ctxt.Out, uint64(exportCount)) // number of exports
		var entry, entryExpName string
		switch ctxt.BuildMode {
		case ld.BuildModeExe:
			entry = "_rt0_wasm_kandelo"
			entryExpName = "_start"
		case ld.BuildModeCShared:
			entry = "_rt0_wasm_kandelo_lib"
			entryExpName = "_initialize"
		}
		s := ldr.Lookup(entry, 0)
		if s == 0 {
			ld.Errorf("export symbol %s not defined", entry)
		}
		idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
		if startTLSFuncIdx != 0 {
			idx = startTLSFuncIdx
		}
		writeName(ctxt.Out, entryExpName)   // process entry point
		ctxt.Out.WriteByte(0x00)            // func export
		writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		for _, s := range ldr.WasmExports {
			idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
			writeName(ctxt.Out, ldr.SymName(s))
			ctxt.Out.WriteByte(0x00)            // func export
			writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		}
		writeName(ctxt.Out, "__abi_version")               // Kandelo ABI marker
		ctxt.Out.WriteByte(0x00)                           // func export
		writeUleb128(ctxt.Out, uint64(abiVersionFuncIdx))  // funcidx
		writeName(ctxt.Out, "__wasm_posix_thread_slots")   // pthread slot declaration
		ctxt.Out.WriteByte(0x00)                           // func export
		writeUleb128(ctxt.Out, uint64(threadSlotsFuncIdx)) // funcidx
		writeName(ctxt.Out, "__wasm_posix_preallocate_thread_slots")
		ctxt.Out.WriteByte(0x00)
		writeUleb128(ctxt.Out, uint64(preallocateThreadSlotsFuncIdx))
		if initTLSFuncIdx != 0 {
			writeName(ctxt.Out, "__wasm_init_tls")
			ctxt.Out.WriteByte(0x00)
			writeUleb128(ctxt.Out, uint64(initTLSFuncIdx))
		}
		// __tls_base global (index 8): the 9 globals are the 8 fixed VM
		// registers (indices 0-7) plus this synthesized channel-base receiver
		// appended in writeGlobalSec.
		const kandeloTLSBaseGlobalIdx = 8
		writeName(ctxt.Out, "__tls_base") // channel-base receiver
		ctxt.Out.WriteByte(0x03)          // global export
		writeUleb128(ctxt.Out, uint64(kandeloTLSBaseGlobalIdx))
		// __heap_base global (index 9): static-data end the host uses to place
		// the syscall channel just above the guest data (see writeGlobalSec).
		const kandeloHeapBaseGlobalIdx = 9
		writeName(ctxt.Out, "__heap_base") // static-data end (channel placement)
		ctxt.Out.WriteByte(0x03)           // global export
		writeUleb128(ctxt.Out, uint64(kandeloHeapBaseGlobalIdx))
		// __indirect_function_table (table index 0): the single anyfunc table
		// writeTableSec declares and writeElementSec fills. The Kandelo host's
		// thread bootstrap invokes a new M's entry via table.get(fnPtr)(), where
		// fnPtr is the PC_F of a Go func value (PC_F = funcValueOffset +
		// funcIndex); exporting the table is the prerequisite for that call.
		// It changes nothing at runtime for a single-M process.
		const kandeloIndirectFuncTableIdx = 0
		writeName(ctxt.Out, "__indirect_function_table") // thread-entry dispatch
		ctxt.Out.WriteByte(0x01)                         // table export
		writeUleb128(ctxt.Out, uint64(kandeloIndirectFuncTableIdx))
		if hasStackPointer {
			stackPointerIndex := uint64(10)
			if hasMemoryBase {
				stackPointerIndex++
			}
			if hasTableBase {
				stackPointerIndex++
			}
			writeName(ctxt.Out, "__stack_pointer")
			ctxt.Out.WriteByte(0x03)
			writeUleb128(ctxt.Out, stackPointerIndex)
		}
		if hasChannelBase {
			channelBaseIndex := uint64(10)
			if hasMemoryBase {
				channelBaseIndex++
			}
			if hasTableBase {
				channelBaseIndex++
			}
			if hasStackPointer {
				channelBaseIndex++
			}
			if hasCTLSBase {
				channelBaseIndex++
			}
			writeName(ctxt.Out, "__channel_base")
			ctxt.Out.WriteByte(0x03)
			writeUleb128(ctxt.Out, channelBaseIndex)
		}
	case "js":
		writeUleb128(ctxt.Out, uint64(4+len(ldr.WasmExports))) // number of exports
		for _, name := range []string{"run", "resume", "getsp"} {
			s := ldr.Lookup("wasm_export_"+name, 0)
			if s == 0 {
				ld.Errorf("export symbol %s not defined", "wasm_export_"+name)
			}
			idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
			writeName(ctxt.Out, name)           // inst.exports.run/resume/getsp in wasm_exec.js
			ctxt.Out.WriteByte(0x00)            // func export
			writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		}
		for _, s := range ldr.WasmExports {
			idx := uint32(lenHostImports) + uint32(ldr.SymValue(s)>>16) - funcValueOffset
			writeName(ctxt.Out, ldr.SymName(s))
			ctxt.Out.WriteByte(0x00)            // func export
			writeUleb128(ctxt.Out, uint64(idx)) // funcidx
		}
		writeName(ctxt.Out, "mem") // inst.exports.mem in wasm_exec.js
		ctxt.Out.WriteByte(0x02)   // mem export
		writeUleb128(ctxt.Out, 0)  // memidx
	default:
		ld.Exitf("internal error: writeExportSec: unrecognized GOOS %s", buildcfg.GOOS)
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeElementSec writes the section that initializes the tables declared by the "table" section.
// The table for CallIndirect gets initialized in a very simple way so that each table index (PC_F value)
// maps linearly to the function index (numImports + PC_F).
func writeElementSec(ctxt *ld.Link, numImports, numFns uint64) {
	sizeOffset := writeSecHeader(ctxt, sectionElement)

	writeUleb128(ctxt.Out, 1) // number of element segments

	writeUleb128(ctxt.Out, 0) // tableidx
	writeI32Const(ctxt.Out, funcValueOffset)
	ctxt.Out.WriteByte(0x0b) // end

	writeUleb128(ctxt.Out, numFns) // number of entries
	for i := uint64(0); i < numFns; i++ {
		writeUleb128(ctxt.Out, numImports+i)
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeCodeSec writes the section that provides the function bodies for the functions
// declared by the "func" section.
func writeCodeSec(ctxt *ld.Link, fns []*wasmFunc) {
	sizeOffset := writeSecHeader(ctxt, sectionCode)

	writeUleb128(ctxt.Out, uint64(len(fns))) // number of code entries
	for _, fn := range fns {
		writeUleb128(ctxt.Out, uint64(len(fn.Code)))
		ctxt.Out.Write(fn.Code)
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeDataSec writes the section that provides data that will be used to initialize the linear memory.
func writeDataSec(ctxt *ld.Link) {
	sizeOffset := writeSecHeader(ctxt, sectionData)

	type dataSegment struct {
		offset int32
		data   []byte
	}

	// Omit blocks of zeroes and instead emit data segments with offsets skipping the zeroes.
	// This reduces the size of the WebAssembly binary. We use 8 bytes as an estimate for the
	// overhead of adding a new segment (same as wasm-opt's memory-packing optimization uses).
	const segmentOverhead = 8

	// Generate at most this many segments. A higher number of segments gets rejected by some WebAssembly runtimes.
	const maxNumSegments = 100000

	var segments []*dataSegment
	for secIndex, ds := range dataSects {
		data := ds.data
		offset := int32(ds.sect.Vaddr)

		// skip leading zeroes
		for len(data) > 0 && data[0] == 0 {
			data = data[1:]
			offset++
		}

		for len(data) > 0 {
			dataLen := int32(len(data))
			var segmentEnd, zeroEnd int32
			if len(segments)+(len(dataSects)-secIndex) == maxNumSegments {
				segmentEnd = dataLen
				zeroEnd = dataLen
			} else {
				for {
					// look for beginning of zeroes
					for segmentEnd < dataLen && data[segmentEnd] != 0 {
						segmentEnd++
					}
					// look for end of zeroes
					zeroEnd = segmentEnd
					for zeroEnd < dataLen && data[zeroEnd] == 0 {
						zeroEnd++
					}
					// emit segment if omitting zeroes reduces the output size
					if zeroEnd-segmentEnd >= segmentOverhead || zeroEnd == dataLen {
						break
					}
					segmentEnd = zeroEnd
				}
			}

			segments = append(segments, &dataSegment{
				offset: offset,
				data:   data[:segmentEnd],
			})
			data = data[zeroEnd:]
			offset += zeroEnd
		}
	}

	writeUleb128(ctxt.Out, uint64(len(segments))) // number of data entries
	for _, seg := range segments {
		writeUleb128(ctxt.Out, 0) // memidx
		writeI32Const(ctxt.Out, seg.offset)
		ctxt.Out.WriteByte(0x0b) // end
		writeUleb128(ctxt.Out, uint64(len(seg.data)))
		ctxt.Out.Write(seg.data)
	}

	writeSecSize(ctxt, sizeOffset)
}

// writeProducerSec writes an optional section that reports the source language and compiler version.
func writeProducerSec(ctxt *ld.Link) {
	sizeOffset := writeSecHeader(ctxt, sectionCustom)
	writeName(ctxt.Out, "producers")

	writeUleb128(ctxt.Out, 2) // number of fields

	writeName(ctxt.Out, "language")       // field name
	writeUleb128(ctxt.Out, 1)             // number of values
	writeName(ctxt.Out, "Go")             // value: name
	writeName(ctxt.Out, buildcfg.Version) // value: version

	writeName(ctxt.Out, "processed-by")   // field name
	writeUleb128(ctxt.Out, 1)             // number of values
	writeName(ctxt.Out, "Go cmd/compile") // value: name
	writeName(ctxt.Out, buildcfg.Version) // value: version

	writeSecSize(ctxt, sizeOffset)
}

var nameRegexp = regexp.MustCompile(`[^\w.]`)

// writeNameSec writes an optional section that assigns names to the functions declared by the "func" section.
// The names are only used by WebAssembly stack traces, debuggers and decompilers.
// TODO(neelance): add symbol table of DATA symbols
func writeNameSec(ctxt *ld.Link, firstFnIndex int, fns []*wasmFunc) {
	sizeOffset := writeSecHeader(ctxt, sectionCustom)
	writeName(ctxt.Out, "name")

	sizeOffset2 := writeSecHeader(ctxt, 0x01) // function names
	writeUleb128(ctxt.Out, uint64(len(fns)))
	for i, fn := range fns {
		writeUleb128(ctxt.Out, uint64(firstFnIndex+i))
		writeName(ctxt.Out, fn.Name)
	}
	writeSecSize(ctxt, sizeOffset2)

	writeSecSize(ctxt, sizeOffset)
}

type nameWriter interface {
	io.ByteWriter
	io.Writer
}

func writeI32Const(w io.ByteWriter, v int32) {
	w.WriteByte(0x41) // i32.const
	writeSleb128(w, int64(v))
}

func writeI64Const(w io.ByteWriter, v int64) {
	w.WriteByte(0x42) // i64.const
	writeSleb128(w, v)
}

func writeName(w nameWriter, name string) {
	writeUleb128(w, uint64(len(name)))
	w.Write([]byte(name))
}

func writeUleb128(w io.ByteWriter, v uint64) {
	if v < 128 {
		w.WriteByte(uint8(v))
		return
	}
	more := true
	for more {
		c := uint8(v & 0x7f)
		v >>= 7
		more = v != 0
		if more {
			c |= 0x80
		}
		w.WriteByte(c)
	}
}

func writeUleb128FixedLength(w io.ByteWriter, v uint64, length int) {
	for i := 0; i < length; i++ {
		c := uint8(v & 0x7f)
		v >>= 7
		if i < length-1 {
			c |= 0x80
		}
		w.WriteByte(c)
	}
	if v != 0 {
		panic("writeUleb128FixedLength: length too small")
	}
}

func writeSleb128(w io.ByteWriter, v int64) {
	more := true
	for more {
		c := uint8(v & 0x7f)
		s := uint8(v & 0x40)
		v >>= 7
		more = !((v == 0 && s == 0) || (v == -1 && s != 0))
		if more {
			c |= 0x80
		}
		w.WriteByte(c)
	}
}

func fieldsToTypes(fields []obj.WasmField) []byte {
	b := make([]byte, len(fields))
	for i, f := range fields {
		switch f.Type {
		case obj.WasmI32, obj.WasmPtr, obj.WasmBool:
			b[i] = I32
		case obj.WasmI64:
			b[i] = I64
		case obj.WasmF32:
			b[i] = F32
		case obj.WasmF64:
			b[i] = F64
		default:
			panic(fmt.Sprintf("fieldsToTypes: unknown field type: %d", f.Type))
		}
	}
	return b
}
