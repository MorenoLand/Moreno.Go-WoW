package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The stable master: the hunter's pets kept in stable slots. The server lists
// them, the player picks one up and drops it on a slot to stable, unstable or
// swap it.
const (
	stableListOpcode    = 0x026F
	stablePetOpcode     = 0x0270
	stableUnstableOp    = 0x0271
	stableBuySlotOpcode = 0x0272
	stableResultOpcode  = 0x0273
	stableSwapOpcode    = 0x0275

	stableMaxSlots   = 4
	stableCursorNone = -2
	stableActive     = -1
	// stableSelectedActive and stableSelectedNone are the selection values for
	// the active pet and for no selection.
	stableSelectedActive = 0xFFFFFFFF
)

type stablePet struct {
	number uint32
	level  uint32
	entry  uint32
	name   string
	flags  uint8
}

// StableFamily is what the client knows of a beast family for the stable.
type StableFamily struct {
	Icon, Name string
	TalentType int32
	FoodMask   uint32
}

// StableTables are the client tables the stable draws on.
type StableTables struct {
	Families map[uint32]StableFamily
	// Foods are the pet food names by bit.
	Foods []StableFood
	// TalentTabs are the pet talent trees with the mask of beast types.
	TalentTabs []StableTalentTab
	// SlotPrices are the copper costs of each stable slot.
	SlotPrices []uint32
}

type StableFood struct {
	ID   uint32
	Name string
}

type StableTalentTab struct {
	Name string
	Mask uint32
}

type stableState struct {
	master   uint64
	slots    int
	pets     []stablePet
	active   stablePet
	selected uint32
	cursor   int
	tables   StableTables
}

// SetStableTables installs the client tables of the stable.
func (rt *Runtime) SetStableTables(tables StableTables) { rt.stable.tables = tables }

// stablePetAt is the pet an index names: 1 and up the stabled ones, anything
// below the active pet.
func (rt *Runtime) stablePetAt(index int) *stablePet {
	state := &rt.stable
	if index < 0 {
		return &state.active
	}
	if index < len(state.pets) {
		return &state.pets[index]
	}
	return nil
}

// stableFamilyOf is the beast family of a stabled pet, once the creature
// cache has the pet's entry.
func (rt *Runtime) stableFamilyOf(pet *stablePet) (StableFamily, bool) {
	if pet == nil || rt.CreatureFamilyOf == nil {
		return StableFamily{}, false
	}
	family, ok := rt.CreatureFamilyOf(pet.entry)
	if !ok {
		return StableFamily{}, false
	}
	info, ok := rt.stable.tables.Families[family]
	return info, ok
}

func (rt *Runtime) stableSelectedIndex() int {
	state := &rt.stable
	if state.selected == stableSelectedActive {
		return stableActive
	}
	if state.selected != 0 {
		for index, pet := range state.pets {
			if pet.number == state.selected {
				return index
			}
		}
	}
	return stableCursorNone
}

func (rt *Runtime) stableSend(opcode world.ClientOpcode, extra ...uint32) {
	body := make([]byte, 8, 12)
	binary.LittleEndian.PutUint64(body, rt.stable.master)
	for _, value := range extra {
		body = binary.LittleEndian.AppendUint32(body, value)
	}
	rt.send(opcode, body)
}

func (rt *Runtime) handleStablePacket(packet world.Packet) bool {
	switch packet.Opcode {
	case stableListOpcode:
		reader := world.NewReader(packet.Body, "MSG_LIST_STABLED_PETS")
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		count, err := reader.U8()
		if err != nil {
			return true
		}
		slots, err := reader.U8()
		if err != nil {
			return true
		}
		state := &rt.stable
		state.master, state.slots = guid, int(slots)
		state.pets, state.active = nil, stablePet{}
		state.selected, state.cursor = 0, stableCursorNone
		for i := 0; i < int(count); i++ {
			var pet stablePet
			if pet.number, err = reader.U32(); err != nil {
				break
			}
			if pet.entry, err = reader.U32(); err != nil {
				break
			}
			if pet.level, err = reader.U32(); err != nil {
				break
			}
			if pet.name, err = reader.CString(); err != nil {
				break
			}
			if pet.flags, err = reader.U8(); err != nil {
				break
			}
			if pet.flags&2 == 0 {
				state.active = pet
			} else {
				state.pets = append(state.pets, pet)
			}
		}
		rt.CloseGossipWindow()
		rt.FireEvent("PET_STABLE_SHOW")
		return true
	case stableResultOpcode:
		reader := world.NewReader(packet.Body, "SMSG_STABLE_RESULT")
		result, err := reader.U8()
		if err != nil {
			return true
		}
		switch result {
		case 1:
			rt.ShowGameError(0x28)
		case 10, 8, 9:
			if result == 10 {
				rt.stable.slots++
			}
			if rt.stable.master != 0 {
				rt.stableSend(stableListOpcode)
			}
		case 0xB:
			rt.FireEvent("PET_STABLE_UPDATE")
		case 0xC:
			text := rt.globalString("PETTAME_CANTCONTROLEXOTIC", "")
			if text != "" {
				rt.FireEvent("UI_ERROR_MESSAGE", lua.LString(text))
			}
		}
		return true
	}
	return false
}

