package ui

import (

	lua "github.com/yuin/gopher-lua"
)

// Enchanting an item asks first when the enchantment replaces another, and
// when the item is not bound yet and will be.
const (
	spellEffectEnchantHeldItem  = 92
	spellEffectEnchantPrismatic = 156

	itemBondingOnEquip = 2
	itemFlagBound      = 0x1
	refundOpcode       = 0x04B4
)

type pendingEnchant struct {
	spell uint32
	item  *ItemInstance
}

// SetEnchantNames installs the names of the enchantments.
func (rt *Runtime) SetEnchantNames(names map[uint32]string) { rt.enchantNames = names }

// enchantOf is the enchantment a spell puts on an item and the slot it takes.
func (rt *Runtime) enchantOf(spell SpellInfo) (id uint32, slot int, ok bool) {
	for _, effect := range spell.Effects {
		switch effect.Effect {
		case spellEffectEnchantItem, spellEffectEnchantHeldItem:
			return uint32(effect.Misc), 0, true
		case spellEffectEnchantTemp:
			return uint32(effect.Misc), 1, true
		case spellEffectEnchantPrismatic:
			return uint32(effect.Misc), 6, true
		}
	}
	return 0, 0, false
}

// castOnItem casts the spell on the item.
func (rt *Runtime) castOnItem(spell uint32, item *ItemInstance) {
	if host, ok := rt.Host.(WorldItemSpellHost); ok {
		_ = host.CastSpellOnItem(spell, item.GUID)
	}
}

// confirmEnchant goes on with an enchantment once the questions are answered:
// it asks to bind the item when it must.
func (rt *Runtime) confirmEnchant(askReplace bool) {
	p := rt.pendingEnchant
	if p == nil {
		return
	}
	spell := rt.gameplay.Catalog[p.spell]
	id, slot, enchants := rt.enchantOf(spell)
	if enchants && askReplace {
		if old := p.item.Enchants[slot]; old != 0 && old != id {
			rt.FireEvent("REPLACE_ENCHANT", lua.LString(rt.enchantNames[old]), lua.LString(rt.enchantNames[id]))
			return
		}
	}
	if template := rt.inventory.templates[p.item.Entry]; enchants && template != nil && template.Bonding == itemBondingOnEquip && p.item.Flags&itemFlagBound == 0 && askReplace {
		rt.FireEvent("BIND_ENCHANT")
		return
	}
	rt.pendingEnchant = nil
	rt.castOnItem(p.spell, p.item)
}

func registerEnchantConfirmAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("BindEnchant", func(L *lua.LState) int {
		if p := rt.pendingEnchant; p != nil {
			rt.pendingEnchant = nil
			rt.castOnItem(p.spell, p.item)
		}
		return 0
	})
	replace := func(L *lua.LState) int {
		if rt.pendingEnchant != nil {
			// The replacement is agreed; the item may still have to be bound.
			p := rt.pendingEnchant
			if template := rt.inventory.templates[p.item.Entry]; template != nil && template.Bonding == itemBondingOnEquip && p.item.Flags&itemFlagBound == 0 {
				rt.FireEvent("BIND_ENCHANT")
				return 0
			}
			rt.pendingEnchant = nil
			rt.castOnItem(p.spell, p.item)
		}
		return 0
	}
	reg("ReplaceEnchant", replace)
	reg("ReplaceTradeEnchant", replace)
	reg("EndBoundTradeable", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: EndBoundTradeable(\"type\")")
			return 0
		}
		rt.pendingEnchant = nil
		return 0
	})
	reg("EndRefund", func(L *lua.LState) int {
		if value, ok := L.Get(1).(lua.LNumber); ok && value == 1 {
			rt.pendingEnchant = nil
		}
		return 0
	})
}
