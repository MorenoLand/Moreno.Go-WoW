package ui

import (
	"encoding/binary"

	lua "github.com/yuin/gopher-lua"
)

const (
	// questFlagSharable lets a quest be handed to the party.
	questFlagSharable = 0x08
	pushQuestToParty  = 0x019D
)

// durability alerts: the slots the paper doll warns about, in the order the
// alert call counts them, and what each can show.
var inventoryAlertSlots = [...]int{0, 2, 4, 5, 6, 7, 8, 9, 15, 16, 17, -1}

const (
	alertNone   = 0
	alertWorn   = 1
	alertBroken = 2
	// itemFieldBroken is the item flag of an item with no durability left.
	itemFieldBroken = 0x10
	// alertDurabilityLow is the durability under which the paper doll warns.
	alertDurabilityLow = 6
)

// inventoryAlert is what the paper doll shows for one of its alert places.
func (rt *Runtime) inventoryAlert(index int) int {
	if index < 0 || index >= len(inventoryAlertSlots) {
		return alertNone
	}
	slot := inventoryAlertSlots[index]
	if slot == -1 {
		// The quiver: the ammunition runs low.
		ammo := rt.field(playerAmmoField)
		if ammo != 0 && rt.itemCountOf(ammo) < 0x15 {
			return alertWorn
		}
		return alertNone
	}
	item := rt.equippedItem(slot + 1)
	if item == nil {
		return alertNone
	}
	if item.Flags&itemFieldBroken != 0 {
		return alertBroken
	}
	if item.Flags&itemFlagWrapped == 0 && item.MaxDurability != 0 {
		if item.Durability == 0 {
			return alertBroken
		}
		if item.Durability < alertDurabilityLow {
			return alertWorn
		}
	}
	return alertNone
}

// playerAmmoField is PLAYER_AMMO_ID.
const playerAmmoField = 1198

// refreshInventoryAlerts tells the interface when a worn item needs repair.
func (rt *Runtime) refreshInventoryAlerts() {
	changed := false
	for index := range rt.alerts {
		if state := rt.inventoryAlert(index); state != rt.alerts[index] {
			rt.alerts[index] = state
			changed = true
		}
	}
	if changed {
		rt.FireEvent("UPDATE_INVENTORY_ALERTS")
	}
	rt.FireEvent("UPDATE_INVENTORY_DURABILITY")
}

func registerQuestShareAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetQuestLogPushable", func(L *lua.LState) int {
		if slot, ok := rt.questByCall(L); ok && rt.quests.info[slot.ID].Flags&questFlagSharable != 0 {
			L.Push(lua.LNumber(1))
			return 1
		}
		return 0
	})
	reg("QuestLogPushQuest", func(L *lua.LState) int {
		slot, ok := rt.questByCall(L)
		if ok && rt.quests.info[slot.ID].Flags&questFlagSharable != 0 && rt.group.in {
			rt.send(pushQuestToParty, binary.LittleEndian.AppendUint32(nil, slot.ID))
		}
		return 0
	})
	reg("IsActiveQuestTrivial", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: IsActiveQuestTrivial(index)")
			return 0
		}
		level := int32(0)
		if quests := rt.greetingQuests(true); L.CheckInt(1) >= 1 && L.CheckInt(1) <= len(quests) {
			level = quests[L.CheckInt(1)-1].Level
		}
		if player := rt.unitInfo("player"); player != nil && level != -1 && int(level)+greyRange(player.Level) < player.Level {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetInventoryAlertStatus", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetInventoryAlertStatus(index)")
			return 0
		}
		L.Push(lua.LNumber(rt.inventoryAlert(L.CheckInt(1) - 1)))
		return 1
	})
}
