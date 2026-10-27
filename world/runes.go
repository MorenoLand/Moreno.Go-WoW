package world

// Death knight rune packets.
const (
	ConvertRunePacket     uint16 = 0x0486
	ResyncRunesPacket     uint16 = 0x0487
	AddRunePowerPacket    uint16 = 0x0488
	castFlagPowerLeft            = 0x00000800
	castFlagRuneList             = 0x00200000
	castTargetUnit               = 0x00000002
	castTargetItem               = 0x00000010
	castTargetSource             = 0x00000020
	castTargetDest               = 0x00000040
	castTargetCorpseEnemy        = 0x00000200
	castTargetGameObject         = 0x00000800
	castTargetTradeItem          = 0x00001000
	castTargetString             = 0x00002000
	castTargetCorpseAlly         = 0x00008000
	castTargetUnitMini           = 0x00010000
)

// Rune is one of the death knight's runes as the server resyncs it.
type Rune struct {
	Type uint8
	// Cooldown is how much of the cooldown has passed, 0..255.
	Cooldown uint8
}

// ParseResyncRunes reads SMSG_RESYNC_RUNES.
func ParseResyncRunes(body []byte) ([]Rune, error) {
	r := NewReader(body, "SMSG_RESYNC_RUNES")
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	var runes []Rune
	for index := uint32(0); index < count && index < 8; index++ {
		var rune Rune
		if rune.Type, err = r.U8(); err != nil {
			return runes, err
		}
		if rune.Cooldown, err = r.U8(); err != nil {
			return runes, err
		}
		runes = append(runes, rune)
	}
	return runes, nil
}

// ParseConvertRune reads SMSG_CONVERT_RUNE: a rune and its new type.
func ParseConvertRune(body []byte) (uint8, uint8, error) {
	r := NewReader(body, "SMSG_CONVERT_RUNE")
	index, err := r.U8()
	if err != nil {
		return 0, 0, err
	}
	kind, err := r.U8()
	return index, kind, err
}

// ParseAddRunePower reads SMSG_ADD_RUNE_POWER: the runes that became ready.
func ParseAddRunePower(body []byte) (uint32, error) {
	return NewReader(body, "SMSG_ADD_RUNE_POWER").U32()
}

// SpellRunes are the runes a cast changed: which were ready before and after.
type SpellRunes struct {
	Before, After uint8
	Cooldowns     []uint8
}

// ParseSpellGoRunes reads the rune list at the end of SMSG_SPELL_GO; found is
// false when the spell carries none.
func ParseSpellGoRunes(body []byte) (SpellRunes, bool, error) {
	var runes SpellRunes
	header, err := ParseSpellCastHeader(body)
	if err != nil {
		return runes, false, err
	}
	if header.Flags&castFlagRuneList == 0 {
		return runes, false, nil
	}
	r := NewReader(body, "SMSG_SPELL_GO")
	if _, err = ReadPackedGUID(r); err != nil {
		return runes, false, err
	}
	if _, err = ReadPackedGUID(r); err != nil {
		return runes, false, err
	}
	if err = r.Skip(1 + 4 + 4 + 4); err != nil {
		return runes, false, err
	}
	hits, err := r.U8()
	if err != nil {
		return runes, false, err
	}
	if err = r.Skip(int(hits) * 8); err != nil {
		return runes, false, err
	}
	misses, err := r.U8()
	if err != nil {
		return runes, false, err
	}
	for index := 0; index < int(misses); index++ {
		if err = r.Skip(8); err != nil {
			return runes, false, err
		}
		kind, err := r.U8()
		if err != nil {
			return runes, false, err
		}
		if kind == 11 { // SPELL_MISS_REFLECT
			if err = r.Skip(1); err != nil {
				return runes, false, err
			}
		}
	}
	mask, err := r.U32()
	if err != nil {
		return runes, false, err
	}
	if mask&(castTargetUnit|castTargetUnitMini|castTargetGameObject|castTargetCorpseEnemy|castTargetCorpseAlly) != 0 {
		if _, err = ReadPackedGUID(r); err != nil {
			return runes, false, err
		}
	}
	if mask&(castTargetItem|castTargetTradeItem) != 0 {
		if _, err = ReadPackedGUID(r); err != nil {
			return runes, false, err
		}
	}
	for _, flag := range []uint32{castTargetSource, castTargetDest} {
		if mask&flag != 0 {
			if _, err = ReadPackedGUID(r); err != nil {
				return runes, false, err
			}
			if err = r.Skip(12); err != nil {
				return runes, false, err
			}
		}
	}
	if mask&castTargetString != 0 {
		if _, err = r.CString(); err != nil {
			return runes, false, err
		}
	}
	if header.Flags&castFlagPowerLeft != 0 {
		if err = r.Skip(4); err != nil {
			return runes, false, err
		}
	}
	if runes.Before, err = r.U8(); err != nil {
		return runes, false, err
	}
	if runes.After, err = r.U8(); err != nil {
		return runes, false, err
	}
	for index := 0; index < 8; index++ {
		bit := uint8(1) << index
		if runes.Before&bit != 0 && runes.After&bit == 0 {
			value, err := r.U8()
			if err != nil {
				return runes, true, nil
			}
			runes.Cooldowns = append(runes.Cooldowns, value)
		}
	}
	return runes, true, nil
}
