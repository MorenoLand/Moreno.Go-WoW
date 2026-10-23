package world

import (
	"encoding/binary"
	"fmt"
)

// Client opcodes for names, melee and the server messages that report them.
const (
	NameQueryOpcode     ClientOpcode = 0x0050
	CreatureQueryOpcode ClientOpcode = 0x0060
	AttackSwingOpcode   ClientOpcode = 0x0141
	AttackStopOpcode    ClientOpcode = 0x0142
)

const (
	NameQueryResponse      uint16 = 0x0051
	CreatureQueryResponse  uint16 = 0x0061
	AttackStartPacket      uint16 = 0x0143
	AttackStopPacket       uint16 = 0x0144
	AttackNotInRange       uint16 = 0x0145
	AttackBadFacing        uint16 = 0x0146
	AttackNotStanding      uint16 = 0x0147
	AttackDeadTarget       uint16 = 0x0148
	AttackCantAttack       uint16 = 0x0149
	AttackerStateUpdate    uint16 = 0x014A
	SpellHealLog           uint16 = 0x0150
	SpellNonMeleeDamageLog uint16 = 0x0250
)

// BuildNameQuery is CMSG_NAME_QUERY: an unpacked GUID.
func BuildNameQuery(guid uint64) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint64(body, guid)
	return body
}

// BuildCreatureQuery is CMSG_CREATURE_QUERY: the entry and the creature's GUID.
func BuildCreatureQuery(entry uint32, guid uint64) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body, entry)
	binary.LittleEndian.PutUint64(body[4:], guid)
	return body
}

// BuildAttackSwing is CMSG_ATTACKSWING: the target's unpacked GUID.
func BuildAttackSwing(guid uint64) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint64(body, guid)
	return body
}

// CreatureInfo is the part of SMSG_CREATURE_QUERY_RESPONSE the interface shows.
type CreatureInfo struct {
	Entry    uint32
	Found    bool
	Name     string
	SubName  string
	TypeFlag uint32
	Type     uint32
	Family   uint32
	Rank     uint32
}

func ParseCreatureQuery(body []byte) (CreatureInfo, error) {
	r := NewReader(body, "SMSG_CREATURE_QUERY_RESPONSE")
	entry, err := r.U32()
	if err != nil {
		return CreatureInfo{}, err
	}
	info := CreatureInfo{Entry: entry &^ 0x80000000}
	if entry&0x80000000 != 0 {
		return info, nil
	}
	info.Found = true
	for i := 0; i < 4; i++ {
		name, err := r.CString()
		if err != nil {
			return info, err
		}
		if i == 0 {
			info.Name = name
		}
	}
	if info.SubName, err = r.CString(); err != nil {
		return info, err
	}
	if _, err = r.CString(); err != nil {
		return info, err
	}
	if info.TypeFlag, err = r.U32(); err != nil {
		return info, err
	}
	if info.Type, err = r.U32(); err != nil {
		return info, err
	}
	if info.Family, err = r.U32(); err != nil {
		return info, err
	}
	if info.Rank, err = r.U32(); err != nil {
		return info, err
	}
	return info, nil
}

// PlayerName is the answer to a name query.
type PlayerName struct {
	GUID   uint64
	Name   string
	Realm  string
	Race   uint8
	Gender uint8
	Class  uint8
}

func ParseNameQuery(body []byte) (PlayerName, error) {
	r := NewReader(body, "SMSG_NAME_QUERY_RESPONSE")
	guid, err := ReadPackedGUID(r)
	if err != nil {
		return PlayerName{}, err
	}
	result := PlayerName{GUID: guid}
	unknown, err := r.U8()
	if err != nil {
		return result, err
	}
	if unknown != 0 {
		return result, fmt.Errorf("name of %#x is not known", guid)
	}
	if result.Name, err = r.CString(); err != nil {
		return result, err
	}
	if result.Realm, err = r.CString(); err != nil {
		return result, err
	}
	if result.Race, err = r.U8(); err != nil {
		return result, err
	}
	if result.Gender, err = r.U8(); err != nil {
		return result, err
	}
	if result.Class, err = r.U8(); err != nil {
		return result, err
	}
	return result, nil
}

// ParseAttackStart reads SMSG_ATTACKSTART: attacker and victim, unpacked.
func ParseAttackStart(body []byte) (attacker, victim uint64, err error) {
	r := NewReader(body, "SMSG_ATTACKSTART")
	if attacker, err = r.U64(); err != nil {
		return
	}
	victim, err = r.U64()
	return
}

// ParseAttackStop reads SMSG_ATTACKSTOP: attacker and victim, packed, then
// whether the victim died.
func ParseAttackStop(body []byte) (attacker, victim uint64, dead bool, err error) {
	r := NewReader(body, "SMSG_ATTACKSTOP")
	if attacker, err = ReadPackedGUID(r); err != nil {
		return
	}
	if victim, err = ReadPackedGUID(r); err != nil {
		return
	}
	if flag, flagErr := r.U32(); flagErr == nil {
		dead = flag != 0
	}
	return
}

// MeleeHit is one SMSG_ATTACKERSTATEUPDATE.
type MeleeHit struct {
	HitInfo      uint32
	Attacker     uint64
	Victim       uint64
	Damage       uint32
	Overkill     int32
	School       uint32
	Absorbed     uint32
	Resisted     uint32
	VictimState  uint32
	BlockedValue uint32
}

// Hit info bits the combat log distinguishes.
const (
	HitInfoMiss     uint32 = 0x00000010
	HitInfoCritical uint32 = 0x00000200
	HitInfoGlancing uint32 = 0x00010000
	HitInfoCrushing uint32 = 0x00020000
)

