package ui

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// Faction flags, as the server sends them in the reputation list.
const (
	factionFlagVisible      = 0x01
	factionFlagAtWar        = 0x02
	factionFlagHidden       = 0x04
	factionFlagForcedHidden = 0x08
	factionFlagPeaceForced  = 0x10
	factionFlagInactive     = 0x20
	// factionAtWarFloor is the standing below which a faction cannot be made
	// peaceful.
	factionAtWarFloor = -3000
)

// FactionEntry is one reputation slot with what the Faction table says of it.
type FactionEntry struct {
	ID          uint32
	ListIndex   int
	ParentID    uint32
	Name        string
	Description string
	Flags       uint8
	// HasRep is set for a header that has a reputation bar of its own.
	HasRep bool
	// Base is the standing the faction starts at for the character's race and
	// class; Standing is what the server added to it.
	Base, Standing int32
}

// Total is the character's standing with the faction.
func (f FactionEntry) Total() int32 { return f.Base + f.Standing }

// factionRank is the standing id (1 Hated .. 8 Exalted) and the span of the
// bar for a total, with the thresholds Wow.exe applies.
func factionRank(total int32) (id int, low, high int32) {
	switch {
	case total >= 42000:
		return 8, 42000, 43000
	case total >= 21000:
		return 7, 21000, 42000
	case total >= 9000:
		return 6, 9000, 21000
	case total >= 3000:
		return 5, 3000, 9000
	case total >= 0:
		return 4, 0, 3000
	case total > -3000:
		return 3, -3000, 0
	case total > -6001:
		return 2, -6000, -3000
	}
	return 1, -42000, -6000
}

type factionRow struct {
	entry  *FactionEntry
	header bool
	child  bool
}

type factionState struct {
	entries   []FactionEntry
	watched   int
	selected  int
	collapsed map[uint32]bool
}

func (e *FactionEntry) shown() bool {
	return e.Flags&factionFlagVisible != 0 && e.Flags&(factionFlagHidden|factionFlagForcedHidden) == 0
}

// SetFactions replaces the reputation list; watched is the list index of the
// faction on the experience bar, or -1.
func (rt *Runtime) SetFactions(entries []FactionEntry, watched int) {
	if rt == nil {
		return
	}
	rt.factions.entries = append(rt.factions.entries[:0], entries...)
	rt.factions.watched = watched
	rt.FireEvent("UPDATE_FACTION")
}

// factionRows lays the shown factions out under their headers.
func (rt *Runtime) factionRows() []factionRow {
	state := &rt.factions
	byID := make(map[uint32]*FactionEntry)
	children := make(map[uint32][]*FactionEntry)
	for index := range state.entries {
		entry := &state.entries[index]
		if entry.ListIndex >= 0 {
			byID[entry.ID] = entry
		}
	}
	var top []*FactionEntry
	for index := range state.entries {
		entry := &state.entries[index]
		if entry.ListIndex < 0 {
			continue
		}
		if _, ok := byID[entry.ParentID]; ok && entry.ParentID != 0 && entry.ParentID != entry.ID {
			children[entry.ParentID] = append(children[entry.ParentID], entry)
		} else {
			top = append(top, entry)
		}
	}
	byIndex := func(list []*FactionEntry) {
		sort.SliceStable(list, func(a, b int) bool { return list[a].ListIndex < list[b].ListIndex })
	}
	byIndex(top)
	var anyShown func(entry *FactionEntry) bool
	anyShown = func(entry *FactionEntry) bool {
		if entry.shown() {
			return true
		}
		for _, child := range children[entry.ID] {
			if anyShown(child) {
				return true
			}
		}
		return false
	}
	var rows []factionRow
	var emit func(entry *FactionEntry, child bool)
	emit = func(entry *FactionEntry, child bool) {
		list := children[entry.ID]
		byIndex(list)
		if len(list) == 0 {
			if entry.shown() {
				rows = append(rows, factionRow{entry: entry, child: child})
			}
			return
		}
		if !anyShown(entry) {
			return
		}
		rows = append(rows, factionRow{entry: entry, header: true, child: child})
		if state.collapsed[entry.ID] {
			return
		}
		for _, sub := range list {
			emit(sub, true)
		}
	}
	for _, entry := range top {
		emit(entry, false)
	}
	return rows
}

func (rt *Runtime) factionAtListIndex(index int) *FactionEntry {
	for position := range rt.factions.entries {
		if rt.factions.entries[position].ListIndex == index {
			return &rt.factions.entries[position]
		}
	}
	return nil
}

