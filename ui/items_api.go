package ui

import (
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func numberValue[T ~int | ~uint32 | ~uint8 | ~float64](value T) lua.LValue { return lua.LNumber(value) }
func stringValue(value string) lua.LValue                                  { return lua.LString(value) }

// inventorySlotNames are the names GetInventorySlotInfo takes, and the 1-based
// slot each stands for.
var inventorySlotNames = map[string]int{
	"HEADSLOT": 1, "NECKSLOT": 2, "SHOULDERSLOT": 3, "SHIRTSLOT": 4, "CHESTSLOT": 5, "WAISTSLOT": 6, "LEGSSLOT": 7,
	"FEETSLOT": 8, "WRISTSLOT": 9, "HANDSSLOT": 10, "FINGER0SLOT": 11, "FINGER1SLOT": 12, "TRINKET0SLOT": 13,
	"TRINKET1SLOT": 14, "BACKSLOT": 15, "MAINHANDSLOT": 16, "SECONDARYHANDSLOT": 17, "RANGEDSLOT": 18, "TABARDSLOT": 19,
	"BAG0SLOT": 20, "BAG1SLOT": 21, "BAG2SLOT": 22, "BAG3SLOT": 23, "AMMOSLOT": 0,
}

var inventorySlotTextures = map[string]string{
	"HEADSLOT": "Head", "NECKSLOT": "Neck", "SHOULDERSLOT": "Shoulder", "SHIRTSLOT": "Shirt", "CHESTSLOT": "Chest",
	"WAISTSLOT": "Waist", "LEGSSLOT": "Legs", "FEETSLOT": "Feet", "WRISTSLOT": "Wrists", "HANDSSLOT": "Hands",
	"FINGER0SLOT": "Finger", "FINGER1SLOT": "Finger", "TRINKET0SLOT": "Trinket", "TRINKET1SLOT": "Trinket",
	"BACKSLOT": "Chest", "MAINHANDSLOT": "MainHand", "SECONDARYHANDSLOT": "SecondaryHand", "RANGEDSLOT": "Ranged",
	"TABARDSLOT": "Tabard", "AMMOSLOT": "Ammo", "BAG0SLOT": "Bag", "BAG1SLOT": "Bag", "BAG2SLOT": "Bag", "BAG3SLOT": "Bag",
}

var inventoryTypeNames = [...]string{"", "INVTYPE_HEAD", "INVTYPE_NECK", "INVTYPE_SHOULDER", "INVTYPE_BODY", "INVTYPE_CHEST",
	"INVTYPE_WAIST", "INVTYPE_LEGS", "INVTYPE_FEET", "INVTYPE_WRIST", "INVTYPE_HAND", "INVTYPE_FINGER", "INVTYPE_TRINKET",
	"INVTYPE_WEAPON", "INVTYPE_SHIELD", "INVTYPE_RANGED", "INVTYPE_CLOAK", "INVTYPE_2HWEAPON", "INVTYPE_BAG", "INVTYPE_TABARD",
	"INVTYPE_ROBE", "INVTYPE_WEAPONMAINHAND", "INVTYPE_WEAPONOFFHAND", "INVTYPE_HOLDABLE", "INVTYPE_AMMO", "INVTYPE_THROWN",
	"INVTYPE_RANGEDRIGHT", "INVTYPE_QUIVER", "INVTYPE_RELIC"}

func inventoryTypeName(inventoryType uint32) string {
	if int(inventoryType) < len(inventoryTypeNames) {
		return inventoryTypeNames[inventoryType]
	}
	return ""
}

// pushItemInfo pushes the GetContainerItemInfo values of an item.
func (rt *Runtime) pushItemInfo(L *lua.LState, item *ItemInstance, locked bool) int {
	if item == nil {
		return 0
	}
	template := rt.itemTemplate(item.Entry)
	quality := -1
	lootable := false
	if template != nil {
		quality = int(template.Quality)
		lootable = template.Flags&0x4 != 0
	}
	L.Push(lua.LString(rt.itemIcon(item.Entry)))
	L.Push(lua.LNumber(item.Count))
	L.Push(lua.LBool(locked))
	L.Push(lua.LNumber(quality))
	L.Push(lua.LBool(template != nil && template.PageText != 0))
	L.Push(lua.LBool(lootable))
	if link := rt.itemLinkFor(item.Entry, item.Enchant); link != "" {
		L.Push(lua.LString(link))
	} else {
		L.Push(lua.LNil)
	}
	return 7
}

// itemSellText writes a copper amount as "1g 2s 3c".
func coinText(copper uint32) string {
	gold, silver, rest := copper/10000, copper/100%100, copper%100
	var parts []string
	if gold > 0 {
		parts = append(parts, fmt.Sprintf("%d%s", gold, rtGlobalFallback("GOLD_AMOUNT_SYMBOL", "g")))
	}
	if silver > 0 || (gold > 0 && rest > 0) {
		parts = append(parts, fmt.Sprintf("%d%s", silver, rtGlobalFallback("SILVER_AMOUNT_SYMBOL", "s")))
	}
	if rest > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d%s", rest, rtGlobalFallback("COPPER_AMOUNT_SYMBOL", "c")))
	}
	return strings.Join(parts, " ")
}

