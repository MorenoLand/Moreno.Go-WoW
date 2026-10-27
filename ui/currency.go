package ui

import (
	"sort"
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// Currency items sit in the inventory slots from 118; the two points that are
// not items (arena and honor) have these item ids as currencies.
const (
	currencySlotStart  = 118
	currencySlotEnd    = 150
	currencyArenaItem  = 43307
	currencyHonorItem  = 43308
	currencyArenaType  = 1
	currencyHonorType  = 2
	currencyUnusedLow  = "currencyTokensUnused1"
	currencyUnusedHigh = "currencyTokensUnused2"
	currencyPackLow    = "currencyTokensBackpack1"
	currencyPackHigh   = "currencyTokensBackpack2"
)

// CurrencyCategory is a row of CurrencyCategory.dbc.
type CurrencyCategory struct {
	ID    uint32
	Flags uint32
	Name  string
}

// CurrencyType is a row of CurrencyTypes.dbc: the item that counts a currency,
// the category it lists under and its bit among the ones the player knows.
type CurrencyType struct {
	ID       uint32
	Item     uint32
	Category uint32
	Bit      int
}

// CurrencyTables are the currency data files.
type CurrencyTables struct {
	Categories []CurrencyCategory
	Types      []CurrencyType
}

type currencyState struct {
	tables CurrencyTables
	// known is the mask of currencies the player has met, honor and arena the
	// two points held in fields.
	known        [2]uint32
	honor, arena uint32
	collapsed    map[uint32]bool
	sorted       []CurrencyCategory
}

type currencyRow struct {
	name     string
	item     uint32
	category uint32
	header   bool
	expanded bool
	unused   bool
	watched  bool
	hasName  bool
	hidden   bool
}

// SetCurrencyTables gives the interface the currency data files.
func (rt *Runtime) SetCurrencyTables(tables CurrencyTables) {
	state := &rt.currency
	state.tables = tables
	state.sorted = append([]CurrencyCategory(nil), tables.Categories...)
	// Categories list with the plain ones first, each group by name.
	sort.SliceStable(state.sorted, func(a, b int) bool {
		first, second := state.sorted[a], state.sorted[b]
		if first.Flags&1 != second.Flags&1 {
			return first.Flags&1 < second.Flags&1
		}
		return first.Name < second.Name
	})
}

// SetCurrencyFields takes the currency fields of the player.
func (rt *Runtime) SetCurrencyFields(known [2]uint32, honor, arena uint32) {
	state := &rt.currency
	changed := state.known != known || state.honor != honor || state.arena != arena
	state.known, state.honor, state.arena = known, honor, arena
	if changed {
		rt.FireEvent("CURRENCY_DISPLAY_UPDATE")
	}
}

// currencyMask reads a cvar pair as one 64-bit mask.
func (rt *Runtime) currencyMask(low, high string) [2]uint32 {
	var mask [2]uint32
	for index, name := range [2]string{low, high} {
		if text, ok := rt.GetCVar(name); ok {
			if value, err := strconv.ParseUint(text, 10, 32); err == nil {
				mask[index] = uint32(value)
			}
		}
	}
	return mask
}

func (rt *Runtime) setCurrencyMask(low, high string, mask [2]uint32) {
	rt.SetCVar(low, strconv.FormatUint(uint64(mask[0]), 10))
	rt.SetCVar(high, strconv.FormatUint(uint64(mask[1]), 10))
}

func currencyBit(mask [2]uint32, bit int) bool {
	if bit < 1 || bit > 64 {
		return false
	}
	return mask[(bit-1)/32]&(1<<uint((bit-1)%32)) != 0
}

func currencyBitSet(mask *[2]uint32, bit int, on bool) {
	if bit < 1 || bit > 64 {
		return
	}
	flag := uint32(1) << uint((bit-1)%32)
	if on {
		mask[(bit-1)/32] |= flag
	} else {
		mask[(bit-1)/32] &^= flag
	}
}

// currencyItemName is the name of the item that counts a currency.
func (rt *Runtime) currencyItemName(item uint32) (string, bool) {
	template := rt.itemTemplate(item)
	if template == nil || template.Name == "" {
		return "", false
	}
	return template.Name, true
}

// currencyRows builds the list the window shows: a header for each category
// with a currency the player knows, and under an open one its currencies.
// The last category holds the ones the player has put away.
func (rt *Runtime) currencyRows() []currencyRow {
	state := &rt.currency
	unused := rt.currencyMask(currencyUnusedLow, currencyUnusedHigh)
	backpack := rt.currencyMask(currencyPackLow, currencyPackHigh)
	var rows []currencyRow
	for index, category := range state.sorted {
		last := index == len(state.sorted)-1
		present := false
		for _, kind := range state.tables.Types {
			if !currencyBit(state.known, kind.Bit) {
				continue
			}
			if (last && currencyBit(unused, kind.Bit)) || (!last && kind.Category == category.ID) {
				present = true
				break
			}
		}
		if !present {
			continue
		}
		expanded := !state.collapsed[category.ID]
		rows = append(rows, currencyRow{name: category.Name, category: category.ID, header: true, expanded: expanded})
		if !expanded {
			continue
		}
		first := len(rows)
		for _, kind := range state.tables.Types {
			if !currencyBit(state.known, kind.Bit) {
				continue
			}
			isUnused := currencyBit(unused, kind.Bit)
			if last != isUnused || (!last && kind.Category != category.ID) {
				continue
			}
			name, hasName := rt.currencyItemName(kind.Item)
			rows = append(rows, currencyRow{name: name, hasName: hasName, item: kind.Item, category: kind.Category, unused: isUnused, watched: currencyBit(backpack, kind.Bit)})
		}
		part := rows[first:]
		sort.SliceStable(part, func(a, b int) bool { return part[a].name < part[b].name })
	}
	return rows
}

// currencyCount is how many of a currency the player holds, with the kind of
// points it is (0 for an item, 1 arena, 2 honor).
func (rt *Runtime) currencyCount(item uint32) (int, int) {
	switch item {
	case currencyArenaItem:
		return int(rt.currency.arena), currencyArenaType
	case currencyHonorItem:
		return int(rt.currency.honor), currencyHonorType
	}
	inventory := &rt.inventory
	for slot := currencySlotStart; slot < currencySlotEnd && slot < invSlotCount; slot++ {
		if instance := inventory.items[inventory.slots[slot]]; instance != nil && instance.Entry == item {
			return int(instance.Count), 0
		}
	}
	return 0, 0
}

func (rt *Runtime) currencyType(item uint32) (CurrencyType, bool) {
	for _, kind := range rt.currency.tables.Types {
		if kind.Item == item {
			return kind, true
		}
	}
	return CurrencyType{}, false
}

// registerCurrencyAPI defines the calls the currency window and the backpack
// make.
func registerCurrencyAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetCurrencyListSize", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.currencyRows())))
		return 1
	})
	reg("GetCurrencyListInfo", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		rows := rt.currencyRows()
		if index < 0 || index >= len(rows) {
			for count := 0; count < 9; count++ {
				L.Push(lua.LNil)
			}
			return 9
		}
		row := rows[index]
		if row.hasName || row.header {
			L.Push(lua.LString(row.name))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LBool(row.header))
		L.Push(lua.LBool(row.expanded))
		L.Push(lua.LBool(row.unused && !row.header))
		L.Push(lua.LBool(row.watched))
		count, extra := 0, 0
		if !row.header {
			count, extra = rt.currencyCount(row.item)
		}
		L.Push(lua.LNumber(count))
		L.Push(lua.LNumber(extra))
		if row.header {
			L.Push(lua.LNil)
			L.Push(lua.LNumber(row.item))
			return 9
		}
		if template := rt.itemTemplate(row.item); template != nil {
			L.Push(lua.LString(rt.itemIcon(row.item)))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(row.item))
		return 9
	})
	reg("ExpandCurrencyList", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		expand := L.CheckInt(2) != 0
		rows := rt.currencyRows()
		if index < 0 || index >= len(rows) || !rows[index].header {
			return 0
		}
		if rt.currency.collapsed == nil {
			rt.currency.collapsed = make(map[uint32]bool)
		}
		rt.currency.collapsed[rows[index].category] = !expand
		return 0
	})
	reg("SetCurrencyUnused", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		rows := rt.currencyRows()
		if index < 0 || index >= len(rows) || rows[index].header {
			return 0
		}
		if kind, ok := rt.currencyType(rows[index].item); ok {
			mask := rt.currencyMask(currencyUnusedLow, currencyUnusedHigh)
			currencyBitSet(&mask, kind.Bit, L.CheckInt(2) != 0)
			rt.setCurrencyMask(currencyUnusedLow, currencyUnusedHigh, mask)
		}
		return 0
	})
	reg("SetCurrencyBackpack", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		rows := rt.currencyRows()
		if index < 0 || index >= len(rows) || rows[index].header {
			return 0
		}
		if kind, ok := rt.currencyType(rows[index].item); ok {
			mask := rt.currencyMask(currencyPackLow, currencyPackHigh)
			currencyBitSet(&mask, kind.Bit, L.CheckInt(2) != 0)
			rt.setCurrencyMask(currencyPackLow, currencyPackHigh, mask)
		}
		return 0
	})
	reg("GetBackpackCurrencyInfo", func(L *lua.LState) int {
		want := L.CheckInt(1) - 1
		backpack := rt.currencyMask(currencyPackLow, currencyPackHigh)
		seen := 0
		for _, kind := range rt.currency.tables.Types {
			if !currencyBit(rt.currency.known, kind.Bit) || !currencyBit(backpack, kind.Bit) {
				continue
			}
			if seen != want {
				seen++
				continue
			}
			template := rt.itemTemplate(kind.Item)
			if template == nil {
				break
			}
			if template.Name != "" {
				L.Push(lua.LString(template.Name))
			} else {
				L.Push(lua.LNil)
			}
			count, extra := rt.currencyCount(kind.Item)
			L.Push(lua.LNumber(count))
			L.Push(lua.LNumber(extra))
			L.Push(lua.LString(rt.itemIcon(kind.Item)))
			L.Push(lua.LNumber(kind.Item))
			return 5
		}
		for count := 0; count < 5; count++ {
			L.Push(lua.LNil)
		}
		return 5
	})
	reg("GetHonorCurrency", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.currency.honor))
		return 1
	})
	reg("GetArenaCurrency", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.currency.arena))
		return 1
	})
}
