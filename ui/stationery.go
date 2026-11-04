package ui

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// StationeryRow is a row of Stationery.dbc: a kind of letter paper, the item
// that grants it and its texture.
type StationeryRow struct {
	ID      uint32
	Item    uint32
	Texture string
}

type stationeryState struct {
	rows     []StationeryRow
	selected uint32
}

// SetStationery installs the stationeries of the client's tables.
func (rt *Runtime) SetStationery(rows []StationeryRow) { rt.stationery.rows = rows }

// stationeries are the papers the character may write on: the ones that need
// no item and the ones whose item it carries.
func (rt *Runtime) stationeries() []StationeryRow {
	var list []StationeryRow
	for _, row := range rt.stationery.rows {
		if row.Texture != "" || rt.itemCountOf(row.Item) > 0 {
			if template := rt.itemTemplateIfKnown(row.Item); template != nil && !template.NotFound {
				list = append(list, row)
			}
		}
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].ID < list[b].ID })
	return list
}

func registerStationeryAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumStationeries", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.stationeries())))
		return 1
	})
	reg("GetStationeryInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetStationeryInfo(index)")
			return 0
		}
		list := rt.stationeries()
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(list) {
			return pushAll(L, lua.LNil, lua.LNil, lua.LNil)
		}
		row := list[index]
		template := rt.itemTemplateIfKnown(row.Item)
		L.Push(lua.LString(template.Name))
		L.Push(lua.LString(rt.itemIcon(row.Item)))
		if rt.itemCountOf(row.Item) > 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(template.BuyPrice))
		}
		return 3
	})
	reg("SelectStationery", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SelectStationery(index)")
			return 0
		}
		list := rt.stationeries()
		if index := L.CheckInt(1) - 1; index >= 0 && index < len(list) {
			rt.stationery.selected = list[index].ID
		} else {
			rt.stationery.selected = 0
		}
		return 0
	})
	reg("GetSelectedStationeryTexture", func(L *lua.LState) int {
		for _, row := range rt.stationery.rows {
			if row.ID == rt.stationery.selected && rt.stationery.selected != 0 {
				L.Push(lua.LString(row.Texture))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
}