func rtGlobalFallback(_ string, fallback string) string { return fallback }

// registerItemAPI defines the inventory, container and item C-API.
func registerItemAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.inventory

	reg("GetMoney", func(L *lua.LState) int { L.Push(lua.LNumber(state.money)); return 1 })
	reg("GetCoinText", func(L *lua.LState) int {
		L.Push(lua.LString(coinText(uint32(L.CheckNumber(1)))))
		return 1
	})
	reg("GetCoinTextureString", func(L *lua.LState) int {
		amount := uint32(L.CheckNumber(1))
		L.Push(lua.LString(coinText(amount)))
		return 1
	})
	reg("GetInventorySlotInfo", func(L *lua.LState) int {
		name := strings.ToUpper(L.CheckString(1))
		slot, ok := inventorySlotNames[name]
		if !ok {
			L.ArgError(1, "Invalid inventory slot name")
			return 0
		}
		texture := inventorySlotTextures[name]
		L.Push(lua.LNumber(slot))
		L.Push(lua.LString(`Interface\PaperDoll\UI-PaperDoll-Slot-` + texture))
		L.Push(lua.LFalse)
		return 3
	})
	reg("GetContainerNumSlots", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.bagSize(L.CheckInt(1))))
		return 1
	})
	reg("GetContainerItemInfo", func(L *lua.LState) int {
		return rt.pushItemInfo(L, rt.bagItem(L.CheckInt(1), L.CheckInt(2)), false)
	})
	reg("GetContainerItemLink", func(L *lua.LState) int {
		item := rt.bagItem(L.CheckInt(1), L.CheckInt(2))
		if item == nil {
			L.Push(lua.LNil)
			return 1
		}
		if link := rt.itemLinkFor(item.Entry, item.Enchant); link != "" {
			L.Push(lua.LString(link))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetContainerItemID", func(L *lua.LState) int {
		if item := rt.bagItem(L.CheckInt(1), L.CheckInt(2)); item != nil {
			L.Push(lua.LNumber(item.Entry))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetContainerItemDurability", func(L *lua.LState) int {
		item := rt.bagItem(L.CheckInt(1), L.CheckInt(2))
		if item == nil || item.MaxDurability == 0 {
			return 0
		}
		L.Push(lua.LNumber(item.Durability))
		L.Push(lua.LNumber(item.MaxDurability))
		return 2
	})
	reg("GetContainerItemCooldown", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(1))
		return 3
	})
	reg("GetContainerItemGems", func(L *lua.LState) int { return 0 })
	reg("GetContainerItemQuestInfo", func(L *lua.LState) int {
		L.Push(lua.LFalse)
		L.Push(lua.LNil)
		L.Push(lua.LFalse)
		return 3
	})
	reg("GetContainerNumFreeSlots", func(L *lua.LState) int {
		bag := L.CheckInt(1)
		free := 0
		for slot := 1; slot <= rt.bagSize(bag); slot++ {
			if rt.bagSlotGUID(bag, slot) == 0 {
				free++
			}
		}
		L.Push(lua.LNumber(free))
		L.Push(lua.LNumber(0))
		return 2
	})
	reg("GetContainerFreeSlots", func(L *lua.LState) int {
		bag := L.CheckInt(1)
		table := L.NewTable()
		for slot := 1; slot <= rt.bagSize(bag); slot++ {
			if rt.bagSlotGUID(bag, slot) == 0 {
				table.Append(lua.LNumber(slot))
			}
		}
		L.Push(table)
		return 1
	})
	reg("GetBagName", func(L *lua.LState) int {
		bag := L.CheckInt(1)
		if bag == 0 {
			L.Push(lua.LString(rtGlobalString(rt, "BACKPACK_TOOLTIP", "Backpack")))
			return 1
		}
		if bag == -1 {
			L.Push(lua.LString(rtGlobalString(rt, "BANK", "Bank")))
			return 1
		}
		if container := rt.bagContainer(bag); container != nil {
			if template := rt.itemTemplate(container.Entry); template != nil {
				L.Push(lua.LString(template.Name))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("ContainerIDToInventoryID", func(L *lua.LState) int {
		bag := L.CheckInt(1)
		if bag >= 5 {
			L.Push(lua.LNumber(bag + 63))
		} else {
			L.Push(lua.LNumber(bag + 19))
		}
		return 1
	})
	reg("BankButtonIDToInvSlotID", func(L *lua.LState) int {
		id := L.CheckInt(1)
		if luaTruthyOpt(L, 2, false) {
			L.Push(lua.LNumber(invBankBagStart + id))
		} else {
			L.Push(lua.LNumber(invBankStart + id))
		}
		return 1
	})
	reg("KeyRingButtonIDToInvSlotID", func(L *lua.LState) int {
		L.Push(lua.LNumber(invKeyStart + L.CheckInt(1)))
		return 1
	})
	reg("GetNumBankSlots", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LFalse); return 2 })

	// Equipped items.
	itemAt := func(L *lua.LState) *ItemInstance {
		if L.CheckString(1) != "player" {
			return nil
		}
		return rt.equippedItem(L.CheckInt(2))
	}
	reg("GetInventoryItemID", func(L *lua.LState) int {
		if item := itemAt(L); item != nil {
			L.Push(lua.LNumber(item.Entry))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetInventoryItemLink", func(L *lua.LState) int {
		item := itemAt(L)
		if item == nil {
			L.Push(lua.LNil)
			return 1
		}
		if link := rt.itemLinkFor(item.Entry, item.Enchant); link != "" {
			L.Push(lua.LString(link))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetInventoryItemTexture", func(L *lua.LState) int {
		if item := itemAt(L); item != nil {
			L.Push(lua.LString(rt.itemIcon(item.Entry)))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetInventoryItemCount", func(L *lua.LState) int {
		if item := itemAt(L); item != nil {
			L.Push(lua.LNumber(item.Count))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	})
	reg("GetInventoryItemQuality", func(L *lua.LState) int {
		if item := itemAt(L); item != nil {
			if template := rt.itemTemplate(item.Entry); template != nil {
				L.Push(lua.LNumber(template.Quality))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetInventoryItemDurability", func(L *lua.LState) int {
		item := itemAt(L)
		if item == nil || item.MaxDurability == 0 {
			return 0
		}
		L.Push(lua.LNumber(item.Durability))
		L.Push(lua.LNumber(item.MaxDurability))
		return 2
	})
	reg("GetInventoryItemBroken", func(L *lua.LState) int {
		item := itemAt(L)
		L.Push(lua.LBool(item != nil && item.MaxDurability > 0 && item.Durability == 0))
		return 1
	})
	reg("GetInventoryAlertStatus", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetInventoryItemCooldown", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(1))
		return 3
	})
	reg("GetInventoryItemGems", func(L *lua.LState) int { return 0 })
	reg("IsInventoryItemLocked", func(L *lua.LState) int {
		L.Push(lua.LBool(state.cursor.active && state.cursor.bag == -3 && state.cursor.slot == L.CheckInt(1)))
		return 1
	})
	reg("GetInventoryItemsForSlot", func(L *lua.LState) int { return 0 })

	// Item lookups.
	reg("GetItemInfo", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		if entry == 0 {
			L.Push(lua.LNil)
			return 1
		}
		template := rt.itemTemplate(entry)
		if template == nil {
			L.Push(lua.LNil)
			return 1
		}
		className := state.classes.Class[template.Class]
		subClassName := state.classes.SubClass[[2]uint32{template.Class, template.SubClass}]
		L.Push(lua.LString(template.Name))
		L.Push(lua.LString(rt.itemLinkFor(entry, 0)))
		L.Push(lua.LNumber(template.Quality))
		L.Push(lua.LNumber(template.ItemLevel))
		L.Push(lua.LNumber(template.RequiredLevel))
		L.Push(lua.LString(className))
		L.Push(lua.LString(subClassName))
		L.Push(lua.LNumber(template.Stackable))
		L.Push(lua.LString(inventoryTypeName(template.InventoryType)))
		L.Push(lua.LString(rt.itemIcon(entry)))
		L.Push(lua.LNumber(template.SellPrice))
		return 11
	})
	reg("GetItemCount", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		total := 0
		if entry != 0 {
			for _, item := range state.items {
				if item.Entry == entry && rt.itemIsCarried(item) {
					total += item.Count
				}
			}
		}
		L.Push(lua.LNumber(total))
		return 1
	})
	reg("IsEquippedItem", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		equipped := false
		for slot := invEquipStart; slot < invEquipEnd && entry != 0; slot++ {
			if item := state.items[state.slots[slot]]; item != nil && item.Entry == entry {
				equipped = true
			}
		}
		L.Push(lua.LBool(equipped))
		return 1
	})
	reg("IsEquippableItem", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		template := rt.itemTemplate(entry)
		L.Push(lua.LBool(template != nil && template.InventoryType != 0 && template.InventoryType != 18))
		return 1
	})
	reg("IsDressableItem", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		template := rt.itemTemplate(entry)
		L.Push(lua.LBool(template != nil && template.InventoryType != 0 && template.InventoryType != 12))
		return 1
	})
	reg("GetItemSpell", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		template := rt.itemTemplate(entry)
		if template == nil {
			return 0
		}
		for _, spell := range template.Spells {
			if spell.ID != 0 {
				info := rt.gameplay.Catalog[spell.ID]
				L.Push(lua.LString(info.Name))
				L.Push(lua.LString(info.Rank))
				return 2
			}
		}
		return 0
	})
	reg("GetItemQualityColor", func(L *lua.LState) int {
		quality := uint32(L.CheckInt(1))
		hex := itemQualityHex(quality)
		var r, g, b int
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		L.Push(lua.LNumber(float64(r) / 255))
		L.Push(lua.LNumber(float64(g) / 255))
		L.Push(lua.LNumber(float64(b) / 255))
		L.Push(lua.LString("|cff" + hex))
		return 4
	})

	// The cursor.
	pickup := func(bag, slot int, guid uint64, entry uint32) {
		state.cursor = cursorItem{active: true, bag: bag, slot: slot, guid: guid}
		rt.gameplay.Cursor.ID, rt.gameplay.Cursor.Type = entry, 128
		rt.FireEvent("ITEM_LOCK_CHANGED", numberValue(bag), numberValue(slot))
		rt.FireEvent("CURSOR_UPDATE")
	}
	drop := func() {
		if !state.cursor.active {
			return
		}
		bag, slot := state.cursor.bag, state.cursor.slot
		state.cursor = cursorItem{}
		rt.gameplay.Cursor.ID, rt.gameplay.Cursor.Type = 0, 0
		rt.FireEvent("ITEM_LOCK_CHANGED", numberValue(bag), numberValue(slot))
		rt.FireEvent("CURSOR_UPDATE")
	}
	rt.clearItemCursor = drop
	reg("PickupContainerItem", func(L *lua.LState) int {
		bag, slot := L.CheckInt(1), L.CheckInt(2)
		if state.cursor.active {
			rt.moveCursorItem(L, bag, slot)
			drop()
			return 0
		}
		if item := rt.bagItem(bag, slot); item != nil {
			pickup(bag, slot, item.GUID, item.Entry)
		}
		return 0
	})
	reg("PickupInventoryItem", func(L *lua.LState) int {
		slot := L.CheckInt(1)
		if state.cursor.active {
			rt.moveCursorItem(L, -3, slot)
			drop()
			return 0
		}
		if item := rt.equippedItem(slot); item != nil {
			pickup(-3, slot, item.GUID, item.Entry)
		}
		return 0
	})
	reg("PickupBagFromSlot", func(L *lua.LState) int {
		slot := L.CheckInt(1)
		if state.cursor.active {
			rt.moveCursorItem(L, -3, slot)
			drop()
			return 0
		}
		if item := rt.equippedItem(slot); item != nil {
			pickup(-3, slot, item.GUID, item.Entry)
		}
		return 0
	})
	reg("PutItemInBackpack", func(L *lua.LState) int {
		if state.cursor.active {
			rt.autoStoreCursor(L, 0)
			drop()
		}
		return 0
	})
	reg("PutItemInBag", func(L *lua.LState) int {
		if state.cursor.active {
			slot := L.CheckInt(1)
			if slot >= invBagStart+1 && slot <= invBagEnd {
				rt.autoStoreCursor(L, slot-invBagStart)
			}
			drop()
		}
		return 0
	})
	reg("AutoEquipCursorItem", func(L *lua.LState) int {
		if state.cursor.active {
			if bag, slot, ok := rt.serverBagAndSlotOfCursor(); ok {
				rt.withItemHost(L, func(host WorldItemHost) error { return host.AutoEquipItem(bag, slot) })
			}
			drop()
		}
		return 0
	})
	reg("DeleteCursorItem", func(L *lua.LState) int {
		if state.cursor.active {
			if bag, slot, ok := rt.serverBagAndSlotOfCursor(); ok {
				rt.withItemHost(L, func(host WorldItemHost) error { return host.DestroyItem(bag, slot, 0) })
			}
			drop()
		}
		return 0
	})
	reg("SplitContainerItem", func(L *lua.LState) int {
		bag, slot, count := L.CheckInt(1), L.CheckInt(2), L.CheckInt(3)
		if item := rt.bagItem(bag, slot); item != nil && count > 0 && count < item.Count {
			state.splitCount = count
			pickup(bag, slot, item.GUID, item.Entry)
		}
		return 0
	})
	reg("UseContainerItem", func(L *lua.LState) int {
		bag, slot := L.CheckInt(1), L.CheckInt(2)
		item := rt.bagItem(bag, slot)
		if item == nil {
			return 0
		}
		srcBag, srcSlot, ok := rt.serverBagAndSlot(bag, slot)
		if !ok {
			return 0
		}
		rt.useItem(L, item, srcBag, srcSlot)
		return 0
	})
	reg("UseInventoryItem", func(L *lua.LState) int {
		slot := L.CheckInt(1)
		if item := rt.equippedItem(slot); item != nil {
			rt.useItem(L, item, world255(), uint8(serverSlotOfInventory(slot)))
		}
		return 0
	})
	reg("EquipItemByName", func(L *lua.LState) int {
		entry := rt.itemArgEntry(L, 1)
		for _, bag := range []int{0, 1, 2, 3, 4} {
			for slot := 1; slot <= rt.bagSize(bag); slot++ {
				if item := rt.bagItem(bag, slot); item != nil && item.Entry == entry {
					if srcBag, srcSlot, ok := rt.serverBagAndSlot(bag, slot); ok {
						rt.withItemHost(L, func(host WorldItemHost) error { return host.AutoEquipItem(srcBag, srcSlot) })
					}
					return 0
				}
			}
		}
		return 0
	})
	reg("CursorCanGoInSlot", func(L *lua.LState) int {
		L.Push(lua.LBool(state.cursor.active))
		return 1
	})
	reg("IsBagOpen", func(L *lua.LState) int {
		frame := rt.widgets[fmt.Sprintf("ContainerFrame%d", L.CheckInt(1)+1)]
		L.Push(lua.LBool(frame != nil && frame.shown))
		return 1
	})
	reg("SetBagPortraitTexture", func(L *lua.LState) int {
		texture := rt.resolveTextureArg(L, 1)
		if texture == nil {
			return 0
		}
		bag := L.CheckInt(2)
		path := `InterfaceButtonsButton-Backpack-Up`
		if bag != 0 {
			if container := rt.bagContainer(bag); container != nil {
				path = rt.itemIcon(container.Entry)
			}
		}
		texture.textureFile = path
		texture.texCoordL, texture.texCoordR, texture.texCoordT, texture.texCoordB = 0, 1, 0, 1
		return 0
	})
	_ = time.Now
}

