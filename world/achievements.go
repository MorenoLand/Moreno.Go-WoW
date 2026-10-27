package world

const (
	// The achievements of the character: all at login, then one at a time as
	// they are earned or their criteria move.
	AchievementEarned      uint16 = 0x0468
	CriteriaUpdate         uint16 = 0x046A
	RespondInspectAchieves uint16 = 0x046C
	AllAchievementData     uint16 = 0x047D
	CriteriaDeleted        uint16 = 0x049E

	ClientQueryInspectAchievements ClientOpcode = 0x046B
)

// AchievementDone is an achievement the character has, with the packed date.
type AchievementDone struct {
	ID   uint32
	Date uint32
}

// CriteriaProgress is how far a criterion has come.
type CriteriaProgress struct {
	ID      uint32
	Counter uint64
	Date    uint32
}

// AchievementData is SMSG_ALL_ACHIEVEMENT_DATA.
type AchievementData struct {
	// Player is whose achievements these are (an inspected player's); zero for
	// the character's own.
	Player   uint64
	Done     []AchievementDone
	Progress []CriteriaProgress
}

func readAchievementData(r *Reader) (AchievementData, error) {
	var data AchievementData
	for {
		id, err := r.U32()
		if err != nil {
			return data, err
		}
		if id == 0xFFFFFFFF {
			break
		}
		date, err := r.U32()
		if err != nil {
			return data, err
		}
		data.Done = append(data.Done, AchievementDone{ID: id, Date: date})
	}
	for {
		id, err := r.U32()
		if err != nil {
			return data, err
		}
		if id == 0xFFFFFFFF {
			break
		}
		counter, err := ReadPackedGUID(r)
		if err != nil {
			return data, err
		}
		if _, err = ReadPackedGUID(r); err != nil {
			return data, err
		}
		// Flags, the packed date, the time elapsed and an unused number.
		if _, err = r.U32(); err != nil {
			return data, err
		}
		date, err := r.U32()
		if err != nil {
			return data, err
		}
		if err = r.Skip(8); err != nil {
			return data, err
		}
		data.Progress = append(data.Progress, CriteriaProgress{ID: id, Counter: counter, Date: date})
	}
	return data, nil
}

// ParseAllAchievementData reads SMSG_ALL_ACHIEVEMENT_DATA.
func ParseAllAchievementData(body []byte) (AchievementData, error) {
	return readAchievementData(NewReader(body, "SMSG_ALL_ACHIEVEMENT_DATA"))
}

// ParseInspectAchievements reads SMSG_RESPOND_INSPECT_ACHIEVEMENTS.
func ParseInspectAchievements(body []byte) (AchievementData, error) {
	r := NewReader(body, "SMSG_RESPOND_INSPECT_ACHIEVEMENTS")
	player, err := ReadPackedGUID(r)
	if err != nil {
		return AchievementData{}, err
	}
	data, err := readAchievementData(r)
	data.Player = player
	return data, err
}

// AchievementEarnedInfo is SMSG_ACHIEVEMENT_EARNED.
type AchievementEarnedInfo struct {
	Player uint64
	ID     uint32
	Date   uint32
}

// ParseAchievementEarned reads SMSG_ACHIEVEMENT_EARNED.
func ParseAchievementEarned(body []byte) (AchievementEarnedInfo, error) {
	r := NewReader(body, "SMSG_ACHIEVEMENT_EARNED")
	var info AchievementEarnedInfo
	var err error
	if info.Player, err = ReadPackedGUID(r); err != nil {
		return info, err
	}
	if info.ID, err = r.U32(); err != nil {
		return info, err
	}
	info.Date, err = r.U32()
	return info, err
}

// ParseCriteriaUpdate reads SMSG_CRITERIA_UPDATE.
func ParseCriteriaUpdate(body []byte) (CriteriaProgress, error) {
	r := NewReader(body, "SMSG_CRITERIA_UPDATE")
	var progress CriteriaProgress
	var err error
	if progress.ID, err = r.U32(); err != nil {
		return progress, err
	}
	if progress.Counter, err = ReadPackedGUID(r); err != nil {
		return progress, err
	}
	if _, err = ReadPackedGUID(r); err != nil {
		return progress, err
	}
	if _, err = r.U32(); err != nil {
		return progress, err
	}
	progress.Date, err = r.U32()
	return progress, err
}

// BuildInspectAchievements asks for another player's achievements.
func BuildInspectAchievements(guid uint64) []byte {
	buffer := make([]byte, 0, 9)
	mask := byte(0)
	body := []byte{}
	for index := 0; index < 8; index++ {
		if value := byte(guid >> uint(index*8)); value != 0 {
			mask |= 1 << uint(index)
			body = append(body, value)
		}
	}
	return append(append(buffer, mask), body...)
}

// UnpackTime splits a packed date into the year, month (1-12), day and hour.
func UnpackTime(packed uint32) (year, month, day, hour, minute int) {
	return 2000 + int(packed>>24), int(packed>>20&0xF) + 1, int(packed>>14&0x3F) + 1, int(packed >> 6 & 0x1F), int(packed & 0x3F)
}

const (
	// Looking at another player: the request names them, the answer carries
	// their talents.
	ClientInspect      ClientOpcode = 0x0114
	InspectTalentReply uint16       = 0x03F4
)

// BuildInspect asks to see another player's talents.
func BuildInspect(guid uint64) []byte { return appendU64(nil, guid) }

// ParseInspectTalent reads SMSG_INSPECT_TALENT: the player and their talents.
func ParseInspectTalent(body []byte) (uint64, Talents, error) {
	r := NewReader(body, "SMSG_INSPECT_TALENT")
	guid, err := ReadPackedGUID(r)
	if err != nil {
		return 0, Talents{}, err
	}
	rest, err := r.Bytes(r.Remaining())
	if err != nil {
		return guid, Talents{}, err
	}
	// The rest is a talent list as SMSG_TALENTS_INFO carries it, without the
	// byte that says whether it is the pet's.
	talents, _, err := ParseTalentsInfo(append([]byte{0}, rest...))
	return guid, talents, err
}
