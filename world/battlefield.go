package world

import "encoding/binary"

// Battleground, arena and world state packets.
const (
	BattlefieldPortDeniedPacket  uint16 = 0x014B
	BattlefieldListPacket        uint16 = 0x023D
	InitWorldStatesPacket        uint16 = 0x02C2
	UpdateWorldStatePacket       uint16 = 0x02C3
	BattlefieldStatusPacket      uint16 = 0x02D4
	PVPLogDataPacket             uint16 = 0x02E0
	AreaSpiritHealerTimePacket   uint16 = 0x02E4
	GroupJoinedBattlegroundPack  uint16 = 0x02E8
	BattlegroundPositionsPacket  uint16 = 0x02E9
	BattlegroundPlayerJoinedPack uint16 = 0x02EC
	BattlegroundPlayerLeftPack   uint16 = 0x02ED
	BattlefieldMgrEntryInvite    uint16 = 0x04DE
	BattlefieldMgrEntered        uint16 = 0x04E0
	BattlefieldMgrQueueInvite    uint16 = 0x04E1
	BattlefieldMgrQueueResponse  uint16 = 0x04E4
	BattlefieldMgrEjectPending   uint16 = 0x04E5
	BattlefieldMgrEjected        uint16 = 0x04E6
	BattlefieldMgrStateChange    uint16 = 0x04E8

	ClientBattlefieldList         ClientOpcode = 0x023C
	ClientBattlefieldPort         ClientOpcode = 0x02D5
	ClientPVPLogData              ClientOpcode = 0x02E0
	ClientLeaveBattlefield        ClientOpcode = 0x02E1
	ClientBattlegroundPositions   ClientOpcode = 0x02E9
	ClientBattlemasterJoin        ClientOpcode = 0x02EE
	ClientBattlemasterJoinArena   ClientOpcode = 0x0358
	ClientReportPVPAFK            ClientOpcode = 0x03E4
	ClientBattlefieldMgrEntry     ClientOpcode = 0x04DF
	ClientBattlefieldMgrQueueResp ClientOpcode = 0x04E2
	ClientBattlefieldMgrQueueReq  ClientOpcode = 0x04E3
	ClientBattlefieldMgrExit      ClientOpcode = 0x04E7
)

// Statuses a battleground queue slot goes through.
const (
	BattlefieldNone    = 0
	BattlefieldQueued  = 1
	BattlefieldConfirm = 2
	BattlefieldActive  = 3
)

// BattlefieldStatus is SMSG_BATTLEFIELD_STATUS: where one queue slot stands.
type BattlefieldStatus struct {
	Slot    uint32
	QueueID uint64
	// Cleared is set when the queue id is zero: the slot is empty.
	Cleared      bool
	MinLevel     uint8
	MaxLevel     uint8
	Instance     uint32
	Rated        bool
	Status       uint32
	Map          uint32
	Other        uint64
	Timeout      uint32 // confirm: how long to answer, ms
	Remaining    uint32 // active: how long until the match ends, ms
	Elapsed      uint32 // active: how long it has run, ms
	ArenaFaction uint8
	AverageWait  uint32 // queued
	TimeInQueue  uint32 // queued
}

// ParseBattlefieldStatus reads SMSG_BATTLEFIELD_STATUS.
func ParseBattlefieldStatus(body []byte) (BattlefieldStatus, error) {
	r := NewReader(body, "SMSG_BATTLEFIELD_STATUS")
	var status BattlefieldStatus
	var err error
	if status.Slot, err = r.U32(); err != nil {
		return status, err
	}
	if status.Slot > 1 {
		return status, nil
	}
	if status.QueueID, err = r.U64(); err != nil {
		return status, err
	}
	if status.QueueID == 0 {
		status.Cleared = true
		return status, nil
	}
	if status.MinLevel, err = r.U8(); err != nil {
		return status, err
	}
	if status.MaxLevel, err = r.U8(); err != nil {
		return status, err
	}
	if status.Instance, err = r.U32(); err != nil {
		return status, err
	}
	rated, err := r.U8()
	if err != nil {
		return status, err
	}
	status.Rated = rated != 0
	if status.Status, err = r.U32(); err != nil {
		return status, err
	}
	switch status.Status {
	case BattlefieldConfirm:
		if status.Map, err = r.U32(); err != nil {
			return status, err
		}
		if status.Other, err = r.U64(); err != nil {
			return status, err
		}
		status.Timeout, err = r.U32()
	case BattlefieldActive:
		if status.Map, err = r.U32(); err != nil {
			return status, err
		}
		if status.Other, err = r.U64(); err != nil {
			return status, err
		}
		if status.Remaining, err = r.U32(); err != nil {
			return status, err
		}
		if status.Elapsed, err = r.U32(); err != nil {
			return status, err
		}
		status.ArenaFaction, err = r.U8()
	case BattlefieldQueued:
		if status.AverageWait, err = r.U32(); err != nil {
			return status, err
		}
		status.TimeInQueue, err = r.U32()
	}
	return status, err
}

