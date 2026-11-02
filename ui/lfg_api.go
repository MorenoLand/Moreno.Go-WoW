package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// lfgTableArg is the table a call fills: the one given, emptied, or a new one.
func lfgTableArg(L *lua.LState, usage string) *lua.LTable {
	switch value := L.Get(1).(type) {
	case *lua.LTable:
		var keys []lua.LValue
		value.ForEach(func(key, _ lua.LValue) { keys = append(keys, key) })
		for _, key := range keys {
			value.RawSet(key, lua.LNil)
		}
		return value
	case *lua.LNilType:
		return L.NewTable()
	}
	L.RaiseError("%s", usage)
	return nil
}

func lfgOptional(value lua.LValue, present bool) lua.LValue {
	if present {
		return value
	}
	return lua.LNil
}

// pushDungeonInfo pushes the values GetLFGDungeonInfo returns and reports how
// many.
func (rt *Runtime) lfgDungeonValues(row *LFGDungeonRow) []lua.LValue {
	s := &rt.lfg
	min, max, minRec, maxRec := s.levels(row)
	players, found := s.tables.MapPlayers[[2]uint32{row.Map, row.Difficulty}]
	return []lua.LValue{
		lua.LString(row.Name), lua.LNumber(row.Type), lua.LNumber(min), lua.LNumber(max),
		lua.LNumber(row.Target), lua.LNumber(minRec), lua.LNumber(maxRec), lua.LNumber(row.Expansion),
		lua.LNumber(-int32(row.Group)), lua.LString(row.Texture), lua.LNumber(row.Difficulty),
		lfgOptional(lua.LNumber(players), found), lua.LString(row.Description),
		lua.LBool(row.Flags&lfgFlagHoliday != 0),
	}
}

// lfgChoiceInfo is the table GetLFDChoiceInfo gives for a place in the list.
func (rt *Runtime) lfgChoiceInfo(L *lua.LState, id int32) *lua.LTable {
	s := &rt.lfg
	info := L.NewTable()
	if id < 0 {
		group := s.groups[uint32(-id)]
		if group == nil {
			return info
		}
		info.RawSetInt(1, lua.LString(group.Name))
		info.RawSetInt(2, lua.LNumber(group.Type))
		return info
	}
	row := s.dungeons[uint32(id)]
	if row == nil {
		return info
	}
	for index, value := range rt.lfgDungeonValues(row) {
		info.RawSetInt(index+1, value)
	}
	return info
}

func (rt *Runtime) unitAuraByID(unit string, spell uint32) *AuraInfo {
	info := rt.unitInfo(unit)
	if info == nil {
		return nil
	}
	for index := range info.Auras {
		if info.Auras[index].SpellID == spell {
			return &info.Auras[index]
		}
	}
	return nil
}

// lfgSend sends a finder packet.
func (rt *Runtime) lfgSend(opcode world.ClientOpcode, body []byte) { rt.send(opcode, body) }

// lfgSelect puts a dungeon among the ones the character asks to queue for.
func (rt *Runtime) lfgSelect(entry uint32) {
	s := &rt.lfg
	kind := entry >> 24
	keep := false
	switch {
	case kind == lfgTypeDungeon || kind == lfgTypeHeroic:
		keep = true
		for _, chosen := range s.local.dungeons {
			if k := chosen >> 24; k != lfgTypeDungeon && k != lfgTypeHeroic {
				keep = false
			}
		}
	case kind == lfgTypeRaid && len(rt.group.members) == 0:
		keep = true
		for _, chosen := range s.local.dungeons {
			if chosen>>24 != lfgTypeRaid {
				keep = false
			}
		}
	}
	if !keep {
		s.local.dungeons = nil
	}
	for _, chosen := range s.local.dungeons {
		if chosen == entry {
			return
		}
	}
	s.local.dungeons = append(s.local.dungeons, entry)
	s.local.dirty = true
}

// lfgJoin asks the server to queue the character.
func (rt *Runtime) lfgJoin() {
	s := &rt.lfg
	if (s.local.joined && !s.local.dirty) || !rt.lfgAllowed("join", 10) {
		return
	}
	roles := rt.availableRoles() & s.roles
	if roles&0xFE == 0 {
		return
	}
	rt.lfgSend(world.ClientLFGJoin, world.BuildLFGJoin(roles, s.local.noPartial, s.local.achievements, s.local.dungeons, s.local.needs, s.local.comment))
	s.local.joined, s.local.dirty = true, false
}

