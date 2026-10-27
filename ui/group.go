package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// PacketHost sends the packets of the systems the interface drives itself.
type PacketHost interface {
	SendPacket(opcode world.ClientOpcode, body []byte) error
}

// send hands a packet to the connection; it reports whether there is one.
func (rt *Runtime) send(opcode world.ClientOpcode, body []byte) bool {
	host, ok := rt.Host.(PacketHost)
	if !ok {
		return false
	}
	return host.SendPacket(opcode, body) == nil
}

// groupMember is one member of the group, as the roster lists it.
type groupMember struct {
	world.GroupMember
	// token is the unit token ("raidN" in a raid, "partyN" otherwise).
	token string
	stats world.MemberStats
	// known is set once the member's stats arrived.
	known bool
}

type lootRoll struct {
	world.LootRollStart
	started float64
	votes   map[uint64]uint8
}

type groupState struct {
	info    world.GroupInfo
	in      bool
	members []groupMember
	invite  string
	invited bool

	targetIcons [8]uint64
	ready       struct {
		active    bool
		initiator uint64
		answers   map[uint64]int
	}
	rolls   []*lootRoll
	saved   []world.SavedInstance
	dungeon uint32
	raid    uint32
	// details are what the world knows about a member besides the group list:
	// class, race, sex and the model.
	details    map[uint64]UnitInfo
	elapsed    float64
	candidates []uint64
	selected   int
}

const (
	maxPartyMembers = 4
	maxRaidMembers  = 40
)

// SetGroupDetails gives what is known of a member by what the world shows of
// them (class, race, sex and what they look like).
func (rt *Runtime) SetGroupDetails(guid uint64, info UnitInfo) {
	state := &rt.group
	if state.details == nil {
		state.details = make(map[uint64]UnitInfo)
	}
	state.details[guid] = info
	for index := range state.members {
		if state.members[index].GUID == guid {
			rt.applyMemberUnit(&state.members[index])
		}
	}
}

// GroupMemberGUIDs lists the members of the group other than the character.
func (rt *Runtime) GroupMemberGUIDs() []uint64 {
	var guids []uint64
	for _, member := range rt.group.members {
		guids = append(guids, member.GUID)
	}
	return guids
}

// InGroup reports whether the character is in a group.
func (rt *Runtime) InGroup() bool { return rt.group.in }

// playerGUID is the guid of the character.
func (rt *Runtime) playerGUID() uint64 {
	if info := rt.unitInfo("player"); info != nil {
		return info.GUID
	}
	return 0
}

// isRaid reports whether the group is a raid.
func (rt *Runtime) isRaid() bool {
	return rt.group.in && rt.group.info.Type&world.GroupTypeRaid != 0
}

// ownFlags is the character's own member flags.
func (rt *Runtime) ownFlags() uint8 { return rt.group.info.OwnFlags }

func (rt *Runtime) isLeader() bool {
	return rt.group.in && rt.group.info.Leader == rt.playerGUID()
}

func (rt *Runtime) isAssistant() bool { return rt.group.in && rt.ownFlags()&world.MemberAssistant != 0 }

// memberByGUID finds a member of the group; the character is not among them.
func (rt *Runtime) memberByGUID(guid uint64) *groupMember {
	for index := range rt.group.members {
		if rt.group.members[index].GUID == guid {
			return &rt.group.members[index]
		}
	}
	return nil
}

func (rt *Runtime) memberByName(name string) *groupMember {
	for index := range rt.group.members {
		if strings.EqualFold(rt.group.members[index].Name, name) {
			return &rt.group.members[index]
		}
	}
	return nil
}

// partyMembers are the members of the character's own party of five, other
// than the character.
func (rt *Runtime) partyMembers() []*groupMember {
	var party []*groupMember
	for index := range rt.group.members {
		member := &rt.group.members[index]
		if !rt.isRaid() || member.Group == rt.group.info.OwnGroup {
			party = append(party, member)
		}
	}
	if len(party) > maxPartyMembers {
		party = party[:maxPartyMembers]
	}
	return party
}

