package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// ExtendedCost is a price in honor, arena points and items.
type ExtendedCost struct {
	Honor, Arena  uint32
	// Bracket and Rating are the arena bracket and the rating a team needs.
	Bracket, Rating uint32
	Items, Counts [5]uint32
}

// SetExtendedCosts installs the prices of ItemExtendedCost.dbc.
func (rt *Runtime) SetExtendedCosts(costs map[uint32]ExtendedCost) { rt.extendedCosts = costs }

const buybackOpcode = 0x0290

// buybackSlots are the inventory slots that hold the items sold, in order.
func (rt *Runtime) buybackSlots() []int {
	var slots []int
	for slot := invBuybackStart; slot < invBuybackEnd; slot++ {
		if rt.inventory.slots[slot] != 0 {
			slots = append(slots, slot)
		}
	}
	return slots
}

// repairItem asks the merchant to repair one item.
func (rt *Runtime) repairItem(item *ItemInstance) {
	if !rt.merchant.open || item == nil {
		return
	}
	rt.send(world.ClientRepairItem, world.BuildRepairItem(rt.merchant.vendor.GUID, item.GUID))
}

func registerMerchantExtraAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	vendorItem := func(index int) (world.VendorItem, bool) {
		if !rt.merchant.open || index < 1 || index > len(rt.merchant.vendor.Items) {
			return world.VendorItem{}, false
		}
		return rt.merchant.vendor.Items[index-1], true
	}
	reg("GetMerchantItemCostInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetMerchantItemCostInfo(index)")
			return 0
		}
		item, ok := vendorItem(L.CheckInt(1))
		cost, found := rt.extendedCosts[item.ExtendedCost]
		if !ok || item.ExtendedCost == 0 || !found {
			return pushAll(L, lua.LNumber(0), lua.LNumber(0), lua.LNumber(0))
		}
		items := 0
		for _, entry := range cost.Items {
			if entry != 0 {
				items++
			}
		}
		return pushAll(L, lua.LNumber(cost.Honor), lua.LNumber(cost.Arena), lua.LNumber(items))
	})
	reg("GetMerchantItemCostItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetMerchantItemCostItem(index, itemIndex)")
			return 0
		}
		item, ok := vendorItem(L.CheckInt(1))
		cost, found := rt.extendedCosts[item.ExtendedCost]
		if ok && found && item.ExtendedCost != 0 {
			seen := 0
			for i, entry := range cost.Items {
				if entry == 0 {
					continue
				}
				seen++
				if seen == L.CheckInt(2) {
					return pushAll(L, lua.LString(rt.itemIcon(entry)), lua.LNumber(cost.Counts[i]), lua.LString(rt.itemLinkFor(entry, 0)))
				}
			}
		}
		return pushAll(L, lua.LNil, lua.LNumber(0))
	})

	buybackEntry := func(index int) (int, *ItemInstance) {
		slots := rt.buybackSlots()
		if index < 1 || index > len(slots) {
			return 0, nil
		}
		return slots[index-1], rt.inventory.items[rt.inventory.slots[slots[index-1]]]
	}
	reg("GetNumBuybackItems", func(L *lua.LState) int {
		if !rt.merchant.open {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(len(rt.buybackSlots())))
		return 1
	})
	reg("GetBuybackItemInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBuybackItemInfo(slot)")
			return 0
		}
		slot, item := buybackEntry(L.CheckInt(1))
		if !rt.merchant.open || item == nil {
			return pushAll(L, lua.LNil, lua.LNil, lua.LNumber(0), lua.LNumber(1), lua.LNumber(0), lua.LNumber(1))
		}
		template := rt.itemTemplateIfKnown(item.Entry)
		if template == nil {
			return pushAll(L, lua.LNil, lua.LNil, lua.LNumber(0), lua.LNumber(1), lua.LNumber(0), lua.LNumber(1))
		}
		price := rt.field(1201 + slot - invBuybackStart)
		return pushAll(L, lua.LString(template.Name), lua.LString(rt.itemIcon(item.Entry)), lua.LNumber(price), lua.LNumber(item.Count), lua.LNumber(0), lua.LNumber(1))
	})
	reg("GetBuybackItemLink", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBuybackItemLink(slot)")
			return 0
		}
		if _, item := buybackEntry(L.CheckInt(1)); item != nil && rt.merchant.open {
			L.Push(lua.LString(rt.itemLinkFor(item.Entry, 0)))
			return 1
		}
		return 0
	})
	reg("BuybackItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: BuybackItem(slot)")
			return 0
		}
		if slot, item := buybackEntry(L.CheckInt(1)); item != nil && rt.merchant.open {
			body := binary.LittleEndian.AppendUint64(nil, rt.merchant.vendor.GUID)
			body = binary.LittleEndian.AppendUint32(body, uint32(slot))
			rt.send(buybackOpcode, body)
		}
		return 0
	})
	reg("PickupMerchantItem", func(L *lua.LState) int {
		if !rt.merchant.open {
			return 0
		}
		state := &rt.inventory
		if state.cursor.active {
			// An item on the cursor dropped on the window is sold.
			if item := state.items[state.cursor.guid]; item != nil && rt.sellToMerchant(item) {
				if rt.clearItemCursor != nil {
					rt.clearItemCursor()
				}
			}
		}
		return 0
	})
	reg("ShowRepairCursor", func(L *lua.LState) int {
		if rt.merchant.open && rt.merchant.canRepair {
			rt.merchant.repairMode = true
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
	reg("HideRepairCursor", func(L *lua.LState) int {
		if rt.merchant.repairMode {
			rt.merchant.repairMode = false
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
	reg("InRepairMode", func(L *lua.LState) int {
		if rt.merchant.repairMode {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
}
