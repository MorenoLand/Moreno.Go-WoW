package ui

import (
	"runtime"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// heapInUse is the bytes the program holds now.
func heapInUse() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// noteAddOnMemory keeps what loading an add-on added to the program's memory,
// in kilobytes.
func (rt *Runtime) noteAddOnMemory(name string, before uint64) {
	after := heapInUse()
	if rt.addonMemory == nil {
		rt.addonMemory = map[string]float64{}
	}
	if after > before {
		rt.addonMemory[strings.ToLower(name)] += float64(after-before) / 1024
	}
}

// addOnArgument reads the index or the name of an add-on the way the usage
// calls do and gives the add-on's name.
func (rt *Runtime) addOnArgument(L *lua.LState, usage string) (string, bool) {
	switch value := L.Get(1).(type) {
	case lua.LNumber:
		index := int(value)
		if index < 1 || index > len(rt.Glue.AddOns) {
			L.RaiseError("AddOn index must be in the range of 1 to %d", len(rt.Glue.AddOns))
			return "", false
		}
		return rt.Glue.AddOns[index-1].Name, true
	case lua.LString:
		return string(value), true
	}
	L.RaiseError("Usage: %s(index or \"name\")", usage)
	return "", false
}

// registerAddOnUsageAPI defines the calls that report what add-ons use. The
// time scripts take is not measured unless script profiling is on, so it
// reads as nothing.
func registerAddOnUsageAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("UpdateAddOnMemoryUsage", func(L *lua.LState) int { return 0 })
	reg("UpdateAddOnCPUUsage", func(L *lua.LState) int { return 0 })
	reg("ResetCPUUsage", func(L *lua.LState) int { return 0 })
	reg("GetAddOnMemoryUsage", func(L *lua.LState) int {
		name, ok := rt.addOnArgument(L, "GetAddOnMemoryUsage")
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(rt.addonMemory[strings.ToLower(name)]))
		return 1
	})
	reg("GetAddOnCPUUsage", func(L *lua.LState) int {
		if _, ok := rt.addOnArgument(L, "GetAddOnCPUUsage"); !ok {
			return 0
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetScriptCPUUsage", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetFunctionCPUUsage", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTFunction {
			L.RaiseError("Usage: GetFunctionCPUUsage(function[, includeSubroutines])")
			return 0
		}
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	})
	reg("GetFrameCPUUsage", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTUserData {
			L.RaiseError("Usage: GetFrameCPUUsage(frame[, includeChildren])")
			return 0
		}
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	})
}