func world255() uint8 { return 0xFF }

func rtGlobalString(rt *Runtime, name, fallback string) string {
	return rt.globalString(name, fallback)
}

// itemArgEntry resolves an item argument: an entry number, an item link, or a
// name of an item that is in the bags.
func (rt *Runtime) itemArgEntry(L *lua.LState, index int) uint32 {
	switch value := L.Get(index).(type) {
	case lua.LNumber:
		return uint32(value)
	case lua.LString:
		text := string(value)
		if entry := itemEntryFromLink(text); entry != 0 {
			return entry
		}
		var number uint32
		if _, err := fmt.Sscanf(text, "%d", &number); err == nil {
			return number
		}
		for _, item := range rt.inventory.items {
			if template := rt.inventory.templates[item.Entry]; template != nil && strings.EqualFold(template.Name, text) {
				return item.Entry
			}
		}
	}
	return 0
}

// itemIsCarried reports whether an item is on the character or in their bags.
func (rt *Runtime) itemIsCarried(item *ItemInstance) bool {
	state := &rt.inventory
	for slot := 0; slot < invBuybackStart; slot++ {
		if state.slots[slot] == item.GUID {
			return true
		}
	}
	for _, container := range []int{invBagStart, invBagStart + 1, invBagStart + 2, invBagStart + 3, invBankBagStart, invBankBagStart + 1,
		invBankBagStart + 2, invBankBagStart + 3, invBankBagStart + 4, invBankBagStart + 5, invBankBagStart + 6} {
		if bag := state.items[state.slots[container]]; bag != nil {
			for _, guid := range bag.Slots {
				if guid == item.GUID {
					return true
				}
			}
		}
	}
	return false
}

