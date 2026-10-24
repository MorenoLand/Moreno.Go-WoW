package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// LootHost is what the loot window asks of the connection.
type LootHost interface {
	LootItemSlot(slot uint8) error
	LootMoney() error
	ReleaseLoot(guid uint64) error
}

// lootSlot is a line of the loot window: the money, or an item.
type lootSlot struct {
	coin    bool
	money   uint32
	item    world.LootItem
	cleared bool
}

type lootState struct {
	open  bool
	guid  uint64
	slots []lootSlot
}

// OpenLoot shows the loot window for what the server listed.
func (rt *Runtime) OpenLoot(loot world.LootContents) {
	if rt == nil {
		return
	}
	rt.loot = lootState{open: true, guid: loot.GUID}
	if loot.Money > 0 {
		rt.loot.slots = append(rt.loot.slots, lootSlot{coin: true, money: loot.Money})
	}
	for _, item := range loot.Items {
		rt.loot.slots = append(rt.loot.slots, lootSlot{item: item})
		rt.itemTemplate(item.Entry)
	}
	auto := 0
	if value, ok := rt.GetCVar("autoLootDefault"); ok && value == "1" {
		auto = 1
	}
	rt.FireEvent("LOOT_OPENED", lua.LNumber(auto))
}

// ClearLootSlot marks the slot with a server number taken.
func (rt *Runtime) ClearLootSlot(serverSlot uint8) {
	for index := range rt.loot.slots {
		slot := &rt.loot.slots[index]
		if !slot.coin && !slot.cleared && slot.item.Slot == serverSlot {
			slot.cleared = true
			rt.FireEvent("LOOT_SLOT_CLEARED", lua.LNumber(index+1))
			return
		}
	}
}

// ClearLootMoney marks the money taken.
func (rt *Runtime) ClearLootMoney() {
	for index := range rt.loot.slots {
		if slot := &rt.loot.slots[index]; slot.coin && !slot.cleared {
			slot.cleared = true
			rt.FireEvent("LOOT_SLOT_CLEARED", lua.LNumber(index+1))
			return
		}
	}
}

// CloseLootWindow closes the loot window, when it is showing the corpse.
func (rt *Runtime) CloseLootWindow(guid uint64) {
	if rt == nil || !rt.loot.open || (guid != 0 && rt.loot.guid != guid) {
		return
	}
	rt.loot = lootState{}
	rt.FireEvent("LOOT_CLOSED")
}

func (rt *Runtime) lootHost() LootHost {
	host, _ := rt.Host.(LootHost)
	return host
}

// moneyText spells an amount of copper as the loot window names it.
func (rt *Runtime) moneyText(copper uint32) string {
	gold, silver, rest := copper/10000, copper/100%100, copper%100
	var parts []string
	if gold > 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("GOLD_AMOUNT", "%d Gold"), gold))
	}
	if silver > 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("SILVER_AMOUNT", "%d Silver"), silver))
	}
	if rest > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("COPPER_AMOUNT", "%d Copper"), rest))
	}
	return strings.Join(parts, "\n")
}

// moneyIcon is the coin picture the loot window draws for an amount.
func moneyIcon(copper uint32) string {
	switch {
	case copper >= 10000:
		return `Interface\Icons\INV_Misc_Coin_01`
	case copper >= 100:
		return `Interface\Icons\INV_Misc_Coin_03`
	}
	return `Interface\Icons\INV_Misc_Coin_05`
}

// registerLootAPI defines the calls the loot window makes.
func registerLootAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	slotAt := func(L *lua.LState) *lootSlot {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.loot.slots) || rt.loot.slots[index-1].cleared {
			return nil
		}
		return &rt.loot.slots[index-1]
	}
	reg("GetNumLootItems", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.loot.slots)))
		return 1
	})
	reg("LootSlotIsItem", func(L *lua.LState) int {
		slot := slotAt(L)
		L.Push(lua.LBool(slot != nil && !slot.coin))
		return 1
	})
	reg("LootSlotIsCoin", func(L *lua.LState) int {
		slot := slotAt(L)
		L.Push(lua.LBool(slot != nil && slot.coin))
		return 1
	})
	reg("GetLootSlotInfo", func(L *lua.LState) int {
		slot := slotAt(L)
		if slot == nil {
			return 0
		}
		if slot.coin {
			L.Push(lua.LString(moneyIcon(slot.money)))
			L.Push(lua.LString(rt.moneyText(slot.money)))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LFalse)
			return 5
		}
		name, quality := "", 1
		if template := rt.itemTemplateIfKnown(slot.item.Entry); template != nil {
			name, quality = template.Name, int(template.Quality)
		}
		L.Push(lua.LString(rt.itemIcon(slot.item.Entry)))
		L.Push(lua.LString(name))
		L.Push(lua.LNumber(slot.item.Count))
		L.Push(lua.LNumber(quality))
		L.Push(lua.LBool(slot.item.SlotType != 0))
		return 5
	})
	reg("GetLootSlotLink", func(L *lua.LState) int {
		slot := slotAt(L)
		if slot == nil || slot.coin {
			return 0
		}
		L.Push(lua.LString(rt.itemLinkFor(slot.item.Entry, 0)))
		return 1
	})
	reg("LootSlot", func(L *lua.LState) int {
		slot := slotAt(L)
		host := rt.lootHost()
		if slot == nil || host == nil {
			return 0
		}
		if slot.coin {
			_ = host.LootMoney()
		} else {
			_ = host.LootItemSlot(slot.item.Slot)
		}
		return 0
	})
	reg("CloseLoot", func(L *lua.LState) int {
		if rt.loot.open {
			guid := rt.loot.guid
			rt.loot = lootState{}
			if host := rt.lootHost(); host != nil {
				_ = host.ReleaseLoot(guid)
			}
			rt.FireEvent("LOOT_CLOSED")
		}
		return 0
	})
	reg("IsFishingLoot", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("ConfirmLootSlot", func(L *lua.LState) int { return 0 })
}
