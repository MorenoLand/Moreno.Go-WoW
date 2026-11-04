package world

// Server combat messages the combat log reports besides damage and healing.
const (
	SpellLogMissPacket        uint16 = 0x024B
	SpellEnergizeLogPacket    uint16 = 0x0151
	PeriodicAuraLogPacket     uint16 = 0x024E
	EnvironmentalDamagePacket uint16 = 0x01FC
	SpellDispelLogPacket      uint16 = 0x027B
	SpellStealLogPacket       uint16 = 0x0333
	SpellInstakillLogPacket   uint16 = 0x032F
	SpellOrDamageImmunePacket uint16 = 0x0263
	SpellProcResistPacket     uint16 = 0x0260
	SpellDamageShieldPacket   uint16 = 0x024F
	PartyKillLogPacket        uint16 = 0x01F5
)

// Spell miss results (SpellMissInfo).
var SpellMissNames = [...]string{"", "MISS", "RESIST", "DODGE", "PARRY", "BLOCK", "EVADE", "IMMUNE", "IMMUNE", "DEFLECT", "ABSORB", "REFLECT"}

// SpellMiss is a spell that did not land on one target.
type SpellMiss struct {
	Caster, Target uint64
	SpellID        uint32
	Miss           uint8
	Reflect        uint8
}

// ParseSpellLogMiss reads SMSG_SPELLLOGMISS.
func ParseSpellLogMiss(body []byte) ([]SpellMiss, error) {
	r := NewReader(body, "SMSG_SPELLLOGMISS")
	spell, err := r.U32()
	if err != nil {
		return nil, err
	}
	caster, err := r.U64()
	if err != nil {
		return nil, err
	}
	if _, err = r.U8(); err != nil {
		return nil, err
	}
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	var misses []SpellMiss
	for i := uint32(0); i < count && i < 256; i++ {
		miss := SpellMiss{Caster: caster, SpellID: spell}
		if miss.Target, err = r.U64(); err != nil {
			return misses, err
		}
		if miss.Miss, err = r.U8(); err != nil {
			return misses, err
		}
		if miss.Miss == 11 {
			if miss.Reflect, err = r.U8(); err != nil {
				return misses, err
			}
		}
		misses = append(misses, miss)
	}
	return misses, nil
}

// SpellEnergize is power a spell gave to a unit.
type SpellEnergize struct {
	Victim, Caster uint64
	SpellID        uint32
	PowerType      uint32
	Amount         int32
}

// ParseSpellEnergizeLog reads SMSG_SPELLENERGIZELOG.
func ParseSpellEnergizeLog(body []byte) (SpellEnergize, error) {
	r := NewReader(body, "SMSG_SPELLENERGIZELOG")
	var out SpellEnergize
	var err error
	if out.Victim, err = ReadPackedGUID(r); err != nil {
		return out, err
	}
	if out.Caster, err = ReadPackedGUID(r); err != nil {
		return out, err
	}
	if out.SpellID, err = r.U32(); err != nil {
		return out, err
	}
	if out.PowerType, err = r.U32(); err != nil {
		return out, err
	}
	amount, err := r.U32()
	out.Amount = int32(amount)
	return out, err
}

// PeriodicTick is one tick of an aura.
type PeriodicTick struct {
	Target, Caster uint64
	SpellID        uint32
	AuraType       uint32
	Amount         uint32
	Overkill       uint32
	School         uint32
	Absorbed       uint32
	Resisted       uint32
	PowerType      uint32
	Critical       bool
}