// SetGroup takes the group list the server sent: who is in the group, who
// leads it and how it loots.
func (rt *Runtime) SetGroup(info world.GroupInfo) {
	state := &rt.group
	previous := *state
	wasIn, wasRaid, previousLeader := previous.in, previous.in && previous.info.Type&world.GroupTypeRaid != 0, previous.info.Leader
	previousLoot := previous.info.LootMethod
	// A list with nobody else in it and no guid is the group going away.
	state.in = len(info.Members) > 0
	state.info = info
	rt.clearGroupUnits()
	state.members = state.members[:0]
	for _, member := range info.Members {
		state.members = append(state.members, groupMember{GroupMember: member})
	}
	for _, old := range previous.members {
		for index := range state.members {
			if state.members[index].GUID == old.GUID {
				state.members[index].stats, state.members[index].known = old.stats, old.known
			}
		}
	}
	rt.assignGroupTokens()
	for index := range state.members {
		rt.applyMemberUnit(&state.members[index])
	}
	if !state.in {
		state.ready.active = false
		state.rolls = nil
		state.targetIcons = [8]uint64{}
	}
	rt.FireEvent("PARTY_MEMBERS_CHANGED")
	if state.in && (!wasIn || previousLeader != info.Leader) {
		rt.FireEvent("PARTY_LEADER_CHANGED")
	}
	if state.in && previousLoot != info.LootMethod {
		rt.FireEvent("PARTY_LOOT_METHOD_CHANGED")
	}
	if state.in && (rt.isRaid() || wasRaid) || (!state.in && wasRaid) {
		rt.FireEvent("RAID_ROSTER_UPDATE")
	}
	rt.FireEvent("PLAYER_ROLES_ASSIGNED")
	if !wasIn && state.in {
		rt.FireEvent("PARTY_MEMBER_ENABLE", lua.LString("party1"))
	}
}

// assignGroupTokens names each member's unit.
func (rt *Runtime) assignGroupTokens() {
	state := &rt.group
	for index := range state.members {
		state.members[index].token = ""
	}
	if rt.isRaid() {
		for index := range state.members {
			state.members[index].token = fmt.Sprintf("raid%d", index+2)
		}
	}
	for index, member := range rt.partyMembers() {
		if !rt.isRaid() {
			member.token = fmt.Sprintf("party%d", index+1)
		}
	}
}

func (rt *Runtime) clearGroupUnits() {
	for index := 1; index <= maxRaidMembers; index++ {
		rt.ClearUnit(fmt.Sprintf("raid%d", index))
		rt.ClearUnit(fmt.Sprintf("raidpet%d", index))
	}
	for index := 1; index <= maxPartyMembers; index++ {
		rt.ClearUnit(fmt.Sprintf("party%d", index))
		rt.ClearUnit(fmt.Sprintf("partypet%d", index))
	}
}

// unitTokens are the tokens a member answers to.
func (rt *Runtime) unitTokens(member *groupMember) []string {
	var tokens []string
	if rt.isRaid() {
		tokens = append(tokens, member.token)
	}
	for index, other := range rt.partyMembers() {
		if other == member {
			tokens = append(tokens, fmt.Sprintf("party%d", index+1))
		}
	}
	return tokens
}

