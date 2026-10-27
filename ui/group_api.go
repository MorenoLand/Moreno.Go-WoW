package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

var lootMethodNames = []string{"freeforall", "roundrobin", "master", "group", "needbeforegreed"}

// SetMasterLootCandidates takes the list of players the master looter may
// give an item to.
func (rt *Runtime) SetMasterLootCandidates(body []byte) {
	reader := world.NewReader(body, "SMSG_LOOT_MASTER_LIST")
	count, err := reader.U8()
	if err != nil {
		return
	}
	rt.group.candidates = rt.group.candidates[:0]
	for index := 0; index < int(count); index++ {
		guid, err := reader.U64()
		if err != nil {
			break
		}
		rt.group.candidates = append(rt.group.candidates, guid)
	}
	rt.FireEvent("OPEN_MASTER_LOOT_LIST")
}

// ReportRandomRoll writes the result of a /roll to the chat.
func (rt *Runtime) ReportRandomRoll(roll world.RollResult) {
	text := fmt.Sprintf(rt.globalString("RANDOM_ROLL_RESULT", "%s rolls %d (%d-%d)"), rt.nameOf(roll.GUID), roll.Roll, roll.Min, roll.Max)
	rt.say("CHAT_MSG_SYSTEM", text)
}

// nameOf is the name of a player the interface knows: the character, a member
// of the group, or one a name query answered.
func (rt *Runtime) nameOf(guid uint64) string {
	if name := rt.memberName(guid); name != "" {
		return name
	}
	if rt.playerNames != nil {
		return rt.playerNames(guid)
	}
	return ""
}

// SetPlayerNames gives the interface a way to look up the name of a player by
// guid.
func (rt *Runtime) SetPlayerNames(lookup func(uint64) string) { rt.playerNames = lookup }

var rollTypeNames = map[uint8]string{0: "Pass", 1: "Need", 2: "Greed", 3: "Disenchant"}

// ReportLootRollWon writes who won a roll to the chat.
func (rt *Runtime) ReportLootRollWon(won world.LootRollResult) {
	link := rt.itemLinkFor(won.Item, 0)
	name := rt.nameOf(won.Winner)
	var text string
	switch won.Type {
	case 1:
		text = fmt.Sprintf(rt.globalString("LOOT_ROLL_WON_NO_SPAM_NEED", "%s won: %s |cff818181(Need - %d)|r"), name, link, won.Number)
	case 3:
		text = fmt.Sprintf(rt.globalString("LOOT_ROLL_WON_NO_SPAM_DE", "%s won: %s |cff818181(Disenchant - %d)|r"), name, link, won.Number)
	default:
		text = fmt.Sprintf(rt.globalString("LOOT_ROLL_WON_NO_SPAM_GREED", "%s won: %s |cff818181(Greed - %d)|r"), name, link, won.Number)
	}
	rt.say("CHAT_MSG_LOOT", text)
	rt.EndLootRoll(won.Source, won.Slot)
}

// ReportLootAllPassed writes that nobody wanted an item.
func (rt *Runtime) ReportLootAllPassed(passed world.LootPassed) {
	rt.say("CHAT_MSG_LOOT", fmt.Sprintf(rt.globalString("LOOT_ROLL_ALL_PASSED", "Everyone passed on: %s"), rt.itemLinkFor(passed.Item, 0)))
	rt.EndLootRoll(passed.Source, passed.Slot)
}

// memberFor finds the member a unit token or name stands for.
func (rt *Runtime) memberFor(unit string) *groupMember {
	if info := rt.unitInfo(unit); info != nil && info.GUID != 0 {
		return rt.memberByGUID(info.GUID)
	}
	return rt.memberByName(unit)
}

func luaFlag(set bool) lua.LValue {
	if set {
		return lua.LNumber(1)
	}
	return lua.LNil
}