// ParsePeriodicAuraLog reads SMSG_PERIODICAURALOG.
func ParsePeriodicAuraLog(body []byte) ([]PeriodicTick, error) {
	r := NewReader(body, "SMSG_PERIODICAURALOG")
	target, err := ReadPackedGUID(r)
	if err != nil {
		return nil, err
	}
	caster, err := ReadPackedGUID(r)
	if err != nil {
		return nil, err
	}
	spell, err := r.U32()
	if err != nil {
		return nil, err
	}
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	var ticks []PeriodicTick
	for i := uint32(0); i < count && i < 64; i++ {
		tick := PeriodicTick{Target: target, Caster: caster, SpellID: spell}
		if tick.AuraType, err = r.U32(); err != nil {
			return ticks, err
		}
		read := func(into ...*uint32) error {
			for _, field := range into {
				v, err := r.U32()
				if err != nil {
					return err
				}
				*field = v
			}
			return nil
		}
		switch tick.AuraType {
		case 3, 89:
			err = read(&tick.Amount, &tick.Overkill, &tick.School, &tick.Absorbed, &tick.Resisted)
		case 8, 20:
			err = read(&tick.Amount, &tick.Overkill, &tick.Absorbed)
		case 21, 24:
			err = read(&tick.PowerType, &tick.Amount)
		case 64:
			if err = read(&tick.PowerType, &tick.Amount); err == nil {
				err = r.Skip(4)
			}
		default:
			return ticks, nil
		}
		if err != nil {
			return ticks, err
		}
		if tick.AuraType == 3 || tick.AuraType == 89 || tick.AuraType == 8 || tick.AuraType == 20 {
			crit, err := r.U8()
			if err != nil {
				return ticks, err
			}
			tick.Critical = crit != 0
		}
		ticks = append(ticks, tick)
	}
	return ticks, nil
}

// EnvironmentalDamage is damage from the world: falling, drowning, lava.
type EnvironmentalDamage struct {
	Victim                     uint64
	Type                       uint8
	Amount, Resisted, Absorbed uint32
}

// ParseEnvironmentalDamage reads SMSG_ENVIRONMENTAL_DAMAGE_LOG.
func ParseEnvironmentalDamage(body []byte) (EnvironmentalDamage, error) {
	r := NewReader(body, "SMSG_ENVIRONMENTAL_DAMAGE_LOG")
	var out EnvironmentalDamage
	var err error
	if out.Victim, err = r.U64(); err != nil {
		return out, err
	}
	if out.Type, err = r.U8(); err != nil {
		return out, err
	}
	if out.Amount, err = r.U32(); err != nil {
		return out, err
	}
	if out.Resisted, err = r.U32(); err != nil {
		return out, err
	}
	out.Absorbed, err = r.U32()
	return out, err
}

// SpellDispel is the auras a spell removed from a unit.
type SpellDispel struct {
	Victim, Caster uint64
	SpellID        uint32
	Removed        []uint32
	Stolen         bool
}

// ParseSpellDispelLog reads SMSG_SPELLDISPELLOG and SMSG_SPELLSTEALLOG.
func ParseSpellDispelLog(body []byte, stolen bool) (SpellDispel, error) {
	r := NewReader(body, "SMSG_SPELLDISPELLOG")
	out := SpellDispel{Stolen: stolen}
	var err error
	if out.Victim, err = ReadPackedGUID(r); err != nil {
		return out, err
	}
	if out.Caster, err = ReadPackedGUID(r); err != nil {
		return out, err
	}
	if out.SpellID, err = r.U32(); err != nil {
		return out, err
	}
	if _, err = r.U8(); err != nil {
		return out, err
	}
	count, err := r.U32()
	if err != nil {
		return out, err
	}
	for i := uint32(0); i < count && i < 64; i++ {
		id, err := r.U32()
		if err != nil {
			return out, err
		}
		if _, err = r.U8(); err != nil {
			return out, err
		}
		out.Removed = append(out.Removed, id)
	}
	return out, nil
}

// SpellPair is a message of a caster, a victim and a spell: an instant kill,
// an immunity or a resisted proc.
type SpellPair struct {
	Caster, Victim uint64
	SpellID        uint32
}

