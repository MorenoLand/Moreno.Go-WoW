package world

type AuraSlot struct {
	SpellID     uint32
	Flags       uint8
	Level       uint8
	Charges     uint8
	CasterGUID  uint64
	MaxDuration int32
	Duration    int32
}

type AuraUpdateData struct {
	GUID    uint64
	Updates []AuraUpdateEntry
}

type AuraUpdateEntry struct {
	Slot uint8
	Aura AuraSlot
}

func (a AuraSlot) Empty() bool { return a.SpellID == 0 }

func ParseAuraUpdate(body []byte, all bool) (AuraUpdateData, error) {
	r := NewReader(body, "SMSG_AURA_UPDATE")
	guid, err := ReadPackedGUID(r)
	if err != nil {
		return AuraUpdateData{}, err
	}
	limit := 1
	if all {
		limit = 512
	}
	result := AuraUpdateData{GUID: guid}
	for len(result.Updates) < limit && r.at < len(r.data) {
		slot, readErr := r.U8()
		if readErr != nil {
			return AuraUpdateData{}, readErr
		}
		spellID, readErr := r.U32()
		if readErr != nil {
			return AuraUpdateData{}, readErr
		}
		aura := AuraSlot{SpellID: spellID, MaxDuration: -1, Duration: -1}
		if spellID != 0 {
			if aura.Flags, err = r.U8(); err != nil {
				return AuraUpdateData{}, err
			}
			if aura.Level, err = r.U8(); err != nil {
				return AuraUpdateData{}, err
			}
			if aura.Charges, err = r.U8(); err != nil {
				return AuraUpdateData{}, err
			}
			if aura.Flags&0x08 == 0 {
				if aura.CasterGUID, err = ReadPackedGUID(r); err != nil {
					return AuraUpdateData{}, err
				}
			}
			if aura.Flags&0x20 != 0 {
				maxDuration, readErr := r.U32()
				if readErr != nil {
					return AuraUpdateData{}, readErr
				}
				duration, readErr := r.U32()
				if readErr != nil {
					return AuraUpdateData{}, readErr
				}
				aura.MaxDuration = int32(maxDuration)
				aura.Duration = int32(duration)
			}
			if aura.Flags&0x40 != 0 {
				for effect := uint8(0); effect < 3; effect++ {
					if aura.Flags&(1<<effect) != 0 {
						if err := r.Skip(4); err != nil {
							return AuraUpdateData{}, err
						}
					}
				}
			}
		}
		result.Updates = append(result.Updates, AuraUpdateEntry{Slot: slot, Aura: aura})
		if !all {
			break
		}
	}
	if err := r.Finish(); err != nil {
		return AuraUpdateData{}, err
	}
	return result, nil
}