// registerGroupAPI defines the calls of parties and raids.
func registerGroupAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	reg("GetNumPartyMembers", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.partyMembers()))) })
	reg("GetRealNumPartyMembers", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.partyMembers()))) })
	raidCount := func() int {
		if rt.isRaid() {
			return len(rt.group.members) + 1
		}
		return 0
	}
	reg("GetNumRaidMembers", func(L *lua.LState) int { return push(L, lua.LNumber(raidCount())) })
	reg("GetRealNumRaidMembers", func(L *lua.LState) int { return push(L, lua.LNumber(raidCount())) })
	reg("GetPartyMember", func(L *lua.LState) int {
		return push(L, lua.LBool(L.CheckInt(1) >= 1 && L.CheckInt(1) <= len(rt.partyMembers())))
	})
	reg("GetPartyLeaderIndex", func(L *lua.LState) int {
		if !rt.group.in {
			return push(L, lua.LNil)
		}
		if rt.isLeader() {
			return push(L, lua.LNumber(0))
		}
		for index, member := range rt.partyMembers() {
			if member.GUID == rt.group.info.Leader {
				return push(L, lua.LNumber(index+1))
			}
		}
		return push(L, lua.LNil)
	})
	reg("IsPartyLeader", func(L *lua.LState) int { return push(L, lua.LBool(rt.isLeader() && !rt.isRaid())) })
	reg("IsRaidLeader", func(L *lua.LState) int { return push(L, luaFlag(rt.isLeader() && rt.isRaid())) })
	reg("IsRaidOfficer", func(L *lua.LState) int { return push(L, luaFlag(rt.isAssistant() && rt.isRaid())) })
	reg("UnitIsPartyLeader", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		return push(L, lua.LBool(rt.group.in && info != nil && info.GUID != 0 && info.GUID == rt.group.info.Leader))
	})
	reg("UnitIsRaidOfficer", func(L *lua.LState) int {
		unit := L.CheckString(1)
		if rt.isSelf(unit) {
			return push(L, luaFlag(rt.isAssistant()))
		}
		member := rt.memberFor(unit)
		return push(L, luaFlag(member != nil && member.Flags&world.MemberAssistant != 0))
	})
	reg("UnitInParty", func(L *lua.LState) int {
		unit := L.CheckString(1)
		if rt.isSelf(unit) {
			return push(L, luaFlag(rt.group.in))
		}
		member := rt.memberFor(unit)
		if member == nil {
			return push(L, lua.LNil)
		}
		for _, party := range rt.partyMembers() {
			if party == member {
				return push(L, lua.LNumber(1))
			}
		}
		return push(L, lua.LNil)
	})
	reg("UnitInRaid", func(L *lua.LState) int {
		unit := L.CheckString(1)
		if !rt.isRaid() {
			return push(L, lua.LNil)
		}
		if rt.isSelf(unit) {
			return push(L, lua.LNumber(0))
		}
		for index := range rt.group.members {
			if member := rt.memberFor(unit); member != nil && member == &rt.group.members[index] {
				return push(L, lua.LNumber(index+1))
			}
		}
		return push(L, lua.LNil)
	})
	reg("UnitPlayerOrPetInParty", func(L *lua.LState) int {
		unit := L.CheckString(1)
		if strings.HasPrefix(unit, "partypet") || strings.HasPrefix(unit, "party") || rt.isSelf(unit) {
			return push(L, luaFlag(rt.group.in))
		}
		return push(L, lua.LNil)
	})
	reg("UnitPlayerOrPetInRaid", func(L *lua.LState) int {
		unit := L.CheckString(1)
		if strings.HasPrefix(unit, "raid") || rt.isSelf(unit) {
			return push(L, luaFlag(rt.isRaid()))
		}
		return push(L, lua.LNil)
	})
	reg("GetRaidRosterInfo", func(L *lua.LState) int {
		roster := rt.rosterEntries()
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(roster) {
			return push(L, lua.LNil)
		}
		entry := roster[index]
		role := lua.LValue(lua.LNil)
		if entry.role != "" {
			role = lua.LString(entry.role)
		}
		return push(L, lua.LString(entry.name), lua.LNumber(entry.rank), lua.LNumber(entry.group), lua.LNumber(entry.level), lua.LString(entry.class),
			lua.LString(entry.file), lua.LString(entry.zone), luaFlag(entry.online), luaFlag(entry.dead), role, luaFlag(entry.master))
	})
	reg("IsInRaid", func(L *lua.LState) int { return push(L, lua.LBool(rt.isRaid())) })
	reg("InCombatLockdown", func(L *lua.LState) int {
		info := rt.unitInfo("player")
		return push(L, lua.LBool(info != nil && info.InCombat))
	})

	// Loot.
	reg("GetLootMethod", func(L *lua.LState) int {
		method := int(rt.group.info.LootMethod)
		if !rt.group.in || method >= len(lootMethodNames) {
			method = 0
		}
		partyMaster, raidMaster := lua.LValue(lua.LNil), lua.LValue(lua.LNil)
		if rt.group.in && method == int(world.LootMaster) {
			master := rt.group.info.Master
			if master == rt.playerGUID() {
				partyMaster = lua.LNumber(0)
				if rt.isRaid() {
					raidMaster = lua.LNumber(1)
				}
			} else {
				for index, member := range rt.partyMembers() {
					if member.GUID == master {
						partyMaster = lua.LNumber(index + 1)
					}
				}
				if rt.isRaid() {
					for index := range rt.group.members {
						if rt.group.members[index].GUID == master {
							raidMaster = lua.LNumber(index + 2)
						}
					}
				}
			}
		}
		return push(L, lua.LString(lootMethodNames[method]), partyMaster, raidMaster)
	})
	reg("GetLootThreshold", func(L *lua.LState) int {
		threshold := int(rt.group.info.Threshold)
		if threshold == 0 {
			threshold = 2
		}
		return push(L, lua.LNumber(threshold))
	})
	reg("SetLootMethod", func(L *lua.LState) int {
		method := -1
		for index, name := range lootMethodNames {
			if strings.EqualFold(name, L.CheckString(1)) {
				method = index
			}
		}
		if method < 0 {
			L.ArgError(1, "invalid loot method")
			return 0
		}
		master := uint64(0)
		if method == int(world.LootMaster) {
			if member := rt.memberFor(L.OptString(2, "")); member != nil {
				master = member.GUID
			} else if rt.isSelf(L.OptString(2, "")) {
				master = rt.playerGUID()
			}
		}
		threshold := uint32(rt.group.info.Threshold)
		if threshold < 2 {
			threshold = 2
		}
		rt.send(world.ClientLootMethod, world.BuildLootMethod(uint32(method), master, threshold))
		return 0
	})
	reg("SetLootThreshold", func(L *lua.LState) int {
		rt.send(world.ClientLootMethod, world.BuildLootMethod(uint32(rt.group.info.LootMethod), rt.group.info.Master, uint32(L.CheckInt(1))))
		return 0
	})
	reg("GetMasterLootCandidate", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.group.candidates) {
			return push(L, lua.LNil)
		}
		return push(L, lua.LString(rt.nameOf(rt.group.candidates[index])))
	})
	reg("GiveMasterLoot", func(L *lua.LState) int {
		slot, index := L.CheckInt(1), L.CheckInt(2)-1
		if index < 0 || index >= len(rt.group.candidates) {
			return 0
		}
		if guid := rt.currentLootGUID(); guid != 0 {
			rt.send(world.ClientLootMasterGive, world.BuildLootMasterGive(guid, uint8(slot-1), rt.group.candidates[index]))
		}
		return 0
	})
	rollItem := func(L *lua.LState) (*lootRoll, *world.ItemTemplate) {
		id := L.CheckInt(1) - 1
		if id < 0 || id >= len(rt.group.rolls) {
			return nil, nil
		}
		roll := rt.group.rolls[id]
		return roll, rt.itemTemplate(roll.Item)
	}
	reg("GetLootRollItemInfo", func(L *lua.LState) int {
		roll, template := rollItem(L)
		if roll == nil {
			return 0
		}
		name, quality, bind := "", 1, false
		if template != nil {
			name, quality, bind = template.Name, int(template.Quality), template.Bonding == 1
		}
		mask := roll.Mask
		return push(L, lua.LString(rt.itemIcon(roll.Item)), lua.LString(name), lua.LNumber(roll.Count), lua.LNumber(quality), lua.LBool(bind),
			lua.LBool(mask&0x02 != 0), lua.LBool(mask&0x04 != 0), lua.LBool(mask&0x08 != 0))
	})
	reg("GetLootRollTimeLeft", func(L *lua.LState) int {
		roll, _ := rollItem(L)
		if roll == nil {
			return push(L, lua.LNumber(0))
		}
		left := float64(roll.Countdown) - (rt.group.elapsed-roll.started)*1000
		if left < 0 {
			left = 0
		}
		return push(L, lua.LNumber(left))
	})
	vote := func(L *lua.LState, confirmed bool) int {
		id, kind := L.CheckInt(1)-1, L.CheckInt(2)
		if id < 0 || id >= len(rt.group.rolls) {
			return 0
		}
		roll := rt.group.rolls[id]
		if template := rt.itemTemplate(roll.Item); !confirmed && kind != 0 && template != nil && template.Bonding == 1 {
			rt.FireEvent("CONFIRM_LOOT_ROLL", lua.LNumber(id+1), lua.LNumber(kind))
			return 0
		}
		rt.send(world.ClientLootRoll, world.BuildLootRoll(roll.Source, roll.Slot, uint8(kind)))
		return 0
	}
	reg("RollOnLoot", func(L *lua.LState) int { return vote(L, false) })
	reg("ConfirmLootRoll", func(L *lua.LState) int { return vote(L, true) })
	reg("RandomRoll", func(L *lua.LState) int {
		low, high := L.CheckInt(1), L.CheckInt(2)
		if low > high || high > 10000 {
			return 0
		}
		rt.send(world.ClientRandomRoll, world.BuildRandomRoll(uint32(low), uint32(high)))
		return 0
	})

	// Membership.
	reg("InviteUnit", func(L *lua.LState) int {
		name := L.CheckString(1)
		if info := rt.unitInfo(name); info != nil {
			name = info.Name
		}
		rt.send(world.ClientGroupInvite, world.BuildGroupInvite(name))
		return 0
	})
	reg("UninviteUnit", func(L *lua.LState) int {
		name := L.CheckString(1)
		if info := rt.unitInfo(name); info != nil {
			name = info.Name
		}
		rt.send(world.ClientGroupUninvite, world.BuildGroupUninvite(name))
		return 0
	})
	reg("LeaveParty", func(L *lua.LState) int { rt.send(world.ClientGroupDisband, nil); return 0 })
	reg("AcceptGroup", func(L *lua.LState) int {
		rt.send(world.ClientGroupAccept, world.BuildGroupAccept())
		rt.group.invited = false
		return 0
	})
	reg("DeclineGroup", func(L *lua.LState) int {
		rt.send(world.ClientGroupDecline, nil)
		rt.group.invited = false
		return 0
	})
	reg("PromoteToLeader", func(L *lua.LState) int {
		if member := rt.memberFor(L.CheckString(1)); member != nil {
			rt.send(world.ClientGroupSetLeader, world.BuildGuid(member.GUID))
		}
		return 0
	})
	assistant := func(apply bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if member := rt.memberFor(L.CheckString(1)); member != nil {
				rt.send(world.ClientAssistantLeader, world.BuildAssistantLeader(member.GUID, apply))
			}
			return 0
		}
	}
	reg("PromoteToAssistant", assistant(true))
	reg("DemoteAssistant", assistant(false))
	reg("ConvertToRaid", func(L *lua.LState) int { rt.send(world.ClientRaidConvert, nil); return 0 })
	nameAt := func(index int) string {
		roster := rt.rosterEntries()
		if index < 1 || index > len(roster) {
			return ""
		}
		return roster[index-1].name
	}
	reg("SetRaidSubgroup", func(L *lua.LState) int {
		if name := nameAt(L.CheckInt(1)); name != "" {
			rt.send(world.ClientChangeSubGroup, world.BuildChangeSubGroup(name, uint8(L.CheckInt(2)-1)))
		}
		return 0
	})
	reg("SwapRaidSubgroup", func(L *lua.LState) int {
		first, second := nameAt(L.CheckInt(1)), nameAt(L.CheckInt(2))
		if first != "" && second != "" {
			rt.send(world.ClientSwapSubGroup, world.BuildSwapSubGroup(first, second))
		}
		return 0
	})
	reg("SetRaidRosterSelection", func(L *lua.LState) int { rt.group.selected = L.CheckInt(1); return 0 })
	reg("GetRaidRosterSelection", func(L *lua.LState) int { return push(L, lua.LNumber(rt.group.selected)) })

	// Marks and assignments.
	reg("GetRaidTargetIndex", func(L *lua.LState) int {
		if info := rt.unitInfo(L.CheckString(1)); info != nil {
			if icon := rt.raidIconOf(info.GUID); icon != 0 {
				return push(L, lua.LNumber(icon))
			}
		}
		return push(L, lua.LNil)
	})
	reg("SetRaidTarget", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		icon := L.CheckInt(2)
		if info == nil || info.GUID == 0 || icon < 0 || icon > 8 {
			return 0
		}
		if icon == 0 {
			if current := rt.raidIconOf(info.GUID); current != 0 {
				rt.send(world.ClientRaidTargetUpdate, world.BuildRaidTarget(uint8(current-1), 0))
			}
			return 0
		}
		rt.send(world.ClientRaidTargetUpdate, world.BuildRaidTarget(uint8(icon-1), info.GUID))
		return 0
	})
	assignment := func(name string) (uint8, bool) {
		switch strings.ToUpper(name) {
		case "MAINTANK":
			return 0, true
		case "MAINASSIST":
			return 1, true
		}
		return 0, false
	}
	hasAssignment := func(guid uint64, kind uint8) bool {
		flags := uint8(0)
		if guid == rt.playerGUID() {
			flags = rt.group.info.OwnFlags
		} else if member := rt.memberByGUID(guid); member != nil {
			flags = member.Flags
		}
		if kind == 0 {
			return flags&world.MemberMainTank != 0
		}
		return flags&world.MemberMainAssist != 0
	}
	reg("SetPartyAssignment", func(L *lua.LState) int {
		kind, ok := assignment(L.CheckString(1))
		info := rt.unitInfo(L.CheckString(2))
		if !ok || info == nil {
			return 0
		}
		apply := !(L.OptBool(3, false) || hasAssignment(info.GUID, kind))
		if L.OptBool(3, false) {
			apply = false
		}
		rt.send(world.ClientPartyAssignment, world.BuildPartyAssignment(kind, apply, info.GUID))
		return 0
	})
	reg("ClearPartyAssignment", func(L *lua.LState) int {
		kind, ok := assignment(L.CheckString(1))
		if !ok {
			return 0
		}
		for _, entry := range rt.rosterEntries() {
			if hasAssignment(entry.guid, kind) {
				rt.send(world.ClientPartyAssignment, world.BuildPartyAssignment(kind, false, entry.guid))
			}
		}
		return 0
	})
	reg("GetPartyAssignment", func(L *lua.LState) int {
		kind, ok := assignment(L.CheckString(1))
		if !ok {
			return push(L, lua.LNil)
		}
		if L.Get(2) != lua.LNil {
			info := rt.unitInfo(L.CheckString(2))
			return push(L, luaFlag(info != nil && hasAssignment(info.GUID, kind)))
		}
		for _, entry := range rt.rosterEntries() {
			if hasAssignment(entry.guid, kind) {
				return push(L, lua.LString(entry.name))
			}
		}
		return push(L, lua.LNil)
	})

	// Ready checks.
	reg("DoReadyCheck", func(L *lua.LState) int { rt.send(world.ClientRaidReadyCheck, nil); return 0 })
	reg("ConfirmReadyCheck", func(L *lua.LState) int {
		ready := L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse
		rt.send(world.ClientRaidReadyCheck, world.BuildReadyCheckAnswer(ready))
		rt.AnswerReadyCheck(rt.playerGUID(), ready)
		return 0
	})
	reg("GetReadyCheckStatus", func(L *lua.LState) int {
		info := rt.unitInfo(L.CheckString(1))
		if info == nil || !rt.group.ready.active && rt.group.ready.answers == nil {
			return push(L, lua.LNil)
		}
		switch answer, ok := rt.group.ready.answers[info.GUID]; {
		case !ok:
			return push(L, lua.LNil)
		case answer == 1:
			return push(L, lua.LString("ready"))
		case answer == 0:
			return push(L, lua.LString("notready"))
		}
		return push(L, lua.LString("waiting"))
	})

	// Instances.
	reg("GetDungeonDifficulty", func(L *lua.LState) int { return push(L, lua.LNumber(rt.group.dungeon+1)) })
	reg("GetRaidDifficulty", func(L *lua.LState) int { return push(L, lua.LNumber(rt.group.raid+1)) })
	reg("SetDungeonDifficulty", func(L *lua.LState) int {
		rt.send(world.ClientSetDungeonDiff, world.BuildDifficulty(uint32(L.CheckInt(1)-1)))
		return 0
	})
	reg("SetRaidDifficulty", func(L *lua.LState) int {
		rt.send(world.ClientSetRaidDiff, world.BuildDifficulty(uint32(L.CheckInt(1)-1)))
		return 0
	})
	reg("ResetInstances", func(L *lua.LState) int { rt.send(world.ClientResetInstances, nil); return 0 })
	reg("RequestRaidInfo", func(L *lua.LState) int { rt.send(world.ClientRequestRaidInfo, nil); return 0 })
	reg("GetNumSavedInstances", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.group.saved))) })
	reg("GetSavedInstanceInfo", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.group.saved) {
			return 0
		}
		saved := rt.group.saved[index]
		raid := rt.isRaidMap(saved.Map)
		players := 5
		if raid {
			players = 10
			if saved.Difficulty&1 != 0 {
				players = 25
			}
		}
		return push(L, lua.LString(rt.mapName(saved.Map)), lua.LNumber(saved.ID&0xFFFFFFFF), lua.LNumber(saved.Reset), lua.LNumber(saved.Difficulty+1),
			lua.LBool(!saved.Expired), lua.LBool(saved.Extended), lua.LNumber(saved.ID>>32), lua.LBool(raid), lua.LNumber(players))
	})
	reg("SetSavedInstanceExtend", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.group.saved) {
			return 0
		}
		saved := rt.group.saved[index]
		body := make([]byte, 0, 9)
		body = append(body, byte(saved.Map), byte(saved.Map>>8), byte(saved.Map>>16), byte(saved.Map>>24))
		body = append(body, byte(saved.Difficulty), byte(saved.Difficulty>>8), byte(saved.Difficulty>>16), byte(saved.Difficulty>>24))
		if L.CheckInt(2) != 0 {
			body = append(body, 1)
		} else {
			body = append(body, 0)
		}
		rt.send(0x0292, body)
		return 0
	})
	reg("RespondInstanceLock", func(L *lua.LState) int {
		accept := byte(0)
		if L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse {
			accept = 1
		}
		rt.send(0x013F, []byte{accept})
		return 0
	})
}

// currentLootGUID is the guid of the corpse or chest whose loot is open.
func (rt *Runtime) currentLootGUID() uint64 {
	if rt.loot.open {
		return rt.loot.guid
	}
	return 0
}

// isRaidMap reports whether a map is a raid (Map.dbc instance type 2).
func (rt *Runtime) isRaidMap(id uint32) bool {
	if catalog := rt.worldMap.catalog; catalog != nil {
		return catalog.MapTypes[id] == 2
	}
	return false
}
