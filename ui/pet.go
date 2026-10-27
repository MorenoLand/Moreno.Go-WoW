package ui

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Pet fields: the fields of the pet's own unit.
const (
	fieldPetExperience  = 6 + 0x47
	fieldPetNextXP      = 6 + 0x48
	fieldPetBytes2      = 6 + 0x74
	fieldPetHappiness   = 0x19 + 4
	petFlagCanBeRenamed = 0x01
	petFlagCanAbandon   = 0x02
	spellDismissPet     = 2641
)

// PetFamily is a row of CreatureFamily.dbc.
type PetFamily struct {
	Name       string
	Icon       string
	FoodMask   uint32
	TalentType int32
}

// PetTables are the pet data files.
type PetTables struct {
	Families map[uint32]PetFamily
	// Foods are the names of the food kinds, by bit (counted from 1).
	Foods map[uint32]string
	// TalentTabs name the talent trees a pet talent type opens, by type bit.
	TalentTabs map[uint32]string
}

type petState struct {
	tables    PetTables
	guid      uint64
	family    uint16
	expires   time.Time
	timed     bool
	react     uint8
	command   uint8
	actions   [10]uint32
	spells    []uint32
	cooldown  map[uint32]time.Time
	cooldownT map[uint32]time.Duration
	attacking bool
	got       bool
	// fields of the pet's own unit.
	xp, nextXP uint32
	bytes2     uint32
	happiness  uint32
	dead       bool
}

// SetPetTables gives the interface the pet data files.
func (rt *Runtime) SetPetTables(tables PetTables) { rt.pet.tables = tables }

// SetPetSpells takes the pet's action bar the server sent.
func (rt *Runtime) SetPetSpells(info world.PetSpellInfo) {
	state := &rt.pet
	if info.GUID == 0 {
		state.guid, state.got, state.spells = 0, false, nil
		state.actions = [10]uint32{}
		rt.FireEvent("PET_BAR_HIDE")
		rt.FireEvent("UNIT_PET", lua.LString("player"))
		return
	}
	state.guid, state.family, state.got = info.GUID, info.Family, true
	state.react, state.command = info.React, info.Command
	state.actions, state.spells = info.Actions, info.Spells
	state.timed = info.Duration != 0
	if state.timed {
		state.expires = time.Now().Add(time.Duration(info.Duration) * time.Millisecond)
	}
	if state.cooldown == nil {
		state.cooldown, state.cooldownT = make(map[uint32]time.Time), make(map[uint32]time.Duration)
	}
	for _, cooldown := range info.Cooldowns {
		duration := time.Duration(cooldown.Time) * time.Millisecond
		if duration == 0 {
			duration = time.Duration(cooldown.Category2) * time.Millisecond
		}
		state.cooldown[cooldown.Spell] = time.Now().Add(duration)
		state.cooldownT[cooldown.Spell] = duration
	}
	rt.FireEvent("PET_BAR_UPDATE")
	rt.FireEvent("PET_UI_UPDATE")
	rt.FireEvent("UNIT_PET", lua.LString("player"))
}

// SetPetMode takes a change of the pet's reaction and command.
func (rt *Runtime) SetPetMode(guid uint64, mode uint32) {
	if guid != rt.pet.guid {
		return
	}
	rt.pet.react, rt.pet.command = uint8(mode>>0&0xFF), uint8(mode>>8&0xFF)
	rt.FireEvent("PET_BAR_UPDATE")
}

// SetPetUnitFields takes the fields of the pet's own unit.
func (rt *Runtime) SetPetUnitFields(fields map[uint16]uint32) {
	state := &rt.pet
	changed := state.xp != fields[fieldPetExperience] || state.nextXP != fields[fieldPetNextXP]
	state.xp, state.nextXP = fields[fieldPetExperience], fields[fieldPetNextXP]
	state.bytes2 = fields[fieldPetBytes2]
	state.happiness = fields[fieldPetHappiness]
	if changed {
		rt.FireEvent("UNIT_PET_EXPERIENCE", lua.LString("pet"))
	}
}

