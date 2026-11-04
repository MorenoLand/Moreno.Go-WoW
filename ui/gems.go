package ui

import lua "github.com/yuin/gopher-lua"

// SetEnchantGems gives the gem item each socket enchantment comes from.
func (rt *Runtime) SetEnchantGems(gems map[uint32]uint32) { rt.enchantGems = gems }

// pushItemGems pushes the three gems set in an item, nil for an empty or an
// unknown socket.
func (rt *Runtime) pushItemGems(L *lua.LState, item *ItemInstance) int {
	template := rt.itemTemplate(item.Entry)
	for socket := 2; socket < 5; socket++ {
		enchant := item.Enchants[socket]
		if template != nil && template.Flags&templateNoEnchantBind != 0 {
			enchant = 0
		}
		if gem := rt.enchantGems[enchant]; enchant != 0 && gem != 0 {
			L.Push(lua.LNumber(gem))
		} else {
			L.Push(lua.LNil)
		}
	}
	return 3
}

func registerGemsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetContainerItemGems", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetContainerItemGems(bag, slot)")
			return 0
		}
		if item := rt.bagItem(L.CheckInt(1), L.CheckInt(2)); item != nil {
			return rt.pushItemGems(L, item)
		}
		return 3
	})
	reg("GetInventoryItemGems", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetInventoryItemGems(slot)")
			return 0
		}
		if item := rt.equippedItem(L.CheckInt(1)); item != nil {
			return rt.pushItemGems(L, item)
		}
		return 0
	})
}