// memberUnit builds the unit of a member from what is known of them.
func (rt *Runtime) memberUnit(member *groupMember) UnitInfo {
	info := rt.group.details[member.GUID]
	info.GUID, info.Name, info.Exists, info.Player = member.GUID, member.Name, true, true
	info.Connected = member.Status&uint8(world.MemberStatusOnline) != 0
	info.Visible = true
	if member.known {
		stats := member.stats
		if stats.Mask&world.GroupUpdateCurHP != 0 || member.known {
			info.Health = int(stats.Health)
		}
		if stats.MaxHealth != 0 {
			info.HealthMax = int(stats.MaxHealth)
		}
		info.PowerType = int(stats.PowerType)
		tokens := []string{"MANA", "RAGE", "FOCUS", "ENERGY", "HAPPINESS", "RUNES", "RUNIC_POWER"}
		if info.PowerType < len(tokens) {
			info.PowerToken = tokens[info.PowerType]
		}
		info.Power, info.PowerMax = int(stats.Power), int(stats.MaxPower)
		if stats.Level != 0 {
			info.Level = int(stats.Level)
		}
		info.Connected = stats.Status&world.MemberStatusOnline != 0
		info.Dead = stats.Status&(world.MemberStatusDead|world.MemberStatusGhost) != 0
		info.Ghost = stats.Status&world.MemberStatusGhost != 0
		info.Auras = rt.memberAuras(stats.Auras)
	}
	return info
}

func (rt *Runtime) memberAuras(auras []world.MemberAura) []AuraInfo {
	var result []AuraInfo
	for _, aura := range auras {
		info := rt.gameplay.Catalog[aura.Spell]
		harmful := aura.Flags&0x20 != 0
		result = append(result, AuraInfo{Name: info.Name, Rank: info.Rank, Texture: info.Icon, SpellID: aura.Spell, Count: 1, Harmful: harmful})
	}
	return result
}

// applyMemberUnit puts a member's unit under their tokens.
func (rt *Runtime) applyMemberUnit(member *groupMember) {
	info := rt.memberUnit(member)
	for _, token := range rt.unitTokens(member) {
		rt.SetUnit(token, info)
	}
}

// ApplyMemberStats takes a member's stats packet.
func (rt *Runtime) ApplyMemberStats(stats world.MemberStats) {
	member := rt.memberByGUID(stats.GUID)
	if member == nil {
		return
	}
	previous := member.stats
	merged := previous
	mask := stats.Mask
	merged.GUID = stats.GUID
	if mask&world.GroupUpdateStatus != 0 {
		merged.Status = stats.Status
	}
	if mask&world.GroupUpdateCurHP != 0 {
		merged.Health = stats.Health
	}
	if mask&world.GroupUpdateMaxHP != 0 {
		merged.MaxHealth = stats.MaxHealth
	}
	if mask&world.GroupUpdatePowerType != 0 {
		merged.PowerType = stats.PowerType
	}
	if mask&world.GroupUpdateCurPower != 0 {
		merged.Power = stats.Power
	}
	if mask&world.GroupUpdateMaxPower != 0 {
		merged.MaxPower = stats.MaxPower
	}
	if mask&world.GroupUpdateLevel != 0 {
		merged.Level = stats.Level
	}
	if mask&world.GroupUpdateZone != 0 {
		merged.Zone = stats.Zone
	}
	if mask&world.GroupUpdatePosition != 0 {
		merged.X, merged.Y = stats.X, stats.Y
	}
	if mask&world.GroupUpdateAuras != 0 {
		merged.Auras = stats.Auras
	}
	if mask&world.GroupUpdatePetGUID != 0 {
		merged.PetGUID = stats.PetGUID
	}
	if mask&world.GroupUpdatePetName != 0 {
		merged.PetName = stats.PetName
	}
	member.stats, member.known = merged, true
	member.Status = uint8(merged.Status & 0xFF)
	rt.applyMemberUnit(member)
	for _, token := range rt.unitTokens(member) {
		lua := lua.LString(token)
		if mask&world.GroupUpdateCurHP != 0 {
			rt.FireEvent("UNIT_HEALTH", lua)
		}
		if mask&world.GroupUpdateMaxHP != 0 {
			rt.FireEvent("UNIT_MAXHEALTH", lua)
		}
		if mask&(world.GroupUpdatePowerType|world.GroupUpdateCurPower) != 0 {
			rt.FireEvent("UNIT_"+rt.unitPowerToken(token), lua)
		}
		if mask&(world.GroupUpdatePowerType|world.GroupUpdateMaxPower) != 0 {
			rt.FireEvent("UNIT_MAX"+rt.unitPowerToken(token), lua)
		}
		if mask&world.GroupUpdateLevel != 0 {
			rt.FireEvent("UNIT_LEVEL", lua)
		}
		if mask&world.GroupUpdateAuras != 0 {
			rt.FireEvent("UNIT_AURA", lua)
		}
		if mask&world.GroupUpdateStatus != 0 {
			if previous.Status&world.MemberStatusOnline != merged.Status&world.MemberStatusOnline {
				if merged.Status&world.MemberStatusOnline != 0 {
					rt.FireEvent("PARTY_MEMBER_ENABLE", lua)
				} else {
					rt.FireEvent("PARTY_MEMBER_DISABLE", lua)
				}
			}
			rt.FireEvent("UNIT_FLAGS", lua)
		}
	}
	if mask&world.GroupUpdateStatus != 0 || mask&world.GroupUpdateZone != 0 {
		rt.FireEvent("RAID_ROSTER_UPDATE")
	}
}