func (rt *Runtime) withItemHost(L *lua.LState, send func(WorldItemHost) error) {
	host, ok := rt.Host.(WorldItemHost)
	if !ok {
		L.RaiseError("the world item transport is unavailable")
		return
	}
	if err := send(host); err != nil {
		L.RaiseError("%v", err)
	}
}

// serverBagAndSlotOfCursor is the server (bag, slot) the item on the cursor
// was picked up from.
func (rt *Runtime) serverBagAndSlotOfCursor() (uint8, uint8, bool) {
	cursor := rt.inventory.cursor
	if cursor.bag == -3 {
		return 0xFF, uint8(serverSlotOfInventory(cursor.slot)), true
	}
	return rt.serverBagAndSlot(cursor.bag, cursor.slot)
}

// moveCursorItem drops the item on the cursor into a bag square (bag >= -2) or
// an equipment slot (bag == -3).
func (rt *Runtime) moveCursorItem(L *lua.LState, bag, slot int) {
	srcBag, srcSlot, ok := rt.serverBagAndSlotOfCursor()
	if !ok {
		return
	}
	var dstBag, dstSlot uint8
	if bag == -3 {
		dstBag, dstSlot = 0xFF, uint8(serverSlotOfInventory(slot))
	} else if dstBag, dstSlot, ok = rt.serverBagAndSlot(bag, slot); !ok {
		return
	}
	if count := rt.inventory.splitCount; count > 0 {
		rt.inventory.splitCount = 0
		rt.withItemHost(L, func(host WorldItemHost) error { return host.SplitItem(srcBag, srcSlot, dstBag, dstSlot, uint8(count)) })
		return
	}
	rt.withItemHost(L, func(host WorldItemHost) error { return host.SwapItem(dstBag, dstSlot, srcBag, srcSlot) })
}

