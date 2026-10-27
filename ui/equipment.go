package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Equipment manager packets.
const (
	packetEquipmentSetList   uint16 = 0x04BC
	packetEquipmentSetSaved  uint16 = 0x0137
	packetEquipmentSetResult uint16 = 0x04D6

	clientEquipmentSetSave   world.ClientOpcode = 0x04BD
	clientEquipmentSetDelete world.ClientOpcode = 0x013E
	clientEquipmentSetUse    world.ClientOpcode = 0x04D5

	equipmentSlots    = 19
	maxEquipmentSets  = 10
	slotMainHand      = 15
	slotOffHand       = 16
	slotRanged        = 17
	ignoredSlotPiece  = 1
	missingIconName   = "INV_Misc_QuestionMark"
	equipmentIconPath = `Interface\Icons\`
)

// equipmentSet is a saved set of gear: its index, name, icon and the item in
// each equipment slot (1 stands for a slot the set leaves alone).
type equipmentSet struct {
	guid   uint64
	index  uint32
	name   string
	icon   string
	pieces [equipmentSlots]uint64
}

type equipmentState struct {
	sets    []*equipmentSet
	ignored [equipmentSlots]bool
	pending bool
	// pendingName is the set a swap in flight belongs to.
	pendingName string
}

func (rt *Runtime) equipmentSetByName(name string) *equipmentSet {
	for _, set := range rt.equipment.sets {
		if strings.EqualFold(set.name, name) {
			return set
		}
	}
	return nil
}

// equipmentManagerOn reports whether the equipment manager option is on.
func (rt *Runtime) equipmentManagerOn() bool {
	return rt.cvarBool("equipmentManager")
}

// itemLocation finds where an item the character owns sits: the bag number
// (0xFF for the backpack, equipment and bank) and the square, the way the
// server numbers them.
func (rt *Runtime) itemLocation(guid uint64) (uint8, uint8, bool) {
	state := &rt.inventory
	for slot := 0; slot < invBagEnd; slot++ {
		if state.slots[slot] == guid {
			return 0xFF, uint8(slot), true
		}
	}
	for slot := invPackStart; slot < invPackEnd; slot++ {
		if state.slots[slot] == guid {
			return 0xFF, uint8(slot), true
		}
	}
	for bag := 0; bag < invBagEnd-invBagStart; bag++ {
		container := state.items[state.slots[invBagStart+bag]]
		if container == nil {
			continue
		}
		for index, held := range container.Slots {
			if held == guid {
				return uint8(invBagStart + bag), uint8(index), true
			}
		}
	}
	return 0, 0, false
}

// handleEquipmentPacket takes the packets of the equipment manager.
func (rt *Runtime) handleEquipmentPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "equipment set")
	switch packet.Opcode {
	case packetEquipmentSetList:
		count, err := reader.U32()
		if err != nil {
			return true
		}
		rt.equipment.sets = rt.equipment.sets[:0]
		for ; count > 0; count-- {
			set := &equipmentSet{}
			if set.guid, err = world.ReadPackedGUID(reader); err != nil {
				break
			}
			if set.index, err = reader.U32(); err != nil {
				break
			}
			if set.name, err = reader.CString(); err != nil {
				break
			}
			if set.icon, err = reader.CString(); err != nil {
				break
			}
			for slot := range set.pieces {
				if set.pieces[slot], err = world.ReadPackedGUID(reader); err != nil {
					break
				}
			}
			rt.equipment.sets = append(rt.equipment.sets, set)
		}
		rt.FireEvent("EQUIPMENT_SETS_CHANGED")
	case packetEquipmentSetSaved:
		index, err := reader.U32()
		if err != nil {
			return true
		}
		guid, _ := world.ReadPackedGUID(reader)
		for _, set := range rt.equipment.sets {
			if set.index == index {
				set.guid = guid
			}
		}
		rt.FireEvent("EQUIPMENT_SETS_CHANGED")
	case packetEquipmentSetResult:
		result, err := reader.U8()
		if err != nil {
			return true
		}
		name := rt.equipment.pendingName
		rt.equipment.pending, rt.equipment.pendingName = false, ""
		rt.FireEvent("EQUIPMENT_SWAP_FINISHED", lua.LBool(result == 0), lua.LString(name))
	default:
		return false
	}
	return true
}

// freeEquipmentIndex is the lowest set index nobody uses, or -1.
func (rt *Runtime) freeEquipmentIndex() int {
	for index := 0; index < maxEquipmentSets; index++ {
		taken := false
		for _, set := range rt.equipment.sets {
			if int(set.index) == index {
				taken = true
			}
		}
		if !taken {
			return index
		}
	}
	return -1
}

// equippedPieces reads what the character wears now, leaving the slots marked
// to be ignored.
func (rt *Runtime) equippedPieces() [equipmentSlots]uint64 {
	var pieces [equipmentSlots]uint64
	for slot := range pieces {
		switch {
		case rt.equipment.ignored[slot]:
			pieces[slot] = ignoredSlotPiece
		default:
			pieces[slot] = rt.inventory.slots[slot]
		}
	}
	return pieces
}

// buildEquipmentSave is the body of CMSG_EQUIPMENT_SET_SAVE.
func buildEquipmentSave(set *equipmentSet) []byte {
	body := packGUID(set.guid)
	body = append(body, byte(set.index), byte(set.index>>8), byte(set.index>>16), byte(set.index>>24))
	body = append(append(body, set.name...), 0)
	body = append(append(body, set.icon...), 0)
	for _, piece := range set.pieces {
		body = append(body, packGUID(piece)...)
	}
	return body
}

// packGUID writes a guid in the packed form.
func packGUID(guid uint64) []byte {
	mask := byte(0)
	packed := make([]byte, 0, 9)
	for index := uint(0); index < 8; index++ {
		if value := byte(guid >> (index * 8)); value != 0 {
			mask |= 1 << index
			packed = append(packed, value)
		}
	}
	return append([]byte{mask}, packed...)
}

// useEquipmentSet asks the server to put a set on.
func (rt *Runtime) useEquipmentSet(set *equipmentSet) bool {
	if rt.equipment.pending {
		return false
	}
	if !rt.equipmentManagerOn() {
		rt.ShowError(rt.globalString("EQUIPMENT_MANAGER_IS_DISABLED", ""))
		return false
	}
	player := rt.unitInfo("player")
	inCombat := player != nil && player.InCombat
	var body []byte
	missingTold, combatTold := false, false
	for slot := 0; slot < equipmentSlots; slot++ {
		piece := set.pieces[slot]
		guid, bag, square := uint64(ignoredSlotPiece), uint8(0), uint8(0)
		weapon := slot == slotMainHand || slot == slotOffHand || slot == slotRanged
		switch {
		case inCombat && !weapon:
			if piece != ignoredSlotPiece && rt.inventory.slots[slot] != piece && !combatTold {
				combatTold = true
				rt.ShowError(fmt.Sprintf(rt.globalString("EQUIPMENT_MANAGER_COMBAT_SWAP", "%s"), set.name))
			}
		case piece == ignoredSlotPiece:
		case piece == 0:
			guid = 0
		default:
			if foundBag, foundSlot, ok := rt.itemLocation(piece); ok {
				guid, bag, square = piece, foundBag, foundSlot
			} else if !missingTold {
				missingTold = true
				rt.ShowError(fmt.Sprintf(rt.globalString("EQUIPMENT_MANAGER_MISSING_ITEM", "%s"), set.name))
			}
		}
		body = append(body, packGUID(guid)...)
		body = append(body, bag, square)
	}
	rt.send(clientEquipmentSetUse, body)
	rt.equipment.pending, rt.equipment.pendingName = true, set.name
	rt.FireEvent("EQUIPMENT_SWAP_PENDING")
	return true
}

// registerEquipmentAPI defines the calls of the equipment manager.
func registerEquipmentAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumEquipmentSets", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.equipment.sets)))
		return 1
	})
	reg("GetEquipmentSetInfo", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > maxEquipmentSets {
			L.RaiseError("Usage: GetEquipmentSetInfo(index)")
			return 0
		}
		if index > len(rt.equipment.sets) {
			return 0
		}
		set := rt.equipment.sets[index-1]
		L.Push(lua.LString(set.name))
		L.Push(lua.LString(equipmentIconPath + set.icon))
		L.Push(lua.LNumber(set.index))
		return 3
	})
	reg("GetEquipmentSetInfoByName", func(L *lua.LState) int {
		set := rt.equipmentSetByName(L.CheckString(1))
		if set == nil {
			return 0
		}
		L.Push(lua.LString(set.icon))
		L.Push(lua.LNumber(set.index))
		return 2
	})
	reg("GetEquipmentSetItemIDs", func(L *lua.LState) int {
		set := rt.equipmentSetByName(L.CheckString(1))
		if set == nil {
			return 0
		}
		table, given := L.Get(2).(*lua.LTable)
		if !given {
			table = L.NewTable()
		}
		for slot, piece := range set.pieces {
			value := lua.LNumber(0)
			switch {
			case piece == ignoredSlotPiece:
				value = 1
			case piece != 0:
				if item := rt.inventory.items[piece]; item != nil {
					value = lua.LNumber(item.Entry)
				}
			}
			table.RawSetInt(slot+1, value)
		}
		L.Push(table)
		return 1
	})
	reg("SaveEquipmentSet", func(L *lua.LState) int {
		name, icon := L.CheckString(1), L.CheckInt(2)
		if name == "" || len(name) > 31 {
			L.RaiseError("Invalid string for setName")
			return 0
		}
		iconName := missingIconName
		if icon < 0 {
			if item := rt.inventory.items[rt.inventory.slots[-icon-1]]; icon >= -equipmentSlots && item != nil {
				iconName = strings.TrimPrefix(rt.itemIcon(item.Entry), equipmentIconPath)
			}
		} else if icon > 0 && icon <= len(rt.macroIcons) {
			iconName = strings.TrimPrefix(rt.macroIcons[icon-1], equipmentIconPath)
		}
		set := rt.equipmentSetByName(name)
		if set == nil {
			index := rt.freeEquipmentIndex()
			if index < 0 {
				L.RaiseError("Too many sets! You can only have %d sets", maxEquipmentSets)
				return 0
			}
			set = &equipmentSet{index: uint32(index), name: name}
			rt.equipment.sets = append(rt.equipment.sets, set)
		}
		set.icon, set.pieces = iconName, rt.equippedPieces()
		rt.send(clientEquipmentSetSave, buildEquipmentSave(set))
		rt.FireEvent("EQUIPMENT_SETS_CHANGED")
		return 0
	})
	reg("DeleteEquipmentSet", func(L *lua.LState) int {
		name := L.CheckString(1)
		for index, set := range rt.equipment.sets {
			if strings.EqualFold(set.name, name) {
				rt.equipment.sets = append(rt.equipment.sets[:index], rt.equipment.sets[index+1:]...)
				rt.send(clientEquipmentSetDelete, packGUID(set.guid))
				rt.FireEvent("EQUIPMENT_SETS_CHANGED")
				break
			}
		}
		return 0
	})
	reg("UseEquipmentSet", func(L *lua.LState) int {
		set := rt.equipmentSetByName(L.CheckString(1))
		if set == nil {
			return 0
		}
		if rt.useEquipmentSet(set) {
			L.Push(lua.LTrue)
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("CanUseEquipmentSets", func(L *lua.LState) int {
		on := !rt.equipment.pending && rt.equipmentManagerOn()
		L.Push(lua.LNumber(boolNumber(on)))
		return 1
	})
	reg("EquipmentSetContainsLockedItems", func(L *lua.LState) int {
		set := rt.equipmentSetByName(L.CheckString(1))
		if set == nil {
			return 0
		}
		for _, piece := range set.pieces {
			if piece > ignoredSlotPiece && rt.itemLocked(piece) {
				L.Push(lua.LTrue)
				return 1
			}
		}
		return 0
	})
	reg("EquipmentManagerClearIgnoredSlotsForSave", func(L *lua.LState) int {
		rt.equipment.ignored = [equipmentSlots]bool{}
		return 0
	})
	reg("EquipmentManagerIgnoreSlotForSave", func(L *lua.LState) int {
		slot := L.CheckInt(1)
		if slot < 1 || slot > equipmentSlots {
			L.RaiseError("Usage: EquipmentManagerIgnoreSlotForSave(slot)")
			return 0
		}
		rt.equipment.ignored[slot-1] = true
		return 0
	})
	reg("EquipmentManagerUnignoreSlotForSave", func(L *lua.LState) int {
		slot := L.CheckInt(1)
		if slot < 1 || slot > equipmentSlots {
			L.RaiseError("Usage: UpdatePendingEquipmentSetSlot(slot)")
			return 0
		}
		rt.equipment.ignored[slot-1] = false
		return 0
	})
}

// cvarBool reads a console variable as a switch.
func (rt *Runtime) cvarBool(name string) bool {
	value, ok := rt.GetCVar(name)
	return ok && (value == "1" || strings.EqualFold(value, "true"))
}

// itemLocked reports whether an item is held on the cursor.
func (rt *Runtime) itemLocked(guid uint64) bool {
	cursor := rt.inventory.cursor
	return cursor.active && cursor.guid == guid
}
