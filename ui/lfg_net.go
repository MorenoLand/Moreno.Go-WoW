package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Numbers of the game errors the finder raises.
const (
	errLFGExperienceGain  = 0xA1
	errLFGMoneyGain       = 0xA4
	errLFGTeleportDead    = 0x87
	errLFGTeleportGeneric = 0x88
	errLFGTeleportCombat  = 0x296
	errLFGTeleportFall    = 0x297
	errLFGTeleportSpell   = 0x2C7
	errLFGKickPassed      = 0x28C
	errLFGJoinBase        = 680
	errLFGDeclinedSelf    = 0x2BA
	errLFGDeclinedParty   = 0x2BB
	errLFGNoSlots         = 700
	errLFGNoRoles         = 0x2BD
	errLFGRoleCheckBegin  = 0x2BE
	errLFGRoleDeclined    = 0x2BF
	errLFGJoinedQueue     = 0x2C0
	errLFGLeftQueue       = 0x2C2
	errLFGRoleCheckFailed = 0x2C4
	errLFGProposalFailed  = 0x2B9
	errLFGDisabled        = 0x22D
)

// lfgJoinErrors is the error of each result of a join the server refused,
// from the exe's table.
var lfgJoinErrors = map[uint32]int{
	1: 680, 2: 683, 4: 684, 5: 685, 6: 686, 7: 687, 8: 688, 9: 689, 10: 690,
	11: 691, 12: 692, 13: 693, 14: 694, 15: 695, 16: 696, 17: 710, 18: 680,
}

func lfgMoney(rt *Runtime, copper uint32) string {
	gold, silver, rest := copper/10000, copper/100%100, copper%100
	var parts []string
	if gold > 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("GOLD_AMOUNT", "%d Gold"), gold))
	}
	if silver > 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("SILVER_AMOUNT", "%d Silver"), silver))
	}
	if rest > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf(rt.globalString("COPPER_AMOUNT", "%d Copper"), rest))
	}
	return strings.Join(parts, ", ")
}

// lfgHasRaid reports whether a list of dungeons names a raid.
func (rt *Runtime) lfgHasRaid(status *lfgStatus) bool {
	if !status.joined {
		return false
	}
	for _, entry := range status.dungeons {
		if row := rt.lfg.dungeon(entry); row != nil && row.Type == lfgTypeRaid {
			return true
		}
	}
	return false
}

func lfgStatusFrom(parsed world.LFGStatus, party bool) lfgStatus {
	status := lfgStatus{
		joined: parsed.Joined, lfgJoined: parsed.Join, queued: parsed.Queued,
		noPartial: parsed.NoPartial, achievements: parsed.Achievements,
		needs: parsed.Needs, dungeons: append([]uint32(nil), parsed.Dungeons...),
		comment: parsed.Comment,
	}
	return status
}

func lfgSameStatus(a, b *lfgStatus) bool {
	if a.joined != b.joined || a.lfgJoined != b.lfgJoined || a.queued != b.queued ||
		a.noPartial != b.noPartial || a.achievements != b.achievements || a.needs != b.needs ||
		a.comment != b.comment || len(a.dungeons) != len(b.dungeons) {
		return false
	}
	for index := range a.dungeons {
		if a.dungeons[index] != b.dungeons[index] {
			return false
		}
	}
	return true
}

func (rt *Runtime) lfgResetProposal() {
	rt.lfg.proposal = lfgProposalState{lastID: rt.lfg.proposal.lastID}
}

