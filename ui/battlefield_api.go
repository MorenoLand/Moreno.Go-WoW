package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

func (rt *Runtime) bfSlotOf(L *lua.LState, usage string) (*bfSlot, bool) {
	if L.Get(1).Type() != lua.LTNumber {
		L.RaiseError("%s", usage)
	}
	index := L.CheckInt(1) - 1
	if index < 0 || index >= bfSlots {
		return nil, false
	}
	return &rt.bf.slots[index], true
}

// bfName is the name a battleground status shows for a queue slot.
func (rt *Runtime) bfStatusName(slot *bfSlot) (string, bool) {
	s := &rt.bf
	mapNamed := func() (string, bool) {
		if row, found := s.maps[slot.mapID]; found && row.Name != "" {
			return row.Name, true
		}
		return "", false
	}
	switch slot.status {
	case world.BattlefieldConfirm:
		if slot.bgType != bfRandomBG || slot.other != 0 || slot.queueID != 0 {
			if name, ok := mapNamed(); ok {
				return name, true
			}
		}
	case world.BattlefieldActive:
		if name, ok := mapNamed(); ok {
			return name, true
		}
	}
	if row := s.byID[slot.bgType]; row != nil {
		return row.Name, true
	}
	return "", false
}

func (rt *Runtime) playerLevel() uint32 {
	if player := rt.unitInfo("player"); player != nil && player.Level > 0 {
		return uint32(player.Level)
	}
	return 0
}

func (rt *Runtime) playerIsHorde() (horde, known bool) {
	player := rt.unitInfo("player")
	if player == nil || player.RaceID == 0 {
		return false, false
	}
	return hordeRace(uint8(player.RaceID)), true
}