// BattlefieldList is SMSG_BATTLEFIELD_LIST: the battleground a battlemaster
// offers and the instances of it that run.
type BattlefieldList struct {
	GUID      uint64
	FromWhere uint8
	BGType    uint32
	MinLevel  uint8
	MaxLevel  uint8
	Holiday   bool
	// HolidayHonor, HolidayArena and HolidayLoser are what the holiday
	// battleground pays.
	HolidayHonor, HolidayArena, HolidayLoser uint32
	Random                                   bool
	RandomHonor, RandomArena, RandomLoser    uint32
	Instances                                []uint32
}

// ParseBattlefieldList reads SMSG_BATTLEFIELD_LIST.
func ParseBattlefieldList(body []byte) (BattlefieldList, error) {
	r := NewReader(body, "SMSG_BATTLEFIELD_LIST")
	var list BattlefieldList
	var err error
	if list.GUID, err = r.U64(); err != nil {
		return list, err
	}
	if list.FromWhere, err = r.U8(); err != nil {
		return list, err
	}
	if list.BGType, err = r.U32(); err != nil {
		return list, err
	}
	if list.MinLevel, err = r.U8(); err != nil {
		return list, err
	}
	if list.MaxLevel, err = r.U8(); err != nil {
		return list, err
	}
	holiday, err := r.U8()
	if err != nil {
		return list, err
	}
	list.Holiday = holiday != 0
	if list.HolidayHonor, err = r.U32(); err != nil {
		return list, err
	}
	if list.HolidayArena, err = r.U32(); err != nil {
		return list, err
	}
	if list.HolidayLoser, err = r.U32(); err != nil {
		return list, err
	}
	hasRandom, err := r.U8()
	if err != nil {
		return list, err
	}
	if hasRandom != 0 {
		random, err := r.U8()
		if err != nil {
			return list, err
		}
		list.Random = random != 0
		if list.RandomHonor, err = r.U32(); err != nil {
			return list, err
		}
		if list.RandomArena, err = r.U32(); err != nil {
			return list, err
		}
		if list.RandomLoser, err = r.U32(); err != nil {
			return list, err
		}
	}
	count, err := r.U32()
	if err != nil {
		return list, err
	}
	for index := uint32(0); index < count && r.Remaining() >= 4; index++ {
		instance, err := r.U32()
		if err != nil {
			return list, err
		}
		list.Instances = append(list.Instances, instance)
	}
	return list, nil
}

// PVPScore is one player's line in the score board.
type PVPScore struct {
	GUID           uint64
	KillingBlows   uint32
	HonorableKills uint32
	Deaths         uint32
	Honor          uint32
	Faction        uint8
	Damage         uint32
	Healing        uint32
	Stats          [8]uint32
}

// PVPLog is MSG_PVP_LOG_DATA from the server.
type PVPLog struct {
	Arena bool
	// TeamRating is, per team, the three numbers the server gives of its rating.
	TeamRating [2][3]uint32
	TeamName   [2]string
	HasWinner  bool
	Winner     uint8
	Scores     []PVPScore
}

// ParsePVPLog reads MSG_PVP_LOG_DATA.
func ParsePVPLog(body []byte) (PVPLog, error) {
	r := NewReader(body, "MSG_PVP_LOG_DATA")
	var log PVPLog
	arena, err := r.U8()
	if err != nil {
		return log, err
	}
	log.Arena = arena != 0
	if log.Arena {
		for team := range log.TeamRating {
			for index := range log.TeamRating[team] {
				if log.TeamRating[team][index], err = r.U32(); err != nil {
					return log, err
				}
			}
		}
		for team := range log.TeamName {
			if log.TeamName[team], err = r.CString(); err != nil {
				return log, err
			}
		}
	}
	winner, err := r.U8()
	if err != nil {
		return log, err
	}
	log.HasWinner = winner != 0
	if log.HasWinner {
		if log.Winner, err = r.U8(); err != nil {
			return log, err
		}
	}
	count, err := r.U32()
	if err != nil {
		return log, err
	}
	for index := uint32(0); index < count; index++ {
		var score PVPScore
		if score.GUID, err = r.U64(); err != nil {
			return log, err
		}
		if score.KillingBlows, err = r.U32(); err != nil {
			return log, err
		}
		if log.Arena {
			if score.Faction, err = r.U8(); err != nil {
				return log, err
			}
		} else {
			if score.HonorableKills, err = r.U32(); err != nil {
				return log, err
			}
			if score.Deaths, err = r.U32(); err != nil {
				return log, err
			}
			if score.Honor, err = r.U32(); err != nil {
				return log, err
			}
		}
		if score.Damage, err = r.U32(); err != nil {
			return log, err
		}
		if score.Healing, err = r.U32(); err != nil {
			return log, err
		}
		stats, err := r.U32()
		if err != nil {
			return log, err
		}
		for slot := uint32(0); slot < stats; slot++ {
			value, err := r.U32()
			if err != nil {
				return log, err
			}
			if slot < uint32(len(score.Stats)) {
				score.Stats[slot] = value
			}
		}
		log.Scores = append(log.Scores, score)
	}
	return log, nil
}

