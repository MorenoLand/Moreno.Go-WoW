package world

import "encoding/binary"

// Pet packets.
const (
	PetNameInvalid  uint16 = 0x0178
	PetSpellsPacket uint16 = 0x0179
	PetModePacket   uint16 = 0x017A
	PetActionBack   uint16 = 0x02C6
	TotemCreated    uint16 = 0x0413
	PetRenameable   uint16 = 0x0475

	ClientPetAction        ClientOpcode = 0x0175
	ClientPetAbandon       ClientOpcode = 0x0176
	ClientPetRename        ClientOpcode = 0x0177
	ClientPetSetAction     ClientOpcode = 0x0174
	ClientPetCancelAura    ClientOpcode = 0x026B
	ClientPetStopAttack    ClientOpcode = 0x02EA
	ClientPetSpellAutocast ClientOpcode = 0x02F3
	ClientTotemDestroyed   ClientOpcode = 0x0414
	ClientDismissCritter   ClientOpcode = 0x048D
	ClientCancelMountAura  ClientOpcode = 0x0375
	ClientRequestPetInfo   ClientOpcode = 0x0279
)

// Pet action button types (the high byte of an action).
const (
	PetActionPassive  uint8 = 0x01
	PetActionDisabled uint8 = 0x81
	PetActionEnabled  uint8 = 0xC1
	PetActionReaction uint8 = 0x06
	PetActionCommand  uint8 = 0x07
)

// Pet commands and reactions.
const (
	PetCommandStay    uint32 = 0
	PetCommandFollow  uint32 = 1
	PetCommandAttack  uint32 = 2
	PetCommandDismiss uint32 = 3

	PetReactPassive    uint32 = 0
	PetReactDefensive  uint32 = 1
	PetReactAggressive uint32 = 2
)

// PetCooldown is a spell the pet cannot cast for a while.
type PetCooldown struct {
	Spell     uint32
	Category  uint16
	Time      uint32
	Category2 uint32
}

// PetSpellInfo is SMSG_PET_SPELLS: the pet's action bar and spells.
type PetSpellInfo struct {
	GUID      uint64
	Family    uint16
	Duration  uint32
	React     uint8
	Command   uint8
	Flags     uint16
	Actions   [10]uint32
	Spells    []uint32
	Cooldowns []PetCooldown
}

// ParsePetSpells reads SMSG_PET_SPELLS.
func ParsePetSpells(body []byte) (PetSpellInfo, error) {
	r := NewReader(body, "SMSG_PET_SPELLS")
	var info PetSpellInfo
	var err error
	if info.GUID, err = r.U64(); err != nil {
		return info, err
	}
	if r.Remaining() == 0 {
		return info, nil
	}
	if info.Family, err = r.U16(); err != nil {
		return info, err
	}
	if info.Duration, err = r.U32(); err != nil {
		return info, err
	}
	if info.React, err = r.U8(); err != nil {
		return info, err
	}
	if info.Command, err = r.U8(); err != nil {
		return info, err
	}
	if info.Flags, err = r.U16(); err != nil {
		return info, err
	}
	for index := range info.Actions {
		if info.Actions[index], err = r.U32(); err != nil {
			return info, err
		}
	}
	count, err := r.U8()
	if err != nil {
		return info, err
	}
	for index := uint8(0); index < count; index++ {
		spell, err := r.U32()
		if err != nil {
			return info, err
		}
		info.Spells = append(info.Spells, spell)
	}
	cooldowns, err := r.U8()
	if err != nil {
		return info, nil
	}
	for index := uint8(0); index < cooldowns; index++ {
		var cooldown PetCooldown
		if cooldown.Spell, err = r.U32(); err != nil {
			return info, err
		}
		if cooldown.Category, err = r.U16(); err != nil {
			return info, err
		}
		if cooldown.Time, err = r.U32(); err != nil {
			return info, err
		}
		if cooldown.Category2, err = r.U32(); err != nil {
			return info, err
		}
		info.Cooldowns = append(info.Cooldowns, cooldown)
	}
	return info, nil
}

// BuildPetAction is the body of CMSG_PET_ACTION.
func BuildPetAction(pet uint64, action uint32, target uint64) []byte {
	body := binary.LittleEndian.AppendUint64(nil, pet)
	body = binary.LittleEndian.AppendUint32(body, action)
	return binary.LittleEndian.AppendUint64(body, target)
}

// BuildPetAutocast is the body of CMSG_PET_SPELL_AUTOCAST.
func BuildPetAutocast(pet uint64, spell uint32, enabled bool) []byte {
	body := binary.LittleEndian.AppendUint64(nil, pet)
	body = binary.LittleEndian.AppendUint32(body, spell)
	if enabled {
		return append(body, 1)
	}
	return append(body, 0)
}

// BuildPetRename is the body of CMSG_PET_RENAME.
func BuildPetRename(pet uint64, name string) []byte {
	body := binary.LittleEndian.AppendUint64(nil, pet)
	body = append(body, cstring(name)...)
	return append(body, 0)
}

// BuildPetSetAction is the body of CMSG_PET_SET_ACTION: an action put on a slot
// of the pet's bar.
func BuildPetSetAction(pet uint64, slot, action uint32) []byte {
	body := binary.LittleEndian.AppendUint64(nil, pet)
	body = binary.LittleEndian.AppendUint32(body, slot)
	return binary.LittleEndian.AppendUint32(body, action)
}

// Totem is SMSG_TOTEM_CREATED: the totem put in a slot.
type Totem struct {
	Slot     uint8
	GUID     uint64
	Duration uint32
	Spell    uint32
}

// ParseTotemCreated reads SMSG_TOTEM_CREATED.
func ParseTotemCreated(body []byte) (Totem, error) {
	r := NewReader(body, "SMSG_TOTEM_CREATED")
	var totem Totem
	var err error
	if totem.Slot, err = r.U8(); err != nil {
		return totem, err
	}
	if totem.GUID, err = r.U64(); err != nil {
		return totem, err
	}
	if totem.Duration, err = r.U32(); err != nil {
		return totem, err
	}
	totem.Spell, err = r.U32()
	return totem, err
}