// registerReputationAPI defines the calls the reputation window and the
// experience bar make.
func registerReputationAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	number := func(value int32) lua.LValue { return lua.LNumber(value) }
	flag := func(on bool) lua.LValue {
		if on {
			return lua.LNumber(1)
		}
		return lua.LNil
	}
	push := func(L *lua.LState, entry *FactionEntry, header, child bool) int {
		id, low, high := factionRank(entry.Total())
		L.Push(lua.LString(entry.Name))
		L.Push(lua.LString(entry.Description))
		L.Push(lua.LNumber(id))
		L.Push(number(low))
		L.Push(number(high))
		L.Push(number(entry.Total()))
		L.Push(flag(entry.Flags&factionFlagAtWar != 0))
		L.Push(flag(entry.Total() >= factionAtWarFloor && entry.Flags&factionFlagPeaceForced == 0))
		L.Push(flag(header))
		L.Push(flag(header && rt.factions.collapsed[entry.ID]))
		L.Push(flag(entry.HasRep))
		L.Push(flag(entry.ListIndex == rt.factions.watched))
		L.Push(flag(child))
		return 13
	}
	reg("GetNumFactions", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.factionRows())))
		return 1
	})
	reg("GetFactionInfo", func(L *lua.LState) int {
		rows := rt.factionRows()
		index := L.CheckInt(1)
		if index < 1 || index > len(rows) {
			return 0
		}
		row := rows[index-1]
		return push(L, row.entry, row.header, row.child)
	})
	reg("GetFactionInfoByID", func(L *lua.LState) int {
		id := uint32(L.CheckInt(1))
		for position := range rt.factions.entries {
			if entry := &rt.factions.entries[position]; entry.ID == id && entry.ListIndex >= 0 {
				return push(L, entry, false, false)
			}
		}
		return 0
	})
	reg("GetSelectedFaction", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.factions.selected))
		return 1
	})
	reg("SetSelectedFaction", func(L *lua.LState) int {
		rt.factions.selected = L.CheckInt(1)
		return 0
	})
	reg("GetWatchedFactionInfo", func(L *lua.LState) int {
		entry := rt.factionAtListIndex(rt.factions.watched)
		if entry == nil {
			return 0
		}
		id, low, high := factionRank(entry.Total())
		L.Push(lua.LString(entry.Name))
		L.Push(lua.LNumber(id))
		L.Push(number(low))
		L.Push(number(high))
		L.Push(number(entry.Total()))
		return 5
	})
	rowEntry := func(L *lua.LState) *FactionEntry {
		rows := rt.factionRows()
		index := L.CheckInt(1)
		if index < 1 || index > len(rows) {
			return nil
		}
		return rows[index-1].entry
	}
	reg("SetWatchedFactionIndex", func(L *lua.LState) int {
		index := L.CheckInt(1)
		list := -1
		if entry := rowEntry(L); entry != nil && index > 0 {
			list = entry.ListIndex
		}
		rt.factions.watched = list
		if host, ok := rt.Host.(interface{ SetWatchedFaction(uint32) error }); ok {
			_ = host.SetWatchedFaction(uint32(list))
		}
		rt.FireEvent("UPDATE_FACTION")
		return 0
	})
	setCollapsed := func(L *lua.LState, collapsed bool) int {
		if entry := rowEntry(L); entry != nil {
			if rt.factions.collapsed == nil {
				rt.factions.collapsed = make(map[uint32]bool)
			}
			rt.factions.collapsed[entry.ID] = collapsed
			rt.FireEvent("UPDATE_FACTION")
		}
		return 0
	}
	reg("ExpandFactionHeader", func(L *lua.LState) int { return setCollapsed(L, false) })
	reg("CollapseFactionHeader", func(L *lua.LState) int { return setCollapsed(L, true) })
	setFlag := func(L *lua.LState, bit uint8, on bool, send func(host interface{}, index uint32, on bool)) int {
		entry := rowEntry(L)
		if entry == nil {
			return 0
		}
		if on {
			entry.Flags |= bit
		} else {
			entry.Flags &^= bit
		}
		send(rt.Host, uint32(entry.ListIndex), on)
		rt.FireEvent("UPDATE_FACTION")
		return 0
	}
	sendAtWar := func(host interface{}, index uint32, on bool) {
		if h, ok := host.(interface{ SetFactionAtWar(uint32, bool) error }); ok {
			_ = h.SetFactionAtWar(index, on)
		}
	}
	sendInactive := func(host interface{}, index uint32, on bool) {
		if h, ok := host.(interface{ SetFactionInactive(uint32, bool) error }); ok {
			_ = h.SetFactionInactive(index, on)
		}
	}
	reg("FactionToggleAtWar", func(L *lua.LState) int {
		entry := rowEntry(L)
		if entry == nil || entry.Total() < factionAtWarFloor || entry.Flags&factionFlagPeaceForced != 0 {
			return 0
		}
		return setFlag(L, factionFlagAtWar, entry.Flags&factionFlagAtWar == 0, sendAtWar)
	})
	reg("SetFactionActive", func(L *lua.LState) int { return setFlag(L, factionFlagInactive, false, sendInactive) })
	reg("SetFactionInactive", func(L *lua.LState) int { return setFlag(L, factionFlagInactive, true, sendInactive) })
}
