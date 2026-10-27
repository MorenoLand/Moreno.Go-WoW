package ui

import (
	"encoding/binary"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// actionTypeItem is the type of an action button that holds an item.
const actionTypeItem = 128

// itemCountOf is how many of an item the player carries, on or in bags.
func (rt *Runtime) itemCountOf(entry uint32) int {
	total := 0
	for _, item := range rt.inventory.items {
		if item.Entry == entry && rt.itemIsCarried(item) {
			total += item.Count
		}
	}
	return total
}

// itemTemplateIfKnown is the template of an entry the server has described,
// asking for it when it has not.
func (rt *Runtime) itemTemplateIfKnown(entry uint32) *world.ItemTemplate {
	return rt.itemTemplate(entry)
}

// itemIsConsumable reports an item the player uses up: a consumable, or one
// whose spell has charges.
func (rt *Runtime) itemIsConsumable(entry uint32) bool {
	template := rt.itemTemplate(entry)
	if template == nil {
		return false
	}
	if template.Class == 0 {
		return true
	}
	for _, spell := range template.Spells {
		if spell.ID != 0 && spell.Charges != 0 {
			return true
		}
	}
	return false
}

func (rt *Runtime) itemIsEquipped(entry uint32) bool {
	for slot := invEquipStart; slot < invEquipEnd; slot++ {
		if item := rt.inventory.items[rt.inventory.slots[slot]]; item != nil && item.Entry == entry {
			return true
		}
	}
	return false
}

// useActionItem uses the first carried copy of an item an action button holds.
func (rt *Runtime) useActionItem(L *lua.LState, entry uint32) {
	for slot := invEquipStart; slot < invEquipEnd; slot++ {
		if item := rt.inventory.items[rt.inventory.slots[slot]]; item != nil && item.Entry == entry {
			rt.useItem(L, item, world255(), uint8(slot))
			return
		}
	}
	for bag := 0; bag <= 4; bag++ {
		for slot := 1; slot <= rt.bagSize(bag); slot++ {
			item := rt.bagItem(bag, slot)
			if item == nil || item.Entry != entry {
				continue
			}
			if serverBag, serverSlot, ok := rt.serverBagAndSlot(bag, slot); ok {
				rt.useItem(L, item, serverBag, serverSlot)
			}
			return
		}
	}
}

// refreshItemActions has the buttons that hold items redraw their icon and
// count, after the bags or an item's description changed.
func (rt *Runtime) refreshItemActions() {
	for index, action := range rt.gameplay.Actions {
		if action.Type == actionTypeItem && action.ID != 0 {
			rt.FireEvent("ACTIONBAR_SLOT_CHANGED", lua.LNumber(index+1))
		}
	}
	rt.FireEvent("ACTIONBAR_UPDATE_USABLE")
}

// Skill lines of the spells a player summons as companions.
const (
	skillLineMounts     = 777
	skillLineCompanions = 778
)

// companionList is the known spells of a companion kind ("MOUNT" or "CRITTER").
func (rt *Runtime) companionList(kind string) []uint32 {
	switch strings.ToUpper(kind) {
	case "MOUNT":
		return rt.gameplay.Mounts
	case "CRITTER":
		return rt.gameplay.Critters
	}
	return nil
}

// registerCompanionAPI defines the calls the Mounts and Companions panels make.
func registerCompanionAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumCompanions", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.companionList(L.CheckString(1)))))
		return 1
	})
	reg("GetCompanionInfo", func(L *lua.LState) int {
		kind := strings.ToUpper(L.CheckString(1))
		list := rt.companionList(kind)
		index := L.CheckInt(2)
		if index < 1 || index > len(list) {
			return 0
		}
		spell := rt.gameplay.Catalog[list[index-1]]
		L.Push(lua.LNumber(rt.companionCreature(spell)))
		L.Push(lua.LString(spell.Name))
		L.Push(lua.LNumber(spell.ID))
		L.Push(lua.LString(spell.Icon))
		L.Push(lua.LBool(rt.companionSummoned(kind, spell)))
		return 5
	})
	reg("PickupCompanion", func(L *lua.LState) int {
		list := rt.companionList(L.CheckString(1))
		if index := L.CheckInt(2); index >= 1 && index <= len(list) {
			rt.pickupSpell(list[index-1])
		}
		return 0
	})
	reg("DismissCompanion", func(L *lua.LState) int {
		switch strings.ToUpper(L.CheckString(1)) {
		case "MOUNT":
			rt.send(world.ClientCancelMountAura, nil)
		case "CRITTER":
			if guid := rt.critterGUID(); guid != 0 {
				rt.send(world.ClientDismissCritter, binary.LittleEndian.AppendUint64(nil, guid))
			}
		}
		return 0
	})
	reg("GetCompanionCooldown", func(L *lua.LState) int {
		list := rt.companionList(L.CheckString(1))
		index := L.CheckInt(2)
		if index < 1 || index > len(list) {
			return 0
		}
		cooldown, ok := rt.gameplay.Cooldowns[list[index-1]]
		if !ok {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(1))
			return 3
		}
		L.Push(lua.LNumber(cooldown.Start))
		L.Push(lua.LNumber(cooldown.Duration))
		L.Push(lua.LNumber(boolNumber(!cooldown.Disabled)))
		return 3
	})
	reg("CallCompanion", func(L *lua.LState) int {
		list := rt.companionList(L.CheckString(1))
		index := L.CheckInt(2)
		if index < 1 || index > len(list) {
			return 0
		}
		if err := rt.cast(list[index-1], ""); err != nil {
			L.RaiseError("CallCompanion: %v", err)
		}
		return 0
	})
}

// Spell effects that bring a companion: summoning a pet and the mounted aura.
const (
	effectSummonPet  = 28
	effectSummonCrit = 56
	auraMounted      = 78
	fieldUnitCritter = 6 + 10
)

// companionCreature is the creature a companion spell brings.
func (rt *Runtime) companionCreature(spell SpellInfo) int {
	for _, effect := range spell.Effects {
		if effect.Aura == auraMounted || effect.Effect == effectSummonPet || effect.Effect == effectSummonCrit {
			return effect.Misc
		}
	}
	return 0
}

// critterGUID is the companion the character has out, or 0.
func (rt *Runtime) critterGUID() uint64 {
	return uint64(rt.field(fieldUnitCritter)) | uint64(rt.field(fieldUnitCritter+1))<<32
}

// companionSummoned reports whether the companion of a spell is out: a mount
// by its aura, a critter by the creature the character has summoned.
func (rt *Runtime) companionSummoned(kind string, spell SpellInfo) bool {
	if kind == "MOUNT" {
		if player := rt.unitInfo("player"); player != nil {
			for _, aura := range player.Auras {
				if aura.SpellID == spell.ID {
					return true
				}
			}
		}
		return false
	}
	guid := rt.critterGUID()
	return guid != 0 && int(guid>>24&0xFFFFFF) == rt.companionCreature(spell)
}
