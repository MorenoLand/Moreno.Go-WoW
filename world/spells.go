package world

import (
	"encoding/binary"
	"fmt"
)

const (
	InitialSpells         uint16       = 0x012A
	ActionButtons         uint16       = 0x0129
	LearnedSpell          uint16       = 0x012B
	RemovedSpell          uint16       = 0x0203
	SpellCooldowns        uint16       = 0x0134
	SpellStart            uint16       = 0x0131
	SpellGo               uint16       = 0x0132
	CastFailed            uint16       = 0x0130
	ChannelStart          uint16       = 0x0139
	ChannelUpdate         uint16       = 0x013A
	CastSpellOpcode       ClientOpcode = 0x012E
	CancelCastOpcode      ClientOpcode = 0x012F
	SetActionButtonOpcode ClientOpcode = 0x0128
	CancelChannelOpcode   ClientOpcode = 0x013B
)

type KnownSpell struct {
	ID   uint32
	Slot uint16
}
type SpellCooldown struct {
	SpellID                        uint32
	ItemID, Category               uint16
	DurationMS, CategoryDurationMS uint32
}
type InitialSpellData struct {
	Spells             []KnownSpell
	Cooldowns          []SpellCooldown
	AnnouncedCooldowns uint16
}
type ActionButton struct {
	ID   uint32
	Type uint8
}

type SpellCastHeader struct {
	Caster                 uint64
	Count                  uint8
	SpellID, Flags, TimeMS uint32
}

func ParseSpellCastHeader(body []byte) (SpellCastHeader, error) {
	r := NewReader(body, "spell cast header")
	result := SpellCastHeader{}
	if _, err := ReadPackedGUID(r); err != nil {
		return result, err
	}
	var err error
	if result.Caster, err = ReadPackedGUID(r); err != nil {
		return result, err
	}
	if result.Count, err = r.U8(); err != nil {
		return result, err
	}
	if result.SpellID, err = r.U32(); err != nil {
		return result, err
	}
	if result.Flags, err = r.U32(); err != nil {
		return result, err
	}
	if result.TimeMS, err = r.U32(); err != nil {
		return result, err
	}
	return result, nil
}

func ParseInitialSpells(body []byte) (InitialSpellData, error) {
	r := NewReader(body, "SMSG_INITIAL_SPELLS")
	result := InitialSpellData{}
	if _, err := r.U8(); err != nil {
		return result, err
	}
	count, err := r.U16()
	if err != nil {
		return result, err
	}
	for i := 0; i < int(count); i++ {
		id, err := r.U32()
		if err != nil {
			return result, err
		}
		slot, err := r.U16()
		if err != nil {
			return result, err
		}
		result.Spells = append(result.Spells, KnownSpell{ID: id, Slot: slot})
	}
	result.AnnouncedCooldowns, err = r.U16()
	if err != nil {
		return result, err
	}
	present := (len(body) - r.at) / 16
	if present > int(result.AnnouncedCooldowns) {
		return result, fmt.Errorf("SMSG_INITIAL_SPELLS has more cooldown records than announced")
	}
	for i := 0; i < present; i++ {
		cooldown := SpellCooldown{}
		if cooldown.SpellID, err = r.U32(); err != nil {
			return result, err
		}
		if cooldown.ItemID, err = r.U16(); err != nil {
			return result, err
		}
		if cooldown.Category, err = r.U16(); err != nil {
			return result, err
		}
		if cooldown.DurationMS, err = r.U32(); err != nil {
			return result, err
		}
		if cooldown.CategoryDurationMS, err = r.U32(); err != nil {
			return result, err
		}
		result.Cooldowns = append(result.Cooldowns, cooldown)
	}
	return result, r.Finish()
}

func ParseActionButtons(body []byte) ([]ActionButton, bool, error) {
	if len(body) < 1 {
		return nil, false, fmt.Errorf("SMSG_ACTION_BUTTONS is truncated")
	}
	if body[0] == 2 {
		if len(body) != 1 {
			return nil, false, fmt.Errorf("SMSG_ACTION_BUTTONS clear has trailing bytes")
		}
		return nil, true, nil
	}
	if body[0] > 1 || len(body) != 1+144*4 {
		return nil, false, fmt.Errorf("SMSG_ACTION_BUTTONS invalid state=%d size=%d", body[0], len(body))
	}
	buttons := make([]ActionButton, 144)
	for i := range buttons {
		packed := binary.LittleEndian.Uint32(body[1+i*4:])
		buttons[i] = ActionButton{ID: packed & 0xFFFFFF, Type: uint8(packed >> 24)}
	}
	return buttons, false, nil
}

func ParseSpellCooldowns(body []byte) (uint64, []SpellCooldown, error) {
	r := NewReader(body, "SMSG_SPELL_COOLDOWN")
	guid, err := r.U64()
	if err != nil {
		return 0, nil, err
	}
	if _, err := r.U8(); err != nil {
		return 0, nil, err
	}
	var cooldowns []SpellCooldown
	for r.at < len(body) {
		id, err := r.U32()
		if err != nil {
			return 0, nil, err
		}
		duration, err := r.U32()
		if err != nil {
			return 0, nil, err
		}
		cooldowns = append(cooldowns, SpellCooldown{SpellID: id, DurationMS: duration})
	}
	return guid, cooldowns, nil
}

func BuildCastSpell(id uint32, target uint64, count uint8) []byte {
	body := make([]byte, 10)
	body[0] = count
	binary.LittleEndian.PutUint32(body[1:5], id)
	if target != 0 {
		binary.LittleEndian.PutUint32(body[6:10], 2)
		mask := uint8(0)
		packed := make([]byte, 0, 8)
		for i := uint(0); i < 8; i++ {
			value := byte(target >> (i * 8))
			if value != 0 {
				mask |= 1 << i
				packed = append(packed, value)
			}
		}
		body = append(body, mask)
		body = append(body, packed...)
	}
	return body
}

func (c *Connection) CastSpell(id uint32, target uint64) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	if id == 0 {
		return fmt.Errorf("spell ID is zero")
	}
	return c.send(CastSpellOpcode, BuildCastSpell(id, target, 0))
}

func (c *Connection) CancelCast(id uint32) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, id)
	return c.send(CancelCastOpcode, body)
}

func (c *Connection) SetActionButton(slot int, action ActionButton) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	if slot < 0 || slot >= 144 || action.ID > 0xFFFFFF {
		return fmt.Errorf("invalid action button")
	}
	body := make([]byte, 5)
	body[0] = byte(slot)
	binary.LittleEndian.PutUint32(body[1:], action.ID|uint32(action.Type)<<24)
	return c.send(SetActionButtonOpcode, body)
}

func (c *Connection) CancelChannel(id uint32) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, id)
	return c.send(CancelChannelOpcode, body)
}