func registerBattlefieldAPI(rt *Runtime) {
	L := rt.L
	s := &rt.bf
	s.init()
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	flag := func(L *lua.LState, value bool) int {
		if value {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}

	reg("GetBattlefieldStatus", func(L *lua.LState) int {
		slot, ok := rt.bfSlotOf(L, "Usage: GetBattlefieldStatus(index)")
		if !ok {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			for range 4 {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LNil)
			return 7
		}
		status := "error"
		switch slot.status {
		case world.BattlefieldNone:
			status = "none"
		case world.BattlefieldQueued:
			status = "queued"
		case world.BattlefieldConfirm:
			status = "confirm"
		case world.BattlefieldActive:
			status = "active"
		}
		L.Push(lua.LString(status))
		if name, found := rt.bfStatusName(slot); found {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(slot.instance))
		L.Push(lua.LNumber(slot.minLevel))
		L.Push(lua.LNumber(slot.maxLevel))
		L.Push(lua.LNumber(slot.arenaType))
		if slot.rated {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 7
	})
	reg("GetBattlefieldEstimatedWaitTime", func(L *lua.LState) int {
		slot, ok := rt.bfSlotOf(L, "Usage: GetBattlefieldEstimatedWaitTime(index)")
		if !ok {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber(slot.averageWait))
		}
		return 1
	})
	reg("GetBattlefieldTimeWaited", func(L *lua.LState) int {
		slot, ok := rt.bfSlotOf(L, "Usage: GetBattlefieldTimeWaited(index)")
		if !ok || slot.queuedAt == 0 {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber(rt.bfNow() - slot.queuedAt))
		}
		return 1
	})
	reg("GetBattlefieldPortExpiration", func(L *lua.LState) int {
		slot, ok := rt.bfSlotOf(L, "Usage: GetBattlefieldPortExpiration(index)")
		if !ok || slot.expires == 0 || rt.bfNow() >= slot.expires {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber((slot.expires - rt.bfNow()) / 1000))
		}
		return 1
	})
	reg("GetBattlefieldInstanceExpiration", func(L *lua.LState) int {
		if s.endsAt == 0 || rt.bfNow() >= s.endsAt {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber(s.endsAt - rt.bfNow()))
		}
		return 1
	})
	reg("GetBattlefieldInstanceRunTime", func(L *lua.LState) int {
		if s.startedAt == 0 {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber(rt.bfNow() - s.startedAt))
		}
		return 1
	})
	reg("GetBattlefieldWinner", func(L *lua.LState) int {
		if !s.hasWinner {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(s.winner))
		}
		return 1
	})
	reg("IsActiveBattlefieldArena", func(L *lua.LState) int {
		if s.activeMapType == bfTypeArena {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if s.active >= 0 && s.slots[s.active].rated {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 2
	})
	reg("GetBattlefieldArenaFaction", func(L *lua.LState) int {
		return flag(L, s.arenaFaction != 0)
	})
	reg("IsBattlefieldArena", func(L *lua.LState) int {
		row := s.byID[s.list.BGType]
		return flag(L, row != nil && row.Type == bfTypeArena)
	})
	reg("CanJoinBattlefieldAsGroup", func(L *lua.LState) int {
		row := s.byID[s.list.BGType]
		return flag(L, row != nil && row.Groups != 0)
	})

	// The score board.
	reg("GetNumBattlefieldScores", func(L *lua.LState) int {
		L.Push(lua.LNumber(s.visible))
		return 1
	})
	reg("GetBattlefieldScore", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldScore(index)")
		}
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(s.scores) && s.pending == 0 {
			entry := s.scores[index]
			if name := rt.nameOf(entry.GUID); name != "" {
				if identity, found := rt.identityOf(entry.GUID); found {
					L.Push(lua.LString(name))
					L.Push(lua.LNumber(entry.KillingBlows))
					L.Push(lua.LNumber(entry.HonorableKills))
					L.Push(lua.LNumber(entry.Deaths))
					L.Push(lua.LNumber(entry.Honor))
					L.Push(lua.LNumber(entry.faction))
					L.Push(lua.LNumber(0))
					L.Push(lua.LString(rt.localizedRaceName(identity.Race, identity.Gender)))
					className, classToken := rt.localizedClassName(identity.Class, identity.Gender)
					L.Push(lua.LString(className))
					L.Push(lua.LString(classToken))
					L.Push(lua.LNumber(entry.Damage))
					L.Push(lua.LNumber(entry.Healing))
					return 12
				}
			}
		}
		L.Push(lua.LNil)
		for range 6 {
			L.Push(lua.LNumber(0))
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 12
	})
	reg("GetBattlefieldStatData", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldStatData(index, statIndex)")
		}
		index, stat := L.CheckInt(1)-1, L.CheckInt(2)-1
		value := uint32(0)
		if index >= 0 && index < len(s.scores) && stat >= 0 && stat < len(s.scores[index].Stats) {
			value = s.scores[index].Stats[stat]
		}
		L.Push(lua.LNumber(value))
		return 1
	})
	reg("GetNumBattlefieldStats", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(s.statIDs)))
		return 1
	})
	reg("GetBattlefieldStatInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldStatInfo(index)")
		}
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(s.statIDs) {
			for _, row := range s.tables.WorldStates {
				if row.ID == s.statIDs[index] {
					L.Push(lua.LString(row.Text))
					L.Push(lua.LString(row.Icon))
					L.Push(lua.LString(row.Tooltip))
					return 3
				}
			}
		}
		for range 3 {
			L.Push(lua.LNil)
		}
		return 3
	})
	reg("GetBattlefieldTeamInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldTeamInfo(index)")
		}
		team := L.CheckInt(1)
		if team >= 0 && team < 2 {
			L.Push(lua.LString(s.log.TeamName[team]))
			for _, value := range s.log.TeamRating[team] {
				L.Push(lua.LNumber(value))
			}
			return 4
		}
		L.Push(lua.LNil)
		for range 3 {
			L.Push(lua.LNumber(0))
		}
		return 4
	})
	reg("RequestBattlefieldScoreData", func(L *lua.LState) int {
		now := rt.bfNow()
		if (s.scoreAsked == 0 || (!s.hasWinner && s.activeMapType != bfTypeArena)) && (s.scoreAsked == 0 || now >= s.scoreAsked) {
			rt.send(world.ClientPVPLogData, nil)
			s.scoreAsked = now + 5000
		}
		return 0
	})
	reg("SetBattlefieldScoreFaction", func(L *lua.LState) int {
		faction := -1
		if L.Get(1).Type() == lua.LTNumber {
			faction = L.CheckInt(1)
			if faction != -1 && faction > 1 {
				return 0
			}
		}
		s.scoreFaction = faction
		rt.sortScores()
		return 0
	})
	reg("SortBattlefieldScoreData", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usgae: SortBattlefieldScoreData("type")`)
		}
		kind := L.ToString(1)
		columns := map[string]int{"kills": 0, "deaths": 1, "honor": 2, "name": 3, "class": 4, "killingblows": 5, "damage": 6, "healing": 7, "faction": 8}
		if column, found := columns[kind]; found {
			rt.setScoreSort(column)
			return 0
		}
		if len(kind) > 4 && kind[:4] == "stat" {
			number := 0
			for _, digit := range kind[4:] {
				if digit < '0' || digit > '9' {
					return 0
				}
				number = number*10 + int(digit-'0')
			}
			if column := number + 8; column <= 16 {
				rt.setScoreSort(column)
			}
		}
		return 0
	})

	// The queues and the list of a battlemaster.
	reg("GetNumBattlegroundTypes", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(s.types)))
		return 1
	})
	reg("SortBGList", func(L *lua.LState) int {
		rt.sortBattlegroundTypes()
		L.Push(lua.LTrue)
		return 1
	})
	reg("GetBattlegroundInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlegroundInfo(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.types) {
			return 0
		}
		row := s.byID[s.types[index]]
		if row == nil {
			return 0
		}
		L.Push(lua.LString(row.Name))
		_, ok := rt.bgBracket(rt.playerLevel(), row)
		if ok {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if rt.worldStateValue(row.Holiday) != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if row.ID == bfRandomBG {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(row.ID))
		return 5
	})
	reg("GetBattlefieldInfo", func(L *lua.LState) int {
		row := s.byID[s.list.BGType]
		if row == nil || rt.unitInfo("player") == nil {
			return 0
		}
		mapRow, found := s.maps[row.Maps[0]]
		if !found {
			return 0
		}
		L.Push(lua.LString(row.Name))
		if horde, known := rt.playerIsHorde(); known && horde {
			L.Push(lua.LString(mapRow.Horde))
		} else if known {
			L.Push(lua.LString(mapRow.Alliance))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(row.MaxGroup))
		if _, ok := rt.bgBracket(rt.playerLevel(), row); ok {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if rt.worldStateValue(row.Holiday) != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if row.ID == bfRandomBG {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 6
	})
	reg("GetNumBattlefields", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(s.list.Instances)))
		return 1
	})
	reg("GetBattlefieldInstanceInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldInfo(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.list.Instances) {
			return 0
		}
		L.Push(lua.LNumber(s.list.Instances[index]))
		return 1
	})
	reg("SetSelectedBattlefield", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetSelectedBattlefield(index)")
		}
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(s.list.Instances) {
			s.selected = s.list.Instances[index]
		} else {
			s.selected = 0
		}
		return 0
	})
	reg("GetSelectedBattlefield", func(L *lua.LState) int {
		found := -1
		for index, instance := range s.list.Instances {
			if instance == s.selected {
				found = index
				break
			}
		}
		L.Push(lua.LNumber(found + 1))
		return 1
	})
	reg("RequestBattlegroundInstanceInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: RequestBattlegroundInstanceInfo(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.types) || rt.unitInfo("player") == nil {
			return 0
		}
		row := s.byID[s.types[index]]
		if row == nil {
			return 0
		}
		rt.send(world.ClientBattlefieldList, world.BuildBattlefieldList(row.ID, true, false))
		s.list.BGType = row.ID
		return 0
	})
	reg("JoinBattlefield", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: JoinBattlefield(index)")
		}
		index := L.CheckInt(1) - 1
		asGroup, rated := L.OptBool(2, false), L.OptBool(3, false)
		if !s.listLoaded {
			return 0
		}
		row := s.byID[s.list.BGType]
		arena := row != nil && row.Type == bfTypeArena
		if asGroup && row != nil {
			if int(row.MaxGroup) < rt.lfgGroupSize() {
				rt.ShowGameError(0x1DF)
				return 0
			}
		}
		if !arena && s.maps[rt.bfCurrentMap()].Type == bfTypeBattleground {
			rt.ShowGameError(0x1E4)
			return 0
		}
		if arena {
			rt.send(world.ClientBattlemasterJoinArena, world.BuildBattlemasterJoinArena(s.npc, uint8(index), asGroup, rated))
			return 0
		}
		instance := uint32(0)
		if index >= 0 && index < len(s.list.Instances) && s.npc != 0 {
			instance = s.list.Instances[index]
		}
		rt.send(world.ClientBattlemasterJoin, world.BuildBattlemasterJoin(s.npc, s.list.BGType, instance, asGroup))
		return 0
	})
	reg("AcceptBattlefieldPort", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: AcceptBattlefieldPort(index, accept)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= bfSlots {
			L.Push(lua.LNil)
			return 1
		}
		slot := &s.slots[index]
		accept := L.OptBool(2, false)
		rt.send(world.ClientBattlefieldPort, world.BuildBattlefieldPort(slot.queueID, accept))
		if !accept && slot.bgType == s.list.BGType {
			rt.FireEvent("BATTLEFIELDS_CLOSED")
		}
		L.Push(lua.LNumber(1))
		return 1
	})
	reg("LeaveBattlefield", func(L *lua.LState) int {
		var queue uint64
		if s.active >= 0 {
			queue = s.slots[s.active].queueID
		}
		rt.send(world.ClientLeaveBattlefield, world.BuildGUIDBody(queue))
		return 0
	})
	reg("CloseBattlefield", func(L *lua.LState) int {
		rt.FireEvent("BATTLEFIELDS_CLOSED")
		s.npc = 0
		return 0
	})

	// Bonuses the battlegrounds pay.
	reg("GetRandomBGHonorCurrencyBonuses", func(L *lua.LState) int {
		L.Push(lua.LBool(s.list.Random))
		L.Push(lua.LNumber(s.list.RandomHonor))
		L.Push(lua.LNumber(s.list.RandomArena))
		L.Push(lua.LNumber(s.list.RandomLoser))
		L.Push(lua.LNumber(0))
		return 5
	})
	reg("GetHolidayBGHonorCurrencyBonuses", func(L *lua.LState) int {
		L.Push(lua.LBool(s.list.Holiday))
		L.Push(lua.LNumber(s.list.HolidayHonor))
		L.Push(lua.LNumber(s.list.HolidayArena))
		L.Push(lua.LNumber(s.list.HolidayLoser))
		L.Push(lua.LNumber(0))
		return 5
	})

	// Players who do not play.
	reg("PlayerIsPVPInactive", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: PlayerIsPVPInactive("unit")`)
		}
		L.Push(lua.LBool(rt.unitAuraByID(L.ToString(1), 43681) != nil))
		return 1
	})
	reg("ReportPlayerIsPVPAFK", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: ReportPlayerIsPVPAFK("unit")`)
		}
		if info := rt.unitInfo(L.ToString(1)); info != nil && info.Exists && info.GUID != rt.playerGUID() {
			rt.send(world.ClientReportPVPAFK, world.BuildGUIDBody(info.GUID))
		}
		return 0
	})

	// The battlefield manager (Wintergrasp).
	reg("CanQueueForWintergrasp", func(L *lua.LState) int {
		return flag(L, rt.worldStateValue(0x1117) != 0)
	})
	reg("GetWorldPVPQueueStatus", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetWorldPVPQueueStatus(index)")
		}
		if L.CheckInt(1) != 1 {
			return 0
		}
		m := &s.mgr
		status := "error"
		switch m.state {
		case 0:
			status = "none"
		case 1:
			status = "queued"
		case 2:
			status = "confirm"
		case 3:
			status = "active"
		}
		L.Push(lua.LString(status))
		if name := rt.bfMapName(m.zone); name != "" {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(m.id))
		if m.state == 2 {
			L.Push(lua.LNumber(m.until - rt.serverNow()))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 4
	})
	reg("BattlefieldMgrQueueRequest", func(L *lua.LState) int {
		rt.send(world.ClientBattlefieldMgrQueueReq, world.BuildBattlefieldMgrID(uint32(L.CheckInt(1))))
		return 0
	})
	reg("BattlefieldMgrExitRequest", func(L *lua.LState) int {
		rt.send(world.ClientBattlefieldMgrExit, world.BuildBattlefieldMgrID(uint32(L.CheckInt(1))))
		return 0
	})
	reg("BattlefieldMgrEntryInviteResponse", func(L *lua.LState) int {
		rt.send(world.ClientBattlefieldMgrEntry, world.BuildBattlefieldMgrResponse(uint32(L.CheckInt(1)), L.OptBool(2, false)))
		return 0
	})
	reg("BattlefieldMgrQueueInviteResponse", func(L *lua.LState) int {
		rt.send(world.ClientBattlefieldMgrQueueResp, world.BuildBattlefieldMgrResponse(uint32(L.CheckInt(1)), L.OptBool(2, false)))
		return 0
	})
}
