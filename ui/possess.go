package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// The possess bar: the buttons shown while the character controls another
// unit through a possess, charm or vehicle aura.
const (
	auraModPossess    = 2
	auraModCharm      = 6
	auraModPossessPet = 128
	auraControlVehicle = 236
	// possessCancelIcon is the SpellIcon row of the cancel button.
	possessCancelIcon = 693
	// spellAttributeCantCancel keeps an aura from being cancelled.
	spellAttributeCantCancel = 0x80000000
)

// SetSpellIcons installs the icon paths of SpellIcon.dbc by row.
func (rt *Runtime) SetSpellIcons(icons map[uint32]string) { rt.spellIcons = icons }

// possessSpell is the spell of the aura through which the character controls
// something, or 0.
func (rt *Runtime) possessSpell() uint32 {
	info := rt.unitInfo("player")
	if info == nil {
		return 0
	}
	for _, aura := range info.Auras {
		if aura.Harmful || aura.SpellID == 0 {
			continue
		}
		spell, ok := rt.gameplay.Catalog[aura.SpellID]
		if !ok {
			continue
		}
		for _, effect := range spell.Effects {
			switch effect.Aura {
			case auraModPossess, auraModCharm, auraModPossessPet:
				return aura.SpellID
			case auraControlVehicle:
				if rt.playerVehicle() != 0 {
					return aura.SpellID
				}
			}
		}
	}
	return 0
}

// updatePossessBar tells the interface when the controlling aura changed.
func (rt *Runtime) updatePossessBar() {
	spell := rt.possessSpell()
	if spell != rt.possessSpellID {
		rt.possessSpellID = spell
		rt.FireEvent("UPDATE_POSSESS_BAR")
	}
}

func registerPossessAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("IsPossessBarVisible", func(L *lua.LState) int {
		visible := false
		if id := rt.possessSpellID; id != 0 {
			visible = true
			for _, effect := range rt.gameplay.Catalog[id].Effects {
				if effect.Aura == auraControlVehicle {
					visible = false
				}
			}
		}
		L.Push(lua.LBool(visible))
		return 1
	})
	reg("GetPossessInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetPossessInfo(index)")
		}
		spell, ok := rt.gameplay.Catalog[rt.possessSpellID]
		if rt.possessSpellID != 0 && ok {
			switch L.CheckInt(1) {
			case 1:
				L.Push(lua.LString(spell.Icon))
				L.Push(lua.LString(spell.Name))
				L.Push(lua.LTrue)
				return 3
			case 2:
				if icon, found := rt.spellIcons[possessCancelIcon]; found {
					L.Push(lua.LString(icon))
				} else {
					L.Push(lua.LNil)
				}
				L.Push(lua.LString(spell.Name))
				L.Push(lua.LBool(spell.Attributes&spellAttributeCantCancel == 0))
				return 3
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 3
	})
}