// handleLFGPacket takes the packets of the dungeon finder.
func (rt *Runtime) handleLFGPacket(packet world.Packet) bool {
	s := &rt.lfg
	switch packet.Opcode {
	case world.LFGOfferContinuePacket:
		r := world.NewReader(packet.Body, "SMSG_LFG_OFFER_CONTINUE")
		if entry, err := r.U32(); err == nil {
			if row := s.dungeon(entry); row != nil {
				rt.FireEvent("LFG_OFFER_CONTINUE", lua.LString(row.Name), lua.LNumber(row.ID), lua.LNumber(row.Type))
			}
		}
	case world.LFGPlayerRewardPacket:
		reward, err := world.ParseLFGReward(packet.Body)
		if err != nil {
			return true
		}
		s.reward, s.rewardSet = reward, true
		bonusXP, bonusMoney := reward.Strangers*reward.XPVar, reward.Strangers*reward.MoneyVar
		if int32(reward.XP) > 0 {
			rt.ShowGameError(errLFGExperienceGain, reward.XP)
		}
		if int32(bonusXP) > 0 {
			rt.ShowGameError(errLFGExperienceGain, bonusXP)
		}
		if int32(reward.Money) > 0 {
			rt.ShowGameError(errLFGMoneyGain, lfgMoney(rt, reward.Money))
		}
		if int32(bonusMoney) > 0 {
			rt.ShowGameError(errLFGMoneyGain, lfgMoney(rt, bonusMoney))
		}
		rt.FireEvent("LFG_COMPLETION_REWARD")
	case world.LFGTeleportDeniedPacket:
		r := world.NewReader(packet.Body, "SMSG_LFG_TELEPORT_DENIED")
		reason, err := r.U32()
		if err != nil {
			return true
		}
		switch reason {
		case 0, 5, 7:
		case 1:
			rt.ShowGameError(errLFGTeleportDead)
		case 2:
			rt.ShowGameError(errLFGTeleportCombat)
		case 3:
			rt.ShowGameError(errLFGTeleportGeneric)
		case 4:
			rt.ShowGameError(errLFGTeleportFall)
		case 6:
			rt.ShowGameError(errLFGTeleportSpell)
			rt.ShowGameError(errLFGTeleportGeneric)
		default:
			rt.ShowGameError(errLFGTeleportGeneric)
		}
	case world.LFGRoleChosenPacket:
		chosen, err := world.ParseLFGRoleChosen(packet.Body)
		if err != nil {
			return true
		}
		if name := rt.nameOf(chosen.GUID); name != "" {
			if !chosen.Ready {
				rt.ShowGameError(errLFGRoleDeclined, name)
			} else {
				rt.fireRoleChosen(name, chosen.Roles)
			}
		}
	case world.LFGProposalUpdatePacket:
		proposal, err := world.ParseLFGProposal(packet.Body)
		if err == nil {
			rt.applyLFGProposal(proposal)
		}
	case world.LFGRoleCheckPacket:
		if check, err := world.ParseLFGRoleCheck(packet.Body); err == nil {
			rt.applyLFGRoleCheck(check)
		}
	case world.LFGJoinResultPacket:
		if result, err := world.ParseLFGJoinResult(packet.Body); err == nil {
			rt.applyLFGJoinResult(result)
		}
	case world.LFGQueueStatusPacket:
		status, err := world.ParseLFGQueueStatus(packet.Body)
		if err != nil {
			return true
		}
		s.queue = status
		s.queueFrom = rt.lfgNow() - float64(status.QueuedSeconds)
		if status.Tanks == 0 && status.Healers == 0 && status.Damage == 0 {
			s.queue.Damage = 1
		}
		rt.FireEvent("LFG_QUEUE_STATUS_UPDATE")
	case world.LFGUpdatePlayerPacket:
		if parsed, err := world.ParseLFGUpdatePlayer(packet.Body); err == nil {
			rt.applyLFGUpdate(parsed, false)
		}
	case world.LFGUpdatePartyPacket:
		if parsed, err := world.ParseLFGUpdateParty(packet.Body); err == nil {
			rt.applyLFGUpdate(parsed, true)
		}
	case world.LFGUpdateSearchPacket:
		r := world.NewReader(packet.Body, "SMSG_LFG_UPDATE_SEARCH")
		if joined, err := r.U8(); err == nil && joined == 0 && s.searched != 0 {
			s.searched = 0
			rt.FireEvent("UPDATE_LFG_LIST")
		}
	case world.LFGBootUpdatePacket:
		vote, err := world.ParseLFGBootVote(packet.Body)
		if err != nil {
			return true
		}
		s.boot = vote
		if !vote.InProgress {
			if name := rt.nameOf(vote.Victim); name != "" {
				index := errLFGKickPassed
				if vote.Agree < vote.Needed {
					index++
				}
				rt.ShowGameError(index, name)
			}
		}
		rt.FireEvent("LFG_BOOT_PROPOSAL_UPDATE")
	case world.LFGPlayerInfoPacket:
		info, err := world.ParseLFGPlayerInfo(packet.Body)
		if err != nil {
			return true
		}
		s.hasPlayer = false
		s.player = nil
		for _, random := range info.Random {
			node := lfgNodeOf(&s.player, random.Entry)
			node.done, node.money, node.xp = random.Done, random.Money, random.XP
			node.moneyVar, node.xpVar, node.items = random.MoneyVar, random.XPVar, random.Items
		}
		for _, lock := range info.Locks {
			node := lfgNodeOf(&s.player, lock.Entry)
			node.locked, node.status = true, lock.Status
		}
		s.hasPlayer = true
		rt.rebuildLFGRandom()
		rt.FireEvent("LFG_LOCK_INFO_RECEIVED")
		rt.FireEvent("LFG_UPDATE_RANDOM_INFO")
	case world.LFGPartyInfoPacket:
		if players, err := world.ParseLFGPartyInfo(packet.Body); err == nil {
			rt.setLFGPartyLocks(players)
		}
		s.hasParty = true
		rt.rebuildLFGRandom()
		rt.FireEvent("LFG_LOCK_INFO_RECEIVED")
		rt.FireEvent("LFG_UPDATE_RANDOM_INFO")
	case world.LFGDisabledPacket:
		s.local, s.solo, s.party = lfgStatus{}, lfgStatus{}, lfgStatus{}
		s.hasPlayer, s.player = false, nil
		s.hasParty, s.partyList = false, nil
		s.queue = world.LFGQueueStatus{}
		s.reward, s.rewardSet = world.LFGReward{}, false
		rt.FireEvent("LFG_UPDATE", lua.LFalse)
		rt.ShowGameError(errLFGDisabled)
	case world.LFGOpenFinderPacket:
		r := world.NewReader(packet.Body, "SMSG_OPEN_LFG_DUNGEON_FINDER")
		if value, err := r.U32(); err == nil {
			rt.FireEvent("LFG_OPEN_FROM_GOSSIP", lua.LNumber(value))
		}
	default:
		return false
	}
	return true
}