// groupRolesOf is the roles of the member a unit token names.
func (rt *Runtime) groupRolesOf(unit string) uint8 {
	info := rt.unitInfo(unit)
	if info == nil || !info.Exists {
		return 0
	}
	if info.GUID == rt.playerGUID() {
		return rt.group.info.OwnRoles
	}
	if member := rt.memberByGUID(info.GUID); member != nil {
		return member.Roles
	}
	return 0
}

func (rt *Runtime) lfgItemName(entry uint32) string {
	if template := rt.itemTemplate(entry); template != nil {
		return template.Name
	}
	return ""
}

// rewardNode finds the reward entry of a dungeon: the list of its items.
func (rt *Runtime) rewardNode(entry uint32) *lfgNode { return rt.lfg.playerNode(entry) }

func registerLFGAPI(rt *Runtime) {
	L := rt.L
	s := &rt.lfg
	s.init()
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }

	reg("GetLFDChoiceOrder", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFDChoiceOrder([table])")
		for index, choice := range s.choice {
			table.RawSetInt(index+1, lua.LNumber(choice.id))
		}
		L.Push(table)
		return 1
	})
	reg("GetLFRChoiceOrder", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFRChoiceOrder([table])")
		for index, choice := range s.raid {
			table.RawSetInt(index+1, lua.LNumber(choice.id))
		}
		L.Push(table)
		return 1
	})
	reg("GetLFDChoiceInfo", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFDChoiceInfo([table])")
		for _, choice := range s.choice {
			table.RawSet(lua.LNumber(choice.id), rt.lfgChoiceInfo(L, choice.id))
		}
		for _, choice := range s.raid {
			if table.RawGet(lua.LNumber(choice.id)) == lua.LNil {
				table.RawSet(lua.LNumber(choice.id), rt.lfgChoiceInfo(L, choice.id))
			}
		}
		L.Push(table)
		return 1
	})
	reg("GetLFDChoiceCollapseState", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFDChoiceCollapseState([table])")
		for _, list := range [][]lfgChoice{s.choice, s.raid} {
			for _, choice := range list {
				group := uint32(-choice.id)
				if choice.id >= 0 {
					group = choice.group
				}
				table.RawSet(lua.LNumber(choice.id), lua.LBool(s.collapsed[group]))
			}
		}
		L.Push(table)
		return 1
	})
	reg("GetLFDChoiceEnabledState", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFDChoiceEnabledState([table])")
		for _, list := range [][]lfgChoice{s.choice, s.raid} {
			for _, choice := range list {
				if choice.id > 0 && s.enabled[uint32(choice.id)] {
					table.RawSet(lua.LNumber(choice.id), lua.LTrue)
				}
			}
		}
		L.Push(table)
		return 1
	})
	reg("GetLFDChoiceLockedState", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFDChoiceLockedState([table])")
		if s.hasParty {
			for _, player := range s.partyList {
				for _, node := range player.locks {
					if node.entry>>24 != lfgTypeRaid && node.locked {
						table.RawSet(lua.LNumber(node.entry&0xFFFFFF), lua.LNumber(node.status))
					}
				}
			}
		}
		if s.hasPlayer {
			for _, node := range s.player {
				if node.locked {
					table.RawSet(lua.LNumber(node.entry&0xFFFFFF), lua.LNumber(node.status))
				}
			}
		}
		L.Push(table)
		return 1
	})
	reg("SetLFGDungeonEnabled", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetLFGDungeonEnabled(dungeonID, isEnabled)")
		}
		if id := L.CheckInt(1); id > 0 {
			s.enabled[uint32(id)] = L.OptBool(2, true)
		}
		return 0
	})
	reg("SetLFGHeaderCollapsed", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetLFGHeaderCollapsed(headerID, isCollapsed)")
		}
		if id := L.CheckInt(1); id < 0 && s.groups[uint32(-id)] != nil {
			s.collapsed[uint32(-id)] = L.OptBool(2, false)
		}
		return 0
	})
	reg("GetLFGDungeonInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGDungeonInfo(dungeonID)")
		}
		row := s.dungeons[uint32(L.CheckInt(1))]
		if row == nil {
			return 0
		}
		values := rt.lfgDungeonValues(row)
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	})
	reg("GetNumRandomDungeons", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(s.random)))
		return 1
	})
	reg("GetLFGRandomDungeonInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGRandomDungeonInfo(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.random) {
			return 0
		}
		entry := s.random[index]
		L.Push(lua.LNumber(entry & 0xFFFFFF))
		row := s.dungeon(entry)
		if row == nil {
			return 1
		}
		values := rt.lfgDungeonValues(row)
		for _, value := range values {
			L.Push(value)
		}
		return len(values) + 1
	})
	reg("GetRandomDungeonBestChoice", func(L *lua.LState) int {
		if s.best == 0 {
			return 0
		}
		L.Push(lua.LNumber(s.best & 0xFFFFFF))
		return 1
	})
	reg("IsLFGDungeonJoinable", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: IsLFGDungeonJoinable(dungeonID)")
		}
		row := s.dungeons[uint32(L.CheckInt(1))]
		if row == nil {
			return 0
		}
		L.Push(lua.LBool(s.joinable(row.Type<<24 | row.ID)))
		return 1
	})
	reg("RequestLFDPlayerLockInfo", func(L *lua.LState) int {
		if !rt.lfgAllowed("playerlocks", 10) {
			L.Push(lua.LFalse)
			return 1
		}
		rt.lfgSend(world.ClientLFGPlayerLockInfo, nil)
		L.Push(lua.LTrue)
		return 1
	})
	reg("RequestLFDPartyLockInfo", func(L *lua.LState) int {
		if !rt.lfgAllowed("partylocks", 10) {
			L.Push(lua.LFalse)
			return 1
		}
		rt.lfgSend(world.ClientLFGPartyLockInfo, nil)
		L.Push(lua.LTrue)
		return 1
	})
	reg("GetLFDLockPlayerCount", func(L *lua.LState) int {
		switch {
		case !s.hasPlayer:
			L.Push(lua.LNumber(0))
		case !s.hasParty:
			L.Push(lua.LNumber(1))
		default:
			L.Push(lua.LNumber(len(s.partyList) + 1))
		}
		return 1
	})
	reg("GetLFDLockInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFDLockInfo(dungeonID, playerIndex)")
		}
		id, index := L.CheckInt(1), L.CheckInt(2)
		if id < 0 {
			return 0
		}
		var locks []*lfgNode
		name := ""
		switch {
		case index == 1:
			if !s.hasPlayer {
				return 0
			}
			locks, name = s.player, rt.playerName()
		default:
			if !s.hasParty || index-2 < 0 || index-2 >= len(s.partyList) {
				return 0
			}
			player := s.partyList[index-2]
			locks, name = player.locks, rt.nameOf(player.guid)
		}
		if name == "" {
			name = rt.globalString("UNKNOWN", "UNKNOWN")
		}
		L.Push(lua.LString(name))
		status := 0
		for _, node := range locks {
			if node.entry&0xFFFFFF == uint32(id) {
				if node.locked {
					status = int(node.status)
				}
				break
			}
		}
		L.Push(lua.LNumber(status))
		return 2
	})

	// The dungeons the character asks to queue for.
	reg("SetLFGDungeon", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetLFGDungeon(type)")
		}
		if row := s.dungeons[uint32(L.CheckInt(1))]; row != nil {
			rt.lfgSelect(row.Type<<24 | row.ID)
		}
		return 0
	})
	reg("ClearAllLFGDungeons", func(L *lua.LState) int {
		s.local.dungeons, s.local.dirty = nil, true
		return 0
	})
	reg("ClearLFGDungeon", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber && L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: ClearLFGDungeon(type, id)")
		}
		kind, id := uint32(L.CheckInt(1)), uint32(L.CheckInt(2))
		if kind >= 7 || s.dungeons[id] == nil {
			return 0
		}
		entry := kind<<24 | id&0xFFFFFF
		for index, chosen := range s.local.dungeons {
			if chosen == entry {
				s.local.dungeons = append(s.local.dungeons[:index:index], s.local.dungeons[index+1:]...)
				s.local.dirty = true
				break
			}
		}
		return 0
	})
	reg("JoinLFG", func(L *lua.LState) int {
		if len(s.local.dungeons) == 0 {
			rt.ShowGameError(errLFGNoSlots)
			return 0
		}
		if s.roles&^lfgRoleLeader == 0 {
			rt.ShowGameError(errLFGNoRoles)
			return 0
		}
		rt.lfgJoin()
		return 0
	})
	reg("LeaveLFG", func(L *lua.LState) int {
		if !s.local.joined && !rt.lfgStatusFor().joined {
			return 0
		}
		rt.lfgSend(world.ClientLFGLeave, nil)
		s.local.joined, s.local.dirty = false, false
		return 0
	})
	reg("SetLFGComment", func(L *lua.LState) int {
		comment := ""
		if L.Get(1).Type() == lua.LTString {
			comment = L.ToString(1)
		}
		if len(comment) > 255 {
			comment = comment[:255]
		}
		s.local.comment = comment
		if !s.local.joined {
			s.local.dirty = true
			return 0
		}
		if rt.lfgAllowed("comment", 2) {
			rt.lfgSend(world.ClientLFGSetComment, world.BuildLFGComment(s.local.comment))
		}
		return 0
	})
	reg("GetLFGInfoLocal", func(L *lua.LState) int {
		L.Push(lua.LBool(s.local.joined))
		L.Push(lua.LBool(s.local.noPartial))
		L.Push(lua.LBool(s.local.achievements))
		L.Push(lua.LString(s.local.comment))
		L.Push(lua.LNumber(len(s.local.dungeons)))
		for _, need := range s.local.needs {
			L.Push(lua.LNumber(need))
		}
		return 8
	})
	reg("GetLFGInfoServer", func(L *lua.LState) int {
		status := rt.lfgStatusFor()
		L.Push(lua.LBool(rt.group.in))
		L.Push(lua.LBool(status.joined))
		L.Push(lua.LBool(status.queued))
		L.Push(lua.LBool(status.noPartial))
		L.Push(lua.LBool(status.achievements))
		L.Push(lua.LString(status.comment))
		L.Push(lua.LNumber(len(status.dungeons)))
		for _, need := range status.needs {
			L.Push(lua.LNumber(need))
		}
		return 10
	})
	reg("GetLFGQueuedList", func(L *lua.LState) int {
		table := lfgTableArg(L, "Usage: GetLFGQueuedList([table])")
		for _, entry := range rt.lfgStatusFor().dungeons {
			table.RawSet(lua.LNumber(entry&0xFFFFFF), lua.LTrue)
		}
		L.Push(table)
		return 1
	})
	reg("GetLFGQueueStats", func(L *lua.LState) int {
		status := rt.lfgStatusFor()
		if !status.joined || !status.queued || s.queue.Entry == 0 {
			return 0
		}
		L.Push(lua.LTrue)
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(s.queue.Tanks))
		L.Push(lua.LNumber(s.queue.Healers))
		L.Push(lua.LNumber(s.queue.Damage))
		if row := s.dungeons[s.queue.Entry]; row != nil {
			L.Push(lua.LNumber(row.Type))
			L.Push(lua.LString(row.Name))
			L.Push(lua.LNumber(s.queue.Wait))
			L.Push(lua.LNumber(s.queue.TankWait))
			L.Push(lua.LNumber(s.queue.HealerWait))
			L.Push(lua.LNumber(s.queue.DamageWait))
			L.Push(lua.LNumber(s.queue.AverageWait))
			L.Push(lua.LNumber(s.queueFrom))
			return 13
		}
		for range 8 {
			L.Push(lua.LNil)
		}
		return 13
	})

	// Roles.
	reg("GetLFGRoles", func(L *lua.LState) int {
		for _, role := range []uint8{lfgRoleLeader, lfgRoleTank, lfgRoleHealer, lfgRoleDamage} {
			L.Push(lua.LBool(s.roles&role != 0))
		}
		return 4
	})
	reg("SetLFGRoles", func(L *lua.LState) int {
		var roles uint8
		for index, role := range []uint8{lfgRoleLeader, lfgRoleTank, lfgRoleHealer, lfgRoleDamage} {
			if L.OptBool(index+1, false) {
				roles |= role
			}
		}
		s.roles = rt.availableRoles() & roles
		rt.FireEvent("LFG_ROLE_UPDATE")
		return 0
	})
	reg("GetAvailableRoles", func(L *lua.LState) int {
		available := rt.availableRoles()
		for _, role := range []uint8{lfgRoleTank, lfgRoleHealer, lfgRoleDamage} {
			L.Push(lua.LBool(available&role != 0))
		}
		return 3
	})
	reg("UnitGroupRolesAssigned", func(L *lua.LState) int {
		roles := rt.groupRolesOf(L.OptString(1, ""))
		for _, role := range []uint8{lfgRoleTank, lfgRoleHealer, lfgRoleDamage} {
			L.Push(lua.LBool(roles&role != 0))
		}
		return 3
	})
	reg("GetLFGRoleUpdate", func(L *lua.LState) int {
		L.Push(lua.LBool(s.roleCheck.State == 2))
		L.Push(lua.LNumber(len(s.roleCheck.Dungeons)))
		L.Push(lua.LNumber(len(s.roleCheck.Members)))
		return 3
	})
	reg("GetLFGRoleUpdateSlot", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGRoleUpdateSlot(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.roleCheck.Dungeons) {
			return 0
		}
		entry := s.roleCheck.Dungeons[index]
		L.Push(lua.LNumber(entry >> 24))
		L.Push(lua.LNumber(entry & 0xFFFFFF))
		return 2
	})
	reg("GetLFGRoleUpdateMember", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGRoleUpdateMember(index)")
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(s.roleCheck.Members) {
			return 0
		}
		member := s.roleCheck.Members[index]
		L.Push(lua.LBool(member.Ready))
		L.Push(lua.LNumber(member.Level))
		if name := rt.nameOf(member.GUID); name != "" {
			L.Push(lua.LString(name))
			L.Push(lua.LString(""))
		} else {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
		}
		if member.Ready {
			for _, role := range []uint32{lfgRoleLeader, lfgRoleTank, lfgRoleHealer, lfgRoleDamage} {
				L.Push(lua.LBool(member.Roles&role != 0))
			}
		} else {
			for range 4 {
				L.Push(lua.LNil)
			}
		}
		return 8
	})
	reg("CompleteLFGRoleCheck", func(L *lua.LState) int {
		if !rt.lfgAllowed("rolecheck", 10) {
			return 0
		}
		var roles uint8
		if L.OptBool(1, false) {
			roles = rt.availableRoles() & s.roles
			if roles&0xE == 0 {
				rt.ShowGameError(errLFGNoRoles)
				L.Push(lua.LFalse)
				return 1
			}
		}
		rt.lfgSend(world.ClientLFGSetRoles, []byte{roles})
		L.Push(lua.LTrue)
		return 1
	})

	// Proposals.
	roleName := func(role uint32) string {
		switch {
		case role&lfgRoleTank != 0:
			return "TANK"
		case role&lfgRoleHealer != 0:
			return "HEALER"
		case role&lfgRoleDamage != 0:
			return "DAMAGER"
		}
		return "UNKNOWN"
	}
	reg("GetLFGProposal", func(L *lua.LState) int {
		p := &s.proposal
		row := s.dungeon(p.entry)
		if p.entry == 0 || row == nil {
			return 0
		}
		role, answered := uint32(0), false
		for _, player := range p.players {
			if player.Self {
				role, answered = player.Role, player.Answered
				break
			}
		}
		var total, completed uint32
		switch {
		case row.Type == lfgTypeRandom:
			completed = uint32(lfgBool(p.encounters != 0))
		case row.Flags&lfgFlagHoliday == 0:
			total, completed = rt.encounterCounts(row.Map, row.Difficulty, p.encounters)
		default:
			total, completed = 1, uint32(lfgBool(p.encounters != 0))
		}
		L.Push(lua.LTrue)
		L.Push(lua.LNumber(p.entry >> 24))
		L.Push(lua.LNumber(p.entry & 0xFFFFFF))
		L.Push(lua.LString(row.Name))
		L.Push(lua.LString(row.Texture))
		L.Push(lua.LString(roleName(role)))
		L.Push(lua.LBool(answered))
		L.Push(lua.LNumber(total))
		L.Push(lua.LNumber(completed))
		L.Push(lua.LNumber(5))
		L.Push(lua.LBool(role&lfgRoleLeader != 0))
		L.Push(lua.LBool(row.Flags>>2&1 != 0))
		return 12
	})
	reg("GetLFGProposalMember", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGProposalMember(index)")
		}
		index := L.CheckInt(1) - 1
		if s.proposal.entry == 0 || index < 0 || index >= 5 {
			return 0
		}
		player := s.proposal.players[index]
		L.Push(lua.LBool(player.Role&lfgRoleLeader != 0))
		L.Push(lua.LString(roleName(player.Role)))
		L.Push(lua.LNumber(0))
		L.Push(lua.LBool(player.Answered))
		L.Push(lua.LBool(player.Accepted))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 7
	})
	reg("GetLFGProposalEncounter", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGProposalEncounter(index)")
		}
		index := L.CheckInt(1) - 1
		var row *LFGDungeonRow
		if s.proposal.entry != 0 {
			row = s.dungeon(s.proposal.entry)
		}
		return rt.pushEncounter(L, row, index, s.proposal.encounters)
	})
	answer := func(accept bool) {
		p := &s.proposal
		if p.entry == 0 || !rt.lfgAllowed("proposal", 10) {
			return
		}
		rt.lfgSend(world.ClientLFGProposalResult, world.BuildLFGProposalResult(p.id, accept))
		if !accept {
			rt.lfgResetProposal()
		} else {
			for index := range p.players {
				if p.players[index].Self {
					p.players[index].Answered, p.players[index].Accepted = true, true
				}
			}
		}
		rt.FireEvent("LFG_PROPOSAL_UPDATE")
	}
	reg("AcceptProposal", func(L *lua.LState) int { answer(true); return 0 })
	reg("RejectProposal", func(L *lua.LState) int { answer(false); return 0 })

	// Votes to remove a player.
	reg("GetLFGBootProposal", func(L *lua.LState) int {
		vote := s.boot
		L.Push(lua.LBool(vote.InProgress))
		if !vote.InProgress {
			return 1
		}
		L.Push(lua.LBool(vote.Voted))
		L.Push(lua.LBool(vote.Agreed))
		if name := rt.nameOf(vote.Victim); name != "" {
			L.Push(lua.LString(name))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(vote.Total))
		L.Push(lua.LNumber(vote.Agree))
		L.Push(lua.LNumber(vote.TimeLeft))
		L.Push(lua.LString(vote.Reason))
		return 8
	})
	reg("SetLFGBootVote", func(L *lua.LState) int {
		if !s.boot.InProgress || !rt.lfgAllowed("boot", 10) {
			return 0
		}
		agree := L.OptBool(1, false)
		s.boot.Agreed = agree
		rt.lfgSend(world.ClientLFGBootVote, world.BuildLFGBootVote(agree))
		return 0
	})

	// Rewards.
	reg("GetLFGDungeonRewards", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGDungeonRewards(dungeonID)")
		}
		row := s.dungeons[uint32(L.CheckInt(1))]
		if row == nil {
			return 0
		}
		node := rt.rewardNode(row.Type<<24 | row.ID)
		if node == nil {
			L.Push(lua.LFalse)
			for range 5 {
				L.Push(lua.LNumber(0))
			}
			return 6
		}
		L.Push(lua.LBool(node.done))
		L.Push(lua.LNumber(node.money))
		L.Push(lua.LNumber(node.moneyVar))
		L.Push(lua.LNumber(node.xp))
		L.Push(lua.LNumber(node.xpVar))
		L.Push(lua.LNumber(len(node.items)))
		return 6
	})
	reg("GetLFGDungeonRewardInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGDungeonRewardInfo(dungeonID, lootIndex)")
		}
		row := s.dungeons[uint32(L.CheckInt(1))]
		if row == nil || (row.Type != lfgTypeRandom && row.Flags&lfgFlagHoliday == 0) {
			return 0
		}
		node := rt.rewardNode(row.Type<<24 | row.ID)
		index := L.CheckInt(2) - 1
		if node == nil || index < 0 || index >= len(node.items) {
			return 0
		}
		item := node.items[index]
		L.Push(lua.LString(rt.lfgItemName(item.Item)))
		L.Push(lua.LString(rt.itemIcon(item.Item)))
		L.Push(lua.LNumber(item.Quantity))
		return 3
	})
	reg("GetLFGDungeonRewardLink", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGDungeonRewardLink(dungeonID, lootIndex)")
		}
		node := rt.rewardNode(uint32(L.CheckInt(1))&0xFFFFFF | lfgTypeRandom<<24)
		index := L.CheckInt(2) - 1
		if node == nil || index < 0 || index >= len(node.items) {
			return 0
		}
		link := rt.itemLinkFor(node.items[index].Item, 0)
		if link == "" {
			return 0
		}
		L.Push(lua.LString(link))
		return 1
	})
	reg("GetLFGCompletionReward", func(L *lua.LState) int {
		if !s.rewardSet {
			return 0
		}
		row := s.dungeon(s.reward.Actual)
		if row == nil {
			return 0
		}
		L.Push(lua.LString(row.Name))
		L.Push(lua.LNumber(row.Type))
		L.Push(lua.LString(row.Texture))
		L.Push(lua.LNumber(s.reward.Money))
		L.Push(lua.LNumber(s.reward.MoneyVar))
		L.Push(lua.LNumber(s.reward.XP))
		L.Push(lua.LNumber(s.reward.XPVar))
		L.Push(lua.LNumber(s.reward.Strangers))
		L.Push(lua.LNumber(len(s.reward.Items)))
		return 9
	})
	reg("GetLFGCompletionRewardItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetLFGCompletionRewardItem(index)")
		}
		index := L.CheckInt(1) - 1
		if !s.rewardSet || index < 0 || index >= len(s.reward.Items) {
			return 0
		}
		item := s.reward.Items[index]
		L.Push(lua.LString(rt.itemIcon(item.Item)))
		L.Push(lua.LNumber(item.Quantity))
		return 2
	})

	// Time held back by the finder.
	deserter := func(spell uint32, usage string) lua.LGFunction {
		return func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTString {
				L.RaiseError("%s", usage)
			}
			unit := L.ToString(1)
			if info := rt.unitInfo(unit); info == nil || !info.Exists {
				return 0
			}
			L.Push(lua.LBool(rt.unitAuraByID(unit, spell) != nil))
			return 1
		}
	}
	reg("UnitHasLFGDeserter", deserter(lfgSpellDeserter, `Usage: UnitHasLFGDeserter("unit")`))
	reg("UnitHasLFGRandomCooldown", deserter(lfgSpellCooldown, `Usage: UnitHasLFGRandomCooldown("unit")`))
	expiration := func(spell uint32) lua.LGFunction {
		return func(L *lua.LState) int {
			aura := rt.unitAuraByID("player", spell)
			if aura == nil {
				return 0
			}
			L.Push(lua.LNumber(aura.ExpirationTime))
			return 1
		}
	}
	reg("GetLFGDeserterExpiration", expiration(lfgSpellDeserter))
	reg("GetLFGRandomCooldownExpiration", expiration(lfgSpellCooldown))

	// The party's dungeon.
	reg("HasLFGRestrictions", func(L *lua.LState) int {
		L.Push(lua.LBool(s.restricted))
		return 1
	})
	reg("IsPartyLFG", func(L *lua.LState) int {
		if s.groupDungeon == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(1))
		}
		return 1
	})
	reg("IsInLFGDungeon", func(L *lua.LState) int {
		if s.groupDungeon == 0 {
			return 0
		}
		row := s.dungeon(s.groupDungeon)
		if row == nil {
			return 0
		}
		inside := rt.worldMap.hasPlayer && row.Map == rt.worldMap.playerMap && int(row.Difficulty) == rt.worldQuery.difficulty
		L.Push(lua.LBool(inside))
		return 1
	})
	canBackfill := func() bool {
		others := len(rt.group.members)
		return others > 0 && others < 4 && s.groupDungeon != 0 && rt.group.info.LFGState&2 == 0
	}
	reg("CanPartyLFGBackfill", func(L *lua.LState) int {
		L.Push(lua.LBool(canBackfill()))
		return 1
	})
	reg("GetPartyLFGBackfillInfo", func(L *lua.LState) int {
		if !canBackfill() {
			return 0
		}
		row := s.dungeon(s.groupDungeon)
		if row == nil {
			return 0
		}
		L.Push(lua.LString(row.Name))
		L.Push(lua.LNumber(row.ID))
		L.Push(lua.LNumber(row.Type))
		return 3
	})
	reg("PartyLFGStartBackfill", func(L *lua.LState) int {
		if s.roles&^lfgRoleLeader == 0 {
			rt.ShowGameError(errLFGNoRoles)
			return 0
		}
		if !canBackfill() {
			return 0
		}
		s.local.dungeons = nil
		rt.lfgSelect(s.groupDungeon)
		rt.lfgJoin()
		L.Push(lua.LTrue)
		return 1
	})
	reg("LFGTeleport", func(L *lua.LState) int {
		if rt.unitInfo("player") == nil || !rt.lfgAllowed("teleport", 10) {
			L.Push(lua.LFalse)
			return 1
		}
		rt.lfgSend(world.ClientLFGTeleport, world.BuildLFGTeleport(L.OptBool(1, false)))
		L.Push(lua.LTrue)
		return 1
	})
	reg("GetLFGTypes", func(L *lua.LState) int {
		for _, name := range []string{"LFG_TYPE_NONE", "LFG_TYPE_DUNGEON", "LFG_TYPE_RAID", "LFG_TYPE_QUEST", "LFG_TYPE_ZONE", "LFG_TYPE_HEROIC_DUNGEON", "LFG_TYPE_RANDOM_DUNGEON"} {
			L.Push(lua.LString(rt.globalString(name, name)))
		}
		return 7
	})

	// The raid browser lists what other groups ask for; the server this client
	// plays on sends it no list.
	reg("IsListedInLFR", func(L *lua.LState) int {
		status := rt.lfgStatusFor()
		listed := false
		if status.joined {
			for _, entry := range status.dungeons {
				if row := s.dungeon(entry); row != nil && row.Type == lfgTypeRaid {
					listed = true
				}
			}
		}
		L.Push(lua.LBool(listed))
		return 1
	})
	reg("SearchLFGJoin", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SearchLFGJoin(typeID, lfgID)")
		}
		kind, id := uint32(L.CheckInt(1)), uint32(L.CheckInt(2))
		if kind < 7 && s.dungeons[id] != nil {
			entry := id&0xFFFFFF | kind<<24
			if s.searched != entry {
				s.searched = entry
				rt.FireEvent("UPDATE_LFG_LIST")
				rt.lfgSend(world.ClientSearchLFGJoin, world.BuildSearchLFG(entry))
			}
		}
		return 0
	})
	reg("SearchLFGLeave", func(L *lua.LState) int {
		if s.searched != 0 {
			rt.lfgSend(world.ClientSearchLFGLeave, binary.LittleEndian.AppendUint32(nil, s.searched))
			s.searched = 0
			rt.FireEvent("UPDATE_LFG_LIST")
		}
		return 0
	})
	reg("SearchLFGGetJoinedID", func(L *lua.LState) int {
		if s.searched&0xFFFFFF == 0 {
			return 0
		}
		L.Push(lua.LNumber(s.searched & 0xFFFFFF))
		return 1
	})
	reg("SearchLFGGetNumResults", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	})
	reg("RefreshLFGList", func(L *lua.LState) int {
		rt.FireEvent("UPDATE_LFG_LIST")
		return 0
	})
	reg("SearchLFGSort", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: SearchLFGSort("type")`)
		}
		rt.FireEvent("UPDATE_LFG_LIST")
		return 0
	})
}

// encounterCounts is how many encounters a map has at a difficulty and how many
// of them a mask marks as killed.
func (rt *Runtime) encounterCounts(mapID, difficulty, mask uint32) (total, killed uint32) {
	for _, encounter := range rt.lfg.tables.Encounters {
		if encounter.Map == mapID && encounter.Difficulty == difficulty {
			total++
			if mask&(1<<(encounter.Bit&31)) != 0 {
				killed++
			}
		}
	}
	return
}

// pushEncounter pushes the name, picture and kill state of an encounter of a
// dungeon's map.
func (rt *Runtime) pushEncounter(L *lua.LState, row *LFGDungeonRow, index int, mask uint32) int {
	if row != nil {
		for _, encounter := range rt.lfg.tables.Encounters {
			if encounter.Map != row.Map || encounter.Difficulty != row.Difficulty {
				continue
			}
			if index > 0 {
				index--
				continue
			}
			L.Push(lua.LString(encounter.Name))
			if encounter.Icon == "" {
				L.Push(lua.LNil)
			} else {
				L.Push(lua.LString(encounter.Icon))
			}
			L.Push(lua.LBool(mask&(1<<(encounter.Bit&31)) != 0))
			return 3
		}
	}
	L.Push(lua.LNil)
	L.Push(lua.LNil)
	L.Push(lua.LNil)
	return 3
}

// lfgTooltipMethods are the tooltip calls that show a reward of a dungeon.
func (rt *Runtime) lfgTooltipMethods() map[string]func(L *lua.LState, w *widget) int {
	return map[string]func(L *lua.LState, w *widget) int{
		"SetLFGDungeonReward": func(L *lua.LState, w *widget) int {
			if L.Get(2).Type() != lua.LTNumber || L.Get(3).Type() != lua.LTNumber {
				L.RaiseError("Usage: Tooltip:SetLFGDungeonReward(dungeonID, lootIndex)")
			}
			row := rt.lfg.dungeons[uint32(L.CheckInt(2))]
			if row == nil {
				return 0
			}
			node := rt.rewardNode(row.Type<<24 | row.ID)
			index := L.CheckInt(3) - 1
			if node != nil && index >= 0 && index < len(node.items) {
				rt.fillItemTooltip(w, node.items[index].Item, nil)
			}
			return 0
		},
		"SetLFGCompletionReward": func(L *lua.LState, w *widget) int {
			if L.Get(2).Type() != lua.LTNumber {
				L.RaiseError("Usage: Tooltip:SetLFGCompletionReward(lootIndex)")
			}
			index := L.CheckInt(2) - 1
			if rt.lfg.rewardSet && index >= 0 && index < len(rt.lfg.reward.Items) {
				rt.fillItemTooltip(w, rt.lfg.reward.Items[index].Item, nil)
			}
			return 0
		},
	}
}