func (rt *Runtime) unitPowerToken(token string) string {
	if info := rt.unitInfo(token); info != nil && info.PowerToken != "" {
		return info.PowerToken
	}
	return "MANA"
}

// SetGroupInvite records an invitation to a group.
func (rt *Runtime) SetGroupInvite(invitation world.GroupInvitation) {
	if !invitation.Valid {
		return
	}
	rt.group.invite, rt.group.invited = invitation.Inviter, true
	rt.FireEvent("PARTY_INVITE_REQUEST", lua.LString(invitation.Inviter))
}

// CancelGroupInvite drops a pending invitation.
func (rt *Runtime) CancelGroupInvite() {
	if rt.group.invited {
		rt.group.invited = false
		rt.FireEvent("PARTY_INVITE_CANCEL")
	}
}

// partyResultErrors name the message of each party result.
var partyResultErrors = map[uint32]string{
	1: "ERR_BAD_PLAYER_NAME_S", 2: "ERR_TARGET_NOT_IN_GROUP_S", 3: "ERR_TARGET_NOT_IN_INSTANCE_S", 4: "ERR_GROUP_FULL",
	5: "ERR_ALREADY_IN_GROUP_S", 6: "ERR_NOT_IN_GROUP", 7: "ERR_NOT_LEADER", 8: "ERR_PLAYER_WRONG_FACTION",
	9: "ERR_IGNORING_YOU_S", 12: "ERR_LFG_PENDING", 13: "ERR_INVITE_RESTRICTED", 14: "ERR_INVITE_IN_COMBAT",
	15: "ERR_INVITE_UNKNOWN_REALM", 16: "ERR_INVITE_NO_PARTY_SERVER", 17: "ERR_INVITE_PARTY_BUSY", 18: "ERR_PARTY_TARGET_AMBIGUOUS",
	19: "ERR_PARTY_LFG_INVITE_RAID_LOCKED", 20: "ERR_PARTY_LFG_BOOT_LIMIT", 21: "ERR_PARTY_LFG_BOOT_COOLDOWN_S",
	22: "ERR_PARTY_LFG_BOOT_IN_PROGRESS", 23: "ERR_PARTY_LFG_BOOT_TOO_FEW_PLAYERS", 24: "ERR_PARTY_LFG_BOOT_NOT_ELIGIBLE_S",
	25: "ERR_RAID_DISALLOWED_BY_LEVEL", 26: "ERR_PARTY_LFG_BOOT_IN_COMBAT", 27: "ERR_VOTE_KICK_REASON_NEEDED",
	28: "ERR_PARTY_LFG_BOOT_DUNGEON_COMPLETE", 29: "ERR_PARTY_LFG_BOOT_LOOT_ROLLS", 30: "ERR_PARTY_LFG_TELEPORT_IN_COMBAT",
}

