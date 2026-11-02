package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Inventory slots of the offhand and the ranged weapon, and the item class and
// subclass the checks below look for.
const (
	offhandSlot     = 17
	rangedSlot      = 18
	itemClassWeapon = 2
	weaponWand      = 19
	// unitRangeYards is how near a group member must be to be in range.
	unitRangeYards = 40
	// corpseHighGUID is the high part of a corpse object's GUID.
	corpseHighGUID = 0x2000
)

func registerUnitExtraAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("HasWandEquipped", func(L *lua.LState) int {
		if item := rt.equippedItem(rangedSlot); item != nil {
			if template := rt.itemTemplateIfKnown(item.Entry); template != nil && template.SubClass == weaponWand {
				L.Push(lua.LNumber(1))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("OffhandHasWeapon", func(L *lua.LState) int {
		if item := rt.equippedItem(offhandSlot); item != nil {
			if template := rt.itemTemplateIfKnown(item.Entry); template != nil && template.Class == itemClassWeapon {
				L.Push(lua.LNumber(1))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	// Wow.exe answers a unit that resolves to an object: UnitIsControlling
	// tests the first two words of its descriptor, its GUID, and UnitIsCorpse
	// asks whether that GUID names a corpse object.
	reg("UnitIsControlling", func(L *lua.LState) int {
		if rt.UnitGUID(L.CheckString(1)) != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("UnitIsCorpse", func(L *lua.LState) int {
		if guid := rt.UnitGUID(L.CheckString(1)); guid>>48 == corpseHighGUID {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("UnitInRange", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: UnitInRange("unit")`)
		}
		unit := L.CheckString(1)
		if rt.memberFor(unit) != nil && rt.unitDistance != nil {
			if distance, ok := rt.unitDistance(rt.UnitGUID(unit)); ok && distance < unitRangeYards {
				L.Push(lua.LNumber(1))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
}

// playerBytesField is the player field whose third byte holds the extra action
// bars the character shows.
const (
	playerBytesField = 1049
	actionBarCount   = 4
	clientActionBars = world.ClientOpcode(0x02BF)
)

func registerActionBarToggleAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetActionBarToggles", L.NewFunction(func(L *lua.LState) int {
		mask := uint8(0)
		if info := rt.unitInfo("player"); info != nil {
			mask = info.ActionBarToggles
		}
		for bit := 0; bit < actionBarCount; bit++ {
			if mask&(1<<bit) != 0 {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
		}
		return actionBarCount
	}))
	L.SetGlobal("SetActionBarToggles", L.NewFunction(func(L *lua.LState) int {
		mask := uint8(0)
		for bit := 0; bit < actionBarCount; bit++ {
			if lua.LVAsBool(L.Get(bit + 1)) {
				mask |= 1 << bit
			}
		}
		if info := rt.unitInfo("player"); info != nil {
			info.ActionBarToggles = mask
		}
		rt.send(clientActionBars, []byte{mask})
		return 0
	}))
}
