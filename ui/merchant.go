package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// MerchantHost is what shopping asks of the connection.
type MerchantHost interface {
	BuyItem(vendor uint64, entry, slot, count uint32) error
	SellItem(vendor, item uint64, count uint32) error
	RepairAll(vendor uint64) error
	CloseVendor() error
}

type merchantState struct {
	open   bool
	vendor world.Vendor
	// canRepair is set for a vendor that repairs gear.
	canRepair bool
}

func (rt *Runtime) merchantHost() MerchantHost {
	host, _ := rt.Host.(MerchantHost)
	return host
}

// ShowMerchant opens a merchant's wares.
func (rt *Runtime) ShowMerchant(vendor world.Vendor, canRepair bool) {
	rt.CloseGossipWindow()
	rt.merchant = merchantState{open: true, vendor: vendor, canRepair: canRepair}
	for _, item := range vendor.Items {
		rt.itemTemplate(item.Entry)
	}
	rt.FireEvent("MERCHANT_SHOW")
}

// CloseMerchantWindow closes the shop.
func (rt *Runtime) CloseMerchantWindow() {
	if !rt.merchant.open {
		return
	}
	rt.merchant = merchantState{}
	rt.FireEvent("MERCHANT_CLOSED")
}

// sellToMerchant offers a bag item to the open merchant.
func (rt *Runtime) sellToMerchant(item *ItemInstance) bool {
	host := rt.merchantHost()
	if !rt.merchant.open || host == nil {
		return false
	}
	count := item.Count
	if count < 1 {
		count = 1
	}
	_ = host.SellItem(rt.merchant.vendor.GUID, item.GUID, uint32(count))
	return true
}

// registerMerchantAPI defines the calls the merchant window makes.
func registerMerchantAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	itemAt := func(index int) (world.VendorItem, bool) {
		if index < 1 || index > len(rt.merchant.vendor.Items) {
			return world.VendorItem{}, false
		}
		return rt.merchant.vendor.Items[index-1], true
	}
	reg("GetMerchantNumItems", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.merchant.vendor.Items)))
		return 1
	})
	reg("GetMerchantItemInfo", func(L *lua.LState) int {
		item, ok := itemAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		name := ""
		if template := rt.itemTemplateIfKnown(item.Entry); template != nil {
			name = template.Name
		}
		quantity := item.StackSize
		if quantity < 1 {
			quantity = 1
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(item.Price))
		L.Push(lua.LNumber(quantity))
		L.Push(lua.LNumber(item.Available))
		L.Push(lua.LTrue)
		L.Push(lua.LBool(item.ExtendedCost != 0))
		return 7
	})
	reg("GetMerchantItemLink", func(L *lua.LState) int {
		if item, ok := itemAt(L.CheckInt(1)); ok {
			L.Push(lua.LString(rt.itemLinkFor(item.Entry, 0)))
			return 1
		}
		return 0
	})
	reg("GetMerchantItemMaxStack", func(L *lua.LState) int {
		item, ok := itemAt(L.CheckInt(1))
		stack := 1
		if ok {
			if template := rt.itemTemplateIfKnown(item.Entry); template != nil && template.Stackable > 0 {
				stack = int(template.Stackable)
			}
		}
		L.Push(lua.LNumber(stack))
		return 1
	})
	reg("GetMerchantItemCostInfo", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetMerchantItemCostItem", func(L *lua.LState) int { return 0 })
	reg("BuyMerchantItem", func(L *lua.LState) int {
		index := L.CheckInt(1)
		item, ok := itemAt(index)
		host := rt.merchantHost()
		if !ok || host == nil {
			return 0
		}
		count := uint32(L.OptInt(2, 1))
		if count < 1 {
			count = 1
		}
		_ = host.BuyItem(rt.merchant.vendor.GUID, item.Entry, item.Slot, count)
		return 0
	})
	reg("CloseMerchant", func(L *lua.LState) int {
		if rt.merchant.open {
			if host := rt.merchantHost(); host != nil {
				_ = host.CloseVendor()
			}
			rt.CloseMerchantWindow()
		}
		return 0
	})
	reg("CanMerchantRepair", func(L *lua.LState) int { L.Push(lua.LBool(rt.merchant.canRepair)); return 1 })
	reg("GetRepairAllCost", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.repairAllCost()))
		L.Push(lua.LBool(rt.repairAllCost() > 0))
		return 2
	})
	reg("RepairAllItems", func(L *lua.LState) int {
		if host := rt.merchantHost(); host != nil && rt.merchant.open {
			_ = host.RepairAll(rt.merchant.vendor.GUID)
		}
		return 0
	})
	reg("GetNumBuybackItems", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetBuybackItemInfo", func(L *lua.LState) int { return 0 })
	reg("GetBuybackItemLink", func(L *lua.LState) int { return 0 })
	reg("BuybackItem", func(L *lua.LState) int { return 0 })
	reg("PickupMerchantItem", func(L *lua.LState) int { return 0 })
	reg("ShowBuybackSellCursor", func(L *lua.LState) int { return 0 })
	reg("ShowRepairCursor", func(L *lua.LState) int { return 0 })
}

// SetRepairPrice gives the merchant window the price of repairing an item: the
// entry and the durability it has lost.
func (rt *Runtime) SetRepairPrice(price func(entry uint32, lost int) int) { rt.repairPrice = price }

// repairAllCost is what repairing every worn item costs.
func (rt *Runtime) repairAllCost() int {
	if rt.repairPrice == nil {
		return 0
	}
	total := 0
	for slot := invEquipStart; slot < invEquipEnd; slot++ {
		item := rt.inventory.items[rt.inventory.slots[slot]]
		if item == nil || item.MaxDurability == 0 || item.Durability >= item.MaxDurability {
			continue
		}
		total += rt.repairPrice(item.Entry, int(item.MaxDurability-item.Durability))
	}
	return total
}