// closeStable ends the visit to the stable master.
func (rt *Runtime) closeStable() {
	if rt.stable.master == 0 {
		return
	}
	rt.stable.master = 0
	rt.stable.cursor = stableCursorNone
	rt.FireEvent("PET_STABLE_CLOSED")
}

func registerStableAPI(rt *Runtime) {
	L := rt.L
	state := &rt.stable
	state.cursor = stableCursorNone
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	// index converts a one-based Lua index to the stable's own: zero is the
	// active pet.
	index := func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetStablePetInfo(index)")
		}
		return L.CheckInt(1) - 1
	}
	reg("IsAtStableMaster", func(L *lua.LState) int { L.Push(lua.LBool(state.master != 0)); return 1 })
	reg("GetNumStablePets", func(L *lua.LState) int { L.Push(lua.LNumber(len(state.pets))); return 1 })
	reg("GetNumStableSlots", func(L *lua.LState) int { L.Push(lua.LNumber(state.slots)); return 1 })
	reg("GetSelectedStablePet", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.stableSelectedIndex() + 1))
		return 1
	})
	reg("GetStablePetInfo", func(L *lua.LState) int {
		pet := rt.stablePetAt(index(L))
		family, ok := rt.stableFamilyOf(pet)
		if pet == nil || !ok {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 3
		}
		L.Push(lua.LString(family.Icon))
		L.Push(lua.LString(pet.name))
		L.Push(lua.LNumber(pet.level))
		L.Push(lua.LString(family.Name))
		for _, tab := range state.tables.TalentTabs {
			if family.TalentType >= 0 && tab.Mask&(1<<(uint32(family.TalentType)&31)) != 0 {
				L.Push(lua.LString(tab.Name))
				return 5
			}
		}
		return 4
	})
	reg("GetStablePetFoodTypes", func(L *lua.LState) int {
		pet := rt.stablePetAt(index(L))
		family, ok := rt.stableFamilyOf(pet)
		if pet == nil || !ok {
			return 0
		}
		count := 0
		for _, food := range state.tables.Foods {
			if family.FoodMask&(1<<((food.ID-1)&31)) != 0 {
				L.Push(lua.LString(food.Name))
				count++
			}
		}
		return count
	})
	reg("PickupStablePet", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: PickupStablePet(index)")
		}
		slot := L.CheckInt(1) - 1
		if pet := rt.stablePetAt(slot); pet != nil && pet.number != 0 {
			state.cursor = slot
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
	reg("ClickStablePet", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: ClickStablePet(index)")
		}
		target := L.CheckInt(1) - 1
		setSelected := func(slot int) {
			switch pet := rt.stablePetAt(slot); {
			case slot == stableActive:
				state.selected = stableSelectedActive
			case pet != nil:
				state.selected = pet.number
			default:
				state.selected = 0
			}
			rt.FireEvent("PET_STABLE_UPDATE_PAPERDOLL")
		}
		if state.cursor == stableCursorNone {
			setSelected(target)
			L.Push(lua.LNumber(1))
			return 1
		}
		held := state.cursor
		state.cursor = stableCursorNone
		rt.FireEvent("CURSOR_UPDATE")
		if held == target {
			return 0
		}
		if held == stableActive {
			// The active pet dropped on a slot: stable it, or swap with the pet there.
			if pet := rt.stablePetAt(target); pet != nil && pet.number != 0 {
				rt.stableSend(stableSwapOpcode, pet.number)
			} else if target >= 0 && target < state.slots {
				rt.stableSend(stablePetOpcode)
			}
		} else if target == stableActive {
			// A stabled pet dropped on the active pet's place: unstable it, or swap.
			if pet := rt.stablePetAt(held); pet != nil {
				if state.active.number != 0 {
					rt.stableSend(stableSwapOpcode, pet.number)
				} else {
					rt.stableSend(stableUnstableOp, pet.number)
				}
			}
		}
		return 0
	})
	reg("BuyStableSlot", func(L *lua.LState) int {
		info := rt.unitInfo("player")
		if info == nil || state.master == 0 || state.slots >= stableMaxSlots {
			return 0
		}
		cost := rt.stableSlotCost()
		if cost == 0 && state.slots+1 >= len(state.tables.SlotPrices) {
			return 0
		}
		if rt.inventory.money < cost {
			rt.ShowGameError(gameErrorNotEnoughMon)
			return 0
		}
		rt.stableSend(stableBuySlotOpcode)
		return 0
	})
	reg("GetNextStableSlotCost", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.stableSlotCost()))
		return 1
	})
	reg("ClosePetStables", func(L *lua.LState) int {
		rt.closeStable()
		return 0
	})
	reg("SetPetStablePaperdoll", func(L *lua.LState) int {
		// The model frame is told which pet to show; model frames are not
		// drawn by this client, so only the argument is checked.
		if L.Get(1).Type() != lua.LTUserData && L.Get(1).Type() != lua.LTTable {
			L.RaiseError("Usage: SetPetStablePaperdoll(characterModel)")
		}
		return 0
	})
}

func (rt *Runtime) stableSlotCost() uint32 {
	next := rt.stable.slots + 1
	if next < 0 || next >= len(rt.stable.tables.SlotPrices) {
		return 0
	}
	return rt.stable.tables.SlotPrices[next]
}

// stableCursorHolds reports a stabled pet held on the cursor.
func (rt *Runtime) stableCursorHolds() bool { return rt.stable.cursor != stableCursorNone }