// ClearPetUnit forgets the pet's unit fields.
func (rt *Runtime) ClearPetUnit() {
	rt.pet.xp, rt.pet.nextXP, rt.pet.bytes2, rt.pet.happiness = 0, 0, 0, 0
}

// SetPetAttacking records whether the pet is fighting.
func (rt *Runtime) SetPetAttacking(attacking bool) { rt.pet.attacking = attacking }

// ShowPetFeedback shows what the pet answered to an action.
func (rt *Runtime) ShowPetFeedback(code uint8) {
	switch code {
	case 1:
		rt.ShowGlobalError("ERR_PET_SPELL_DEAD")
	case 2:
		rt.ShowGlobalError("ERR_PET_SPELL_TARGETS_DEAD")
	case 3:
		rt.ShowGlobalError("ERR_ATTACK_PACIFIED")
	}
}

func (rt *Runtime) petActionAt(index int) uint32 {
	if rt.pet.guid == 0 || index < 1 || index > len(rt.pet.actions) {
		return 0
	}
	return rt.pet.actions[index-1]
}

func petActionType(action uint32) uint8 { return uint8(action >> 24) }

// petSpellOf is the spell of an action button.
func petSpellOf(action uint32) uint32 { return action & 0xFFFFFF }

// petCommandNames and petReactionNames are the tokens of the commands and the
// reactions, as the client names them.
var (
	petCommandNames  = []string{"WAIT", "FOLLOW", "ATTACK", "DISMISS"}
	petReactionNames = []string{"PASSIVE", "DEFENSIVE", "AGGRESSIVE"}
)

// HunterPet reports whether the pet is a hunter's own.
func (rt *Runtime) hunterPet() bool {
	player := rt.unitInfo("player")
	return player != nil && player.ClassID == classHunter && !rt.pet.timed && rt.pet.got
}

// petTarget is the guid a pet action is aimed at: the unit given, or the target.
func (rt *Runtime) petTarget(L *lua.LState, index int) uint64 {
	unit := "target"
	if L.Get(index) != lua.LNil {
		unit = L.CheckString(index)
	}
	if info := rt.unitInfo(unit); info != nil && info.Exists {
		return info.GUID
	}
	return 0
}

func (rt *Runtime) petCommand(action uint32, target uint64) {
	if rt.pet.guid != 0 {
		rt.send(world.ClientPetAction, world.BuildPetAction(rt.pet.guid, action, target))
	}
}

// handlePetPacket takes the packets of pets and totems.
func (rt *Runtime) handlePetPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.PetSpellsPacket:
		if info, err := world.ParsePetSpells(packet.Body); err == nil {
			rt.SetPetSpells(info)
		}
	case world.PetModePacket:
		reader := world.NewReader(packet.Body, "SMSG_PET_MODE")
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		mode, _ := reader.U32()
		rt.SetPetMode(guid, mode)
	case world.PetActionBack:
		if code, err := world.NewReader(packet.Body, "SMSG_PET_ACTION_FEEDBACK").U8(); err == nil {
			rt.ShowPetFeedback(code)
		}
	case world.PetNameInvalid:
		rt.ShowGlobalError("ERR_PET_NAME_INVALID")
	case world.PetRenameable:
		rt.FireEvent("PET_UI_UPDATE")
	case world.TotemCreated:
		if totem, err := world.ParseTotemCreated(packet.Body); err == nil {
			rt.SetTotem(totem)
		}
	default:
		return false
	}
	return true
}