// Victim states.
const (
	VictimStateNormal  uint32 = 1
	VictimStateDodge   uint32 = 2
	VictimStateParry   uint32 = 3
	VictimStateBlocks  uint32 = 5
	VictimStateEvades  uint32 = 6
	VictimStateImmune  uint32 = 7
	VictimStateDeflect uint32 = 8
)

func ParseAttackerStateUpdate(body []byte) (MeleeHit, error) {
	const (
		fullAbsorb    = 0x20
		partialAbsorb = 0x40
		fullResist    = 0x80
		partialResist = 0x100
		blockInfo     = 0x2000
		rageGain      = 0x800000
	)
	r := NewReader(body, "SMSG_ATTACKERSTATEUPDATE")
	hit := MeleeHit{}
	var err error
	if hit.HitInfo, err = r.U32(); err != nil {
		return hit, err
	}
	if hit.Attacker, err = ReadPackedGUID(r); err != nil {
		return hit, err
	}
	if hit.Victim, err = ReadPackedGUID(r); err != nil {
		return hit, err
	}
	if hit.Damage, err = r.U32(); err != nil {
		return hit, err
	}
	overkill, err := r.U32()
	if err != nil {
		return hit, err
	}
	hit.Overkill = int32(overkill)
	count, err := r.U8()
	if err != nil {
		return hit, err
	}
	for i := 0; i < int(count); i++ {
		school, err := r.U32()
		if err != nil {
			return hit, err
		}
		if _, err = r.F32(); err != nil {
			return hit, err
		}
		if _, err = r.U32(); err != nil {
			return hit, err
		}
		if i == 0 {
			hit.School = school
		}
	}
	if hit.HitInfo&(fullAbsorb|partialAbsorb) != 0 {
		for i := 0; i < int(count); i++ {
			value, err := r.U32()
			if err != nil {
				return hit, err
			}
			hit.Absorbed += value
		}
	}
	if hit.HitInfo&(fullResist|partialResist) != 0 {
		for i := 0; i < int(count); i++ {
			value, err := r.U32()
			if err != nil {
				return hit, err
			}
			hit.Resisted += value
		}
	}
	state, err := r.U8()
	if err != nil {
		return hit, err
	}
	hit.VictimState = uint32(state)
	if err = r.Skip(8); err != nil { // unknown attacker state, melee spell id
		return hit, err
	}
	if hit.HitInfo&blockInfo != 0 {
		if hit.BlockedValue, err = r.U32(); err != nil {
			return hit, err
		}
	}
	return hit, nil
}

// SpellDamage is SMSG_SPELLNONMELEEDAMAGELOG.
type SpellDamage struct {
	Victim, Caster uint64
	SpellID        uint32
	Damage         uint32
	Overkill       int32
	School         uint8
	Absorbed       uint32
	Resisted       uint32
	Blocked        uint32
	Flags          uint32
}

func ParseSpellDamageLog(body []byte) (SpellDamage, error) {
	r := NewReader(body, "SMSG_SPELLNONMELEEDAMAGELOG")
	result := SpellDamage{}
	var err error
	if result.Victim, err = ReadPackedGUID(r); err != nil {
		return result, err
	}
	if result.Caster, err = ReadPackedGUID(r); err != nil {
		return result, err
	}
	if result.SpellID, err = r.U32(); err != nil {
		return result, err
	}
	if result.Damage, err = r.U32(); err != nil {
		return result, err
	}
	overkill, err := r.U32()
	if err != nil {
		return result, err
	}
	result.Overkill = int32(overkill)
	if result.School, err = r.U8(); err != nil {
		return result, err
	}
	if result.Absorbed, err = r.U32(); err != nil {
		return result, err
	}
	if result.Resisted, err = r.U32(); err != nil {
		return result, err
	}
	if _, err = r.U8(); err != nil {
		return result, err
	}
	if _, err = r.U8(); err != nil {
		return result, err
	}
	if result.Blocked, err = r.U32(); err != nil {
		return result, err
	}
	result.Flags, err = r.U32()
	return result, err
}

// SpellHeal is SMSG_SPELLHEALLOG.
type SpellHeal struct {
	Victim, Caster uint64
	SpellID        uint32
	Amount         uint32
	Overheal       uint32
	Absorbed       uint32
	Critical       bool
}

func ParseSpellHealLog(body []byte) (SpellHeal, error) {
	r := NewReader(body, "SMSG_SPELLHEALLOG")
	result := SpellHeal{}
	var err error
	if result.Victim, err = ReadPackedGUID(r); err != nil {
		return result, err
	}
	if result.Caster, err = ReadPackedGUID(r); err != nil {
		return result, err
	}
	if result.SpellID, err = r.U32(); err != nil {
		return result, err
	}
	if result.Amount, err = r.U32(); err != nil {
		return result, err
	}
	if result.Overheal, err = r.U32(); err != nil {
		return result, err
	}
	if result.Absorbed, err = r.U32(); err != nil {
		return result, err
	}
	crit, err := r.U8()
	if err != nil {
		return result, err
	}
	result.Critical = crit != 0
	return result, nil
}

// CancelAuraOpcode is CMSG_CANCEL_AURA: the spell id of the aura to remove.
const CancelAuraOpcode ClientOpcode = 0x0136

// CancelMountAuraOpcode is CMSG_CANCEL_MOUNT_AURA: no body.
const CancelMountAuraOpcode ClientOpcode = 0x0375