// ShowPartyResult shows why a group request failed.
func (rt *Runtime) ShowPartyResult(result world.PartyResultInfo) {
	if result.Result == 0 {
		return
	}
	name := partyResultErrors[result.Result]
	if result.Result == 14 && result.Operation == 4 {
		name = "ERR_GROUP_SWAP_FAILED"
	}
	text, ok := rt.L.GetGlobal(name).(lua.LString)
	if !ok {
		return
	}
	rt.ShowError(strings.Replace(string(text), "%s", result.Name, 1))
}

// SetTargetIcons takes a change of the raid marks.
func (rt *Runtime) SetTargetIcons(update world.TargetIconUpdate) {
	state := &rt.group
	if update.List {
		state.targetIcons = [8]uint64{}
	}
	for _, icon := range update.Icons {
		if int(icon.Icon) < len(state.targetIcons) {
			state.targetIcons[icon.Icon] = icon.GUID
		}
	}
	rt.FireEvent("RAID_TARGET_UPDATE")
}

// RaidIconOf is the mark a unit carries, 0 for none.
func (rt *Runtime) raidIconOf(guid uint64) int {
	if guid == 0 {
		return 0
	}
	for index, marked := range rt.group.targetIcons {
		if marked == guid {
			return index + 1
		}
	}
	return 0
}

// BeginReadyCheck starts a ready check an officer called.
func (rt *Runtime) BeginReadyCheck(initiator uint64) {
	state := &rt.group.ready
	state.active, state.initiator = true, initiator
	state.answers = make(map[uint64]int)
	for _, member := range rt.group.members {
		state.answers[member.GUID] = -1
	}
	state.answers[rt.playerGUID()] = -1
	if initiator == rt.playerGUID() {
		state.answers[initiator] = 1
	} else {
		state.answers[initiator] = 1
	}
	name := rt.memberName(initiator)
	rt.FireEvent("READY_CHECK", lua.LString(name))
}

// AnswerReadyCheck takes one member's answer.
func (rt *Runtime) AnswerReadyCheck(guid uint64, ready bool) {
	state := &rt.group.ready
	if state.answers == nil {
		state.answers = make(map[uint64]int)
	}
	if ready {
		state.answers[guid] = 1
	} else {
		state.answers[guid] = 0
	}
	rt.FireEvent("READY_CHECK_CONFIRM", lua.LString(rt.unitTokenOf(guid)))
}

// FinishReadyCheck ends the ready check.
func (rt *Runtime) FinishReadyCheck() {
	rt.group.ready.active = false
	rt.FireEvent("READY_CHECK_FINISHED")
}

func (rt *Runtime) memberName(guid uint64) string {
	if guid == rt.playerGUID() {
		if info := rt.unitInfo("player"); info != nil {
			return info.Name
		}
	}
	if member := rt.memberByGUID(guid); member != nil {
		return member.Name
	}
	return ""
}

// unitTokenOf is the token a guid answers to: the raid or party token of a
// member, "player" for the character.
func (rt *Runtime) unitTokenOf(guid uint64) string {
	if guid == rt.playerGUID() {
		return "player"
	}
	if member := rt.memberByGUID(guid); member != nil {
		if tokens := rt.unitTokens(member); len(tokens) > 0 {
			return tokens[0]
		}
	}
	return ""
}

// StartLootRoll opens the roll of an item the group is rolling for.
func (rt *Runtime) StartLootRoll(start world.LootRollStart) {
	roll := &lootRoll{LootRollStart: start, started: rt.group.elapsed, votes: make(map[uint64]uint8)}
	rt.group.rolls = append(rt.group.rolls, roll)
	rt.itemTemplate(start.Item)
	rt.FireEvent("START_LOOT_ROLL", lua.LNumber(len(rt.group.rolls)), lua.LNumber(start.Countdown))
}

// AddLootRollVote records a vote on a roll.
func (rt *Runtime) AddLootRollVote(vote world.LootRollVote) {
	for _, roll := range rt.group.rolls {
		if roll.Source == vote.Source && roll.Slot == vote.Slot {
			roll.votes[vote.Player] = vote.Type
		}
	}
	rt.FireEvent("CHAT_MSG_LOOT_ROLL", lua.LString(rt.memberName(vote.Player)))
}

