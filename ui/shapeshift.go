package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// Spell auras that make a form: shapeshifting and stealth.
const (
	auraModShapeshift = 36
	auraModStealth    = 16
)

// shapeshiftForms lists the spells of the forms the character knows, in the
// order of the spellbook: druid forms, warrior stances, death knight
// presences, shadowform, ghost wolf and a rogue's stealth.
func (rt *Runtime) shapeshiftForms() []uint32 {
	var forms []uint32
	seen := make(map[uint32]bool)
	_, class := rt.playerLevelClass()
	for _, id := range rt.gameplay.Book {
		info, ok := rt.gameplay.Catalog[id]
		if !ok || !rt.gameplay.Known[id] || seen[id] {
			continue
		}
		for _, effect := range info.Effects {
			if effect.Effect == 6 && (effect.Aura == auraModShapeshift || effect.Aura == auraModStealth && class == classRogue) {
				seen[id] = true
				forms = append(forms, id)
				break
			}
		}
	}
	return forms
}

// shapeshiftForm is the index of the form the character is in, 0 for none.
func (rt *Runtime) shapeshiftForm() int {
	player := rt.unitInfo("player")
	if player == nil {
		return 0
	}
	for index, form := range rt.shapeshiftForms() {
		for _, aura := range player.Auras {
			if aura.SpellID == form {
				return index + 1
			}
		}
	}
	return 0
}

// registerShapeshiftAPI defines the calls of the stance bar.
func registerShapeshiftAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	reg("GetNumShapeshiftForms", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.shapeshiftForms()))) })
	reg("GetShapeshiftForm", func(L *lua.LState) int { return push(L, lua.LNumber(rt.shapeshiftForm())) })
	reg("GetShapeshiftFormInfo", func(L *lua.LState) int {
		forms := rt.shapeshiftForms()
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(forms) {
			return 0
		}
		info := rt.gameplay.Catalog[forms[index]]
		player := rt.unitInfo("player")
		castable := player != nil && !player.Dead && player.Power >= info.Cost
		return push(L, lua.LString(info.Icon), lua.LString(info.Name), luaFlag(rt.shapeshiftForm() == index+1), luaFlag(castable))
	})
	reg("GetShapeshiftFormCooldown", func(L *lua.LState) int {
		forms := rt.shapeshiftForms()
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(forms) {
			if cooldown, ok := rt.gameplay.Cooldowns[forms[index]]; ok && !cooldown.Disabled {
				return push(L, lua.LNumber(cooldown.Start), lua.LNumber(cooldown.Duration), lua.LNumber(1))
			}
		}
		return push(L, lua.LNumber(0), lua.LNumber(0), lua.LNumber(1))
	})
	reg("CastShapeshiftForm", func(L *lua.LState) int {
		forms := rt.shapeshiftForms()
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(forms) {
			if host, ok := rt.Host.(interface{ CastSpell(uint32, uint64) error }); ok {
				_ = host.CastSpell(forms[index], 0)
			}
		}
		return 0
	})
	reg("CancelShapeshiftForm", func(L *lua.LState) int {
		forms := rt.shapeshiftForms()
		if form := rt.shapeshiftForm(); form > 0 {
			if host, ok := rt.Host.(interface{ CancelAura(uint32) error }); ok {
				_ = host.CancelAura(forms[form-1])
			}
		}
		return 0
	})
	reg("UnitHasRelicSlot", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		if info == nil {
			return push(L, lua.LNil)
		}
		switch info.ClassID {
		case classPaladin, classShaman, classDruid, 6:
			return push(L, lua.LNumber(1))
		}
		return push(L, lua.LNil)
	})
}