// BattlegroundPosition is where a player of the battleground is.
type BattlegroundPosition struct {
	GUID uint64
	X, Y float32
}

// BattlegroundPositions is MSG_BATTLEGROUND_PLAYER_POSITIONS: the players of
// each team, and the flag carriers.
type BattlegroundPositions struct {
	Players  []BattlegroundPosition
	Carriers []BattlegroundPosition
}

// ParseBattlegroundPositions reads MSG_BATTLEGROUND_PLAYER_POSITIONS.
func ParseBattlegroundPositions(body []byte) (BattlegroundPositions, error) {
	r := NewReader(body, "MSG_BATTLEGROUND_PLAYER_POSITIONS")
	var positions BattlegroundPositions
	read := func(count uint32) ([]BattlegroundPosition, error) {
		list := make([]BattlegroundPosition, 0, min(int(count), 80))
		for index := uint32(0); index < count; index++ {
			var entry BattlegroundPosition
			var err error
			if entry.GUID, err = r.U64(); err != nil {
				return list, err
			}
			if entry.X, err = r.F32(); err != nil {
				return list, err
			}
			if entry.Y, err = r.F32(); err != nil {
				return list, err
			}
			list = append(list, entry)
		}
		return list, nil
	}
	count, err := r.U32()
	if err != nil {
		return positions, err
	}
	if positions.Players, err = read(count); err != nil {
		return positions, err
	}
	carriers, err := r.U32()
	if err != nil {
		return positions, err
	}
	positions.Carriers, err = read(carriers)
	return positions, err
}

// WorldStateValue is one variable of the world state.
type WorldStateValue struct {
	ID, Value uint32
}

// InitWorldStates is SMSG_INIT_WORLD_STATES.
type InitWorldStates struct {
	Map, Zone, Area uint32
	States          []WorldStateValue
}

// ParseInitWorldStates reads SMSG_INIT_WORLD_STATES.
func ParseInitWorldStates(body []byte) (InitWorldStates, error) {
	r := NewReader(body, "SMSG_INIT_WORLD_STATES")
	var init InitWorldStates
	var err error
	if init.Map, err = r.U32(); err != nil {
		return init, err
	}
	if init.Zone, err = r.U32(); err != nil {
		return init, err
	}
	if init.Area, err = r.U32(); err != nil {
		return init, err
	}
	count, err := r.U16()
	if err != nil {
		return init, err
	}
	for index := uint16(0); index < count; index++ {
		var state WorldStateValue
		if state.ID, err = r.U32(); err != nil {
			return init, err
		}
		if state.Value, err = r.U32(); err != nil {
			return init, err
		}
		init.States = append(init.States, state)
	}
	return init, nil
}

// ParseUpdateWorldState reads SMSG_UPDATE_WORLD_STATE.
func ParseUpdateWorldState(body []byte) (WorldStateValue, error) {
	r := NewReader(body, "SMSG_UPDATE_WORLD_STATE")
	var state WorldStateValue
	var err error
	if state.ID, err = r.U32(); err != nil {
		return state, err
	}
	state.Value, err = r.U32()
	return state, err
}

// BuildBattlefieldList is the body of CMSG_BATTLEFIELD_LIST.
func BuildBattlefieldList(bgType uint32, fromWhere, extra bool) []byte {
	body := binary.LittleEndian.AppendUint32(nil, bgType)
	return append(body, boolByte(fromWhere), boolByte(extra))
}

// BuildBattlefieldPort is the body of CMSG_BATTLEFIELD_PORT.
func BuildBattlefieldPort(queueID uint64, accept bool) []byte {
	body := binary.LittleEndian.AppendUint64(nil, queueID)
	return append(body, boolByte(accept))
}

// BuildBattlemasterJoin is the body of CMSG_BATTLEMASTER_JOIN.
func BuildBattlemasterJoin(master uint64, bgType, instance uint32, asGroup bool) []byte {
	body := binary.LittleEndian.AppendUint64(nil, master)
	body = binary.LittleEndian.AppendUint32(body, bgType)
	body = binary.LittleEndian.AppendUint32(body, instance)
	return append(body, boolByte(asGroup))
}

// BuildBattlemasterJoinArena is the body of CMSG_BATTLEMASTER_JOIN_ARENA.
func BuildBattlemasterJoinArena(master uint64, slot uint8, asGroup, rated bool) []byte {
	body := binary.LittleEndian.AppendUint64(nil, master)
	return append(body, slot, boolByte(asGroup), boolByte(rated))
}

// BuildGUIDBody is a body that is only a guid.
func BuildGUIDBody(guid uint64) []byte { return binary.LittleEndian.AppendUint64(nil, guid) }

// BuildBattlefieldMgrResponse is the body of the battlefield manager's replies.
func BuildBattlefieldMgrResponse(id uint32, accept bool) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, id), boolByte(accept))
}

// BuildBattlefieldMgrID is the body of the battlefield manager's requests.
func BuildBattlefieldMgrID(id uint32) []byte { return binary.LittleEndian.AppendUint32(nil, id) }