func (rt *Runtime) fireRoleChosen(name string, roles uint32) {
	rt.FireEvent("LFG_ROLE_CHECK_ROLE_CHOSEN", lua.LString(name),
		lua.LBool(roles>>1&1 != 0), lua.LBool(roles>>2&1 != 0), lua.LBool(roles>>3&1 != 0))
}

func (rt *Runtime) setLFGPartyLocks(players []world.LFGPlayerLocks) {
	s := &rt.lfg
	s.partyList = nil
	for _, player := range players {
		entry := &lfgPartyPlayer{guid: player.GUID, has: true}
		for _, lock := range player.Locks {
			node := lfgNodeOf(&entry.locks, lock.Entry)
			node.locked, node.status = true, lock.Status
		}
		s.partyList = append(s.partyList, entry)
	}
}

func (rt *Runtime) applyLFGJoinResult(result world.LFGJoinResult) {
	s := &rt.lfg
	if result.Result == 6 {
		rt.setLFGPartyLocks(result.Locks)
		s.hasParty = true
	}
	if result.Result == 1 {
		switch result.State {
		case 3:
			rt.ShowGameError(0x2A9)
			return
		case 4:
			rt.ShowGameError(0x2AA)
			return
		}
	}
	if index, found := lfgJoinErrors[result.Result]; found {
		rt.ShowGameError(index)
	}
}