// ParseSpellPair reads SMSG_SPELLINSTAKILLLOG, SMSG_SPELLORDAMAGE_IMMUNE and
// SMSG_PROCRESIST.
func ParseSpellPair(body []byte) (SpellPair, error) {
	r := NewReader(body, "spell pair")
	var out SpellPair
	var err error
	if out.Caster, err = r.U64(); err != nil {
		return out, err
	}
	if out.Victim, err = r.U64(); err != nil {
		return out, err
	}
	out.SpellID, err = r.U32()
	return out, err
}

// DamageShield is damage a unit's shield dealt back.
type DamageShield struct {
	Victim, Caster uint64
	SpellID        uint32
	Damage         uint32
	Overkill       uint32
	School         uint32
}

// ParseDamageShield reads SMSG_SPELLDAMAGESHIELD.
func ParseDamageShield(body []byte) (DamageShield, error) {
	r := NewReader(body, "SMSG_SPELLDAMAGESHIELD")
	var out DamageShield
	var err error
	if out.Victim, err = r.U64(); err != nil {
		return out, err
	}
	if out.Caster, err = r.U64(); err != nil {
		return out, err
	}
	if out.SpellID, err = r.U32(); err != nil {
		return out, err
	}
	if out.Damage, err = r.U32(); err != nil {
		return out, err
	}
	if out.Overkill, err = r.U32(); err != nil {
		return out, err
	}
	out.School, err = r.U32()
	return out, err
}

// SpellLogExecutePacket is SMSG_SPELLLOGEXECUTE.
const SpellLogExecutePacket uint16 = 0x024C

// SpellFailedOtherPacket is SMSG_SPELL_FAILED_OTHER.
const SpellFailedOtherPacket uint16 = 0x02A6

// SpellExecuteEntry is something a spell effect did that the log names.
type SpellExecuteEntry struct {
	Effect uint32
	// Target is the unit the effect interrupted or the thing it summoned.
	Target uint64
	// Spell is the spell that was interrupted, or the amount an effect took or
	// the attacks it gave.
	Spell uint32
	// Power is the kind of power an effect took.
	Power uint32
}

// ParseSpellLogExecute reads the interrupts and the summons out of
// SMSG_SPELLLOGEXECUTE; the other kinds of effect end the reading.
func ParseSpellLogExecute(body []byte) (caster uint64, spell uint32, entries []SpellExecuteEntry, err error) {
	r := NewReader(body, "SMSG_SPELLLOGEXECUTE")
	if caster, err = ReadPackedGUID(r); err != nil {
		return
	}
	if spell, err = r.U32(); err != nil {
		return
	}
	effects, err := r.U32()
	if err != nil {
		return
	}
	for i := uint32(0); i < effects && i < 3; i++ {
		var effect, count uint32
		if effect, err = r.U32(); err != nil {
			return
		}
		if count, err = r.U32(); err != nil {
			return
		}
		for j := uint32(0); j < count && j < 64; j++ {
			entry := SpellExecuteEntry{Effect: effect}
			switch effect {
			case 68:
				if entry.Target, err = ReadPackedGUID(r); err != nil {
					return
				}
				if entry.Spell, err = r.U32(); err != nil {
					return
				}
			case 19:
				if entry.Target, err = ReadPackedGUID(r); err != nil {
					return
				}
				if entry.Spell, err = r.U32(); err != nil {
					return
				}
			case 111, 115:
				if entry.Target, err = ReadPackedGUID(r); err != nil {
					return
				}
				if err = r.Skip(8); err != nil {
					return
				}
			case 8, 62:
				if entry.Target, err = ReadPackedGUID(r); err != nil {
					return
				}
				if entry.Spell, err = r.U32(); err != nil {
					return
				}
				var power uint32
				if power, err = r.U32(); err != nil {
					return
				}
				entry.Power = power
				if err = r.Skip(4); err != nil {
					return
				}
			case 18, 113, 28, 56, 76, 104, 105, 106, 107:
				if entry.Target, err = ReadPackedGUID(r); err != nil {
					return
				}
			default:
				return
			}
			entries = append(entries, entry)
		}
	}
	return
}
