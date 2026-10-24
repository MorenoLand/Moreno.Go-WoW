package world

const (
	// LogXPGain (SMSG_LOG_XPGAIN) reports experience gained; LevelUpInfo
	// (SMSG_LEVELUP_INFO) what a new level brought.
	LogXPGain   uint16 = 0x01D0
	LevelUpInfo uint16 = 0x01D4
)

// XPGain is SMSG_LOG_XPGAIN.
type XPGain struct {
	Victim uint64
	// Total is the experience with the rested bonus, Base without it.
	Total, Base uint32
	Kill        bool
}

// ParseLogXPGain reads SMSG_LOG_XPGAIN.
func ParseLogXPGain(body []byte) (XPGain, error) {
	r := NewReader(body, "SMSG_LOG_XPGAIN")
	var gain XPGain
	var err error
	if gain.Victim, err = r.U64(); err != nil {
		return gain, err
	}
	if gain.Total, err = r.U32(); err != nil {
		return gain, err
	}
	kind, err := r.U8()
	if err != nil {
		return gain, err
	}
	gain.Kill = kind == 0
	gain.Base = gain.Total
	if gain.Kill {
		if gain.Base, err = r.U32(); err != nil {
			return gain, err
		}
	}
	return gain, nil
}

// LevelUp is SMSG_LEVELUP_INFO.
type LevelUp struct {
	Level  uint32
	Health uint32
	Powers [7]uint32
	Stats  [5]uint32
}

// ParseLevelUpInfo reads SMSG_LEVELUP_INFO.
func ParseLevelUpInfo(body []byte) (LevelUp, error) {
	r := NewReader(body, "SMSG_LEVELUP_INFO")
	var info LevelUp
	var err error
	if info.Level, err = r.U32(); err != nil {
		return info, err
	}
	if info.Health, err = r.U32(); err != nil {
		return info, err
	}
	for index := range info.Powers {
		if info.Powers[index], err = r.U32(); err != nil {
			return info, err
		}
	}
	for index := range info.Stats {
		if info.Stats[index], err = r.U32(); err != nil {
			return info, err
		}
	}
	return info, nil
}