func (rt *Runtime) applyLFGProposal(proposal world.LFGProposal) {
	s := &rt.lfg
	state := &s.proposal
	state.entry, state.state, state.id, state.encounters, state.silent = proposal.Entry, proposal.State, proposal.ID, proposal.Encounters, proposal.Silent
	state.players = [5]world.LFGProposalPlayer{}
	declinedSelf, declinedParty := false, false
	for index, player := range proposal.Players {
		state.players[index] = player
		if player.Answered && !player.Accepted {
			if player.Self {
				declinedSelf = true
			} else if player.SameGroup {
				declinedParty = true
			}
		}
	}
	rt.FireEvent("LFG_PROPOSAL_UPDATE")
	if state.lastID != state.id {
		state.lastID = state.id
		if !state.silent {
			rt.FireEvent("LFG_PROPOSAL_SHOW")
		}
	}
	switch {
	case declinedParty:
		rt.ShowGameError(errLFGDeclinedParty)
		rt.lfgResetProposal()
		rt.FireEvent("LFG_PROPOSAL_FAILED")
	case declinedSelf:
		rt.ShowGameError(errLFGDeclinedSelf)
		rt.lfgResetProposal()
		rt.FireEvent("LFG_PROPOSAL_FAILED")
	}
}

func (rt *Runtime) applyLFGRoleCheck(check world.LFGRoleCheck) {
	s := &rt.lfg
	s.roleCheck = check
	own := -1
	player := rt.playerGUID()
	for index, member := range check.Members {
		if member.GUID == player {
			own = index
		}
	}
	if check.Beginning {
		rt.ShowGameError(errLFGRoleCheckBegin)
		for _, member := range check.Members {
			if !member.Ready {
				continue
			}
			if name := rt.nameOf(member.GUID); name != "" {
				rt.fireRoleChosen(name, member.Roles)
			}
		}
	}
	rt.FireEvent("LFG_ROLE_CHECK_UPDATE")
	ownReady := own >= 0 && check.Members[own].Ready
	if !ownReady && check.State == 2 {
		rt.FireEvent("LFG_ROLE_CHECK_SHOW")
		return
	}
	if check.State == 5 {
		rt.ShowGameError(errLFGRoleCheckFailed)
	}
	rt.FireEvent("LFG_ROLE_CHECK_HIDE")
}

// applyLFGUpdate handles the update of the character's or the party's entry in
// the finder.
func (rt *Runtime) applyLFGUpdate(parsed world.LFGStatus, party bool) {
	s := &rt.lfg
	incoming := lfgStatusFrom(parsed, party)
	inGroup := rt.group.in
	target := &s.solo
	if party {
		target = &s.party
	}
	// A proposal the finder no longer stands behind falls.
	if s.proposal.entry != 0 && (!parsed.Joined || incoming.queued) && inGroup == party {
		rt.lfgResetProposal()
		if parsed.Joined {
			rt.ShowGameError(errLFGProposalFailed)
		}
		succeeded := parsed.UpdateType == 10
		if party {
			succeeded = succeeded || parsed.UpdateType == 1 || parsed.UpdateType == 0x10
		}
		if succeeded {
			rt.FireEvent("LFG_PROPOSAL_SUCCEEDED")
		} else {
			rt.FireEvent("LFG_PROPOSAL_FAILED")
		}
	}
	if lfgSameStatus(&incoming, target) {
		return
	}
	leave := func() {
		if parsed.UpdateType == 7 || parsed.UpdateType == 0xF {
			rt.ShowGameError(errLFGLeftQueue + lfgBool(rt.lfgHasRaid(target)))
		}
	}
	retained := func() {
		if !incoming.queued && target.queued {
			leave()
		}
	}
	switch {
	case !incoming.lfgJoined:
		if target.lfgJoined {
			leave()
		} else {
			retained()
		}
	case party:
		if !target.lfgJoined && len(incoming.dungeons) > 0 {
			rt.ShowGameError(errLFGJoinedQueue + lfgBool(rt.lfgHasRaid(&incoming)))
		} else {
			retained()
		}
	default:
		if !target.lfgJoined {
			rt.ShowGameError(errLFGJoinedQueue + lfgBool(rt.lfgHasRaid(&incoming)))
		} else {
			retained()
		}
	}
	*target = incoming
	if inGroup != party {
		return
	}
	if !incoming.queued {
		s.queue = world.LFGQueueStatus{}
	}
	rt.FireEvent("LFG_UPDATE", lua.LFalse)
}

func lfgBool(value bool) int {
	if value {
		return 1
	}
	return 0
}
