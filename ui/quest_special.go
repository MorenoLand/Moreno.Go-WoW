package ui

import (
	lua "github.com/yuin/gopher-lua"
)

// questFlagItemDrops is the quest flag that makes the items objectives drop
// the quest's special items.
const questFlagItemDrops = 0x20000

// itemUseSpell is the spell an item casts when it is used, and 0 when it has
// none.
func (rt *Runtime) itemUseSpell(entry uint32) uint32 {
	template := rt.itemTemplate(entry)
	if template == nil {
		return 0
	}
	for _, spell := range template.Spells {
		if spell.ID != 0 && spell.Trigger == 0 {
			return spell.ID
		}
	}
	return 0
}

// carriedUsableItem is the item of an entry the character carries and can use.
func (rt *Runtime) carriedUsableItem(entry uint32) *ItemInstance {
	if entry == 0 || rt.itemUseSpell(entry) == 0 {
		return nil
	}
	for _, item := range rt.inventory.items {
		if item.Entry == entry && rt.itemIsCarried(item) {
			return item
		}
	}
	return nil
}

// questSpecialItem is the item a quest of the log gives to use: the one it
// handed out, or one of the objectives' drops.
func (rt *Runtime) questSpecialItem(index int) *ItemInstance {
	rows := rt.questRows()
	if index < 1 || index > len(rows) || rows[index-1].slot < 0 {
		return nil
	}
	info := rt.quests.info[rt.quests.slots[rows[index-1].slot].ID]
	if info.StartItem == 0 && info.Flags&questFlagItemDrops == 0 {
		return nil
	}
	if item := rt.carriedUsableItem(info.StartItem); item != nil {
		return item
	}
	if info.Flags&questFlagItemDrops != 0 {
		for _, entry := range info.ItemDrop {
			if item := rt.carriedUsableItem(entry); item != nil {
				return item
			}
		}
	}
	return nil
}

func registerQuestSpecialAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetQuestLogSpecialItemInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestLogSpecialItemInfo(index)")
		}
		item := rt.questSpecialItem(L.CheckInt(1))
		if item == nil {
			return 0
		}
		charges := 0
		if template := rt.itemTemplate(item.Entry); template != nil {
			for _, spell := range template.Spells {
				if spell.ID != 0 && spell.Trigger == 0 {
					charges = int(spell.Charges)
					break
				}
			}
		}
		L.Push(lua.LString(rt.itemLinkFor(item.Entry, 0)))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(charges))
		return 3
	})
	reg("GetQuestLogSpecialItemCooldown", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestLogSpecialItemCooldown(index)")
		}
		if item := rt.questSpecialItem(L.CheckInt(1)); item != nil {
			return rt.pushCooldown(L, rt.itemUseSpell(item.Entry))
		}
		return 0
	})
	reg("UseQuestLogSpecialItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: UseQuestLogSpecialItem(index)")
		}
		if item := rt.questSpecialItem(L.CheckInt(1)); item != nil {
			if bag, slot, ok := rt.itemLocation(item.GUID); ok {
				rt.useItem(L, item, bag, slot)
			}
		}
		return 0
	})
	reg("IsQuestLogSpecialItemInRange", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: IsQuestLogSpecialItemInRange(index)")
		}
		L.Push(lua.LNil)
		return 1
	})
}