// SetTotem puts a totem in its slot.
func (rt *Runtime) SetTotem(totem world.Totem) {
	if int(totem.Slot) >= len(rt.totems) {
		return
	}
	rt.totems[totem.Slot] = totemSlot{guid: totem.GUID, spell: totem.Spell, start: time.Now(), duration: time.Duration(totem.Duration) * time.Millisecond}
	if totem.Duration == 0 {
		rt.totems[totem.Slot] = totemSlot{}
	}
	rt.FireEvent("PLAYER_TOTEM_UPDATE", lua.LNumber(totem.Slot+1))
}

type totemSlot struct {
	guid     uint64
	spell    uint32
	start    time.Time
	duration time.Duration
}

// registerPetAPI defines the calls of the pet bar and of totems.
func registerPetAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	reg("HasPetUI", func(L *lua.LState) int {
		if rt.pet.guid == 0 {
			return push(L, lua.LNil, lua.LNil)
		}
		return push(L, lua.LNumber(1), luaFlag(rt.hunterPet()))
	})
	reg("PetHasActionBar", func(L *lua.LState) int { return push(L, luaFlag(rt.pet.guid != 0 && rt.pet.got)) })
	reg("HasPetSpells", func(L *lua.LState) int {
		if len(rt.pet.spells) == 0 {
			return push(L, lua.LNil, lua.LNil)
		}
		return push(L, lua.LNumber(len(rt.pet.spells)), lua.LString("PET"))
	})
	reg("GetPetActionInfo", func(L *lua.LState) int {
		action := rt.petActionAt(L.CheckInt(1))
		if action == 0 {
			return push(L, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil)
		}
		kind := petActionType(action) & 0x3F
		switch kind {
		case 6:
			index := int(petSpellOf(action))
			if index >= len(petReactionNames) {
				index = 0
			}
			return push(L, lua.LString("PET_MODE_"+petReactionNames[index]), lua.LNil, lua.LString("PET_"+petReactionNames[index]+"_TEXTURE"),
				lua.LNumber(1), luaFlag(int(rt.pet.react) == index), lua.LNil, lua.LNil)
		case 7:
			index := int(petSpellOf(action))
			if index >= len(petCommandNames) {
				index = 0
			}
			active := rt.pet.attacking && index == int(world.PetCommandAttack) || index != int(world.PetCommandAttack) && int(rt.pet.command) == index
			return push(L, lua.LString("PET_ACTION_"+petCommandNames[index]), lua.LNil, lua.LString("PET_"+petCommandNames[index]+"_TEXTURE"),
				lua.LNumber(1), luaFlag(active), lua.LNil, lua.LNil)
		}
		info, known := rt.gameplay.Catalog[petSpellOf(action)]
		if !known {
			return push(L, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil)
		}
		allowed := action&0x80000000 != 0
		enabled := action&0x40000000 != 0
		return push(L, lua.LString(info.Name), lua.LString(info.Rank), lua.LString(info.Icon), lua.LNil, lua.LNil, luaFlag(allowed), luaFlag(allowed && enabled))
	})
	reg("GetPetActionCooldown", func(L *lua.LState) int {
		action := rt.petActionAt(L.CheckInt(1))
		kind := petActionType(action) & 0x3F
		if action != 0 && (kind == 1 || kind >= 8 && kind < 18) {
			spell := petSpellOf(action)
			if end, ok := rt.pet.cooldown[spell]; ok && time.Now().Before(end) {
				duration := rt.pet.cooldownT[spell]
				return push(L, lua.LNumber(float64(end.Add(-duration).UnixNano())/1e9), lua.LNumber(duration.Seconds()), lua.LNumber(1))
			}
		}
		return push(L, lua.LNumber(0), lua.LNumber(0), lua.LNumber(1))
	})
	reg("GetPetActionSlotUsable", func(L *lua.LState) int {
		action := rt.petActionAt(L.CheckInt(1))
		pet := rt.unitInfo("pet")
		return push(L, luaFlag(action != 0 && pet != nil && !pet.Dead))
	})
	reg("CastPetAction", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.pet.actions) {
			L.RaiseError("Invalid slot in CastPetAction")
			return 0
		}
		if action := rt.pet.actions[index-1]; action != 0 {
			rt.petCommand(action, rt.petTarget(L, 2))
		}
		return 0
	})
	reg("TogglePetAutocast", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.pet.actions) {
			L.RaiseError("Invalid slot in TogglePetAutocast")
			return 0
		}
		action := rt.pet.actions[index-1]
		if action&0x80000000 == 0 {
			return 0
		}
		enable := action&0x40000000 == 0
		if enable {
			rt.pet.actions[index-1] = action | 0x40000000
		} else {
			rt.pet.actions[index-1] = action &^ 0x40000000
		}
		rt.send(world.ClientPetSpellAutocast, world.BuildPetAutocast(rt.pet.guid, petSpellOf(action), enable))
		rt.FireEvent("PET_BAR_UPDATE")
		return 0
	})
	reg("PickupPetAction", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(rt.pet.actions) {
			L.RaiseError("Invalid slot in PickupPetAction")
			return 0
		}
		if action := rt.pet.actions[index-1]; action != 0 && petActionType(action)&0x3F != 6 && petActionType(action)&0x3F != 7 {
			rt.pickupSpell(petSpellOf(action))
		}
		return 0
	})
	reg("PetAttack", func(L *lua.LState) int {
		rt.petCommand(uint32(7)<<24|world.PetCommandAttack, rt.petTarget(L, 1))
		return 0
	})
	reg("PetFollow", func(L *lua.LState) int { rt.petCommand(uint32(7)<<24|world.PetCommandFollow, 0); return 0 })
	reg("PetWait", func(L *lua.LState) int { rt.petCommand(uint32(7)<<24|world.PetCommandStay, 0); return 0 })
	reg("PetPassiveMode", func(L *lua.LState) int { rt.petCommand(uint32(6)<<24|world.PetReactPassive, 0); return 0 })
	reg("PetDefensiveMode", func(L *lua.LState) int { rt.petCommand(uint32(6)<<24|world.PetReactDefensive, 0); return 0 })
	reg("PetAggressiveMode", func(L *lua.LState) int { rt.petCommand(uint32(6)<<24|world.PetReactAggressive, 0); return 0 })
	reg("PetDismiss", func(L *lua.LState) int {
		if rt.hunterPet() {
			if host, ok := rt.Host.(interface{ CastSpell(uint32, uint64) error }); ok {
				_ = host.CastSpell(spellDismissPet, 0)
			}
			return 0
		}
		rt.petCommand(uint32(7)<<24|world.PetCommandDismiss, 0)
		return 0
	})
	reg("PetAbandon", func(L *lua.LState) int {
		if rt.pet.guid != 0 {
			rt.send(world.ClientPetAbandon, world.BuildGuid(rt.pet.guid))
		}
		return 0
	})
	reg("PetRename", func(L *lua.LState) int {
		name := L.OptString(1, "")
		if rt.pet.guid == 0 || rt.pet.bytes2>>16&petFlagCanBeRenamed == 0 {
			rt.ShowGlobalError("ERR_PET_NOT_RENAMEABLE")
			return 0
		}
		if name == "" {
			rt.ShowGlobalError("ERR_PET_NAME_INVALID")
			return 0
		}
		rt.send(world.ClientPetRename, world.BuildPetRename(rt.pet.guid, name))
		return 0
	})
	reg("PetCanBeRenamed", func(L *lua.LState) int {
		return push(L, luaFlag(rt.pet.guid != 0 && rt.pet.bytes2>>16&petFlagCanBeRenamed != 0))
	})
	reg("PetCanBeAbandoned", func(L *lua.LState) int {
		return push(L, luaFlag(rt.pet.guid != 0 && rt.pet.bytes2>>16&petFlagCanAbandon != 0))
	})
	reg("PetCanBeDismissed", func(L *lua.LState) int { return push(L, lua.LNumber(1)) })
	reg("GetPetExperience", func(L *lua.LState) int {
		return push(L, lua.LNumber(rt.pet.xp), lua.LNumber(rt.pet.nextXP))
	})
	reg("GetPetTimeRemaining", func(L *lua.LState) int {
		if !rt.pet.timed {
			return push(L, lua.LNil)
		}
		left := time.Until(rt.pet.expires).Milliseconds()
		if left < 0 {
			left = 0
		}
		return push(L, lua.LNumber(left))
	})
	reg("GetPetHappiness", func(L *lua.LState) int {
		if !rt.hunterPet() || rt.pet.guid == 0 {
			return push(L, lua.LNil, lua.LNumber(0))
		}
		index, damage := 3, 125
		switch happiness := rt.pet.happiness; {
		case happiness < 333000:
			index, damage = 1, 75
		case happiness < 666000:
			index, damage = 2, 100
		}
		return push(L, lua.LNumber(index), lua.LNumber(damage))
	})
	reg("GetPetIcon", func(L *lua.LState) int {
		if family, ok := rt.pet.tables.Families[uint32(rt.pet.family)]; ok && rt.hunterPet() {
			return push(L, lua.LString(family.Icon))
		}
		return push(L, lua.LNil)
	})
	reg("GetPetFoodTypes", func(L *lua.LState) int {
		family, ok := rt.pet.tables.Families[uint32(rt.pet.family)]
		if !ok || !rt.hunterPet() {
			return 0
		}
		count := 0
		for bit := uint32(1); bit <= 32; bit++ {
			if family.FoodMask&(1<<(bit-1)) != 0 {
				if name, known := rt.pet.tables.Foods[bit]; known {
					L.Push(lua.LString(name))
					count++
				}
			}
		}
		return count
	})
	reg("GetPetTalentTree", func(L *lua.LState) int {
		family, ok := rt.pet.tables.Families[uint32(rt.pet.family)]
		if ok && family.TalentType >= 0 {
			if name, known := rt.pet.tables.TalentTabs[uint32(family.TalentType)]; known {
				return push(L, lua.LString(name))
			}
		}
		return push(L, lua.LNil)
	})

	// Totems.
	slot := func(L *lua.LState) (int, bool) {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.totems) {
			L.RaiseError("Totem slot must be in range of 1 to %d", len(rt.totems))
			return 0, false
		}
		return index, true
	}
	reg("GetTotemInfo", func(L *lua.LState) int {
		index, ok := slot(L)
		if !ok {
			return 0
		}
		totem := rt.totems[index]
		if totem.guid == 0 {
			return push(L, lua.LFalse, lua.LString(""), lua.LNumber(0), lua.LNumber(0), lua.LString(""))
		}
		info := rt.gameplay.Catalog[totem.spell]
		return push(L, lua.LTrue, lua.LString(info.Name), lua.LNumber(totem.start.Unix()), lua.LNumber(totem.duration.Seconds()), lua.LString(info.Icon))
	})
	reg("GetTotemTimeLeft", func(L *lua.LState) int {
		index, ok := slot(L)
		if !ok {
			return 0
		}
		totem := rt.totems[index]
		if totem.guid == 0 {
			return push(L, lua.LNumber(0))
		}
		left := totem.duration - time.Since(totem.start)
		if left < 0 {
			left = 0
		}
		return push(L, lua.LNumber(int(left.Seconds())))
	})
	reg("DestroyTotem", func(L *lua.LState) int {
		index, ok := slot(L)
		if ok {
			rt.send(world.ClientTotemDestroyed, []byte{byte(index)})
		}
		return 0
	})
}

// pickupSpell puts a spell on the cursor, as PickupSpell does.
func (rt *Runtime) pickupSpell(id uint32) {
	if id == 0 {
		return
	}
	rt.gameplay.Cursor = world.ActionButton{ID: id}
	rt.FireEvent("CURSOR_UPDATE")
}
