package ui

import (
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Battleground map and queue constants.
const (
	bfRandomBG         = 0x20 // the "random battleground" entry of BattlemasterList.dbc
	bfTypeBattleground = 3    // InstanceType of a battleground map
	bfTypeArena        = 4
	bfSlots            = 2
	bfScoreStats       = 8
	bfSortKeys         = 17
)

// BattlemasterRow is a row of BattlemasterList.dbc.
type BattlemasterRow struct {
	ID       uint32
	Maps     [8]uint32
	Type     uint32 // 3 for a battleground, 4 for an arena
	Groups   uint32
	Name     string
	MaxGroup uint32
	Holiday  uint32 // the world state that says the holiday weekend is on
	MinLevel uint32
	MaxLevel uint32
}

// PvpDifficultyRow is a row of PvpDifficulty.dbc: the levels of a bracket.
type PvpDifficultyRow struct {
	Map, Bracket, MinLevel, MaxLevel uint32
}

// WorldStateUIRow is a row of WorldStateUI.dbc.
type WorldStateUIRow struct {
	ID, Map, Area, Phase                  uint32
	Icon, Text, Tooltip                   string
	Variable, Type                        uint32
	DynamicIcon, DynamicTooltip, Extended string
	ExtendedVariables                     [3]uint32
}

// MapRow is what the finder of battlegrounds needs of Map.dbc.
type MapRow struct {
	Type            uint32
	Name            string
	Horde, Alliance string
}

// BattlefieldTables are the tables the battleground interface reads.
type BattlefieldTables struct {
	Battlemasters []BattlemasterRow
	Difficulty    []PvpDifficultyRow
	Maps          map[uint32]MapRow
	WorldStates   []WorldStateUIRow
}

// bfSlot is one of the two places in the queues.
type bfSlot struct {
	queueID            uint64
	other              uint64
	bgType, mapID      uint32
	status             uint32
	minLevel, maxLevel uint32
	instance           uint32
	expires            int64 // ms of the runtime clock the invitation ends, 0 when it does not
	averageWait        uint32
	queuedAt           int64
	arenaType          uint32
	rated              bool
}

// bfScore is one line of the score board with what the exe derives of it.
type bfScore struct {
	world.PVPScore
	faction int
	known   bool
}

// bfManager is the state of a battlefield the battlefield manager runs (Wintergrasp).
type bfManager struct {
	id, zone uint32
	state    uint32
	until    int64
}

type battlefieldState struct {
	tables BattlefieldTables
	maps   map[uint32]MapRow
	byID   map[uint32]*BattlemasterRow
	types  []uint32 // the battlegrounds, in the order the interface lists them

	slots  [bfSlots]bfSlot
	active int // the slot the character is playing in, -1 when none

	// The battlemaster window.
	listLoaded bool
	list       world.BattlefieldList
	npc        uint64
	selected   uint32 // instance chosen in the window

	// The match the character is in.
	activeMap       uint32
	activeMapType   uint32
	endsAt          int64
	startedAt       int64
	arenaFaction    uint8
	hasArenaFaction bool
	statIDs         []uint32 // the extra columns of the score board

	// The score board.
	log          world.PVPLog
	scores       []*bfScore
	visible      int
	pending      int
	hasWinner    bool
	winner       uint8
	scoreFaction int
	sortKeys     [bfSortKeys][2]int
	scoreAsked   int64
	posAsked     int64

	positions world.BattlegroundPositions

	mgr bfManager
}

func (s *battlefieldState) init() {
	s.active = -1
	s.scoreFaction = -1
	s.maps = map[uint32]MapRow{}
	s.byID = map[uint32]*BattlemasterRow{}
	for index := range s.sortKeys {
		s.sortKeys[index] = [2]int{index, 0}
	}
}

// SetBattlefieldTables gives the interface the tables it reads.
func (rt *Runtime) SetBattlefieldTables(tables BattlefieldTables) {
	s := &rt.bf
	s.tables = tables
	s.maps = tables.Maps
	if s.maps == nil {
		s.maps = map[uint32]MapRow{}
	}
	s.byID = make(map[uint32]*BattlemasterRow, len(tables.Battlemasters))
	s.types = nil
	for index := range s.tables.Battlemasters {
		row := &s.tables.Battlemasters[index]
		s.byID[row.ID] = row
		if row.Type == bfTypeBattleground {
			s.types = append(s.types, row.ID)
		}
	}
	rt.sortBattlegroundTypes()
	rt.rebuildWorldStateList()
}

func (rt *Runtime) sortBattlegroundTypes() {
	s := &rt.bf
	sort.SliceStable(s.types, func(i, j int) bool {
		a, b := s.types[i], s.types[j]
		if a == bfRandomBG {
			return b != bfRandomBG
		}
		if b == bfRandomBG {
			return false
		}
		ra, rb := s.byID[a], s.byID[b]
		if ra == nil || rb == nil {
			return a < b
		}
		holidayA, holidayB := rt.worldStateValue(ra.Holiday) != 0, rt.worldStateValue(rb.Holiday) != 0
		if holidayA != holidayB {
			return holidayA
		}
		if ra.MinLevel != rb.MinLevel {
			return ra.MinLevel < rb.MinLevel
		}
		return a < b
	})
}

// PlayerIdentity is what the name query of a player tells of the player.
type PlayerIdentity struct {
	Race, Class, Gender uint8
}

// SetPlayerIdentity gives the interface a way to learn the race, class and sex
// of a player by guid, once the player's name is known.
func (rt *Runtime) SetPlayerIdentity(lookup func(uint64) (PlayerIdentity, bool)) {
	rt.playerIdentity = lookup
}

func (rt *Runtime) identityOf(guid uint64) (PlayerIdentity, bool) {
	if rt.playerIdentity == nil {
		return PlayerIdentity{}, false
	}
	return rt.playerIdentity(guid)
}

func (rt *Runtime) bfMapName(id uint32) string {
	if row, found := rt.bf.maps[id]; found && row.Name != "" {
		return row.Name
	}
	return rt.mapName(id)
}

// hordeRace reports whether a race plays on the Horde side.
func hordeRace(race uint8) bool {
	switch race {
	case 2, 5, 6, 8, 9, 10:
		return true
	}
	return false
}

// bgPlayerBracket finds the bracket of a level for a battleground and says
// whether the level has one.
func (rt *Runtime) bgBracket(level uint32, row *BattlemasterRow) (uint32, bool) {
	if row == nil {
		return 0, false
	}
	if row.ID == bfRandomBG {
		return 0, level == 80
	}
	found := false
	for _, difficulty := range rt.bf.tables.Difficulty {
		if difficulty.Map != row.Maps[0] {
			if found {
				break
			}
			continue
		}
		if difficulty.MinLevel <= level && level <= difficulty.MaxLevel {
			return difficulty.Bracket, true
		}
		found = true
	}
	return 0, false
}

func (rt *Runtime) bfNow() int64 { return rt.nowMS() }

// handleBattlefieldPacket takes the packets of battlegrounds and arenas.
func (rt *Runtime) handleBattlefieldPacket(packet world.Packet) bool {
	s := &rt.bf
	switch packet.Opcode {
	case world.BattlefieldStatusPacket:
		if status, err := world.ParseBattlefieldStatus(packet.Body); err == nil {
			rt.applyBattlefieldStatus(status)
		}
	case world.BattlefieldListPacket:
		if list, err := world.ParseBattlefieldList(packet.Body); err == nil {
			rt.applyBattlefieldList(list)
		}
	case world.PVPLogDataPacket:
		if log, err := world.ParsePVPLog(packet.Body); err == nil {
			rt.applyPVPLog(log)
		}
	case world.BattlegroundPositionsPacket:
		if positions, err := world.ParseBattlegroundPositions(packet.Body); err == nil {
			s.positions = positions
		}
	case world.BattlegroundPlayerJoinedPack, world.BattlegroundPlayerLeftPack:
		r := world.NewReader(packet.Body, "battleground player")
		guid, err := r.U64()
		if err != nil {
			return true
		}
		name := rt.nameOf(guid)
		if name == "" {
			return true
		}
		if packet.Opcode == world.BattlegroundPlayerJoinedPack {
			rt.ShowGameError(0x1E6, name, name)
		} else {
			rt.ShowGameError(0x1E7, name)
		}
	case world.GroupJoinedBattlegroundPack:
		r := world.NewReader(packet.Body, "SMSG_GROUP_JOINED_BATTLEGROUND")
		code, err := r.U32()
		if err != nil {
			return true
		}
		rt.groupJoinedBattleground(int32(code), r)
	case world.BattlefieldPortDeniedPacket:
		rt.ShowGameError(0x87)
	case world.AreaSpiritHealerTimePacket:
		// the countdown to the spirit healer's next resurrection
	case world.InitWorldStatesPacket:
		if init, err := world.ParseInitWorldStates(packet.Body); err == nil {
			rt.applyInitWorldStates(init)
		}
	case world.UpdateWorldStatePacket:
		if state, err := world.ParseUpdateWorldState(packet.Body); err == nil {
			rt.setWorldState(state.ID, state.Value)
			rt.FireEvent("UPDATE_WORLD_STATES")
		}
	case world.BattlefieldMgrEntryInvite, world.BattlefieldMgrEntered, world.BattlefieldMgrQueueInvite,
		world.BattlefieldMgrQueueResponse, world.BattlefieldMgrEjectPending, world.BattlefieldMgrEjected,
		world.BattlefieldMgrStateChange:
		rt.handleBattlefieldMgr(packet)
	default:
		return false
	}
	return true
}

func (rt *Runtime) groupJoinedBattleground(code int32, r *world.Reader) {
	switch code {
	case -15:
		rt.ShowGameError(0x2D3)
	case -14:
		rt.ShowGameError(0x2D2)
	case -13:
		rt.ShowGameError(0x2C5)
	case -12, -11:
		guid, err := r.U64()
		if err != nil {
			return
		}
		if name := rt.nameOf(guid); name != "" {
			rt.ShowGameError(0x2A5, name)
		} else {
			rt.ShowGameError(0x2A4)
		}
	case -10:
		rt.ShowGameError(0x2A2)
	case -9:
		rt.ShowGameError(0x29C)
	case -8:
		rt.ShowGameError(0x1E4)
	case -7:
		rt.ShowGameError(0x1E3)
	case -6:
		rt.ShowGameError(0x1E2)
	case -5:
		rt.ShowGameError(0x1E1)
	case -4:
		rt.ShowGameError(0x1E0)
	case -3:
		rt.ShowGameError(0x2D0)
	case -2:
		rt.ShowGameError(0x1DC)
	case -1:
	default:
		if row := rt.bf.byID[uint32(code)]; row != nil {
			rt.ShowGameError(0x1DD, row.Name)
		} else {
			rt.ShowGameError(0x1DE)
		}
	}
}

func (rt *Runtime) applyBattlefieldStatus(status world.BattlefieldStatus) {
	s := &rt.bf
	if status.Slot > 1 {
		return
	}
	slot := &s.slots[status.Slot]
	if status.Cleared {
		wasActive := s.active == int(status.Slot)
		*slot = bfSlot{}
		if wasActive {
			s.log, s.scores, s.visible, s.pending = world.PVPLog{}, nil, 0, 0
			s.hasWinner, s.statIDs = false, nil
			s.endsAt, s.startedAt, s.activeMapType = 0, 0, 0
			s.active = -1
		}
		rt.FireEvent("UPDATE_BATTLEFIELD_STATUS", lua.LNumber(status.Slot+1))
		return
	}
	slot.queueID = status.QueueID
	low, high := uint32(status.QueueID), uint32(status.QueueID>>32)
	slot.bgType = low>>16 | high<<16
	slot.arenaType = low & 0x7F
	slot.minLevel, slot.maxLevel = uint32(status.MinLevel), uint32(status.MaxLevel)
	slot.instance = status.Instance
	slot.rated = status.Rated
	slot.status = status.Status
	switch status.Status {
	case world.BattlefieldConfirm:
		slot.mapID = status.Map
		slot.other = status.Other
		if status.Timeout == 0 {
			slot.expires = 0
		} else {
			slot.expires = rt.bfNow() + int64(status.Timeout)
		}
	default:
		slot.expires = 0
	}
	if status.Status == world.BattlefieldActive {
		s.activeMap = status.Map
		slot.other = status.Other
		s.activeMapType = s.maps[status.Map].Type
		if status.Remaining == 0 {
			s.endsAt = 0
		} else {
			s.endsAt = rt.bfNow() + int64(status.Remaining)
		}
		if status.Elapsed == 0 {
			s.startedAt = 0
		} else {
			s.startedAt = rt.bfNow() - int64(status.Elapsed)
		}
		s.arenaFaction, s.hasArenaFaction = status.ArenaFaction, status.ArenaFaction != 0
		s.active = int(status.Slot)
		s.statIDs = nil
		for _, row := range rt.bf.tables.WorldStates {
			if row.Type == 2 && (row.Map == s.activeMap || row.Map == 0xFFFFFFFF) {
				s.statIDs = append(s.statIDs, row.ID)
			}
		}
		rt.sortScores()
	} else {
		s.endsAt, s.startedAt = 0, 0
		if s.active == int(status.Slot) {
			s.active = -1
		}
	}
	if status.Status == world.BattlefieldQueued {
		slot.averageWait = status.AverageWait
		slot.queuedAt = rt.bfNow() - int64(status.TimeInQueue)
	} else {
		slot.averageWait, slot.queuedAt = 0, 0
	}
	rt.FireEvent("UPDATE_BATTLEFIELD_STATUS", lua.LNumber(status.Slot+1))
}

func (rt *Runtime) applyBattlefieldList(list world.BattlefieldList) {
	s := &rt.bf
	s.list = list
	s.listLoaded = true
	if list.BGType == bfRandomBG && list.GUID != 0 {
		s.npc = list.GUID
		rt.FireEvent("NPC_PVPQUEUE_ANYWHERE")
		return
	}
	if list.FromWhere != 0 {
		rt.FireEvent("PVPQUEUE_ANYWHERE_SHOW")
		return
	}
	if list.GUID == 0 {
		rt.FireEvent("BATTLEFIELDS_CLOSED")
		s.npc = 0
		rt.FireEvent("BATTLEFIELDS_SHOW")
		return
	}
	s.npc = list.GUID
	rt.FireEvent("BATTLEFIELDS_SHOW")
}

func (rt *Runtime) applyPVPLog(log world.PVPLog) {
	s := &rt.bf
	s.log = log
	s.hasWinner, s.winner = log.HasWinner, log.Winner
	s.scores = nil
	s.pending = 0
	for _, score := range log.Scores {
		entry := &bfScore{PVPScore: score}
		if _, ok := rt.identityOf(score.GUID); ok {
			entry.known = true
		}
		if !entry.known {
			s.pending++
			rt.nameOf(score.GUID)
		}
		s.scores = append(s.scores, entry)
	}
	if s.pending == 0 {
		rt.sortScores()
	}
}

// tickBattlefield finishes the score board once the names of its players arrive.
func (rt *Runtime) tickBattlefield() {
	s := &rt.bf
	if s.pending == 0 {
		return
	}
	pending := 0
	for _, entry := range s.scores {
		if entry.known {
			continue
		}
		if _, ok := rt.identityOf(entry.GUID); ok {
			entry.known = true
			continue
		}
		pending++
	}
	s.pending = pending
	if pending == 0 {
		rt.sortScores()
	}
}

// scoreFactionOf is the side a player of the score board stands on.
func (rt *Runtime) scoreFactionOf(entry *bfScore) int {
	if rt.bf.activeMapType == bfTypeArena {
		return int(entry.PVPScore.Faction)
	}
	identity, ok := rt.identityOf(entry.GUID)
	if !ok {
		return -1
	}
	if hordeRace(identity.Race) {
		return 0
	}
	return 1
}

func (rt *Runtime) scoreOrder(a, b *bfScore) int {
	s := &rt.bf
	if s.scoreFaction != -1 && a.faction != b.faction {
		if a.faction == s.scoreFaction {
			return -1
		}
		return 1
	}
	for _, key := range s.sortKeys {
		var result int
		compare := func(x, y uint32) int {
			switch {
			case x < y:
				return 1
			case x > y:
				return -1
			}
			return 0
		}
		switch key[0] {
		case 0:
			result = compare(a.KillingBlows, b.KillingBlows)
		case 1:
			result = compare(a.Deaths, b.Deaths)
		case 2:
			result = compare(a.Honor, b.Honor)
		case 3:
			result = strings.Compare(strings.ToLower(rt.nameOf(a.GUID)), strings.ToLower(rt.nameOf(b.GUID)))
		case 4:
			ia, _ := rt.identityOf(a.GUID)
			ib, _ := rt.identityOf(b.GUID)
			ca, cb := ia.Class, ib.Class
			switch {
			case cb < ca:
				result = -1
			case cb > ca:
				result = 1
			}
		case 5:
			result = compare(a.HonorableKills, b.HonorableKills)
		case 6:
			result = compare(a.Damage, b.Damage)
		case 7:
			result = compare(a.Healing, b.Healing)
		case 8:
			if a.Faction != b.Faction {
				if int32(b.Faction) <= int32(a.Faction) {
					result = 1
				} else {
					result = -1
				}
			}
		default:
			if key[0] >= 9 && key[0]-9 < len(s.statIDs) {
				index := key[0] - 9
				if index < len(a.Stats) && a.Stats[index] != b.Stats[index] {
					if int32(a.Stats[index]) <= int32(b.Stats[index]) {
						result = 1
					} else {
						result = -1
					}
				}
			}
		}
		if result != 0 {
			if key[1] == 0 {
				return result
			}
			return -result
		}
	}
	return 0
}

// sortScores places the lines of the score board.
func (rt *Runtime) sortScores() {
	s := &rt.bf
	if s.pending != 0 {
		return
	}
	s.visible = 0
	for _, entry := range s.scores {
		entry.faction = rt.scoreFactionOf(entry)
		if s.scoreFaction == -1 || entry.faction == s.scoreFaction {
			s.visible++
		}
	}
	sort.SliceStable(s.scores, func(i, j int) bool { return rt.scoreOrder(s.scores[i], s.scores[j]) < 0 })
	rt.FireEvent("UPDATE_BATTLEFIELD_SCORE")
}

// setScoreSort puts a column first in the order of the score board.
func (rt *Runtime) setScoreSort(column int) {
	s := &rt.bf
	if s.pending != 0 {
		return
	}
	for index := range s.sortKeys {
		if s.sortKeys[index][0] != column {
			continue
		}
		flag := s.sortKeys[index][1]
		if index == 0 {
			if flag == 0 {
				flag = 1
			} else {
				flag = 0
			}
		} else {
			copy(s.sortKeys[1:index+1], s.sortKeys[:index])
		}
		s.sortKeys[0] = [2]int{column, flag}
		break
	}
	rt.sortScores()
}

func (rt *Runtime) handleBattlefieldMgr(packet world.Packet) {
	s := &rt.bf
	r := world.NewReader(packet.Body, "battlefield manager")
	u32 := func() uint32 { value, _ := r.U32(); return value }
	u8 := func() uint8 { value, _ := r.U8(); return value }
	record := func(id uint32, state uint32, zone uint32, until uint32) {
		m := &s.mgr
		if m.id != id && m.zone != 0 && m.state != 0 {
			return
		}
		if zone != 0 {
			m.zone = zone
		}
		if until != 0 {
			m.until = int64(until)
		}
		m.id, m.state = id, state
	}
	switch packet.Opcode {
	case world.BattlefieldMgrEntryInvite:
		id, zone, until := u32(), u32(), u32()
		record(id, 2, zone, until)
		rt.FireEvent("BATTLEFIELD_MGR_ENTRY_INVITE", lua.LNumber(id))
	case world.BattlefieldMgrEntered:
		id := u32()
		a, b, cleared := u8(), u8(), u8()
		if cleared != 0 {
			rt.ChatLine("CHAT_MSG_SYSTEM", rt.globalString("CLEARED_AFK", ""))
		}
		m := &s.mgr
		if m.id == id || m.zone == 0 || m.state == 0 {
			m.id, m.state = id, 0
		}
		rt.FireEvent("BATTLEFIELD_MGR_ENTERED", lua.LNumber(id), lua.LBool(a != 0), lua.LBool(b != 0))
	case world.BattlefieldMgrQueueInvite:
		id, flag := u32(), u8()
		rt.FireEvent("BATTLEFIELD_MGR_QUEUE_INVITE", lua.LNumber(id), lua.LBool(flag == 1))
	case world.BattlefieldMgrQueueResponse:
		id, zone := u32(), u32()
		a, b, c := u8(), u8(), u8()
		record(id, uint32(lfgBool(a != 0)), zone, 0)
		rt.FireEvent("BATTLEFIELD_MGR_QUEUE_REQUEST_RESPONSE", lua.LNumber(id), lua.LBool(a != 0), lua.LBool(c == 1), lua.LFalse, lua.LBool(b != 0))
	case world.BattlefieldMgrEjectPending:
		id, flag := u32(), u8()
		rt.FireEvent("BATTLEFIELD_MGR_EJECT_PENDING", lua.LNumber(id), lua.LBool(flag != 0))
	case world.BattlefieldMgrEjected:
		id := u32()
		reason, b, c := u8(), u8(), u8()
		switch reason {
		case 1, 2, 4, 5, 7, 8, 9, 10, 11:
			m := &s.mgr
			if m.id == id || m.zone == 0 || m.state == 0 {
				m.id, m.state = id, 0
			}
		}
		rt.FireEvent("BATTLEFIELD_MGR_EJECTED", lua.LNumber(id), lua.LBool(reason == 8), lua.LBool(c != 0), lua.LBool(b == 2), lua.LBool(reason == 10))
	case world.BattlefieldMgrStateChange:
		a, b := u32(), u32()
		rt.FireEvent("BATTLEFIELD_MGR_STATE_CHANGE", lua.LNumber(a), lua.LNumber(b))
	}
}