// EndLootRoll closes the roll of an item.
func (rt *Runtime) EndLootRoll(source uint64, slot uint32) {
	kept := rt.group.rolls[:0]
	for _, roll := range rt.group.rolls {
		if roll.Source == source && roll.Slot == slot {
			rt.FireEvent("CANCEL_LOOT_ROLL", lua.LNumber(len(kept)+1))
			continue
		}
		kept = append(kept, roll)
	}
	rt.group.rolls = kept
}

// TickGroup advances the clocks of the group, the roll countdowns.
func (rt *Runtime) TickGroup(elapsed float64) {
	state := &rt.group
	state.elapsed += elapsed
	for _, roll := range state.rolls {
		if state.elapsed-roll.started > float64(roll.Countdown)/1000+1 {
			rt.EndLootRoll(roll.Source, roll.Slot)
			break
		}
	}
}

// SetSavedInstances takes the list of the instances the character is saved to.
func (rt *Runtime) SetSavedInstances(saved []world.SavedInstance) {
	rt.group.saved = saved
	rt.FireEvent("UPDATE_INSTANCE_INFO")
}

// raidRoster is the roster the raid window lists: the character first, then
// the others.
type rosterEntry struct {
	name    string
	rank    int
	group   int
	level   int
	class   string
	file    string
	zone    string
	online  bool
	dead    bool
	role    string
	master  bool
	token   string
	guid    uint64
	assist  bool
	tank    bool
	mainAss bool
}

func (rt *Runtime) rosterEntries() []rosterEntry {
	state := &rt.group
	if !rt.isRaid() {
		return nil
	}
	own := rt.unitInfo("player")
	var roster []rosterEntry
	if own != nil {
		roster = append(roster, rt.entryFor(own.Name, rt.playerGUID(), "player", state.info.OwnGroup, state.info.OwnFlags, own))
	}
	for index := range state.members {
		member := &state.members[index]
		roster = append(roster, rt.entryFor(member.Name, member.GUID, member.token, member.Group, member.Flags, rt.unitInfo(member.token)))
	}
	return roster
}

func (rt *Runtime) entryFor(name string, guid uint64, token string, group, flags uint8, info *UnitInfo) rosterEntry {
	entry := rosterEntry{name: name, guid: guid, token: token, group: int(group) + 1, assist: flags&world.MemberAssistant != 0,
		tank: flags&world.MemberMainTank != 0, mainAss: flags&world.MemberMainAssist != 0, online: true}
	switch {
	case guid == rt.group.info.Leader:
		entry.rank = 2
	case entry.assist:
		entry.rank = 1
	}
	switch {
	case entry.tank:
		entry.role = "MAINTANK"
	case entry.mainAss:
		entry.role = "MAINASSIST"
	}
	entry.master = rt.group.info.LootMethod == world.LootMaster && guid == rt.group.info.Master
	if info != nil {
		entry.level, entry.class, entry.file = info.Level, info.ClassName, info.ClassFile
		entry.online, entry.dead = info.Connected, info.Dead
	}
	if member := rt.memberByGUID(guid); member != nil && member.known {
		entry.zone = rt.zoneName(uint32(member.stats.Zone))
	}
	return entry
}

// zoneName is the name of an area by its id.
func (rt *Runtime) zoneName(id uint32) string {
	if catalog := rt.worldMap.catalog; catalog != nil {
		return catalog.AreaNames[id]
	}
	return ""
}

// mapName is the name of a map by its id.
func (rt *Runtime) mapName(id uint32) string {
	if catalog := rt.worldMap.catalog; catalog != nil {
		return catalog.MapNames[id]
	}
	return ""
}

// memberIndexSort orders members for the lists that show them in group order.
func sortedByGroup(entries []rosterEntry) []rosterEntry {
	sorted := append([]rosterEntry(nil), entries...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].group < sorted[b].group })
	return sorted
}
