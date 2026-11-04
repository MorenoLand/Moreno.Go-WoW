package ui

import (
	"sort"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The totem bar: for each of the four elements the totems the shaman knows,
// the highest rank of each.
const (
	multiCastFirstSlot = 132
	multiCastLastSlot  = 143
)

// totemSlots says which totem bar slots a totem category belongs to (a bit per
// slot: fire, earth, water, air).
func totemSlots(category uint32) uint32 {
	switch category {
	case 2:
		return 2
	case 3:
		return 8
	case 4:
		return 1
	case 5:
		return 4
	case 0x15:
		return 0xf
	}
	return 0
}

// rankNumber is the number a rank text such as "Rank 3" ends in.
func rankNumber(rank string) int {
	number := 0
	for i := 0; i < len(rank); i++ {
		if rank[i] >= '0' && rank[i] <= '9' {
			number = number*10 + int(rank[i]-'0')
		} else if number != 0 {
			break
		}
	}
	return number
}

// multiCastSpells are the totems of one element.
func (rt *Runtime) multiCastSpells(element int) []uint32 {
	ids := make([]uint32, 0, len(rt.gameplay.Known))
	for id, known := range rt.gameplay.Known {
		if known {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	var list []uint32
	for _, id := range ids {
		spell := rt.gameplay.Catalog[id]
		fits := false
		for _, category := range spell.Tools {
			if totemSlots(category)&(1<<element) != 0 {
				fits = true
			}
		}
		if !fits {
			continue
		}
		replaced := false
		for i, other := range list {
			if rt.gameplay.Catalog[other].Name == spell.Name {
				if rankNumber(spell.Rank) > rankNumber(rt.gameplay.Catalog[other].Rank) {
					list[i] = id
				}
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, id)
		}
	}
	return list
}

func registerMultiCastAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetMultiCastTotemSpells", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetMultiCastTotemSpells(slot)")
			return 0
		}
		list := rt.multiCastSpells((L.CheckInt(1) - 1) & 3)
		for _, id := range list {
			L.Push(lua.LNumber(id))
		}
		return len(list)
	}))
	L.SetGlobal("SetMultiCastSpell", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber && L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetMultiCastSpell(slot, spellID)")
			return 0
		}
		slot := L.CheckInt(1) - 1
		if slot < multiCastFirstSlot || slot > multiCastLastSlot {
			return 0
		}
		spell := uint32(L.OptInt(2, 0))
		host, ok := rt.Host.(WorldActionHost)
		if !ok {
			return 0
		}
		button := world.ActionButton{}
		if spell != 0 {
			found := false
			for _, id := range rt.multiCastSpells(slot & 3) {
				if id == spell {
					found = true
				}
			}
			if !found {
				return 0
			}
			button = world.ActionButton{ID: spell}
		}
		if host.SetActionButton(slot, button) == nil {
			if len(rt.gameplay.Actions) <= slot {
				rt.gameplay.Actions = append(rt.gameplay.Actions, make([]world.ActionButton, slot+1-len(rt.gameplay.Actions))...)
			}
			rt.gameplay.Actions[slot] = button
			rt.FireEvent("ACTIONBAR_SLOT_CHANGED", lua.LNumber(slot+1))
		}
		return 0
	}))
}