// autoStoreCursor stores the cursor item in a bag the server picks the slot of.
func (rt *Runtime) autoStoreCursor(L *lua.LState, dstBagID int) {
	srcBag, srcSlot, ok := rt.serverBagAndSlotOfCursor()
	if !ok {
		return
	}
	dstBag := uint8(0xFF)
	if dstBagID > 0 {
		dstBag = uint8(invBagStart + dstBagID - 1)
	}
	rt.withItemHost(L, func(host WorldItemHost) error { return host.AutoStoreBagItem(srcBag, srcSlot, dstBag) })
}

// useItem uses an item through the spell it carries, or equips it when it is
// wearable and the bags are open.
func (rt *Runtime) useItem(L *lua.LState, item *ItemInstance, bag, slot uint8) {
	template := rt.itemTemplate(item.Entry)
	if template == nil {
		return
	}
	for _, spell := range template.Spells {
		if spell.ID != 0 && spell.Trigger == 0 {
			rt.withItemHost(L, func(host WorldItemHost) error {
				return host.UseItem(bag, slot, 1, spell.ID, item.GUID, 0, 0)
			})
			return
		}
	}
	if template.InventoryType != 0 && bag != 0xFF || (bag == 0xFF && slot >= invPackStart) {
		rt.withItemHost(L, func(host WorldItemHost) error { return host.AutoEquipItem(bag, slot) })
	}
}
