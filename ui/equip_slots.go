package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// inventorySlotMasks say, for each inventory type, the equipment slots it can
// go in (a bit for each slot).
var inventorySlotMasks = [...]uint32{
	0x0, 0x1, 0x2, 0x4, 0x8, 0x10, 0x20, 0x40, 0x80, 0x100, 0x200, 0xc00, 0x3000, 0x18000, 0x10000, 0x20000,
	0x4000, 0x18000, 0x780000, 0x40000, 0x10, 0x18000, 0x18000, 0x10000, 0x0, 0x20000, 0x20000, 0x0, 0x20000,
}

// The inventory types the slot rules single out.
const (
	invTypeRanged     = 15
	invTypeTwoHand    = 17
	invTypeMainHand   = 21
	invTypeOffHand    = 22
	invTypeHeldOff    = 23
	invTypeRelic      = 28
	invTypeThrown     = 25
	itemFlagNoOffhand = 0x40000000
	itemSubClassFist  = 13
	itemClassAmmo     = 6
	spellEffectDual   = 40
)

// Where an item may be found, as the equipment manager names places.
const (
	locationPlayer = 0x100000
	locationBags   = 0x200000
	locationBank   = 0x400000
)

// SetRelicClasses says which classes carry a relic in the ranged slot.
func (rt *Runtime) SetRelicClasses(classes map[uint32]bool) { rt.relicClasses = classes }

// canDualWield is whether the character has learned to wield two weapons.
func (rt *Runtime) canDualWield() bool {
	for id := range rt.gameplay.Known {
		for _, effect := range rt.gameplay.Catalog[id].Effects {
			if effect.Effect == spellEffectDual {
				return true
			}
		}
	}
	return false
}

// slotAllows is whether a spell the character knows lets the item go in the slot.
func (rt *Runtime) spellAllowsSlot(local ItemLocal, slot uint32) bool {
	for id := range rt.gameplay.Known {
		spell := rt.gameplay.Catalog[id]
		if spell.EquipClass != int32(local.Class) || spell.EquipSubClassMask&(1<<(local.SubClass&31)) == 0 {
			continue
		}
		for kind, mask := range inventorySlotMasks {
			if spell.EquipInvMask&(1<<uint(kind)) != 0 && mask&(1<<slot) != 0 {
				return true
			}
		}
	}
	return false
}

// itemFitsSlot is whether the kind of an item goes in an equipment slot.
func (rt *Runtime) itemFitsSlot(entry uint32, slot uint32, usesRelic bool) bool {
	local, ok := rt.inventory.local[entry]
	template := rt.itemTemplate(entry)
	if !ok || template == nil {
		return false
	}
	kind := local.InventoryType
	if slot == 0xFFFFFFFF {
		return local.Class == itemClassAmmo
	}
	if slot == slotRanged {
		if usesRelic != (kind == invTypeRelic) {
			return false
		}
	} else if (slot == slotMainHand || slot == slotOffHand) && local.Class == itemClassWeapon {
		special := slot == slotOffHand && (kind == invTypeTwoHand || kind == invTypeMainHand) ||
			slot == slotMainHand && kind == invTypeOffHand
		if special {
			if template.Flags&itemFlagNoOffhand != 0 || local.SubClass == itemSubClassFist {
				return false
			}
			return rt.spellAllowsSlot(local, slot)
		}
	}
	if slot > 22 {
		return true
	}
	return kind < uint32(len(inventorySlotMasks)) && inventorySlotMasks[kind]&(1<<slot) != 0
}

// equipUsable is whether the character is able to use an item in a slot: level,
// proficiency, class, race and standing.
func (rt *Runtime) equipUsable(entry uint32, slot uint32) bool {
	template := rt.itemTemplate(entry)
	player := rt.unitInfo("player")
	if template == nil || player == nil {
		return false
	}
	if template.RequiredLevel > 1 && uint32(player.Level) < template.RequiredLevel {
		return false
	}
	if mask := rt.bind.proficiency[template.Class]; template.Class <= 0x10 && mask != 0 && mask&(1<<(template.SubClass&31)) == 0 {
		return false
	}
	if slot == slotOffHand && (template.InventoryType == invTypeOffHand || template.InventoryType == 13) && !rt.canDualWield() {
		return false
	}
	if mask := uint32(template.AllowableClass); mask != 0 && mask != 0xFFFFFFFF && mask&(1<<((uint(player.ClassID)-1)&31)) == 0 {
		return false
	}
	if mask := uint32(template.AllowableRace); mask != 0 && mask != 0xFFFFFFFF && mask&(1<<((uint(player.RaceID)-1)&31)) == 0 {
		return false
	}
	if template.RequiredFaction != 0 {
		for index := range rt.factions.entries {
			faction := &rt.factions.entries[index]
			if faction.ID == template.RequiredFaction && int(template.RequiredFactionRank) < len(reputationRankStanding) &&
				faction.Total() < reputationRankStanding[template.RequiredFactionRank] {
				return false
			}
		}
	}
	return true
}

// equipSlotOf names the equipment slot a call means: a slot number or the name
// of the slot.
func (rt *Runtime) equipSlotOf(L *lua.LState) (uint32, bool) {
	switch value := L.Get(1).(type) {
	case lua.LNumber:
		slot := int(value) - 1
		return uint32(slot), slot >= 0 && slot < 0x13
	case lua.LString:
		if slot, ok := inventorySlotNames[strings.ToUpper(string(value))]; ok && slot >= 1 && slot <= 0x13 {
			return uint32(slot - 1), true
		}
	}
	return 0, false
}

func registerEquipSlotsAPI(rt *Runtime) {
	rt.L.SetGlobal("GetInventoryItemsForSlot", rt.L.NewFunction(func(L *lua.LState) int {
		slot, ok := rt.equipSlotOf(L)
		if !ok {
			if L.Get(1).Type() == lua.LTNumber {
				return 0
			}
			L.RaiseError("Usage: GetInventoryItemsForSlot(slot [, returnTable])")
			return 0
		}
		result, isTable := L.Get(2).(*lua.LTable)
		if !isTable {
			result = L.NewTable()
		}
		player := rt.unitInfo("player")
		if player == nil {
			return 0
		}
		usesRelic := rt.relicClasses[uint32(player.ClassID)]
		add := func(location int, item *ItemInstance) {
			if item != nil && rt.itemFitsSlot(item.Entry, slot, usesRelic) && rt.equipUsable(item.Entry, slot) {
				result.RawSetInt(location, lua.LNumber(item.Entry))
			}
		}
		for index := 0; index < 0x13; index++ {
			add(locationPlayer+index+1, rt.equippedItem(index+1))
		}
		for index := 0; index < backpackSlots; index++ {
			add(locationPlayer+locationBags+index+1, rt.bagItem(0, index+1))
		}
		for bag := 1; bag <= 4; bag++ {
			for index := 0; index < rt.bagSize(bag); index++ {
				add(locationPlayer+locationBags+bag<<8+index+1, rt.bagItem(bag, index+1))
			}
		}
		if rt.bank.open {
			for index := 0; index < bankSlots; index++ {
				add(locationBank+index+1, rt.bagItem(-1, index+1))
			}
			for bag := 5; bag <= 11; bag++ {
				for index := 0; index < rt.bagSize(bag); index++ {
					add(locationBank+locationBags+(bag-4)<<8+index+1, rt.bagItem(bag, index+1))
				}
			}
		}
		L.Push(result)
		return 1
	}))
}
