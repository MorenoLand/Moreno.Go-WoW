package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Spell autocast: the spells of a pet that it may cast by itself. The pet's
// action bar carries which spells allow it and which have it on.
const (
	petActionAllowsAutocast = 0x80000000
	petActionAutocastOn     = 0x40000000
)

// petActionSlot is the slot of the pet's bar that holds a spell, or -1.
func (rt *Runtime) petActionSlot(spell uint32) int {
	for index, action := range rt.pet.actions {
		if action != 0 && petActionType(action)&0x3F != 6 && petActionType(action)&0x3F != 7 && petSpellOf(action) == spell {
			return index
		}
	}
	return -1
}

// setSpellAutocast turns the autocast of a pet spell on (1), off (0) or
// flips it (-1).
func (rt *Runtime) setSpellAutocast(spell uint32, mode int) {
	index := rt.petActionSlot(spell)
	if index < 0 || rt.pet.guid == 0 {
		return
	}
	action := rt.pet.actions[index]
	if action&petActionAllowsAutocast == 0 {
		return
	}
	enable := mode > 0
	if mode < 0 {
		enable = action&petActionAutocastOn == 0
	}
	if enable {
		rt.pet.actions[index] = action | petActionAutocastOn
	} else {
		rt.pet.actions[index] = action &^ petActionAutocastOn
	}
	rt.send(world.ClientPetSpellAutocast, world.BuildPetAutocast(rt.pet.guid, spell, enable))
	rt.FireEvent("PET_BAR_UPDATE")
}

func registerAutocastAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	for name, mode := range map[string]int{"EnableSpellAutocast": 1, "DisableSpellAutocast": 0, "ToggleSpellAutocast": -1} {
		mode := mode
		reg(name, func(L *lua.LState) int {
			if spell := rt.spellForCall(L); spell != 0 {
				rt.setSpellAutocast(spell, mode)
			}
			return 0
		})
	}
	reg("GetSpellAutocast", func(L *lua.LState) int {
		var action uint32
		if spell := rt.spellForCall(L); spell != 0 {
			if index := rt.petActionSlot(spell); index >= 0 {
				action = rt.pet.actions[index]
			}
		}
		for _, bit := range []uint32{petActionAllowsAutocast, petActionAutocastOn} {
			if action&bit != 0 {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
		}
		return 2
	})
}
